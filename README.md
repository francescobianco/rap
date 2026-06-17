# RAP - Real Apply Patch

RAP is a tiny non-interactive file editing tool for coding agents. It exists because watching an agent fight shell quoting for ten minutes to change one line is not engineering; it is performance art with worse error messages.

`apply_patch` is fine until the sandbox, transport, or patch grammar decides today is not your day. `sed` is great if your replacement text politely avoids slashes, ampersands, newlines, and reality. `perl -i` can do anything, which is exactly why an agent will eventually summon a quoting ceremony instead of editing the file.

RAP keeps the useful part: exact file edits, short commands, no interactivity, loud failure on ambiguity, and automatic backups. If you want coding agents to edit code effectively instead of improvising shell archaeology, this is the boring sharp tool they should reach for.

## Install

```sh
go install github.com/francescobianco/rap@latest
```

From a local checkout:

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
@inv:NAME      read text from this project's inventory
@i:NAME        shorthand for @inv:NAME
@@text         literal text that starts with @
```

This is the most important quoting escape hatch. If text contains quotes, backslashes, JSON, shell fragments, or multiple lines, put it in a file or pass it on stdin and use `@path` or `@-`.

## Commands

### Project inventory

```sh
rap inv put NAME TEXT      # or: rap i put NAME TEXT
rap inv get NAME           # or: rap i get NAME
rap inv list               # or: rap i list
rap inv rm NAME            # or: rap i rm NAME
rap inv path [NAME]        # or: rap i path [NAME]
```

Inventory stores reusable snippets for the current project under `$HOME/.rap/project/<pwd-with-dashes>/inventory`. Use it for markers, boilerplate, generated blocks, or any text an agent would otherwise keep re-quoting until everyone involved loses patience.

```sh
rap inv put start '<!-- generated:start -->'
rap inv put end '<!-- generated:end -->'
rap br README.md @inv:start @inv:end @/tmp/generated.md
```

### Match preflight

```sh
rap m FILE TEXT
```

`m` prints every literal match location and a final count. Run it before `s`, `ia`, `ib`, or `br` when uniqueness is not obvious.

```sh
rap m README.md @inv:start
```

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

### Create, append, and prepend files

```sh
rap write FILE TEXT
rap append FILE TEXT
rap prepend FILE TEXT
```

`write` creates a new file with `TEXT` and refuses to overwrite an existing file. `append` and `prepend` add text to the end or start of an existing file without needing a marker. These commands are useful for preparing temporary RAP payload files without a heredoc or helper script.

```sh
rap write /tmp/payload @b64:aGVsbG8K
rap append CHANGELOG.md @/tmp/generated-entry.md
rap prepend notes.md $'# Title\n\n'
```

### Edit flags

Replacement, insertion, block, line, append, and prepend commands accept small transform flags:

```sh
-pad N
-trim
-indent N
```

`-pad N` prepends `N` spaces to each non-empty inserted or replacement line; it does not change `OLD`, `NEEDLE`, or marker text used for matching. `-trim` cleans trailing whitespace and normalizes the final newline after the edit. `-indent N` reindents inserted, replaced, appended, prepended, or moved text using the indentation of line `N`.

```sh
rap s -pad 4 app.go 'old()' 'new()'
rap ia -trim README.md '<!-- rap:start -->' @/tmp/block.md
rap mv -indent 12 main.go 80 95 100
rap preview app.go 10 20 -- s -pad 4 'old()' 'new()'
```

### Preview a partial result

```sh
rap preview [-n] FILE FROM TO -- COMMAND [ARGS...]
rap preview [-n] -o OUT FILE FROM TO -- COMMAND [ARGS...]
rap p [-n] FILE FROM TO -- COMMAND [ARGS...]
```

`preview` runs a RAP edit against a temporary copy of `FILE`, then prints only the selected line range from the edited result. The source file is not changed. The command after `--` is written like the normal RAP operation but without repeating `FILE`; RAP injects the temporary file after that command's flags and reports a hint if `FILE` is accidentally repeated. With `-o`, the selected block is written to `OUT`. Add `-n` to include a left gutter with line numbers from the edited result.

```sh
rap preview app.go 10 20 -- s 'oldName' 'newName'
rap preview -n app.go 10 20 -- s -pad 4 'old()' 'new()'
rap preview -n -o /tmp/snippet.go app.go 10 20 -- ia 'func main() {' @/tmp/insert.go
rap preview README.md 40 65 -- mv -indent 12 80 95 45
```

Useful cases:

- review the local effect of a replacement in a large file without reading a full `-dry-run` dump
- see the edited-result line numbers after insertions, deletions, or moves change the block structure
- save a numbered before/after review snippet for a PR comment, issue, or another tool
- test `-pad`, `-trim`, or `-indent` combinations before applying them to the real file
- inspect the destination area after a move or generated block insertion

### Literal replacement

```sh
rap s [-pad N] [-trim] [-indent N] FILE OLD NEW
rap s -all [-pad N] [-trim] [-indent N] FILE OLD NEW
```

`rap s` replaces one exact literal match. If `OLD` matches zero times or more than once, RAP exits with an error. Use `-all` only when replacing every match is intentional.

Pass an empty `NEW` argument (or `@b64:` from `rap q -token ""`) to delete a literal match. `OLD` must not be empty because it would match every position in the file.

```sh
rap s README.md 'old text' 'new text'
rap s -pad 4 app.go 'old()' 'new()'
rap s -all app.go @/tmp/old.txt @/tmp/new.txt
```

### Insert after or before a marker

```sh
rap ia [-pad N] [-trim] [-indent N] FILE NEEDLE TEXT
rap ib [-pad N] [-trim] [-indent N] FILE NEEDLE TEXT
```

`ia` inserts after a unique marker. `ib` inserts before a unique marker. When the insertion point is a line boundary and `TEXT` does not already provide its own newline, RAP terminates the inserted block so adjacent lines are not fused.

```sh
rap ia main.go 'func main() {' @/tmp/insert.txt
rap ib README.md '## Commands' $'## Quick Start\n\n'
```

### Replace a block between markers

```sh
rap br [-pad N] [-trim] [-indent N] FILE START END TEXT
```

`br` replaces the content between `START` and `END`, while keeping both markers. The start and end markers must identify exactly one block.

```sh
rap br config.yml '# rap:start' '# rap:end' @/tmp/generated.yml
rap br -indent 20 main.go '// rap:start generated' '// rap:end generated' @/tmp/new.go
```

### Replace inside required context

```sh
rap rb [-pad N] [-trim] [-indent N] FILE BEFORE OLD AFTER NEW
```

`rb` replaces `OLD` only when the full literal context `BEFORE + OLD + AFTER` exists exactly once. `OLD` must be non-empty, and at least one of `BEFORE` or `AFTER` must be non-empty, so prefix-only or suffix-only anchors are allowed when they are still unique. It is safer than line ranges when nearby lines may shift, and more compact than manually building a larger `OLD` block when only the middle should change.

```sh
rap rb app.go @/tmp/before.txt @/tmp/old.txt @/tmp/after.txt @/tmp/new.txt
rap p -n app.go 40 55 -- rb @i:before @i:old @i:after @/tmp/new.txt
```

### Replace or delete line ranges

```sh
rap lr [-pad N] [-trim] [-indent N] FILE FROM TO TEXT
rap dl FILE FROM TO
```

Line numbers are 1-based and inclusive. `lr` treats `TEXT` as a line block and adds a missing trailing newline for non-empty replacements, preventing the replacement from fusing with the following line. Line numbers are useful for quick local edits, but they are intentionally the least stable selector: if the file changes between inspection and application, the same range can point at different text. Prefer `s`, `rb`, `br`, or `mark` when the target can be named by content or context.

```sh
rap lr README.md 10 12 @/tmp/replacement.md
rap lr -indent 9 main.go 20 30 @/tmp/replacement.go
rap dl debug.log 1 20
```

### Move, trim, and reindent line ranges

### Add manipulation handles

```sh
rap mark FILE FROM TO NAME
```

`mark` wraps a line range with language-aware `rap:start NAME` and `rap:end NAME` comments. It turns anonymous code into a stable target for later `br`, `m`, `mv`, `trim`, or `indent` operations.

```sh
rap mark main.go 80 110 generated-loader
rap br main.go '// rap:start generated-loader' '// rap:end generated-loader' @/tmp/new-loader.go
```

For Markdown and HTML-family files it uses `<!-- rap:start NAME -->`; for Go/JS/C-style files it uses `//`; for Python/YAML/shell it uses `#`; for CSS it uses `/* ... */`.

