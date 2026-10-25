package metrics

import (
	"encoding/json"
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

// The usage endpoint's payload, as documented at
// https://docs.github.com/rest/actions/workflows#get-workflow-usage. go-github
// v88 decodes "billable" into a map keyed by runner OS rather than fixed fields.
const workflowUsageJSON = `{
  "billable": {
    "UBUNTU": {"total_ms": 180000, "jobs": 1},
    "MACOS": {"total_ms": 240000, "jobs": 4},
    "WINDOWS": {"total_ms": 0, "jobs": 0}
  }
}`

func TestBillableMSFromAPIPayload(t *testing.T) {
	var usage github.WorkflowUsage
	if err := json.Unmarshal([]byte(workflowUsageJSON), &usage); err != nil {
		t.Fatalf("decoding usage: %v", err)
	}

	for os, want := range map[string]int64{"UBUNTU": 180000, "MACOS": 240000, "WINDOWS": 0} {
		if got := billableMS(&usage, os); got != want {
			t.Errorf("billableMS(%s) = %d, want %d", os, got, want)
		}
	}
}
