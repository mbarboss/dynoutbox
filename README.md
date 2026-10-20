# dynoutbox

A Go library for the transactional outbox pattern on AWS DynamoDB.

Status: early development. The library API does not exist yet. This file only covers
the development setup for now.

## Development setup

You need four tools. Commands are given for Linux, macOS and Windows.

### 1. Go

Install any recent Go from https://go.dev/dl/. The module pins its toolchain
(`toolchain go1.27.2` in `go.mod`), and Go downloads that version on first use as long
as `GOTOOLCHAIN` is left at its default (`auto`).

### 2. A C compiler

`check` runs the tests with `-race`, which needs cgo and a C compiler.

- Linux (Debian, Ubuntu): `sudo apt install gcc`
- macOS: `xcode-select --install`
- Windows: a MinGW-w64 `gcc` in `PATH`, for example from MSYS2 (https://www.msys2.org/)

### 3. golangci-lint 2.15.0

The task runner refuses other versions, because lint and format results change between
releases. Install the official binary, not `go install`.

- Linux and macOS:

  ```sh
  curl -sSfL https://golangci-lint.run/install.sh | sh -s -- -b "$(go env GOPATH)/bin" v2.15.0
  ```

- Windows: run the same command in Git Bash, or download
  `golangci-lint-2.15.0-windows-amd64.zip` from
  https://github.com/golangci/golangci-lint/releases/tag/v2.15.0 and put
  `golangci-lint.exe` in a directory on your `PATH`.

Make sure `$(go env GOPATH)/bin` is on your `PATH`, then confirm with
`golangci-lint version`.

### 4. pre-commit 4.7.0 (through uv)

Git hooks are managed by [pre-commit](https://pre-commit.com/). Install it with
[uv](https://docs.astral.sh/uv/), which also provides the Python it needs.

- Install uv on Linux and macOS:

  ```sh
  curl -LsSf https://astral.sh/uv/install.sh | sh
  ```

- Install uv on Windows (PowerShell):

  ```powershell
  powershell -ExecutionPolicy ByPass -c "irm https://astral.sh/uv/install.ps1 | iex"
  ```

Then, on any OS, from the repository root:

```sh
uv tool install pre-commit==4.7.0
pre-commit install
```

The first commit after `pre-commit install` takes a minute or two while pre-commit
builds gitleaks and sets up its hook environments. Later runs use the cache.

## Tasks

| Command | What it does |
| --- | --- |
| `go run ./tools/task format` | Formats Go code (gofumpt, goimports). |
| `go run ./tools/task lint` | Runs golangci-lint. |
| `go run ./tools/task check` | Tidy check, lint, tests with `-race`, 85% coverage floor on library packages, govulncheck. Run it before opening a pull request. |
| `pre-commit run --all-files` | Runs every git hook on the whole repository. |

## Git hooks

On each commit, pre-commit checks the staged files:

- `format` and `lint` through the task runner, when Go files, `go.mod`, `go.sum` or
  `.golangci.yml` change;
- gitleaks for hardcoded secrets (findings are redacted in the output);
- added files over 500 KB, private keys, merge conflict markers, file names that clash
  on case-insensitive file systems or are invalid on Windows, and CRLF line endings
  (fixed to LF).

If a hook changes a file, the commit stops so you can review the change and stage it
again. Hooks are a fast local check and can be skipped with `--no-verify`, so they are
not a security control on their own.