```sh
rap mv [-trim] [-indent N] FILE FROM TO DEST
rap trim FILE [FROM TO]
rap indent FILE FROM TO REF
```

`mv` moves the inclusive line range `FROM..TO` before line `DEST` in the same file, using coordinates from the original file. Add `-indent N` to reindent the moved block during the move, and `-trim` to clean the resulting file.

`trim` removes trailing spaces/tabs, normalizes dirty line endings, and leaves a clean final newline for whole-file runs. With `FROM TO`, it cleans only that range.

`indent` reindents a range using the indentation of line `REF` as the base while preserving relative indentation inside the moved or generated block.

```sh
rap mv README.md 40 52 20
rap mv -indent 79 main.go 80 95 120
rap trim README.md
rap indent main.go 80 95 79
```

### Version

```sh
rap version
rap --version
```

`version` prints the RAP version string, which is useful in agent logs and reproducible bug reports.

### Revert

```sh
rap revert FILE
rap revert FILE BACKUP
```

Before each write, RAP stores the previous version under `$HOME/.rap/backup`. `rap revert FILE` restores the latest backup for that file. Passing an explicit backup path restores that snapshot instead.

Backup directories are derived from absolute file paths by replacing path separators with dashes. For example, `/home/francesco/project/mio-dir/mio-file` is stored under `$HOME/.rap/backup/-home-francesco-project-mio-dir-mio-file`.

