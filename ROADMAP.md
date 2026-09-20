# RAP Roadmap

RAP has one goal: make a coding agent's edit loop **cheap, deterministic, and legible to a machine**. Every item below is judged against that goal. A feature that adds convenience for a human but costs an agent tokens or certainty does not belong here.

Status legend: `[x]` done · `[~]` in progress · `[ ]` planned

---

## 1.0.0 — First stable release

The 1.0 line freezes the command surface and the output contract. From here on, the shape of what RAP prints is part of the public API: agents branch on it, so breaking it breaks them.

### Command surface

- [x] Text argument forms: `text`, `@path`, `@-`, `@b64:`, `@inv:`, `@i:`, `@@`
- [x] Preflight: `q` (quoting risk), `m` (match locations and count)
- [x] Literal editing: `s`, `rb`, `ia`, `ib`, `br`
- [x] Line editing: `lr`, `dl`, `mv`, `trim`, `indent`
- [x] File creation: `write`, `append`, `prepend`
- [x] Handles: `mark` with language-aware comment styles
- [x] Inspection: `preview` / `p`, with `-n` line numbers and `-o` output
- [x] Safety: automatic backups, `revert`, `-dry-run`, `-no-backup`
- [x] Transforms: `-pad`, `-trim`, `-indent`
- [x] `version` command with build-time version injection

### Release engineering

- [x] Version injected at build time via `-ldflags -X main.version`
- [x] `make dist` cross-compiling linux/darwin/windows on amd64 and arm64
- [x] `SHA256SUMS` published with every release
- [x] `install.sh` — one-line install from the GitHub release, checksum-verified, no Go toolchain required
- [x] CI on push and pull request: build, test, `go vet`, `gofmt` on Linux, macOS and Windows
- [x] Tag-triggered release workflow publishing prebuilt binaries

### Repository hygiene

- [x] English-only sources, docs and scripts
- [x] Go-appropriate `.gitignore`; IDE state untracked
- [x] README rewritten around the agent token-economy thesis
- [x] `CLAUDE.md` and `AGENTS.md` kept in sync with the command surface
- [x] GitHub description and topics set
- [x] `CHANGELOG.md` following Keep a Changelog

### Shipped

- [x] Green CI run on Linux, macOS and Windows
- [x] Published release with verified assets for all five platform targets
- [x] `install.sh` verified end to end against the published release

---

## 1.1 — Output as a contract

The theme: make RAP's output *formally* consumable, so an agent never has to parse prose.

- [ ] `--json` global flag. Every command emits a single structured object: operation, file, `lines_before`, `lines_after`, `bytes_delta`, `replacements`, `status`. Human output stays the default; agents opt in once and stop guessing.
- [ ] Stable, documented exit codes: `0` success, `1` usage error, `2` no match, `3` ambiguous match, `4` I/O error. An agent branches on a number instead of on an error string.
- [ ] `rap explain FILE` — one compact block describing a file's size, line count, and any RAP markers present, so an agent can orient itself without reading the file.
- [ ] Machine-readable `m` output under `--json`, with byte offsets alongside line/column.

## 1.2 — Selectors that survive time

The theme: line numbers are coordinates, not identity. Give agents selectors that stay valid while a file changes underneath them.

- [ ] Line fingerprints: `preview --hash` prints a short content hash per line.
- [ ] `lrh FILE FROM_HASH TO_HASH TEXT` — range replacement guarded by fingerprints. Fails on zero matches, fails on collisions, never guesses.
- [ ] `rap m --context N` to return surrounding lines with a match, so one call can replace a match check *and* a targeted read.
- [ ] Multi-anchor `rb`: accept several context candidates and succeed only if exactly one resolves.

## 1.3 — Batching

The theme: one process invocation per edit is a tax an agent pays on every step.

- [ ] `rap batch @plan.json` — apply an ordered list of edits atomically. All succeed or the file is untouched, with one receipt for the whole batch.
- [ ] Cross-file batches with a single rollback point.
- [ ] `rap batch --check` to validate every selector in a plan before applying anything, turning a multi-step edit into a single yes/no question.

## 1.4 — Inventory and project context

- [ ] `rap inv` import/export, so a project's snippet vocabulary can be committed to the repository instead of living only in `$HOME`.
- [ ] Optional per-repository inventory location, discovered from the working tree.
- [ ] `rap inv list --json` with sizes and hashes.

## Beyond

- [ ] MCP server mode: expose the same commands as tools, so an agent calls RAP natively instead of through a shell.
- [ ] A published benchmark measuring tokens spent per successful edit against conventional read-modify-write loops, so the claim in the README is a number rather than an argument.
- [ ] Distribution through Homebrew, Scoop and a Nix flake.
- [ ] Language-aware `mark` support for more comment styles, driven by real usage rather than speculation.

---

## Non-goals

These are deliberate exclusions. Saying no keeps the tool small and its behaviour predictable.

- **Regular expressions in selectors.** Literal matching is what makes failure meaningful. A regex that matches too much fails silently; a literal that matches too much fails loudly.
- **Interactive prompts.** RAP is called by processes with no terminal. Every command must run to completion unattended.
- **Being a patch format.** When a real patch is the right representation, use `apply_patch` or `git apply`. RAP changes files.
- **Runtime dependencies.** A single static binary is part of the value.
- **Colour, spinners, progress bars.** Decoration costs tokens and carries no information.

---

## Compatibility promise

From 1.0.0, within the 1.x line:

- Existing command names, flags and argument orders keep working.
- The success receipt keeps its field names and their meaning; new fields may be appended.
- Exit codes are only added, never renumbered.
- Behaviour that refuses an ambiguous edit is never relaxed into a guess.

Anything that would break one of these waits for 2.0.
