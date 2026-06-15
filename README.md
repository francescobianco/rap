# RAP - Real Apply Patch

RAP is a tiny non-interactive file editing tool for coding agents. It exists because watching an agent fight shell quoting for ten minutes to change one line is not engineering; it is performance art with worse error messages.

`apply_patch` is fine until the sandbox, transport, or patch grammar decides today is not your day. `sed` is great if your replacement text politely avoids slashes, ampersands, newlines, and reality. `perl -i` can do anything, which is exactly why an agent will eventually summon a quoting ceremony instead of editing the file.

RAP keeps the useful part: exact file edits, short commands, no interactivity, loud failure on ambiguity, and automatic backups. If you want coding agents to edit code effectively instead of improvising shell archaeology, this is the boring sharp tool they should reach for.

## Install

```sh
make install
```

By default this builds RAP and installs it as `$(HOME)/.local/bin/rap`. Override `PREFIX` or `BINDIR` when needed:

```sh
make install PREFIX=/opt/rap
make install BINDIR=$(HOME)/bin
```

For a local build without installing:

```sh
make build
```

## Core Syntax

```text
rap [global flags] <command> [command flags] ...
```

Global flags:

```text
-dry-run       print the resulting file instead of writing it
-no-backup     do not create a $HOME/.rap/backup copy before writing
```

Every text argument supports the same input forms:

```text
text           literal text
@path          read text from a file
@-             read text from stdin
@b64:BASE64    decode text from base64
@@text         literal text that starts with @
```

This is the most important quoting escape hatch. If text contains quotes, backslashes, JSON, shell fragments, or multiple lines, put it in a file or pass it on stdin and use `@path` or `@-`.

## Commands

### Quoting preflight

```sh
rap q TEXT
rap q -token TEXT
```

`q` inspects a text argument before you try to use it in an edit. It reports byte count, line count, shell risk, the recommended input form, and a ready-to-use token. With `-token`, it prints only the safest RAP argument.

```sh
rap q 'simple-value'
rap q @/tmp/replacement.txt
rap q -token @/tmp/replacement.txt
```

For shell-hostile text, `q -token` returns `@b64:...`. That token can be passed directly to any RAP command without quotes, heredocs, or a small religious service for escaping punctuation.

```sh
TOKEN=$(rap q -token @/tmp/replacement.txt)
rap s app.json @/tmp/old.json "$TOKEN"
```

### Literal replacement

```sh
rap s FILE OLD NEW
rap s -all FILE OLD NEW
```

`rap s` replaces one exact literal match. If `OLD` matches zero times or more than once, RAP exits with an error. Use `-all` only when replacing every match is intentional.

```sh
rap s README.md 'old text' 'new text'
rap s -all app.go @/tmp/old.txt @/tmp/new.txt
```

### Insert after or before a marker

```sh
rap ia FILE NEEDLE TEXT
rap ib FILE NEEDLE TEXT
```

`ia` inserts after a unique marker. `ib` inserts before a unique marker.

```sh
rap ia main.go 'func main() {' @/tmp/insert.txt
rap ib README.md '## Commands' $'## Quick Start\n\n'
```

### Replace a block between markers

```sh
rap br FILE START END TEXT
```

`br` replaces the content between `START` and `END`, while keeping both markers. The start and end markers must identify exactly one block.

```sh
rap br config.yml '# rap:start' '# rap:end' @/tmp/generated.yml
```

### Replace or delete line ranges

```sh
rap lr FILE FROM TO TEXT
rap dl FILE FROM TO
```

Line numbers are 1-based and inclusive.

```sh
rap lr README.md 10 12 @/tmp/replacement.md
rap dl debug.log 1 20
```

### Revert

```sh
rap revert FILE
rap revert FILE BACKUP
```

Before each write, RAP stores the previous version under `$HOME/.rap/backup`. `rap revert FILE` restores the latest backup for that file. Passing an explicit backup path restores that snapshot instead.

Backup directories are derived from absolute file paths by replacing path separators with dashes. For example, `/home/francesco/project/mio-dir/mio-file` is stored under `$HOME/.rap/backup/-home-francesco-project-mio-dir-mio-file`.

## Agent Notes

Use RAP when the edit is one of these shapes:

- replace this exact text with that exact text
- insert text before or after a unique marker
- replace generated content inside stable markers
- replace or delete a known line range
- revert the last RAP edit for a file

If that sounds like most edits a coding agent performs, that is the point. Agents do not need another opportunity to rediscover how many escaping layers exist between JSON, the shell, regex syntax, and a source file. They need a small deterministic command that either edits the file or refuses to guess.

Before constructing an edit command, run `rap q -token` on generated text when there is any doubt. If the output starts with `@b64:`, use that token directly. If the generated block is large, write it to a temp file and pass `@file`; nobody gets bonus points for making Bash carry a novella.

RAP intentionally fails on ambiguous matches. That is not a lack of confidence; that is the feature. A failed command is a signal to use a more specific marker or inspect the file before editing. Compare that with a heroic `sed -i` one-liner silently editing the wrong occurrence and then pretending it helped.

For complex replacement text, avoid shell quoting entirely:

```sh
rap s app.json @/tmp/old.json @/tmp/new.json
rap br README.md '<!-- generated:start -->' '<!-- generated:end -->' @-
```

Use `apply_patch` when you actually want a patch. Use RAP when you want the file changed. Use `sed` and `perl -i` when you miss debugging punctuation.