## Agent Adoption

RAP is meant to be discovered without a human repeating the same instruction in every prompt. This repository includes:

- `AGENTS.md` for Codex/OpenAI-style coding agents
- `CLAUDE.md` for Claude Code
- `README.md` command docs for every other agent that scans project documentation

Agents should read those files and use RAP as the default edit path. The short rule is: inspect with `rap m`, control quoting with `rap q`, reuse snippets with `rap inv` or `rap i`, edit with RAP commands, revert with `rap revert`.

## Agent Notes

Use RAP when the edit is one of these shapes:

- create a file from a text argument
- append or prepend text without a marker
- preview a selected line range after a RAP edit without changing the source file
- replace this exact text with that exact text
- insert text before or after a unique marker
- replace generated content inside stable markers
- replace text inside required literal context
- replace, move, or delete a known line range
- combine insertion/replacement/move with padding, trimming, or indentation
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

## Design Notes

### Line fingerprints

Line-number commands are convenient, but line numbers are coordinates, not identity. A future RAP locator could print a short per-line fingerprint next to preview output, then accept that fingerprint as a guard when applying a range edit. The useful idea is not "replace line 42"; it is "replace the line that used to be line 42 and still has this content fingerprint".

A four-character CRC is a good human-facing hint, but it should not be the only authority. With many lines, short hashes collide. RAP should treat fingerprints as checked selectors: fail on zero matches, fail on multiple matches, and ideally combine the fingerprint with nearby context or the original line text. That keeps the property RAP cares about most: deterministic edits that refuse to guess.

A practical shape could be:

```sh
rap preview -n --hash FILE FROM TO -- s OLD NEW
rap lrh FILE FROM_HASH TO_HASH @/tmp/replacement.txt
```

The exact command names are open, but the rule should stay simple: hashes can make moved unchanged lines findable, while ambiguity still fails loudly.
