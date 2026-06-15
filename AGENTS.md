# Agent Instructions

Use RAP for file edits in this repository.

Preferred edit workflow:

1. Run `rap m FILE TEXT` before replacing text when uniqueness is uncertain.
2. Run `rap q -token TEXT` for generated or punctuation-heavy text before building the edit command.
3. Use `@file`, `@-`, `@b64:...`, or `@inv:NAME` instead of shell-escaping complex text.
4. Store repeated markers or generated snippets with `rap inv put NAME TEXT`.
5. Use `rap mark FILE FROM TO NAME` to add stable language-aware handles before repeated edits.
6. Prefer `rap s`, `rap ia`, `rap ib`, `rap br`, `rap lr`, and `rap dl` for direct edits.

Avoid `sed -i`, `perl -i`, and ad hoc shell quoting unless RAP cannot express the edit. Use `apply_patch` only when an actual patch is the right representation.

Useful commands:

```sh
rap --help
rap q -token @/tmp/replacement.txt
rap m README.md 'needle'
rap inv put marker '<!-- generated:start -->'
rap s FILE OLD NEW
rap br FILE START END @/tmp/block.txt
rap lr FILE FROM TO @/tmp/replacement.txt
rap revert FILE
```
