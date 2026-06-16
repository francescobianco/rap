package main

import (
	"bytes"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const usageText = `rap - Real Apply Patch

Usage:
  rap [global flags] <command> [command flags] ...

Global flags:
  -dry-run       print the resulting file instead of writing it
  -no-backup     do not create a $HOME/.rap/backup copy before writing

Text arguments:
  text           literal text
  @path          read text from path
  @-             read text from stdin
  @b64:BASE64    decode text from base64
  @inv:NAME      read text from this project's inventory
  @@text         literal text starting with @

Commands:
  q  [-token] TEXT                         inspect quoting risk and recommend an argument form
  m  FILE TEXT                             print literal match locations and count
  inv put NAME TEXT                        store reusable project text
  inv get|rm|path NAME                     read, remove, or locate inventory text
  inv list                                 list reusable project text names
  write [-pad N] [-trim] FILE TEXT         create FILE with TEXT, refusing to overwrite
  append [-pad N] [-trim] [-indent REF] FILE TEXT
                                           append TEXT to FILE
  prepend [-pad N] [-trim] [-indent REF] FILE TEXT
                                           prepend TEXT to FILE
  preview [-n] [-o OUT] FILE FROM TO -- COMMAND [ARGS...]
                                           print selected lines after a RAP edit preview
  s  [-all] [-pad N] [-trim] [-indent REF] FILE OLD NEW
                                           replace literal OLD with NEW
  ia [-pad N] [-trim] [-indent REF] FILE NEEDLE TEXT
                                           insert TEXT after NEEDLE
  ib [-pad N] [-trim] [-indent REF] FILE NEEDLE TEXT
                                           insert TEXT before NEEDLE
  br [-pad N] [-trim] [-indent REF] FILE START END TEXT
                                           replace text between START and END, keeping markers
  lr [-pad N] [-trim] [-indent REF] FILE FROM TO TEXT
                                           replace 1-based inclusive line range
  dl FILE FROM TO                          delete 1-based inclusive line range
  mark FILE FROM TO NAME                   wrap range with language-aware markers
  mv [-trim] [-indent REF] FILE FROM TO DEST
                                           move line range before DEST line
  trim FILE [FROM TO]                      strip trailing whitespace and clean newlines
  indent FILE FROM TO REF                  reindent range using REF line indentation
  revert FILE [BACKUP]                     restore FILE from BACKUP or its latest backup

Edit flags:
  -pad N        prepend N spaces to each non-empty line in text arguments
  -trim         clean trailing whitespace and final newline after the edit
  -indent REF   reindent inserted/replaced/moved text using line REF

Examples:
  rap q -token @/tmp/replacement.txt
  rap m README.md 'old'
  rap inv put marker '<!-- generated:start -->'
  rap write /tmp/payload @b64:aGVsbG8K
  rap append notes.md @/tmp/generated.md
  rap prepend notes.md '# Title\n\n'
  rap preview app.go 10 20 -- s 'old' 'new'
  rap preview -n app.go 10 20 -- s 'old' 'new'
  rap preview -o /tmp/snippet.go app.go 10 20 -- ia 'func main() {' @/tmp/insert.txt
  rap s README.md 'old' 'new'
  rap s -pad 4 app.go 'old()' 'new()'
  rap s -all app.go @/tmp/old.txt @/tmp/new.txt
  rap ia -trim main.go 'func main() {' @/tmp/insert.txt
  rap br -indent 12 config.yml '# rap:start' '# rap:end' @/tmp/block.yml
  rap lr README.md 10 12 @/tmp/replacement.md
  rap mv -indent 19 README.md 20 30 10
  rap mark main.go 20 35 generated
  rap trim README.md
  rap indent main.go 20 28 19
  rap revert README.md
`

type options struct {
	dryRun   bool
	noBackup bool
}

type editResult struct {
	data    []byte
	changed bool
}

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "rap:", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	opts := options{}
	if len(args) > 0 && (args[0] == "help" || args[0] == "-h" || args[0] == "--help") {
		fmt.Fprint(stdout, usageText)
		return nil
	}
	fs := flag.NewFlagSet("rap", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.BoolVar(&opts.dryRun, "dry-run", false, "")
	fs.BoolVar(&opts.noBackup, "no-backup", false, "")
	fs.Usage = func() { fmt.Fprint(stderr, usageText) }
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	rest := fs.Args()
	if len(rest) == 0 || rest[0] == "help" || rest[0] == "-h" || rest[0] == "--help" {
		fmt.Fprint(stdout, usageText)
		return nil
	}

	cmd := rest[0]
	cmdArgs := rest[1:]
	switch cmd {
	case "q", "quote":
		return cmdQuote(cmdArgs, stdin, stdout)
	case "m", "match":
		return cmdMatch(cmdArgs, stdin, stdout)
	case "inv", "inventory":
		return cmdInventory(cmdArgs, stdin, stdout)
	case "write":
		return cmdWrite(opts, cmdArgs, stdin, stdout)
	case "append", "prepend":
		return cmdAppendPrepend(opts, cmd, cmdArgs, stdin, stdout)
	case "preview":
		return cmdPreview(opts, cmdArgs, stdin, stdout)
	case "s":
		return cmdSubstitute(opts, cmdArgs, stdin, stdout)
	case "ia", "ib":
		return cmdInsert(opts, cmd, cmdArgs, stdin, stdout)
	case "br":
		return cmdBlockReplace(opts, cmdArgs, stdin, stdout)
	case "lr":
		return cmdLineReplace(opts, cmdArgs, stdin, stdout)
	case "mark":
		return cmdMark(opts, cmdArgs, stdout)
	case "trim":
		return cmdTrim(opts, cmdArgs, stdout)
	case "indent":
		return cmdIndent(opts, cmdArgs, stdout)
	case "mv", "move":
		return cmdMove(opts, cmdArgs, stdout)
	case "dl":
		return cmdLineDelete(opts, cmdArgs, stdout)
	case "revert":
		return cmdRevert(opts, cmdArgs, stdout)
	default:
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func cmdQuote(args []string, stdin io.Reader, stdout io.Writer) error {
	fs := flag.NewFlagSet("q", flag.ContinueOnError)
	tokenOnly := fs.Bool("token", false, "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 1 {
		return errors.New("usage: rap q [-token] TEXT")
	}
	text, err := textArg(rest[0], stdin)
	if err != nil {
		return err
	}
	token := safeTextToken(text)
	if *tokenOnly {
		fmt.Fprintln(stdout, token)
		return nil
	}

	risk, reason := quoteRisk(text)
	fmt.Fprintf(stdout, "bytes: %d\n", len(text))
	fmt.Fprintf(stdout, "lines: %d\n", lineCount(text))
	fmt.Fprintf(stdout, "shell-risk: %s\n", risk)
	fmt.Fprintf(stdout, "reason: %s\n", reason)
	fmt.Fprintf(stdout, "recommended: %s\n", recommendation(text))
	fmt.Fprintf(stdout, "token: %s\n", token)
	return nil
}

func cmdMatch(args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) != 2 {
		return errors.New("usage: rap m FILE TEXT")
	}
	file := args[0]
	needle, err := textArg(args[1], stdin)
	if err != nil {
		return err
	}
	if needle == "" {
		return errors.New("TEXT must not be empty")
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	needleBytes := []byte(needle)
	offset := 0
	count := 0
	for {
		idx := bytes.Index(data[offset:], needleBytes)
		if idx < 0 {
			break
		}
		pos := offset + idx
		line, col := byteLineCol(data, pos)
		fmt.Fprintf(stdout, "%s:%d:%d\n", file, line, col)
		count++
		offset = pos + len(needleBytes)
	}
	fmt.Fprintf(stdout, "matches: %d\n", count)
	return nil
}

func cmdInventory(args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: rap inv <put|get|list|rm|path> ...")
	}
	switch args[0] {
	case "put":
		if len(args) != 3 {
			return errors.New("usage: rap inv put NAME TEXT")
		}
		text, err := textArg(args[2], stdin)
		if err != nil {
			return err
		}
		path, err := inventoryPath(args[1])
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "stored %s\n", args[1])
		return nil
	case "get":
		if len(args) != 2 {
			return errors.New("usage: rap inv get NAME")
		}
		text, err := inventoryText(args[1])
		if err != nil {
			return err
		}
		_, err = io.WriteString(stdout, text)
		return err
	case "list":
		if len(args) != 1 {
			return errors.New("usage: rap inv list")
		}
		dir, err := inventoryDir()
		if err != nil {
			return err
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		var names []string
		for _, entry := range entries {
			if !entry.IsDir() {
				names = append(names, entry.Name())
			}
		}
		sort.Strings(names)
		for _, name := range names {
			fmt.Fprintln(stdout, name)
		}
		return nil
	case "rm":
		if len(args) != 2 {
			return errors.New("usage: rap inv rm NAME")
		}
		path, err := inventoryPath(args[1])
		if err != nil {
			return err
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		fmt.Fprintf(stdout, "removed %s\n", args[1])
		return nil
	case "path":
		if len(args) != 1 && len(args) != 2 {
			return errors.New("usage: rap inv path [NAME]")
		}
		if len(args) == 1 {
			dir, err := inventoryDir()
			if err != nil {
				return err
			}
			fmt.Fprintln(stdout, dir)
			return nil
		}
		path, err := inventoryPath(args[1])
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, path)
		return nil
	default:
		return fmt.Errorf("unknown inventory command %q", args[0])
	}
}

type editTransformOptions struct {
	pad       int
	trim      bool
	indentRef int
}

func addEditTransformFlags(fs *flag.FlagSet, allowIndent bool) *editTransformOptions {
	tf := &editTransformOptions{}
	fs.IntVar(&tf.pad, "pad", 0, "")
	fs.BoolVar(&tf.trim, "trim", false, "")
	if allowIndent {
		fs.IntVar(&tf.indentRef, "indent", 0, "")
	}
	return tf
}

func (tf editTransformOptions) validate() error {
	if tf.pad < 0 {
		return errors.New("-pad must be non-negative")
	}
	if tf.indentRef < 0 {
		return errors.New("-indent must be non-negative")
	}
	return nil
}

func (tf editTransformOptions) text(text string) string {
	if tf.pad == 0 {
		return text
	}
	return padTextLines(text, strings.Repeat(" ", tf.pad))
}

func (tf editTransformOptions) block(data []byte, text string) ([]byte, error) {
	block := []byte(tf.text(text))
	if tf.indentRef == 0 {
		return block, nil
	}
	refStart, refEnd, err := lineByteRange(data, tf.indentRef, tf.indentRef)
	if err != nil {
		return nil, err
	}
	refIndent := leadingIndent(lineWithoutNewline(data[refStart:refEnd]))
	return reindentBlock(block, refIndent), nil
}

func (tf editTransformOptions) finish(data []byte) []byte {
	if tf.trim {
		return cleanWhitespace(data, true)
	}
	return data
}

func padTextLines(text, prefix string) string {
	if text == "" || prefix == "" {
		return text
	}
	var out strings.Builder
	out.Grow(len(text) + len(prefix)*lineCount(text))
	lineStart := true
	for i := 0; i < len(text); i++ {
		if lineStart && text[i] != '\n' {
			out.WriteString(prefix)
		}
		out.WriteByte(text[i])
		lineStart = text[i] == '\n'
	}
	return out.String()
}

func cmdWrite(opts options, args []string, stdin io.Reader, stdout io.Writer) error {
	fs := flag.NewFlagSet("write", flag.ContinueOnError)
	tf := addEditTransformFlags(fs, false)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := tf.validate(); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 2 {
		return errors.New("usage: rap write [-pad N] [-trim] FILE TEXT")
	}
	file := rest[0]
	text, err := textArg(rest[1], stdin)
	if err != nil {
		return err
	}
	data := []byte(tf.finish([]byte(tf.text(text))))
	if _, err := os.Stat(file); err == nil {
		return fmt.Errorf("%s already exists", file)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if opts.dryRun {
		_, err := stdout.Write(data)
		return err
	}
	if err := writeAtomic(file, data); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "created %s\n", file)
	return nil
}

func cmdAppendPrepend(opts options, cmd string, args []string, stdin io.Reader, stdout io.Writer) error {
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	tf := addEditTransformFlags(fs, true)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := tf.validate(); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 2 {
		return fmt.Errorf("usage: rap %s [-pad N] [-trim] [-indent REF] FILE TEXT", cmd)
	}
	file := rest[0]
	text, err := textArg(rest[1], stdin)
	if err != nil {
		return err
	}
	return editFile(opts, file, stdout, func(data []byte) (editResult, error) {
		block, err := tf.block(data, text)
		if err != nil {
			return editResult{}, err
		}
		out := make([]byte, 0, len(data)+len(block))
		if cmd == "prepend" {
			out = append(out, block...)
			out = append(out, data...)
		} else {
			out = append(out, data...)
			out = append(out, block...)
		}
		return editResult{data: tf.finish(out), changed: true}, nil
	})
}

func cmdPreview(opts options, args []string, stdin io.Reader, stdout io.Writer) error {
	fs := flag.NewFlagSet("preview", flag.ContinueOnError)
	lineNumbers := fs.Bool("n", false, "")
	outPath := fs.String("o", "", "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	sep := -1
	for i, arg := range rest {
		if arg == "--" {
			sep = i
			break
		}
	}
	if sep < 0 || sep+1 >= len(rest) || sep != 3 {
		return errors.New("usage: rap preview [-n] [-o OUT] FILE FROM TO -- COMMAND [ARGS...]")
	}
	file := rest[0]
	from, to, err := parseRange(rest[1], rest[2])
	if err != nil {
		return err
	}
	cmd := rest[sep+1]
	cmdArgs := rest[sep+2:]

	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "rap-preview-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	tmpFile := filepath.Join(dir, filepath.Base(file))
	if err := os.WriteFile(tmpFile, data, 0o644); err != nil {
		return err
	}
	previewArgs, err := previewCommandArgs(cmd, tmpFile, cmdArgs)
	if err != nil {
		return err
	}
	var opOut, opErr bytes.Buffer
	if err := run(append([]string{"-no-backup", cmd}, previewArgs...), stdin, &opOut, &opErr); err != nil {
		if opErr.Len() > 0 {
			return fmt.Errorf("preview %s failed: %w: %s", cmd, err, strings.TrimSpace(opErr.String()))
		}
		return fmt.Errorf("preview %s failed: %w", cmd, err)
	}
	result, err := os.ReadFile(tmpFile)
	if err != nil {
		return err
	}
	start, end, err := lineByteRange(result, from, to)
	if err != nil {
		return err
	}
	snippet := result[start:end]
	if *lineNumbers {
		snippet = numberLines(snippet, from, to)
	}
	if *outPath == "" || opts.dryRun {
		_, err := stdout.Write(snippet)
		return err
	}
	if err := writeAtomic(*outPath, snippet); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "wrote %s\n", *outPath)
	return nil
}

func numberLines(data []byte, from, to int) []byte {
	width := len(strconv.Itoa(to))
	out := bytes.Buffer{}
	line := from
	for _, chunk := range splitLines(data) {
		fmt.Fprintf(&out, "%*d | ", width, line)
		out.Write(chunk)
		if len(chunk) == 0 || chunk[len(chunk)-1] != '\n' {
			out.WriteByte('\n')
		}
		line++
	}
	return out.Bytes()
}

func previewCommandArgs(cmd, file string, args []string) ([]string, error) {
	switch cmd {
	case "append", "prepend", "s", "ia", "ib", "br", "lr":
		return injectFileAfterCommandFlags(cmd, file, args)
	case "mv", "move":
		return injectFileAfterCommandFlags("mv", file, args)
	case "trim", "indent", "dl", "mark":
		return append([]string{file}, args...), nil
	default:
		return nil, fmt.Errorf("preview does not support command %q", cmd)
	}
}

func injectFileAfterCommandFlags(cmd, file string, args []string) ([]string, error) {
	boolFlags := map[string]bool{}
	valueFlags := map[string]bool{}
	switch cmd {
	case "s":
		boolFlags["all"] = true
		boolFlags["trim"] = true
		valueFlags["pad"] = true
		valueFlags["indent"] = true
	case "append", "prepend", "ia", "ib", "br", "lr":
		boolFlags["trim"] = true
		valueFlags["pad"] = true
		valueFlags["indent"] = true
	case "mv":
		boolFlags["trim"] = true
		valueFlags["indent"] = true
	default:
		return nil, fmt.Errorf("preview cannot inject FILE for command %q", cmd)
	}

	idx := 0
	for idx < len(args) {
		arg := args[idx]
		if arg == "--" || arg == "-" || !strings.HasPrefix(arg, "-") {
			break
		}
		name := strings.TrimPrefix(arg, "-")
		if strings.HasPrefix(name, "-") {
			return nil, fmt.Errorf("unsupported flag %q", arg)
		}
		if eq := strings.IndexByte(name, '='); eq >= 0 {
			name = name[:eq]
		}
		if boolFlags[name] {
			idx++
			continue
		}
		if valueFlags[name] {
			idx++
			if !strings.Contains(arg, "=") {
				if idx >= len(args) {
					return nil, fmt.Errorf("flag %s needs a value", arg)
				}
				idx++
			}
			continue
		}
		return nil, fmt.Errorf("unsupported flag %q", arg)
	}

	out := make([]string, 0, len(args)+1)
	out = append(out, args[:idx]...)
	out = append(out, file)
	out = append(out, args[idx:]...)
	return out, nil
}

func cmdSubstitute(opts options, args []string, stdin io.Reader, stdout io.Writer) error {
	fs := flag.NewFlagSet("s", flag.ContinueOnError)
	all := fs.Bool("all", false, "")
	tf := addEditTransformFlags(fs, true)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := tf.validate(); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 3 {
		return errors.New("usage: rap s [-all] [-pad N] [-trim] [-indent REF] FILE OLD NEW")
	}
	file := rest[0]
	oldText, err := textArg(rest[1], stdin)
	if err != nil {
		return err
	}
	oldText = tf.text(oldText)
	newText, err := textArg(rest[2], stdin)
	if err != nil {
		return err
	}
	return editFile(opts, file, stdout, func(data []byte) (editResult, error) {
		oldBytes := []byte(oldText)
		if len(oldBytes) == 0 {
			return editResult{}, errors.New("OLD must not be empty")
		}
		newBytes, err := tf.block(data, newText)
		if err != nil {
			return editResult{}, err
		}
		count := bytes.Count(data, oldBytes)
		if count == 0 {
			return editResult{}, errors.New("OLD not found")
		}
		if !*all && count > 1 {
			return editResult{}, fmt.Errorf("OLD matched %d times; use -all or a more specific OLD", count)
		}
		n := 1
		if *all {
			n = -1
		}
		return editResult{data: tf.finish(bytes.Replace(data, oldBytes, newBytes, n)), changed: true}, nil
	})
}

func cmdInsert(opts options, cmd string, args []string, stdin io.Reader, stdout io.Writer) error {
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	tf := addEditTransformFlags(fs, true)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := tf.validate(); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 3 {
		return fmt.Errorf("usage: rap %s [-pad N] [-trim] [-indent REF] FILE NEEDLE TEXT", cmd)
	}
	file := rest[0]
	needle, err := textArg(rest[1], stdin)
	if err != nil {
		return err
	}
	needle = tf.text(needle)
	insert, err := textArg(rest[2], stdin)
	if err != nil {
		return err
	}
	return editFile(opts, file, stdout, func(data []byte) (editResult, error) {
		needleBytes := []byte(needle)
		if len(needleBytes) == 0 {
			return editResult{}, errors.New("NEEDLE must not be empty")
		}
		insertBytes, err := tf.block(data, insert)
		if err != nil {
			return editResult{}, err
		}
		count := bytes.Count(data, needleBytes)
		if count == 0 {
			return editResult{}, errors.New("NEEDLE not found")
		}
		if count > 1 {
			return editResult{}, fmt.Errorf("NEEDLE matched %d times; make it unique", count)
		}
		idx := bytes.Index(data, needleBytes)
		if cmd == "ia" {
			idx += len(needleBytes)
		}
		out := make([]byte, 0, len(data)+len(insertBytes))
		out = append(out, data[:idx]...)
		out = append(out, insertBytes...)
		out = append(out, data[idx:]...)
		return editResult{data: tf.finish(out), changed: true}, nil
	})
}

func cmdBlockReplace(opts options, args []string, stdin io.Reader, stdout io.Writer) error {
	fs := flag.NewFlagSet("br", flag.ContinueOnError)
	tf := addEditTransformFlags(fs, true)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := tf.validate(); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 4 {
		return errors.New("usage: rap br [-pad N] [-trim] [-indent REF] FILE START END TEXT")
	}
	file := rest[0]
	start, err := textArg(rest[1], stdin)
	if err != nil {
		return err
	}
	start = tf.text(start)
	end, err := textArg(rest[2], stdin)
	if err != nil {
		return err
	}
	end = tf.text(end)
	replacement, err := textArg(rest[3], stdin)
	if err != nil {
		return err
	}
	return editFile(opts, file, stdout, func(data []byte) (editResult, error) {
		startBytes := []byte(start)
		endBytes := []byte(end)
		if len(startBytes) == 0 || len(endBytes) == 0 {
			return editResult{}, errors.New("START and END must not be empty")
		}
		replacementBytes, err := tf.block(data, replacement)
		if err != nil {
			return editResult{}, err
		}
		startIdx := bytes.Index(data, startBytes)
		if startIdx < 0 {
			return editResult{}, errors.New("START not found")
		}
		if bytes.Index(data[startIdx+len(startBytes):], startBytes) >= 0 {
			return editResult{}, errors.New("START matched more than once after first match")
		}
		contentStart := startIdx + len(startBytes)
		endRel := bytes.Index(data[contentStart:], endBytes)
		if endRel < 0 {
			return editResult{}, errors.New("END not found after START")
		}
		contentEnd := contentStart + endRel
		if bytes.Index(data[contentEnd+len(endBytes):], endBytes) >= 0 {
			return editResult{}, errors.New("END matched more than once after selected block")
		}
		out := make([]byte, 0, len(data)-contentEnd+contentStart+len(replacementBytes))
		out = append(out, data[:contentStart]...)
		out = append(out, replacementBytes...)
		out = append(out, data[contentEnd:]...)
		return editResult{data: tf.finish(out), changed: true}, nil
	})
}

func cmdLineReplace(opts options, args []string, stdin io.Reader, stdout io.Writer) error {
	fs := flag.NewFlagSet("lr", flag.ContinueOnError)
	tf := addEditTransformFlags(fs, true)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := tf.validate(); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 4 {
		return errors.New("usage: rap lr [-pad N] [-trim] [-indent REF] FILE FROM TO TEXT")
	}
	file := rest[0]
	from, to, err := parseRange(rest[1], rest[2])
	if err != nil {
		return err
	}
	replacement, err := textArg(rest[3], stdin)
	if err != nil {
		return err
	}
	return editFile(opts, file, stdout, func(data []byte) (editResult, error) {
		start, end, err := lineByteRange(data, from, to)
		if err != nil {
			return editResult{}, err
		}
		replacementBytes, err := tf.block(data, replacement)
		if err != nil {
			return editResult{}, err
		}
		out := make([]byte, 0, len(data)-(end-start)+len(replacementBytes))
		out = append(out, data[:start]...)
		out = append(out, replacementBytes...)
		out = append(out, data[end:]...)
		return editResult{data: tf.finish(out), changed: true}, nil
	})
}

func cmdMark(opts options, args []string, stdout io.Writer) error {
	if len(args) != 4 {
		return errors.New("usage: rap mark FILE FROM TO NAME")
	}
	file := args[0]
	from, to, err := parseRange(args[1], args[2])
	if err != nil {
		return err
	}
	name := args[3]
	if !validInventoryName(name) {
		return fmt.Errorf("invalid marker name %q; use letters, numbers, dot, dash, or underscore", name)
	}
	return editFile(opts, file, stdout, func(data []byte) (editResult, error) {
		start, end, err := lineByteRange(data, from, to)
		if err != nil {
			return editResult{}, err
		}
		lineEnd, err := lineStartOffset(data, from+1)
		if err != nil {
			lineEnd = end
		}
		indent := leadingIndent(lineWithoutNewline(data[start:lineEnd]))
		startMarker, endMarker := markerLines(file, name, indent)
		if bytes.Contains(data, bytes.TrimRight(startMarker, "\n")) || bytes.Contains(data, bytes.TrimRight(endMarker, "\n")) {
			return editResult{}, fmt.Errorf("marker %q already exists", name)
		}
		block := data[start:end]
		out := make([]byte, 0, len(data)+len(startMarker)+len(endMarker)+1)
		out = append(out, data[:start]...)
		out = append(out, startMarker...)
		out = append(out, block...)
		if len(block) > 0 && block[len(block)-1] != '\n' {
			out = append(out, '\n')
		}
		out = append(out, endMarker...)
		out = append(out, data[end:]...)
		return editResult{data: out, changed: true}, nil
	})
}

func cmdTrim(opts options, args []string, stdout io.Writer) error {
	if len(args) != 1 && len(args) != 3 {
		return errors.New("usage: rap trim FILE [FROM TO]")
	}
	file := args[0]
	return editFile(opts, file, stdout, func(data []byte) (editResult, error) {
		if len(args) == 1 {
			return editResult{data: cleanWhitespace(data, true), changed: true}, nil
		}
		from, to, err := parseRange(args[1], args[2])
		if err != nil {
			return editResult{}, err
		}
		start, end, err := lineByteRange(data, from, to)
		if err != nil {
			return editResult{}, err
		}
		cleaned := cleanWhitespace(data[start:end], false)
		out := make([]byte, 0, len(data)-(end-start)+len(cleaned))
		out = append(out, data[:start]...)
		out = append(out, cleaned...)
		out = append(out, data[end:]...)
		return editResult{data: out, changed: true}, nil
	})
}

func cmdIndent(opts options, args []string, stdout io.Writer) error {
	if len(args) != 4 {
		return errors.New("usage: rap indent FILE FROM TO REF")
	}
	file := args[0]
	from, to, err := parseRange(args[1], args[2])
	if err != nil {
		return err
	}
	ref, err := strconv.Atoi(args[3])
	if err != nil {
		return fmt.Errorf("REF must be an integer: %w", err)
	}
	if ref < 1 {
		return errors.New("REF must be 1-based")
	}
	return editFile(opts, file, stdout, func(data []byte) (editResult, error) {
		start, end, err := lineByteRange(data, from, to)
		if err != nil {
			return editResult{}, err
		}
		refStart, refEnd, err := lineByteRange(data, ref, ref)
		if err != nil {
			return editResult{}, err
		}
		refIndent := leadingIndent(lineWithoutNewline(data[refStart:refEnd]))
		block := reindentBlock(data[start:end], refIndent)
		out := make([]byte, 0, len(data)-(end-start)+len(block))
		out = append(out, data[:start]...)
		out = append(out, block...)
		out = append(out, data[end:]...)
		return editResult{data: out, changed: true}, nil
	})
}

func cmdMove(opts options, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("mv", flag.ContinueOnError)
	trim := fs.Bool("trim", false, "")
	indentRef := fs.Int("indent", 0, "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *indentRef < 0 {
		return errors.New("-indent must be non-negative")
	}
	rest := fs.Args()
	if len(rest) != 4 {
		return errors.New("usage: rap mv [-trim] [-indent REF] FILE FROM TO DEST")
	}
	file := rest[0]
	from, to, err := parseRange(rest[1], rest[2])
	if err != nil {
		return err
	}
	dest, err := strconv.Atoi(rest[3])
	if err != nil {
		return fmt.Errorf("DEST must be an integer: %w", err)
	}
	if dest < 1 {
		return errors.New("DEST must be 1-based")
	}
	return editFile(opts, file, stdout, func(data []byte) (editResult, error) {
		start, end, err := lineByteRange(data, from, to)
		if err != nil {
			return editResult{}, err
		}
		destOffset, err := lineStartOffset(data, dest)
		if err != nil {
			return editResult{}, err
		}
		if destOffset >= start && destOffset <= end {
			if *trim {
				return editResult{data: cleanWhitespace(data, true), changed: true}, nil
			}
			return editResult{data: data, changed: false}, nil
		}
		block := append([]byte(nil), data[start:end]...)
		if *indentRef > 0 {
			refStart, refEnd, err := lineByteRange(data, *indentRef, *indentRef)
			if err != nil {
				return editResult{}, err
			}
			refIndent := leadingIndent(lineWithoutNewline(data[refStart:refEnd]))
			block = reindentBlock(block, refIndent)
		}
		without := make([]byte, 0, len(data)-(end-start))
		without = append(without, data[:start]...)
		without = append(without, data[end:]...)
		if destOffset > end {
			destOffset -= end - start
		}
		out := make([]byte, 0, len(without)+len(block))
		out = append(out, without[:destOffset]...)
		out = append(out, block...)
		out = append(out, without[destOffset:]...)
		if *trim {
			out = cleanWhitespace(out, true)
		}
		return editResult{data: out, changed: true}, nil
	})
}

func cmdLineDelete(opts options, args []string, stdout io.Writer) error {
	if len(args) != 3 {
		return errors.New("usage: rap dl FILE FROM TO")
	}
	file := args[0]
	from, to, err := parseRange(args[1], args[2])
	if err != nil {
		return err
	}
	return editFile(opts, file, stdout, func(data []byte) (editResult, error) {
		start, end, err := lineByteRange(data, from, to)
		if err != nil {
			return editResult{}, err
		}
		out := make([]byte, 0, len(data)-(end-start))
		out = append(out, data[:start]...)
		out = append(out, data[end:]...)
		return editResult{data: out, changed: true}, nil
	})
}

func cmdRevert(opts options, args []string, stdout io.Writer) error {
	if len(args) != 1 && len(args) != 2 {
		return errors.New("usage: rap revert FILE [BACKUP]")
	}
	file := args[0]
	backup := ""
	var err error
	if len(args) == 2 {
		backup = args[1]
	} else {
		backup, err = latestBackup(file)
		if err != nil {
			return err
		}
	}
	data, err := os.ReadFile(backup)
	if err != nil {
		return err
	}
	if opts.dryRun {
		_, err = stdout.Write(data)
		return err
	}
	if !opts.noBackup {
		if err := createBackup(file); err != nil {
			return err
		}
	}
	if err := writeAtomic(file, data); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "restored %s from %s\n", file, backup)
	return nil
}

func editFile(opts options, file string, stdout io.Writer, edit func([]byte) (editResult, error)) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	result, err := edit(data)
	if err != nil {
		return err
	}
	if bytes.Equal(data, result.data) {
		fmt.Fprintf(stdout, "unchanged %s\n", file)
		return nil
	}
	if opts.dryRun {
		_, err = stdout.Write(result.data)
		return err
	}
	if !opts.noBackup {
		if err := createBackup(file); err != nil {
			return err
		}
	}
	if err := writeAtomic(file, result.data); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "updated %s\n", file)
	return nil
}

