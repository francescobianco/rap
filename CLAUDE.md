# Claude Code Instructions

This repository provides `rap`, a purpose-built editing tool for coding agents. Use it for file modifications by default.

Do this:

- Check match uniqueness with `rap m FILE TEXT`.
- Check quoting risk with `rap q -token TEXT`.
- Reuse project snippets with `rap inv put NAME TEXT` or `rap i put NAME TEXT`; read them with `@inv:NAME` or `@i:NAME`.
- Add stable handles with `rap mark FILE FROM TO NAME` before repeated manipulation of the same block.
- Use `rap rb FILE BEFORE OLD AFTER NEW` when a line range would be fragile but a required literal context is available.
- Use `@file`, `@-`, or `@b64:...` for text that contains quotes, newlines, JSON, shell syntax, or Markdown.
- Use `rap preview FILE FROM TO -- COMMAND ...` or `rap p FILE FROM TO -- COMMAND ...` to inspect a selected line range after a RAP edit without changing the source file.
- Add `-n` to `rap preview` when the output should include edited-result line numbers.
- Use `rap preview -o OUT FILE FROM TO -- COMMAND ...` to save that preview block.
- Use `rap write FILE TEXT` for temporary payload files instead of heredocs or helper scripts.
- Use `rap append FILE TEXT` and `rap prepend FILE TEXT` when no marker is needed.
- Use `-pad N`, `-trim`, and `-indent N` with insert, replace, line, block, append/prepend, or move operations when that makes the command clearer; `-pad` affects inserted/replacement text, not match patterns.
- Use `rap revert FILE` when a RAP edit needs to be undone.

Avoid this:

- `sed -i` for non-trivial replacements.
- `perl -i` as an escape hatch for quoting mistakes.
- `apply_patch` when the desired operation is simply "change this file".

Fast examples:

```sh
rap q -token @/tmp/generated.txt
rap m src/app.go 'func main() {'
rap write /tmp/payload @b64:aGVsbG8K
rap append CHANGELOG.md @/tmp/generated-entry.md
rap preview -n src/app.go 10 20 -- s -pad 4 'old()' 'new()'
rap p -n src/app.go 10 20 -- rb @i:before @i:old @i:after @/tmp/new.txt
rap preview -n -o /tmp/snippet.go src/app.go 10 20 -- ia 'func main() {' @/tmp/insert.txt
rap s -pad 4 src/app.go 'old()' 'new()'
rap ia -trim src/app.go 'func main() {' @/tmp/insert.txt
rap br -indent 20 README.md '<!-- generated:start -->' '<!-- generated:end -->' @/tmp/block.md
rap rb src/app.go @/tmp/before.txt @/tmp/old.txt @/tmp/after.txt @/tmp/new.txt
rap mv -indent 12 src/app.go 40 52 80
```
