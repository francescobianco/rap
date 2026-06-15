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
  @@text         literal text starting with @

Commands:
  q  [-token] TEXT              inspect quoting risk and recommend an argument form
  s  [-all] FILE OLD NEW        replace literal OLD with NEW
  ia FILE NEEDLE TEXT           insert TEXT after NEEDLE
  ib FILE NEEDLE TEXT           insert TEXT before NEEDLE
  br FILE START END TEXT        replace text between START and END, keeping markers
  lr FILE FROM TO TEXT          replace 1-based inclusive line range
  dl FILE FROM TO               delete 1-based inclusive line range
  revert FILE [BACKUP]          restore FILE from BACKUP or its latest backup

Examples:
  rap s README.md 'old' 'new'
  rap q -token @/tmp/replacement.txt
  rap s -all app.go @/tmp/old.txt @/tmp/new.txt
  rap ia main.go 'func main() {' @/tmp/insert.txt
  rap br config.yml '# rap:start' '# rap:end' @/tmp/block.yml
  rap lr README.md 10 12 @/tmp/replacement.md
  rap revert README.md
  rap br config.yml '# rap:start' '# rap:end' @/tmp/block.yml
  rap lr README.md 10 12 @/tmp/replacement.md
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
	case "s":
		return cmdSubstitute(opts, cmdArgs, stdin, stdout)
	case "ia", "ib":
		return cmdInsert(opts, cmd, cmdArgs, stdin, stdout)
	case "br":
		return cmdBlockReplace(opts, cmdArgs, stdin, stdout)
	case "lr":
		return cmdLineReplace(opts, cmdArgs, stdin, stdout)
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

func cmdSubstitute(opts options, args []string, stdin io.Reader, stdout io.Writer) error {
	fs := flag.NewFlagSet("s", flag.ContinueOnError)
	all := fs.Bool("all", false, "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 3 {
		return errors.New("usage: rap s [-all] FILE OLD NEW")
	}
	file := rest[0]
	oldText, err := textArg(rest[1], stdin)
	if err != nil {
		return err
	}
	newText, err := textArg(rest[2], stdin)
	if err != nil {
		return err
	}
	return editFile(opts, file, stdout, func(data []byte) (editResult, error) {
		oldBytes := []byte(oldText)
		if len(oldBytes) == 0 {
			return editResult{}, errors.New("OLD must not be empty")
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
		return editResult{data: bytes.Replace(data, oldBytes, []byte(newText), n), changed: true}, nil
	})
}

func cmdInsert(opts options, cmd string, args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) != 3 {
		return fmt.Errorf("usage: rap %s FILE NEEDLE TEXT", cmd)
	}
	file := args[0]
	needle, err := textArg(args[1], stdin)
	if err != nil {
		return err
	}
	insert, err := textArg(args[2], stdin)
	if err != nil {
		return err
	}
	return editFile(opts, file, stdout, func(data []byte) (editResult, error) {
		needleBytes := []byte(needle)
		if len(needleBytes) == 0 {
			return editResult{}, errors.New("NEEDLE must not be empty")
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
		out := make([]byte, 0, len(data)+len(insert))
		out = append(out, data[:idx]...)
		out = append(out, insert...)
		out = append(out, data[idx:]...)
		return editResult{data: out, changed: true}, nil
	})
}

func cmdBlockReplace(opts options, args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) != 4 {
		return errors.New("usage: rap br FILE START END TEXT")
	}
	file := args[0]
	start, err := textArg(args[1], stdin)
	if err != nil {
		return err
	}
	end, err := textArg(args[2], stdin)
	if err != nil {
		return err
	}
	replacement, err := textArg(args[3], stdin)
	if err != nil {
		return err
	}
	return editFile(opts, file, stdout, func(data []byte) (editResult, error) {
		startBytes := []byte(start)
		endBytes := []byte(end)
		if len(startBytes) == 0 || len(endBytes) == 0 {
			return editResult{}, errors.New("START and END must not be empty")
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
		out := make([]byte, 0, len(data)-contentEnd+contentStart+len(replacement))
		out = append(out, data[:contentStart]...)
		out = append(out, replacement...)
		out = append(out, data[contentEnd:]...)
		return editResult{data: out, changed: true}, nil
	})
}

func cmdLineReplace(opts options, args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) != 4 {
		return errors.New("usage: rap lr FILE FROM TO TEXT")
	}
	file := args[0]
	from, to, err := parseRange(args[1], args[2])
	if err != nil {
		return err
	}
	replacement, err := textArg(args[3], stdin)
	if err != nil {
		return err
	}
	return editFile(opts, file, stdout, func(data []byte) (editResult, error) {
		start, end, err := lineByteRange(data, from, to)
		if err != nil {
			return editResult{}, err
		}
		out := make([]byte, 0, len(data)-(end-start)+len(replacement))
		out = append(out, data[:start]...)
		out = append(out, replacement...)
		out = append(out, data[end:]...)
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
	info, err := os.Stat(file)
	if err != nil {
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
	if err := tmp.Chmod(info.Mode()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, file)
}