func textArg(arg string, stdin io.Reader) (string, error) {
	if arg == "@-" {
		data, err := io.ReadAll(stdin)
		return string(data), err
	}
	if strings.HasPrefix(arg, "@@") {
		return arg[1:], nil
	}
	if strings.HasPrefix(arg, "@b64:") {
		data, err := base64.StdEncoding.DecodeString(arg[len("@b64:"):])
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	if strings.HasPrefix(arg, "@inv:") {
		return inventoryText(arg[len("@inv:"):])
	}
	if strings.HasPrefix(arg, "@") {
		data, err := os.ReadFile(arg[1:])
		return string(data), err
	}
	return arg, nil
}

func safeTextToken(text string) string {
	if isBareShellSafe(text) && !strings.HasPrefix(text, "@") {
		return text
	}
	if isSingleQuoteSafe(text) && !strings.Contains(text, "\n") {
		return "'" + text + "'"
	}
	return "@b64:" + base64.StdEncoding.EncodeToString([]byte(text))
}

func quoteRisk(text string) (string, string) {
	switch {
	case text == "":
		return "medium", "empty strings are easy to lose in command construction"
	case strings.Contains(text, "\x00"):
		return "high", "contains NUL bytes; use @b64 or a file"
	case strings.Contains(text, "\n"):
		return "high", "contains newlines; use @b64, @file, or @-"
	case !utf8.ValidString(text):
		return "high", "contains non-UTF-8 bytes; use @b64 or a file"
	case isBareShellSafe(text) && !strings.HasPrefix(text, "@"):
		return "low", "safe as a bare shell argument"
	case isSingleQuoteSafe(text):
		return "medium", "single quotes are enough, but @b64 avoids shell rules entirely"
	default:
		return "high", "contains shell-sensitive punctuation; use @b64, @file, or @-"
	}
}

func recommendation(text string) string {
	if isBareShellSafe(text) && !strings.HasPrefix(text, "@") {
		return "bare literal"
	}
	if isSingleQuoteSafe(text) && !strings.Contains(text, "\n") {
		return "single-quoted literal or @b64"
	}
	return "@b64 for short text; @file or @- for larger generated blocks"
}

func isBareShellSafe(text string) bool {
	if text == "" {
		return false
	}
	for _, r := range text {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			continue
		}
		switch r {
		case '_', '-', '.', '/', ':', '+', '=', ',', '%':
			continue
		default:
			return false
		}
	}
	return true
}

