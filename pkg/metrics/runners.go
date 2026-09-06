package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// runnerSample is one runner's state, captured while fetching so that a
// collector can publish a whole set at once rather than as it goes.
type runnerSample struct {
	labels []string
	online bool
	busy   bool
}

// publishRunners swaps a freshly fetched set of runners into the status and
// busy gauges. Callers must only reach here once every API call in the cycle
// succeeded: publishing resets the gauges first, so publishing a partial set
// would blank the runner-down signal exactly when the GitHub API is failing
// or rate limiting us — the moment the signal matters most.
func publishRunners(status, busy *prometheus.GaugeVec, samples []runnerSample) {
	status.Reset()
	busy.Reset()
	for _, s := range samples {
		status.WithLabelValues(s.labels...).Set(boolToFloat(s.online))
		busy.WithLabelValues(s.labels...).Set(boolToFloat(s.busy))
	}
}

func boolToFloat(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
