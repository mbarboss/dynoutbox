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