func isSingleQuoteSafe(text string) bool {
	return text != "" && !strings.ContainsAny(text, "'\n\r\x00")
}

func lineCount(text string) int {
	if text == "" {
		return 0
	}
	lines := strings.Count(text, "\n")
	if !strings.HasSuffix(text, "\n") {
		lines++
	}
	return lines
}

type commentStyle struct {
	prefix string
	suffix string
}

func markerLines(file, name string, indent []byte) ([]byte, []byte) {
	style := commentStyleFor(file)
	start := markerLine(style, indent, "rap:start "+name)
	end := markerLine(style, indent, "rap:end "+name)
	return start, end
}

func markerLine(style commentStyle, indent []byte, label string) []byte {
	var out []byte
	out = append(out, indent...)
	out = append(out, style.prefix...)
	if style.prefix != "" && !strings.HasSuffix(style.prefix, " ") {
		out = append(out, ' ')
	}
	out = append(out, label...)
	if style.suffix != "" {
		out = append(out, ' ')
		out = append(out, style.suffix...)
	}
	out = append(out, '\n')
	return out
}

func commentStyleFor(file string) commentStyle {
	switch strings.ToLower(filepath.Ext(file)) {
	case ".go", ".js", ".jsx", ".ts", ".tsx", ".java", ".c", ".h", ".cc", ".cpp", ".cs", ".rs", ".swift", ".kt", ".kts", ".scala", ".php":
		return commentStyle{prefix: "//"}
	case ".py", ".rb", ".sh", ".bash", ".zsh", ".fish", ".yaml", ".yml", ".toml", ".ini", ".conf", ".cfg", ".dockerfile", ".mk", ".make":
		return commentStyle{prefix: "#"}
	case ".sql", ".lua", ".hs", ".elm":
		return commentStyle{prefix: "--"}
	case ".html", ".htm", ".xml", ".md", ".markdown", ".vue", ".svelte":
		return commentStyle{prefix: "<!--", suffix: "-->"}
	case ".css", ".scss", ".sass", ".less":
		return commentStyle{prefix: "/*", suffix: "*/"}
	case ".el", ".lisp", ".clj", ".cljs", ".scm":
		return commentStyle{prefix: ";;"}
	default:
		base := strings.ToLower(filepath.Base(file))
		if base == "dockerfile" || strings.HasPrefix(base, "makefile") {
			return commentStyle{prefix: "#"}
		}
		return commentStyle{prefix: "#"}
	}
}

