package metrics

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-github/v45/github"
	"github.com/prometheus/client_golang/prometheus"
)

func newPauseCounter() {
	rateLimitPausesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "github_exporter_rate_limit_pauses_total", Help: "h"},
		[]string{"collector", "kind"})
}

// A reset timestamp in the past used to make time.Until negative, so the sleep
// returned instantly and the caller retried in a tight loop against the very
// API that was rate limiting it.
func TestPastResetStillPauses(t *testing.T) {
	newPauseCounter()
	err := &github.RateLimitError{Rate: github.Rate{Reset: github.Timestamp{Time: time.Now().Add(-time.Hour)}}}

	start := time.Now()
	retry, isRL := pauseForRateLimit(context.Background(), err, "test", "Call")
	elapsed := time.Since(start)

	if !isRL || !retry {
		t.Fatalf("expected a rate limit that retries, got isRL=%v retry=%v", isRL, retry)
	}
	if elapsed < minRateLimitPause {
		t.Errorf("paused %s, expected at least %s", elapsed, minRateLimitPause)
	}
}

func TestPauseIsCappedAndCancellable(t *testing.T) {
	newPauseCounter()
	err := &github.RateLimitError{Rate: github.Rate{Reset: github.Timestamp{Time: time.Now().Add(24 * time.Hour)}}}

	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(50 * time.Millisecond); cancel() }()

	start := time.Now()
	retry, isRL := pauseForRateLimit(ctx, err, "test", "Call")

	if !isRL {
		t.Fatal("expected a rate limit error to be recognised")
	}
	if retry {
		t.Error("a cancelled context must not ask the caller to retry")
	}
	if time.Since(start) > maxRateLimitPause {
		t.Errorf("pause exceeded the %s cap", maxRateLimitPause)
	}
}

func TestSecondaryRateLimitUsesRetryAfter(t *testing.T) {
	newPauseCounter()
	after := 20 * time.Second
	err := &github.AbuseRateLimitError{RetryAfter: &after}

	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(50 * time.Millisecond); cancel() }()

	if _, isRL := pauseForRateLimit(ctx, err, "test", "Call"); !isRL {
		t.Error("secondary rate limits must be recognised, not treated as generic errors")
	}
}

func TestNonRateLimitErrorIsNotHandled(t *testing.T) {
	newPauseCounter()
	if retry, isRL := pauseForRateLimit(context.Background(), context.Canceled, "test", "Call"); isRL || retry {
		t.Errorf("a plain error must fall through, got isRL=%v retry=%v", isRL, retry)
	}
}
