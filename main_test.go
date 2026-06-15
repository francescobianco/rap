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