func cleanWhitespace(data []byte, finalNewline bool) []byte {
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	data = bytes.ReplaceAll(data, []byte("\r"), []byte("\n"))
	lines := bytes.SplitAfter(data, []byte("\n"))
	out := make([]byte, 0, len(data))
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		hasNewline := line[len(line)-1] == '\n'
		body := line
		if hasNewline {
			body = line[:len(line)-1]
		}
		body = bytes.TrimRight(body, " \t")
		out = append(out, body...)
		if hasNewline {
			out = append(out, '\n')
		}
	}
	out = bytes.TrimRight(out, " \t\n")
	if finalNewline && len(out) > 0 {
		out = append(out, '\n')
	}
	return out
}

func reindentBlock(data []byte, refIndent []byte) []byte {
	lines := splitLines(data)
	common := commonIndent(lines)
	out := make([]byte, 0, len(data)+len(lines)*len(refIndent))
	for _, line := range lines {
		body, newline := splitLineEnding(line)
		trimmed := bytes.TrimRight(body, " \t")
		if len(bytes.TrimSpace(trimmed)) == 0 {
			out = append(out, newline...)
			continue
		}
		out = append(out, refIndent...)
		out = append(out, trimIndent(trimmed, common)...)
		out = append(out, newline...)
	}
	return out
}

