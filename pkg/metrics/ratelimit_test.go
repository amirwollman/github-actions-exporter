package metrics

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/go-github/v45/github"
	"github.com/gregjones/httpcache"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
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

// Responses served from the local HTTP cache carry the rate-limit headers
// stored when they were first fetched. Publishing those made the gauge
// alternate between the real remaining count and a stale higher one.
func TestCachedResponseDoesNotOverwriteRateLimit(t *testing.T) {
	apiRateLimitRemaining = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Name: "github_exporter_api_rate_limit_remaining", Help: "h"}, []string{"resource"})
	apiRateLimitLimit = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Name: "github_exporter_api_rate_limit_limit", Help: "h"}, []string{"resource"})

	live := &github.Response{Response: &http.Response{Header: http.Header{}}}
	live.Rate = github.Rate{Remaining: 2100, Limit: 5000}
	updateRateLimit(live)

	if got := testutil.ToFloat64(apiRateLimitRemaining.WithLabelValues("core")); got != 2100 {
		t.Fatalf("live response should set the gauge, got %v", got)
	}

	cached := &github.Response{Response: &http.Response{Header: http.Header{}}}
	cached.Header.Set(httpcache.XFromCache, "1")
	cached.Rate = github.Rate{Remaining: 4980, Limit: 5000}
	updateRateLimit(cached)

	if got := testutil.ToFloat64(apiRateLimitRemaining.WithLabelValues("core")); got != 2100 {
		t.Errorf("cached response overwrote the gauge with a stale value: %v", got)
	}
}
