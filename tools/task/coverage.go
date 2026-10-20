package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

var (
	errMalformedProfile   = errors.New("malformed coverage profile")
	errCoverageBelowFloor = errors.New("coverage below floor")
)

// coverage counts statements across the packages that count toward the floor.
type coverage struct {
	covered int
	total   int
}

func (c coverage) percent() float64 {
	if c.total == 0 {
		return 0
	}
	return float64(c.covered) * 100 / float64(c.total)
}

// parseCoverProfile reads a Go cover profile and sums statements, skipping
// blocks whose file path starts with any of excludePrefixes. A block that
// appears more than once (one entry per test binary) is counted once and is
// covered if any entry ran it.
func parseCoverProfile(r io.Reader, excludePrefixes []string) (coverage, error) {
	type block struct {
		stmts   int
		covered bool
	}
	blocks := make(map[string]block)

	sc := bufio.NewScanner(r)
	if !sc.Scan() || !strings.HasPrefix(sc.Text(), "mode: ") {
		if err := sc.Err(); err != nil {
			return coverage{}, fmt.Errorf("read coverage profile: %w", err)
		}
		return coverage{}, fmt.Errorf("%w: missing mode header", errMalformedProfile)
	}

	line := 1
	for sc.Scan() {
		line++
		text := strings.TrimSpace(sc.Text())
		if text == "" {
			continue
		}
		key, stmts, count, err := parseBlock(text)
		if err != nil {
			return coverage{}, fmt.Errorf("%w: line %d: %w", errMalformedProfile, line, err)
		}
		if hasAnyPrefix(key, excludePrefixes) {
			continue
		}
		b := blocks[key]
		b.stmts = stmts
		b.covered = b.covered || count > 0
		blocks[key] = b
	}
	if err := sc.Err(); err != nil {
		return coverage{}, fmt.Errorf("read coverage profile: %w", err)
	}

	var c coverage
	for _, b := range blocks {
		c.total += b.stmts
		if b.covered {
			c.covered += b.stmts
		}
	}
	return c, nil
}

// parseBlock splits "file.go:1.1,2.2 <statements> <count>" into its parts.
func parseBlock(text string) (key string, stmts, count int, err error) {
	fields := strings.Fields(text)
	if len(fields) != 3 {
		return "", 0, 0, fmt.Errorf("want 3 fields, got %d", len(fields))
	}
	if !strings.Contains(fields[0], ":") {
		return "", 0, 0, errors.New("block has no position")
	}
	stmts, err = strconv.Atoi(fields[1])
	if err != nil || stmts < 0 {
		return "", 0, 0, fmt.Errorf("invalid statement count %q", fields[1])
	}
	count, err = strconv.Atoi(fields[2])
	if err != nil || count < 0 {
		return "", 0, 0, fmt.Errorf("invalid hit count %q", fields[2])
	}
	return fields[0], stmts, count, nil
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// checkFloor fails when coverage is below floor. A profile with no statements
// passes: the floor starts to apply once the first library package exists.
func checkFloor(c coverage, floor float64) error {
	if c.total == 0 {
		return nil
	}
	if c.percent() < floor {
		return fmt.Errorf("%w: %.1f%% < %.1f%%", errCoverageBelowFloor, c.percent(), floor)
	}
	return nil
}