func splitLines(data []byte) [][]byte {
	if len(data) == 0 {
		return nil
	}
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			lines = append(lines, data[start:i+1])
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}

func splitLineEnding(line []byte) ([]byte, []byte) {
	if len(line) > 0 && line[len(line)-1] == '\n' {
		return line[:len(line)-1], line[len(line)-1:]
	}
	return line, nil
}

func lineWithoutNewline(line []byte) []byte {
	body, _ := splitLineEnding(line)
	return bytes.TrimRight(body, "\r")
}

func leadingIndent(line []byte) []byte {
	idx := 0
	for idx < len(line) && (line[idx] == ' ' || line[idx] == '\t') {
		idx++
	}
	return append([]byte(nil), line[:idx]...)
}

func commonIndent(lines [][]byte) []byte {
	var common []byte
	set := false
	for _, line := range lines {
		body, _ := splitLineEnding(line)
		body = bytes.TrimRight(body, " \t\r")
		if len(bytes.TrimSpace(body)) == 0 {
			continue
		}
		indent := leadingIndent(body)
		if !set {
			common = append([]byte(nil), indent...)
			set = true
			continue
		}
		common = sharedPrefix(common, indent)
	}
	return common
}

func sharedPrefix(a, b []byte) []byte {
	limit := len(a)
	if len(b) < limit {
		limit = len(b)
	}
	idx := 0
	for idx < limit && a[idx] == b[idx] {
		idx++
	}
	return a[:idx]
}

