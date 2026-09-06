package metrics

import (
	"log"
	"strings"
)

// knownWorkflowFields are the values EXPORT_FIELDS accepts. Anything else
// would become a label name on github_workflow_run_status, and a name that
// isn't a valid Prometheus label (say "head-sha") makes the descriptor
// invalid — which takes down the whole exporter at MustRegister, not just
// the field that was misspelled.
var knownWorkflowFields = map[string]struct{}{
	"repo":        {},
	"id":          {},
	"node_id":     {},
	"head_branch": {},
	"head_sha":    {},
	"run_number":  {},
	"run_attempt": {},
	"workflow_id": {},
	"workflow":    {},
	"event":       {},
	"status":      {},
}

// reservedWorkflowFields are appended to the label set unconditionally, so
// listing them in EXPORT_FIELDS repeats a label name — the same invalid
// descriptor, the same MustRegister panic.
var reservedWorkflowFields = map[string]struct{}{
	"conclusion": {},
	"phase":      {},
}

// parseWorkflowFields turns the operator-supplied EXPORT_FIELDS into the
// label set for github_workflow_run_status. EXPORT_FIELDS is free text, and
// every way of getting it wrong — a repeat, a reserved name, a typo — builds
// an invalid descriptor that panics MustRegister, so the exporter refuses to
// start at all. Bad entries are dropped with a log line instead, leaving it
// running on a reduced label set.
func parseWorkflowFields(raw string) []string {
	seen := make(map[string]struct{})
	fields := make([]string, 0)

	for _, f := range strings.Split(raw, ",") {
		f = strings.TrimSpace(f)
		switch {
		case f == "":
			continue
		case !isKnownWorkflowField(f):
			log.Printf("EXPORT_FIELDS: ignoring unknown field %q", f)
		case isReservedWorkflowField(f):
			log.Printf("EXPORT_FIELDS: ignoring %q, it is always exported", f)
		default:
			if _, dup := seen[f]; dup {
				log.Printf("EXPORT_FIELDS: ignoring duplicate field %q", f)
				continue
			}
			seen[f] = struct{}{}
			fields = append(fields, f)
		}
	}

	return fields
}

func isKnownWorkflowField(f string) bool {
	_, ok := knownWorkflowFields[f]
	if ok {
		return true
	}
	return isReservedWorkflowField(f)
}

func isReservedWorkflowField(f string) bool {
	_, ok := reservedWorkflowFields[f]
	return ok
}
