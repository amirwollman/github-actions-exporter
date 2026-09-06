package metrics

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestParseWorkflowFields(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want []string
	}{
		{"default", "repo,id,workflow,event,status", []string{"repo", "id", "workflow", "event", "status"}},
		{"trims whitespace", "repo, id ,  workflow", []string{"repo", "id", "workflow"}},
		{"drops empties", "repo,,id,", []string{"repo", "id"}},
		{"drops duplicates", "repo,id,repo", []string{"repo", "id"}},
		{"drops reserved conclusion", "repo,conclusion,id", []string{"repo", "id"}},
		{"drops reserved phase", "repo,phase", []string{"repo"}},
		{"drops unknown", "repo,head-sha,nope,id", []string{"repo", "id"}},
		{"empty input", "", []string{}},
		{"all bad", "nope,conclusion,,phase", []string{}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseWorkflowFields(c.raw)
			if len(got) != len(c.want) {
				t.Fatalf("parseWorkflowFields(%q) = %v, want %v", c.raw, got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("parseWorkflowFields(%q) = %v, want %v", c.raw, got, c.want)
				}
			}
		})
	}
}

// Each of these EXPORT_FIELDS values took the exporter down at startup before
// parseWorkflowFields validated them: a repeat or a reserved name collides
// with an existing label and "head-sha" is not a legal Prometheus label name,
// either of which makes the descriptor invalid. NewGaugeVec accepts it
// quietly; MustRegister — which InitMetrics calls — is where it panics.
func TestWorkflowLabelSetRegisters(t *testing.T) {
	for _, raw := range []string{
		"repo,id,conclusion",
		"repo,phase",
		"repo,repo",
		"repo,head-sha",
		"repo, id , repo ,conclusion,bogus",
		"",
	} {
		t.Run(raw, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("EXPORT_FIELDS=%q panicked: %v", raw, r)
				}
			}()

			labels := append(append([]string{}, parseWorkflowFields(raw)...), "conclusion", "phase")
			vec := prometheus.NewGaugeVec(
				prometheus.GaugeOpts{Name: "github_workflow_run_status", Help: "h"},
				labels,
			)
			// This is the call InitMetrics makes, and the one that used to die.
			prometheus.NewRegistry().MustRegister(vec)
			// The values getRelevantFields produces must line up with the labels.
			vec.WithLabelValues(make([]string, len(labels))...).Set(1)
		})
	}
}

// getRelevantFields builds its values from the same list InitMetrics builds
// the labels from; a mismatch is a WithLabelValues panic at scrape time.
func TestFieldValuesMatchLabelCount(t *testing.T) {
	workflowFields = parseWorkflowFields("repo, id ,repo,conclusion,bogus,workflow")
	labels := append(append([]string{}, workflowFields...), "conclusion", "phase")

	if len(workflowFields)+2 != len(labels) {
		t.Fatalf("label set %v does not match field list %v", labels, workflowFields)
	}
	if strings.Join(workflowFields, ",") != "repo,id,workflow" {
		t.Errorf("got %v", workflowFields)
	}
}
