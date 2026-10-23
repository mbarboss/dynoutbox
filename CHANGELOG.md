# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this
project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Go module `github.com/mbarboss/dynoutbox` targeting Go 1.27.
- Task runner (`go run ./tools/task`) with `format`, `lint` and `check`, the same on
  Linux, macOS and Windows.
- pre-commit hooks for formatting, lint, secrets, private keys, large files, portable
  file names and workflow files.
- CI on Linux, macOS and Windows, a full-history secret scan, CodeQL and Dependabot.
- README, license, security policy and pull request template.
- Threat model based on the OWASP Top 10:2025 (`docs/threat-model.md`).
- Emulator spike findings and the choice of LocalStack for end-to-end tests
  (`docs/spikes/s1-emulator.md`).
- `NewEvent`, which builds a validated event: UUIDv7 ID, source, detail type, JSON
  object payload of at most 256 KiB, occurrence time in UTC and an optional correlation
  ID. Invalid input returns a `*ValidationError` that matches `ErrInvalidEvent` and never
  includes the rejected value.

### Security

- Go toolchain pinned to 1.27.3, which fixes GO-2026-6809 in `crypto/x509`. The library
  did not call the affected code.
