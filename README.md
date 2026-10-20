# dynoutbox

[![CI](https://github.com/mbarboss/dynoutbox/actions/workflows/ci.yml/badge.svg)](https://github.com/mbarboss/dynoutbox/actions/workflows/ci.yml)
[![CodeQL](https://github.com/mbarboss/dynoutbox/actions/workflows/codeql.yml/badge.svg)](https://github.com/mbarboss/dynoutbox/actions/workflows/codeql.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

A Go library for the transactional outbox pattern on AWS DynamoDB.

## Status

Early development. There is no public API yet and nothing is released. The design below
is the plan; the guarantees and limits will be documented once they are implemented and
tested.

## The problem

A service that updates its database and then publishes an event does two separate
writes. If the process crashes between them, or the publish fails, the database and
the rest of the system disagree: either a change happened and nobody heard about it, or
an event announced a change that never committed.

The outbox pattern fixes this by writing the event to the database in the same
transaction as the business change. A separate relay then reads committed events and
publishes them. An event exists if and only if the change committed.

Existing Go outbox libraries target SQL databases. dynoutbox targets DynamoDB.

## Planned design

```
app --Save()--> TransactWriteItems [business items + outbox Puts]
                         |
              outbox table stream (NEW_IMAGE)
                         |
      Lambda event source mapping (batching, bisect, partial batch failures)
                         |
       Lambda adapter --> relay core --PutEvents--> EventBridge bus --> consumers
                              |                                   (idempotency helper)
                              +--retries exhausted--> SQS dead-letter queue (full event)
```

- `Save()` adds one outbox item per event to the caller's `TransactWriteItems`, after
  checking DynamoDB and EventBridge size limits.
- The outbox is a separate, insert-only table. Items expire through TTL.
- The relay reads new outbox items from DynamoDB Streams and publishes them to
  EventBridge, retrying failed entries and sending events it cannot deliver to an SQS
  dead-letter queue with the full event.
- Delivery is at least once, so consumers can receive duplicates. A consumer-side helper
  records each processed event ID in the consumer's own transaction to make handlers
  idempotent.
- Ordering is not guaranteed, because EventBridge does not guarantee it.

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
- actionlint for GitHub Actions workflows;
- added files over 500 KB, private keys, merge conflict markers, file names that clash
  on case-insensitive file systems or are invalid on Windows, and CRLF line endings
  (fixed to LF).

If a hook changes a file, the commit stops so you can review the change and stage it
again. CI runs the same hooks on every pull request, plus `check` on Linux, macOS and
Windows and a gitleaks scan of the full history, so skipping hooks with `--no-verify`
only moves the failure there.

## Security

Report vulnerabilities privately as described in [SECURITY.md](SECURITY.md). Do not open
a public issue.

## License

[MIT](LICENSE)
