package main

import (
	"errors"
	"strings"
	"testing"
)

const toolsPrefix = "github.com/mbarboss/dynoutbox/tools/"

func TestParseCoverProfile(t *testing.T) {
	tests := []struct {
		name    string
		profile string
		want    coverage
	}{
		{
			name:    "header only has no statements",
			profile: "mode: atomic\n",
			want:    coverage{},
		},
		{
			name: "counts covered and total statements",
			profile: "mode: atomic\n" +
				"github.com/mbarboss/dynoutbox/a.go:1.1,2.2 3 1\n" +
				"github.com/mbarboss/dynoutbox/a.go:3.1,4.2 1 0\n",
			want: coverage{covered: 3, total: 4},
		},
		{
			name: "excludes tool packages",
			profile: "mode: atomic\n" +
				"github.com/mbarboss/dynoutbox/a.go:1.1,2.2 2 1\n" +
				"github.com/mbarboss/dynoutbox/tools/task/main.go:1.1,9.2 10 0\n",
			want: coverage{covered: 2, total: 2},
		},
		{
			name: "duplicate block counts once and is covered if any run covered it",
			profile: "mode: atomic\n" +
				"github.com/mbarboss/dynoutbox/a.go:1.1,2.2 2 0\n" +
				"github.com/mbarboss/dynoutbox/a.go:1.1,2.2 2 5\n",
			want: coverage{covered: 2, total: 2},
		},
		{
			name: "ignores blank lines",
			profile: "mode: atomic\n\n" +
				"github.com/mbarboss/dynoutbox/a.go:1.1,2.2 1 1\n",
			want: coverage{covered: 1, total: 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseCoverProfile(strings.NewReader(tt.profile), []string{toolsPrefix})
			if err != nil {
				t.Fatalf("parseCoverProfile() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("parseCoverProfile() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestParseCoverProfileRejectsMalformedInput(t *testing.T) {
	tests := []struct {
		name    string
		profile string
	}{
		{name: "empty input", profile: ""},
		{name: "missing mode header", profile: "github.com/x/a.go:1.1,2.2 1 1\n"},
		{name: "too few fields", profile: "mode: set\ngithub.com/x/a.go:1.1,2.2 1\n"},
		{name: "non-numeric statements", profile: "mode: set\ngithub.com/x/a.go:1.1,2.2 x 1\n"},
		{name: "negative statements", profile: "mode: set\ngithub.com/x/a.go:1.1,2.2 -1 1\n"},
		{name: "non-numeric count", profile: "mode: set\ngithub.com/x/a.go:1.1,2.2 1 y\n"},
		{name: "block without position", profile: "mode: set\ngithub.com/x/a.go 1 1\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseCoverProfile(strings.NewReader(tt.profile), nil)
			if !errors.Is(err, errMalformedProfile) {
				t.Errorf("parseCoverProfile() error = %v, want %v", err, errMalformedProfile)
			}
		})
	}
}

func TestCheckFloor(t *testing.T) {
	tests := []struct {
		name    string
		cov     coverage
		floor   float64
		wantErr error
	}{
		{name: "no statements passes", cov: coverage{}, floor: 85},
		{name: "exactly at floor passes", cov: coverage{covered: 85, total: 100}, floor: 85},
		{name: "above floor passes", cov: coverage{covered: 9, total: 10}, floor: 85},
		{name: "below floor fails", cov: coverage{covered: 84, total: 100}, floor: 85, wantErr: errCoverageBelowFloor},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkFloor(tt.cov, tt.floor)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("checkFloor() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
