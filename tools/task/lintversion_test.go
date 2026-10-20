package main

import (
	"errors"
	"testing"
)

func TestParseGolangciVersion(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want string
	}{
		{
			name: "release build output",
			out:  "golangci-lint has version 2.15.0 built with go1.27.0 from 25c5369a on 2026-10-16T11:03:09Z\n",
			want: "2.15.0",
		},
		{
			name: "version with v prefix",
			out:  "golangci-lint has version v2.15.0 built with go1.27.0",
			want: "2.15.0",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseGolangciVersion(tt.out)
			if err != nil {
				t.Fatalf("parseGolangciVersion() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("parseGolangciVersion() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseGolangciVersionRejectsUnknownOutput(t *testing.T) {
	for _, out := range []string{"", "golangci-lint has version (devel)", "command not found"} {
		t.Run(out, func(t *testing.T) {
			_, err := parseGolangciVersion(out)
			if !errors.Is(err, errUnknownVersionOutput) {
				t.Errorf("parseGolangciVersion(%q) error = %v, want %v", out, err, errUnknownVersionOutput)
			}
		})
	}
}
