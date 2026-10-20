// Command task is the project task runner: `go run ./tools/task <format|lint|check>`.
//
// It is written in Go so it runs the same way on Linux, macOS and Windows
// without a Makefile or shell scripts. Commands are spawned with argument
// lists, never through a shell.
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
)

const (
	// golangciVersion must match the installed binary; formatting and lint
	// results differ between versions.
	golangciVersion = "2.15.0"
	// govulncheckVersion is run with `go run pkg@version` so it never enters go.mod.
	govulncheckVersion = "v1.9.0"

	coverProfile  = "coverage.out"
	coverageFloor = 85.0
	modulePath    = "github.com/mbarboss/dynoutbox"
)

// Exit codes.
const (
	exitOK      = 0
	exitFailure = 1
	exitUsage   = 2
)

const usage = "usage: go run ./tools/task <format|lint|check>"

// toolPrefixes are excluded from the coverage floor, which applies to library code.
var toolPrefixes = []string{modulePath + "/tools/"}

type commandRunner interface {
	Run(ctx context.Context, name string, args ...string) error
	Output(ctx context.Context, name string, args ...string) (string, error)
}

type deps struct {
	runner   commandRunner
	stdout   io.Writer
	stderr   io.Writer
	readFile func(name string) ([]byte, error)
}

type step struct {
	name string
	run  func(ctx context.Context, d deps) error
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := run(ctx, os.Args[1:], deps{
		runner:   execRunner{stdout: os.Stdout, stderr: os.Stderr},
		stdout:   os.Stdout,
		stderr:   os.Stderr,
		readFile: os.ReadFile,
	})
	stop()
	os.Exit(code)
}

func run(ctx context.Context, args []string, d deps) int {
	if len(args) != 1 {
		_, _ = fmt.Fprintln(d.stderr, usage)
		return exitUsage
	}
	var steps []step
	switch args[0] {
	case "format":
		steps = formatSteps()
	case "lint":
		steps = lintSteps()
	case "check":
		steps = checkSteps()
	default:
		_, _ = fmt.Fprintln(d.stderr, usage)
		return exitUsage
	}

	for _, s := range steps {
		_, _ = fmt.Fprintf(d.stdout, "==> %s\n", s.name)
		if err := s.run(ctx, d); err != nil {
			_, _ = fmt.Fprintf(d.stderr, "task: %s failed: %v\n", s.name, err)
			return exitFailure
		}
	}
	return exitOK
}

func formatSteps() []step {
	return []step{
		{name: "linter version", run: requireGolangciVersion},
		command("format", "golangci-lint", "fmt"),
	}
}

// lintSteps is the fast subset of check used by the pre-commit hook.
func lintSteps() []step {
	return []step{
		{name: "linter version", run: requireGolangciVersion},
		command("lint", "golangci-lint", "run"),
	}
}

func checkSteps() []step {
	steps := []step{command("go.mod tidy", "go", "mod", "tidy", "-diff")}
	steps = append(steps, lintSteps()...)
	return append(steps,
		command("test", "go", "test", "-race", "-covermode=atomic", "-coverprofile="+coverProfile, "./..."),
		step{name: "coverage floor", run: requireCoverageFloor},
		command("vulnerabilities", "go", "run", "golang.org/x/vuln/cmd/govulncheck@"+govulncheckVersion, "./..."),
	)
}

func command(name, program string, args ...string) step {
	return step{name: name, run: func(ctx context.Context, d deps) error {
		return d.runner.Run(ctx, program, args...)
	}}
}

func requireGolangciVersion(ctx context.Context, d deps) error {
	out, err := d.runner.Output(ctx, "golangci-lint", "version")
	if err != nil {
		return fmt.Errorf("golangci-lint %s is required (https://golangci-lint.run/docs/welcome/install/): %w", golangciVersion, err)
	}
	got, err := parseGolangciVersion(out)
	if err != nil {
		return err
	}
	if got != golangciVersion {
		return fmt.Errorf("golangci-lint %s is required, found %s", golangciVersion, got)
	}
	return nil
}

func requireCoverageFloor(_ context.Context, d deps) error {
	data, err := d.readFile(coverProfile)
	if err != nil {
		return fmt.Errorf("read %s: %w", coverProfile, err)
	}
	c, err := parseCoverProfile(bytes.NewReader(data), toolPrefixes)
	if err != nil {
		return err
	}
	if c.total == 0 {
		_, _ = fmt.Fprintln(d.stdout, "no library statements yet; coverage floor not applied")
		return nil
	}
	_, _ = fmt.Fprintf(d.stdout, "library coverage %.1f%% (floor %.1f%%)\n", c.percent(), coverageFloor)
	return checkFloor(c, coverageFloor)
}

// execRunner runs real processes, streaming their output to the terminal.
type execRunner struct {
	stdout io.Writer
	stderr io.Writer
}

func (r execRunner) Run(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // G204: programs come only from the fixed step list in this file
	cmd.Stdout = r.stdout
	cmd.Stderr = r.stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

func (r execRunner) Output(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // G204: programs come only from the fixed step list in this file
	cmd.Stderr = r.stderr
	out, err := cmd.Output()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return "", fmt.Errorf("%s not found in PATH: %w", name, err)
		}
		return "", fmt.Errorf("%s: %w", name, err)
	}
	return string(out), nil
}
