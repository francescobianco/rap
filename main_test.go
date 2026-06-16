package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHelpExitsCleanly(t *testing.T) {
	var out, errOut bytes.Buffer
	if err := run([]string{"--help"}, strings.NewReader(""), &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Real Apply Patch") {
		t.Fatalf("help output missing title: %q", out.String())
	}
}

func TestQuoteTokenUsesBase64ForRiskyText(t *testing.T) {
	var out, errOut bytes.Buffer
	text := "value with 'quotes' and $(shell)\n"
	if err := run([]string{"q", "-token", "@-"}, strings.NewReader(text), &out, &errOut); err != nil {
		t.Fatal(err)
	}
	token := strings.TrimSpace(out.String())
	if !strings.HasPrefix(token, "@b64:") {
		t.Fatalf("expected @b64 token, got %q", token)
	}
	decoded, err := textArg(token, strings.NewReader(""))
	if err != nil {
		t.Fatal(err)
	}
	if decoded != text {
		t.Fatalf("decoded token mismatch:\nwant %q\n got %q", text, decoded)
	}
}

func TestSubstituteWithBase64Argument(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(file, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	newToken := safeTextToken("new 'quoted' value\n")
	if !strings.HasPrefix(newToken, "@b64:") {
		t.Fatalf("expected @b64 token, got %q", newToken)
	}
	if err := run([]string{"-no-backup", "s", file, "old\n", newToken}, strings.NewReader(""), &out, &errOut); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new 'quoted' value\n" {
		t.Fatalf("unexpected file content: %q", got)
	}
}

func TestInventoryCanStoreAndReuseProjectText(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	t.Setenv("HOME", home)
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)

	file := "file.txt"
	if err := os.WriteFile(file, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	if err := run([]string{"inv", "put", "replacement", "new\n"}, strings.NewReader(""), &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"-no-backup", "s", file, "old\n", "@inv:replacement"}, strings.NewReader(""), &out, &errOut); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new\n" {
		t.Fatalf("unexpected file content: %q", got)
	}

	out.Reset()
	if err := run([]string{"inv", "list"}, strings.NewReader(""), &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "replacement" {
		t.Fatalf("unexpected inventory list: %q", out.String())
	}
}

