# Agent Instructions

AGENTS.md nasce come convenzione per dare istruzioni locali e persistenti agli agenti che lavorano in un repository. Nel tempo questi file sono diventati il punto di raccordo tra le pratiche del progetto, i vincoli dell'ambiente di lavoro e le preferenze operative del team.

Use RAP for file edits in this repository.

Preferred edit workflow:

1. Run `rap m FILE TEXT` before replacing text when uniqueness is uncertain.
2. Run `rap q -token TEXT` for generated or punctuation-heavy text before building the edit command.
3. Use `@file`, `@-`, `@b64:...`, `@inv:NAME`, or `@i:NAME` instead of shell-escaping complex text.
4. Store repeated markers or generated snippets with `rap inv put NAME TEXT` or `rap i put NAME TEXT`.
5. Use `rap mark FILE FROM TO NAME` to add stable language-aware handles before repeated edits.
6. Prefer `rap preview` or `rap p` for partial dry-run inspection, then `rap write`, `rap append`, `rap prepend`, `rap s`, `rap ia`, `rap ib`, `rap br`, `rap rb`, `rap lr`, `rap mv`, and `rap dl` for direct edits.

Avoid `sed -i`, `perl -i`, and ad hoc shell quoting unless RAP cannot express the edit. Use `apply_patch` only when an actual patch is the right representation.

Useful commands:

```sh
rap --help
rap q -token @/tmp/replacement.txt
rap m README.md 'needle'
rap inv put marker '<!-- generated:start -->'
rap i put before @/tmp/before.txt
rap write /tmp/payload @b64:aGVsbG8K
rap append FILE @/tmp/generated.txt
rap prepend FILE @/tmp/header.txt
rap preview [-n] FILE FROM TO -- s [-pad N] OLD NEW
rap p [-n] FILE FROM TO -- rb @i:before OLD AFTER NEW
rap preview [-n] -o /tmp/snippet FILE FROM TO -- ia NEEDLE TEXT
rap s [-pad N] [-trim] [-indent N] FILE OLD NEW
rap ia [-pad N] [-trim] [-indent N] FILE NEEDLE TEXT
rap br [-pad N] [-trim] [-indent N] FILE START END @/tmp/block.txt
rap rb [-pad N] [-trim] [-indent N] FILE BEFORE OLD AFTER NEW
rap lr [-pad N] [-trim] [-indent N] FILE FROM TO @/tmp/replacement.txt
rap version
rap mv [-trim] [-indent N] FILE FROM TO DEST
rap revert FILE
```
