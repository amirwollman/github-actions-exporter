package metrics

import (
	"testing"

	"github.com/google/go-github/v88/github"
)

func TestBillableMS(t *testing.T) {
	bills := github.WorkflowBillMap{
		"UBUNTU": {TotalMS: github.Ptr(int64(1500))},
		"MACOS":  {},
	}

	cases := []struct {
		name  string
		usage *github.WorkflowUsage
		os    string
		want  int64
	}{
		{"reported", &github.WorkflowUsage{Billable: &bills}, "UBUNTU", 1500},
		{"no total", &github.WorkflowUsage{Billable: &bills}, "MACOS", 0},
		{"os missing", &github.WorkflowUsage{Billable: &bills}, "WINDOWS", 0},
		{"no billable map", &github.WorkflowUsage{}, "UBUNTU", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := billableMS(c.usage, c.os); got != c.want {
				t.Errorf("billableMS(%s) = %d, want %d", c.os, got, c.want)
			}
		})
	}
}