func trimIndent(line, indent []byte) []byte {
	if len(indent) == 0 || !bytes.HasPrefix(line, indent) {
		return line
	}
	return line[len(indent):]
}

func parseRange(fromText, toText string) (int, int, error) {
	from, err := strconv.Atoi(fromText)
	if err != nil {
		return 0, 0, fmt.Errorf("FROM must be an integer: %w", err)
	}
	to, err := strconv.Atoi(toText)
	if err != nil {
		return 0, 0, fmt.Errorf("TO must be an integer: %w", err)
	}
	if from < 1 || to < from {
		return 0, 0, errors.New("line range must be 1-based and FROM <= TO")
	}
	return from, to, nil
}

func lineStartOffset(data []byte, line int) (int, error) {
	if line < 1 {
		return 0, errors.New("line must be 1-based")
	}
	if line == 1 {
		return 0, nil
	}
	current := 1
	for i, b := range data {
		if b != '\n' {
			continue
		}
		current++
		if current == line {
			return i + 1, nil
		}
	}
	if line == current+1 {
		return len(data), nil
	}
	return 0, errors.New("DEST is past end of file")
}

func lineByteRange(data []byte, from, to int) (int, int, error) {
	line := 1
	start := -1
	for i := 0; i <= len(data); i++ {
		if line == from && start < 0 {
			start = i
		}
		if i == len(data) || data[i] == '\n' {
			if line == to {
				end := i
				if i < len(data) {
					end = i + 1
				}
				if start < 0 {
					return 0, 0, errors.New("FROM is past end of file")
				}
				return start, end, nil
			}
			line++
		}
	}
	return 0, 0, errors.New("TO is past end of file")
}