func TestMatchReportsLocationsAndCount(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(file, []byte("one\ntwo one\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	if err := run([]string{"m", file, "one"}, strings.NewReader(""), &out, &errOut); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, file+":1:1") || !strings.Contains(got, file+":2:5") || !strings.Contains(got, "matches: 2") {
		t.Fatalf("unexpected match output: %q", got)
	}
}

func TestSubstituteRequiresUniqueMatch(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(file, []byte("one two one\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	err := run([]string{"-no-backup", "s", file, "one", "three"}, strings.NewReader(""), &out, &errOut)
	if err == nil {
		t.Fatal("expected ambiguous replacement to fail")
	}

	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "one two one\n" {
		t.Fatalf("file changed after failed edit: %q", got)
	}
}

func TestSubstituteAll(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(file, []byte("one two one\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	err := run([]string{"-no-backup", "s", "-all", file, "one", "three"}, strings.NewReader(""), &out, &errOut)
	if err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "three two three\n" {
		t.Fatalf("unexpected file content: %q", got)
	}
}

func TestSubstituteAllowsEmptyReplacement(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(file, []byte("keep remove keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	err := run([]string{"-no-backup", "s", file, " remove", ""}, strings.NewReader(""), &out, &errOut)
	if err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "keep keep\n" {
		t.Fatalf("unexpected file content: %q", got)
	}
}

func TestWriteAppendAndPrepend(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file.txt")

	var out, errOut bytes.Buffer
	if err := run([]string{"-no-backup", "write", file, "body\n"}, strings.NewReader(""), &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"-no-backup", "append", file, "tail\n"}, strings.NewReader(""), &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"-no-backup", "prepend", file, "head\n"}, strings.NewReader(""), &out, &errOut); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "head\nbody\ntail\n" {
		t.Fatalf("unexpected file content: %q", got)
	}
}

func TestWriteRefusesToOverwrite(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(file, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	err := run([]string{"-no-backup", "write", file, "new\n"}, strings.NewReader(""), &out, &errOut)
	if err == nil {
		t.Fatal("expected write to refuse overwriting an existing file")
	}

	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "old\n" {
		t.Fatalf("unexpected file content: %q", got)
	}
}

func TestSubstitutePadAppliesToOldAndNew(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(file, []byte("func x() {\n    old()\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	err := run([]string{"-no-backup", "s", "-pad", "4", file, "old()", "new()"}, strings.NewReader(""), &out, &errOut)
	if err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "func x() {\n    new()\n}\n" {
		t.Fatalf("unexpected file content: %q", got)
	}
}

func TestInsertTrimCleansResult(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(file, []byte("a  \nmarker\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	err := run([]string{"-no-backup", "ia", "-trim", file, "marker", "\nb  \n"}, strings.NewReader(""), &out, &errOut)
	if err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "a\nmarker\nb\n" {
		t.Fatalf("unexpected file content: %q", got)
	}
}

func TestMoveIndentReindentsMovedBlock(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file.txt")
	input := "func x() {\n    ref\n}\na\n  b\nz\n"
	if err := os.WriteFile(file, []byte(input), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	err := run([]string{"-no-backup", "mv", "-indent", "2", file, "4", "5", "3"}, strings.NewReader(""), &out, &errOut)
	if err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	want := "func x() {\n    ref\n    a\n      b\n}\nz\n"
	if string(got) != want {
		t.Fatalf("unexpected file content:\nwant %q\n got %q", want, got)
	}
}

func TestBlockReplaceKeepsMarkers(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file.txt")
	input := "a\n# start\nold\n# end\nz\n"
	if err := os.WriteFile(file, []byte(input), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	err := run([]string{"-no-backup", "br", file, "# start", "# end", "\nnew\n"}, strings.NewReader(""), &out, &errOut)
	if err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	want := "a\n# start\nnew\n# end\nz\n"
	if string(got) != want {
		t.Fatalf("unexpected file content:\nwant %q\n got %q", want, got)
	}
}

func TestLineReplaceFromStdin(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(file, []byte("a\nb\nc\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	err := run([]string{"-no-backup", "lr", file, "2", "2", "@-"}, strings.NewReader("B\n"), &out, &errOut)
	if err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "a\nB\nc\n" {
		t.Fatalf("unexpected file content: %q", got)
	}
}

func TestMarkWrapsGoRangeWithLineComments(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file.go")
	input := "package main\n\nfunc x() {\n\tprintln(1)\n}\n"
	if err := os.WriteFile(file, []byte(input), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	if err := run([]string{"-no-backup", "mark", file, "4", "4", "body"}, strings.NewReader(""), &out, &errOut); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	want := "package main\n\nfunc x() {\n\t// rap:start body\n\tprintln(1)\n\t// rap:end body\n}\n"
	if string(got) != want {
		t.Fatalf("unexpected file content:\nwant %q\n got %q", want, got)
	}
}

func TestMarkWrapsMarkdownRangeWithHtmlComments(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "README.md")
	input := "# Title\n\nbody\n"
	if err := os.WriteFile(file, []byte(input), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	if err := run([]string{"-no-backup", "mark", file, "3", "3", "section"}, strings.NewReader(""), &out, &errOut); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	want := "# Title\n\n<!-- rap:start section -->\nbody\n<!-- rap:end section -->\n"
	if string(got) != want {
		t.Fatalf("unexpected file content:\nwant %q\n got %q", want, got)
	}
}

func TestMoveLineRangeBeforeDestination(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(file, []byte("a\nb\nc\nd\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	if err := run([]string{"-no-backup", "mv", file, "3", "4", "2"}, strings.NewReader(""), &out, &errOut); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "a\nc\nd\nb\n" {
		t.Fatalf("unexpected file content: %q", got)
	}
}

func TestMoveLineRangeDownAndAppend(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(file, []byte("a\nb\nc\nd\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	if err := run([]string{"-no-backup", "mv", file, "1", "2", "5"}, strings.NewReader(""), &out, &errOut); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "c\nd\na\nb\n" {
		t.Fatalf("unexpected file content: %q", got)
	}
}

func TestTrimCleansTrailingWhitespaceAndFinalNewline(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(file, []byte("a  \r\nb\t\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	if err := run([]string{"-no-backup", "trim", file}, strings.NewReader(""), &out, &errOut); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "a\nb\n" {
		t.Fatalf("unexpected file content: %q", got)
	}
}

func TestIndentUsesReferenceLineIndent(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file.txt")
	input := "func x() {\n    if ok {\na\n  b\n    }\n}\n"
	if err := os.WriteFile(file, []byte(input), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	if err := run([]string{"-no-backup", "indent", file, "3", "4", "2"}, strings.NewReader(""), &out, &errOut); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	want := "func x() {\n    if ok {\n    a\n      b\n    }\n}\n"
	if string(got) != want {
		t.Fatalf("unexpected file content:\nwant %q\n got %q", want, got)
	}
}

func TestRevertLatestBackup(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	t.Setenv("HOME", home)
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)

	file := "file.txt"
	if err := os.WriteFile(file, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	if err := run([]string{"s", file, "old", "new"}, strings.NewReader(""), &out, &errOut); err != nil {
		t.Fatal(err)
	}
	abs, backupPath, err := backupDir(file)
	if err != nil {
		t.Fatal(err)
	}
	wantBackupPath := filepath.Join(home, ".rap", "backup", backupKey(abs))
	if backupPath != wantBackupPath {
		t.Fatalf("unexpected backup path:\nwant %q\n got %q", wantBackupPath, backupPath)
	}
	if _, err := os.Stat(filepath.Join(backupPath, "path")); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"revert", file}, strings.NewReader(""), &out, &errOut); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "old\n" {
		t.Fatalf("unexpected reverted content: %q", got)
	}
}
