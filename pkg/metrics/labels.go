package metrics

import (
	"sort"
	"strings"

	"github.com/google/go-github/v45/github"
	"github.com/spendesk/github-actions-exporter/pkg/config"
)

// joinPoolLabels renders a set of runs-on labels as one series value. Sorting
// matters: GitHub does not promise an order, and an unsorted join would mint a
// fresh series every time the same pool came back shuffled.
//
// Returns "" when the dimension is switched off, collapsing every pool onto a
// single series rather than changing the metric's shape.
func joinPoolLabels(labels []string) string {
	if !config.Metrics.RunnerLabels || len(labels) == 0 {
		return ""
	}
	out := make([]string, 0, len(labels))
	for _, l := range labels {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

// runnerPoolLabels is joinPoolLabels for the shape the runner endpoints return.
func runnerPoolLabels(labels []*github.RunnerLabels) string {
	names := make([]string, 0, len(labels))
	for _, l := range labels {
		names = append(names, l.GetName())
	}
	return joinPoolLabels(names)
}
