package metrics

import "testing"

func TestNormalizeWorkflowPhase(t *testing.T) {
	cases := []struct {
		conclusion string
		want       string
	}{
		{"", "running"},
		{"success", "success"},
		{"neutral", "success"},
		{"cancelled", "cancelled"},
		{"skipped", "cancelled"},
		{"stale", "cancelled"},
		{"failure", "failed"},
		{"timed_out", "failed"},
		{"action_required", "failed"},
		{"startup_failure", "failed"},
	}

	for _, c := range cases {
		if got := normalizeWorkflowPhase(c.conclusion); got != c.want {
			t.Errorf("normalizeWorkflowPhase(%q) = %q, want %q", c.conclusion, got, c.want)
		}
	}
}
