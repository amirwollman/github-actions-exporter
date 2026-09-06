package metrics

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func newRunnerGauges() (*prometheus.GaugeVec, *prometheus.GaugeVec) {
	status := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Name: "github_runner_status", Help: "h"},
		[]string{"organization", "name"},
	)
	busy := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Name: "github_runner_busy", Help: "h"},
		[]string{"organization", "name"},
	)
	return status, busy
}

func TestPublishRunnersReplacesPreviousSet(t *testing.T) {
	status, busy := newRunnerGauges()

	publishRunners(status, busy, []runnerSample{
		{labels: []string{"acme", "runner-a"}, online: true, busy: false},
		{labels: []string{"acme", "runner-b"}, online: false, busy: false},
	})

	if got := testutil.CollectAndCount(status); got != 2 {
		t.Fatalf("after first publish: got %d series, want 2", got)
	}

	// runner-b is gone from the second fetch; it must not linger.
	publishRunners(status, busy, []runnerSample{
		{labels: []string{"acme", "runner-a"}, online: false, busy: true},
	})

	if got := testutil.CollectAndCount(status); got != 1 {
		t.Fatalf("after second publish: got %d series, want 1", got)
	}

	expected := `
# HELP github_runner_status h
# TYPE github_runner_status gauge
github_runner_status{name="runner-a",organization="acme"} 0
`
	if err := testutil.CollectAndCompare(status, strings.NewReader(expected)); err != nil {
		t.Error(err)
	}
}

// An incomplete fetch must leave the last good values in place: a rate-limit
// pause or API error would otherwise blank github_runner_*_status, and an
// alert on "runner is offline" cannot fire on a series that has vanished.
func TestIncompleteFetchKeepsPreviousValues(t *testing.T) {
	status, busy := newRunnerGauges()

	publishRunners(status, busy, []runnerSample{
		{labels: []string{"acme", "runner-a"}, online: false, busy: false},
	})

	// Simulate a cycle whose listing failed: publishRunners is not called.
	if got := testutil.CollectAndCount(status); got != 1 {
		t.Fatalf("got %d series, want the previous 1 to survive", got)
	}
	if got := testutil.ToFloat64(status.WithLabelValues("acme", "runner-a")); got != 0 {
		t.Errorf("runner-a status = %v, want the offline 0 to survive", got)
	}
}

func TestBoolToFloat(t *testing.T) {
	if boolToFloat(true) != 1 {
		t.Error("boolToFloat(true) != 1")
	}
	if boolToFloat(false) != 0 {
		t.Error("boolToFloat(false) != 0")
	}
}
