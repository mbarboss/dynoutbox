package main

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"slices"
	"strings"
	"testing"
)

// fakeRunner records every command and fails the first one whose program and
// first argument match failOn.
type fakeRunner struct {
	calls   []string
	failOn  string
	version string
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) error {
	call := strings.Join(append([]string{name}, args...), " ")
	f.calls = append(f.calls, call)
	if f.failOn != "" && strings.HasPrefix(call, f.failOn) {
		return errors.New("exit status 1")
	}
	return nil
}

func (f *fakeRunner) Output(ctx context.Context, name string, args ...string) (string, error) {
	if err := f.Run(ctx, name, args...); err != nil {
		return "", err
	}
	return "golangci-lint has version " + f.version + " built with go1.27.0", nil
}

func newDeps(r *fakeRunner, profile string) (deps, *bytes.Buffer) {
	var out bytes.Buffer
	return deps{
		runner: r,
		stdout: &out,
		stderr: &out,
		readFile: func(_ string) ([]byte, error) {
			if profile == "" {
				return nil, fs.ErrNotExist
			}
			return []byte(profile), nil
		},
	}, &out
}

const fullProfile = "mode: atomic\ngithub.com/mbarboss/dynoutbox/a.go:1.1,2.2 1 1\n"

func TestRunCheckRunsStepsInOrder(t *testing.T) {
	r := &fakeRunner{version: golangciVersion}
	d, _ := newDeps(r, fullProfile)

	code := run(context.Background(), []string{"check"}, d)

	if code != exitOK {
		t.Fatalf("run() = %d, want %d", code, exitOK)
	}
	want := []string{
		"go mod tidy -diff",
		"golangci-lint version",
		"golangci-lint run",
		"go test -race -covermode=atomic -coverprofile=" + coverProfile + " ./...",
		"go run golang.org/x/vuln/cmd/govulncheck@" + govulncheckVersion + " ./...",
	}
	if !slices.Equal(r.calls, want) {
		t.Errorf("calls =\n%q\nwant\n%q", r.calls, want)
	}
}

func TestRunFormatRunsFormatter(t *testing.T) {
	r := &fakeRunner{version: golangciVersion}
	d, _ := newDeps(r, "")

	code := run(context.Background(), []string{"format"}, d)

	if code != exitOK {
		t.Fatalf("run() = %d, want %d", code, exitOK)
	}
	want := []string{"golangci-lint version", "golangci-lint fmt"}
	if !slices.Equal(r.calls, want) {
		t.Errorf("calls = %q, want %q", r.calls, want)
	}
}

func TestRunLintRunsLinterOnly(t *testing.T) {
	r := &fakeRunner{version: golangciVersion}
	d, _ := newDeps(r, "")

	code := run(context.Background(), []string{"lint"}, d)

	if code != exitOK {
		t.Fatalf("run() = %d, want %d", code, exitOK)
	}
	want := []string{"golangci-lint version", "golangci-lint run"}
	if !slices.Equal(r.calls, want) {
		t.Errorf("calls = %q, want %q", r.calls, want)
	}
}

func TestRunLintFailsWhenLinterReportsIssues(t *testing.T) {
	r := &fakeRunner{version: golangciVersion, failOn: "golangci-lint run"}
	d, out := newDeps(r, "")

	code := run(context.Background(), []string{"lint"}, d)

	if code != exitFailure {
		t.Fatalf("run() = %d, want %d", code, exitFailure)
	}
	if !strings.Contains(out.String(), "lint failed") {
		t.Errorf("output %q does not name the failed step", out.String())
	}
}

func TestRunLintFailsOnWrongLinterVersion(t *testing.T) {
	r := &fakeRunner{version: "2.0.0"}
	d, out := newDeps(r, "")

	code := run(context.Background(), []string{"lint"}, d)

	if code != exitFailure {
		t.Fatalf("run() = %d, want %d", code, exitFailure)
	}
	if slices.Contains(r.calls, "golangci-lint run") {
		t.Error("lint ran with the wrong linter version")
	}
	if !strings.Contains(out.String(), golangciVersion) {
		t.Errorf("output %q does not name the required version", out.String())
	}
}

func TestUsageListsEveryCommand(t *testing.T) {
	for _, name := range []string{"format", "lint", "check"} {
		if !strings.Contains(usage, name) {
			t.Errorf("usage %q does not list %q", usage, name)
		}
	}
}

func TestRunCheckStopsAtFirstFailure(t *testing.T) {
	r := &fakeRunner{version: golangciVersion, failOn: "golangci-lint run"}
	d, out := newDeps(r, fullProfile)

	code := run(context.Background(), []string{"check"}, d)

	if code != exitFailure {
		t.Fatalf("run() = %d, want %d", code, exitFailure)
	}
	if last := r.calls[len(r.calls)-1]; last != "golangci-lint run" {
		t.Errorf("last call = %q, want the failing lint step", last)
	}
	if !strings.Contains(out.String(), "lint") {
		t.Errorf("output %q does not name the failed step", out.String())
	}
}

func TestRunCheckFailsOnWrongLinterVersion(t *testing.T) {
	r := &fakeRunner{version: "2.0.0"}
	d, out := newDeps(r, fullProfile)

	code := run(context.Background(), []string{"check"}, d)

	if code != exitFailure {
		t.Fatalf("run() = %d, want %d", code, exitFailure)
	}
	if slices.Contains(r.calls, "golangci-lint run") {
		t.Error("lint ran with the wrong linter version")
	}
	if !strings.Contains(out.String(), golangciVersion) {
		t.Errorf("output %q does not name the required version", out.String())
	}
}

func TestRunCheckFailsBelowCoverageFloor(t *testing.T) {
	r := &fakeRunner{version: golangciVersion}
	d, _ := newDeps(r, "mode: atomic\ngithub.com/mbarboss/dynoutbox/a.go:1.1,2.2 1 0\n")

	code := run(context.Background(), []string{"check"}, d)

	if code != exitFailure {
		t.Fatalf("run() = %d, want %d", code, exitFailure)
	}
	if slices.ContainsFunc(r.calls, func(c string) bool { return strings.Contains(c, "govulncheck") }) {
		t.Error("govulncheck ran after the coverage floor failed")
	}
}

func TestRunCheckFailsWhenProfileIsMissing(t *testing.T) {
	r := &fakeRunner{version: golangciVersion}
	d, _ := newDeps(r, "")

	code := run(context.Background(), []string{"check"}, d)

	if code != exitFailure {
		t.Fatalf("run() = %d, want %d", code, exitFailure)
	}
}

func TestRunRejectsUsageErrors(t *testing.T) {
	for _, args := range [][]string{nil, {"deploy"}, {"check", "extra"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			r := &fakeRunner{version: golangciVersion}
			d, out := newDeps(r, fullProfile)

			code := run(context.Background(), args, d)

			if code != exitUsage {
				t.Errorf("run() = %d, want %d", code, exitUsage)
			}
			if len(r.calls) != 0 {
				t.Errorf("ran %q on a usage error", r.calls)
			}
			if !strings.Contains(out.String(), "usage") {
				t.Errorf("output %q has no usage text", out.String())
			}
		})
	}
}