func createBackup(file string) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	abs, dir, err := backupDir(file)
	if err != nil {
		return err
	}
	name := time.Now().UTC().Format("20060102T150405.000000000Z")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "path"), []byte(abs+"\n"), 0o644)
}

func latestBackup(file string) (string, error) {
	_, dir, err := backupDir(file)
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var backups []string
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == "path" {
			continue
		}
		backups = append(backups, filepath.Join(dir, entry.Name()))
	}
	if len(backups) == 0 {
		return "", fmt.Errorf("no backups found for %s", file)
	}
	sort.Strings(backups)
	return backups[len(backups)-1], nil
}

func byteLineCol(data []byte, pos int) (int, int) {
	line := 1
	col := 1
	for i := 0; i < pos && i < len(data); i++ {
		if data[i] == '\n' {
			line++
			col = 1
			continue
		}
		col++
	}
	return line, col
}

func inventoryText(name string) (string, error) {
	path, err := inventoryPath(name)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	return string(data), err
}

func inventoryPath(name string) (string, error) {
	if !validInventoryName(name) {
		return "", fmt.Errorf("invalid inventory name %q; use letters, numbers, dot, dash, or underscore", name)
	}
	dir, err := inventoryDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

func inventoryDir() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".rap", "project", backupKey(filepath.Clean(wd)), "inventory"), nil
}

func validInventoryName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			continue
		}
		switch r {
		case '.', '-', '_':
			continue
		default:
			return false
		}
	}
	return true
}

func backupDir(file string) (string, string, error) {
	abs, err := filepath.Abs(file)
	if err != nil {
		return "", "", err
	}
	abs = filepath.Clean(abs)
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", err
	}
	return abs, filepath.Join(home, ".rap", "backup", backupKey(abs)), nil
}

func backupKey(abs string) string {
	key := strings.ReplaceAll(filepath.ToSlash(abs), "/", "-")
	key = strings.ReplaceAll(key, ":", "")
	if key == "" {
		return "-"
	}
	return key
}

func writeAtomic(file string, data []byte) error {
	mode := os.FileMode(0o644)
	info, err := os.Stat(file)
	if err == nil {
		mode = info.Mode()
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	dir := filepath.Dir(file)
	tmp, err := os.CreateTemp(dir, ".rap-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, file)
}
