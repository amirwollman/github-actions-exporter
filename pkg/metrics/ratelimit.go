package metrics

import (
	"context"
	"log"
	"time"

	"github.com/google/go-github/v45/github"
	"github.com/prometheus/client_golang/prometheus"
)

// GitHub can hand back a reset timestamp that has already passed: the clock
// skews, or the window rolls over between the response and our handling of it.
// time.Until is then negative, and sleeping for a negative duration returns
// immediately - so the caller retries at once, is limited again, and spins at
// full speed against the API precisely while it is asking us to back off. That
// loop is what exhausts the hourly quota. Flooring every pause is what stops it.
const (
	minRateLimitPause = 5 * time.Second
	maxRateLimitPause = 15 * time.Minute
)

var rateLimitPausesTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "github_exporter_rate_limit_pauses_total",
		Help: "Times a collector was paused by a GitHub rate limit, by collector and limit kind (primary/secondary)",
	},
	[]string{"collector", "kind"},
)

// pauseForRateLimit backs off when err is a rate-limit error. It reports
// whether err was a rate limit at all, and if so whether the caller should
// retry (false means the context was cancelled mid-pause).
func pauseForRateLimit(ctx context.Context, err error, collector, call string) (retry bool, isRateLimit bool) {
	var (
		wait time.Duration
		kind string
	)

	switch e := err.(type) {
	case *github.RateLimitError:
		wait, kind = time.Until(e.Rate.Reset.Time), "primary"
	case *github.AbuseRateLimitError:
		// Secondary limits carry a retry-after instead of a reset time, and
		// are the ones a burst of concurrent requests trips.
		kind = "secondary"
		if e.RetryAfter != nil {
			wait = *e.RetryAfter
		}
	default:
		return false, false
	}

	if wait < minRateLimitPause {
		wait = minRateLimitPause
	}
	if wait > maxRateLimitPause {
		wait = maxRateLimitPause
	}

	rateLimitPausesTotal.WithLabelValues(collector, kind).Inc()
	log.Printf("%s hit a %s rate limit, pausing %s", call, kind, wait.Truncate(time.Second))

	return sleepWithContext(ctx, wait), true
}
