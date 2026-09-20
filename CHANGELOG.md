# Changelog

All notable changes to RAP are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

See [ROADMAP.md](ROADMAP.md) for what is planned.

## [1.0.0] - 2026-09-20

First stable release. The command surface and the output contract are now public API:
agents branch on what RAP prints, so within the 1.x line the shape of that output does not change.

### Added

- Prebuilt binaries for linux/amd64, linux/arm64, darwin/amd64, darwin/arm64 and windows/amd64,
  published on every tagged release with `SHA256SUMS`.
- `install.sh` — one-line, checksum-verified installation from the GitHub release, with no Go
  toolchain required. Honours `RAP_VERSION` and `RAP_BINDIR`.
- Tag-triggered release workflow that builds, tests and publishes the artifacts.
- CI workflow running build, tests, `go vet` and a `gofmt` check on Linux, macOS and Windows.
- `make dist` cross-compiling every released platform, and `make check` running vet, format and tests.
- `ROADMAP.md` describing the path beyond 1.0, including explicit non-goals and the 1.x
  compatibility promise.
- `CHANGELOG.md`.

### Changed

- The version string is now injected at build time with `-ldflags -X main.version`, so released
  binaries report their tag instead of `dev`.
- README rewritten around what RAP is actually for: reducing the tokens an agent spends per edit,
  and returning output shaped for the next decision in the loop rather than for a human reading a
  terminal.
- `make install` uses `install -m 0755` and reports the installed path and version.
- The sandbox diagnostic script moved to `scripts/sandbox-check.sh` and was translated to English.

### Fixed

- `.gitignore` replaced with a Go-appropriate one. The previous file was a Laravel template, which
  is why IDE state under `.idea/` had been committed; it is now untracked and ignored.
- All remaining Italian text removed from the repository's sources, documentation and scripts.

[Unreleased]: https://github.com/francescobianco/rap/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/francescobianco/rap/releases/tag/v1.0.0
