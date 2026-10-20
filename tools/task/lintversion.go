package main

import (
	"errors"
	"fmt"
	"regexp"
)

var errUnknownVersionOutput = errors.New("unknown golangci-lint version output")

var golangciVersionRe = regexp.MustCompile(`has version v?(\d+\.\d+\.\d+)`)

// parseGolangciVersion extracts "X.Y.Z" from the output of `golangci-lint version`.
func parseGolangciVersion(out string) (string, error) {
	m := golangciVersionRe.FindStringSubmatch(out)
	if m == nil {
		return "", fmt.Errorf("%w: %q", errUnknownVersionOutput, out)
	}
	return m[1], nil
}
