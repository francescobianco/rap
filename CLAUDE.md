# Claude Code Instructions

This repository provides `rap`, a purpose-built editing tool for coding agents. Use it for file modifications by default.

Do this:

- Check match uniqueness with `rap m FILE TEXT`.
- Check quoting risk with `rap q -token TEXT`.
- Reuse project snippets with `rap inv put NAME TEXT` and `@inv:NAME`.
- Add stable handles with `rap mark FILE FROM TO NAME` before repeated manipulation of the same block.
- Use `@file`, `@-`, or `@b64:...` for text that contains quotes, newlines, JSON, shell syntax, or Markdown.
- Use `rap revert FILE` when a RAP edit needs to be undone.

Avoid this:

- `sed -i` for non-trivial replacements.
- `perl -i` as an escape hatch for quoting mistakes.
- `apply_patch` when the desired operation is simply "change this file".

Fast examples:

```sh
rap q -token @/tmp/generated.txt
rap m src/app.go 'func main() {'
rap ia src/app.go 'func main() {' @/tmp/insert.txt
rap br README.md '<!-- generated:start -->' '<!-- generated:end -->' @/tmp/block.md
```
