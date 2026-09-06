package metrics

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/google/go-github/v45/github"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/spendesk/github-actions-exporter/pkg/config"
)

// initCollectorMetrics builds the metrics InitMetrics would normally create;
// the collector writes to them on every cycle and they are nil until then.
func initCollectorMetrics() {
	scrapeErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "github_exporter_scrape_errors_total", Help: "h"}, []string{"collector"})
	scrapeDurationSeconds = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Name: "github_exporter_scrape_duration_seconds", Help: "h"}, []string{"collector"})
	apiRateLimitRemaining = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Name: "github_exporter_api_rate_limit_remaining", Help: "h"}, []string{"resource"})
	apiRateLimitLimit = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Name: "github_exporter_api_rate_limit_limit", Help: "h"}, []string{"resource"})
}

func runsPayload() string {
	// Run 1 has finished; run 2 is still executing on a self-hosted runner.
	return `{"total_count":2,"workflow_runs":[
		{"id":1,"status":"completed","conclusion":"success","workflow_id":10,
		 "created_at":"2026-09-06T10:00:00Z","updated_at":"2026-09-06T10:05:00Z"},
		{"id":2,"status":"in_progress","conclusion":null,"workflow_id":10,
		 "created_at":"2026-09-06T10:10:00Z","updated_at":"2026-09-06T10:11:00Z"}
	]}`
}

// A completed run's jobs never change, so the collector must fetch them once
// and replay them; an executing run must be re-fetched every cycle, because it
// is the only thing that says which job a runner is on right now.
func TestJobsCacheRefetchesOnlyUnfinishedRuns(t *testing.T) {
	initCollectorMetrics()
	completedJobs = make(map[int64][]jobSample)
	jobStatusGauge.Reset()
	jobDurationHistogram.Reset()

	var mu sync.Mutex
	calls := map[string]int{}
	const cycles = 3

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/o/r/actions/runs", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls["runs"]++
		done := calls["runs"] >= cycles
		mu.Unlock()
		fmt.Fprint(w, runsPayload())
		if done {
			// Stop after the third listing so the assertions are deterministic.
			go cancel()
		}
	})
	for _, id := range []string{"1", "2"} {
		id := id
		mux.HandleFunc("/repos/o/r/actions/runs/"+id+"/jobs", func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			calls["jobs"+id]++
			mu.Unlock()
			status, conclusion, runner := "completed", `"success"`, "runner-a"
			if id == "2" {
				status, conclusion, runner = "in_progress", "null", "runner-b"
			}
			fmt.Fprintf(w, `{"total_count":1,"jobs":[{"id":%s01,"name":"build","status":%q,"conclusion":%s,
				"runner_name":%q,"started_at":"2026-09-06T10:01:00Z","completed_at":"2026-09-06T10:04:00Z"}]}`,
				id, status, conclusion, runner)
		})
	}

	srv := httptest.NewServer(mux)
	defer srv.Close()

	oldClient := client
	defer func() { client = oldClient }()
	client = github.NewClient(nil)
	client.BaseURL, _ = url.Parse(srv.URL + "/")

	setFixtureRepos()
	config.Github.Refresh = 0
	config.Metrics.WorkflowRunWindowHours = 12

	done := make(chan struct{})
	go func() { getJobsFromGithub(ctx); close(done) }()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("collector did not stop")
	}

	mu.Lock()
	defer mu.Unlock()
	if calls["jobs1"] != 1 {
		t.Errorf("completed run: %d job listings over %d cycles, want 1 (cached)", calls["jobs1"], cycles)
	}
	if calls["jobs2"] < 2 {
		t.Errorf("in-progress run: %d job listings over %d cycles, want one per cycle", calls["jobs2"], cycles)
	}

	// Fetching a finished run exactly once is also what stops its jobs being
	// observed into the histogram on every cycle.
	if n := testutil.CollectAndCount(jobDurationHistogram); n != 1 {
		t.Errorf("got %d duration histogram series, want 1", n)
	}
	if got := readHistogramCount(t, "o/r", "CI", "build"); got != 1 {
		t.Errorf("job observed %d times, want exactly 1", got)
	}
}

func readHistogramCount(t *testing.T, repo, workflow, job string) uint64 {
	t.Helper()
	reg := prometheus.NewRegistry()
	if err := reg.Register(jobDurationHistogram); err != nil {
		t.Fatalf("register: %v", err)
	}
	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, f := range families {
		for _, m := range f.GetMetric() {
			if m.GetHistogram() != nil {
				return m.GetHistogram().GetSampleCount()
			}
		}
	}
	return 0
}

// The running job must be exported, and carry the runner executing it.
func TestJobsExportInProgressWithRunner(t *testing.T) {
	initCollectorMetrics()
	completedJobs = make(map[int64][]jobSample)
	jobStatusGauge.Reset()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mux := http.NewServeMux()
	var once sync.Once
	mux.HandleFunc("/repos/o/r/actions/runs", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, runsPayload())
		once.Do(func() { go func() { time.Sleep(300 * time.Millisecond); cancel() }() })
	})
	mux.HandleFunc("/repos/o/r/actions/runs/1/jobs", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"total_count":1,"jobs":[{"id":101,"name":"build","status":"completed","conclusion":"success",
			"runner_name":"runner-a","started_at":"2026-09-06T10:01:00Z","completed_at":"2026-09-06T10:04:00Z"}]}`)
	})
	mux.HandleFunc("/repos/o/r/actions/runs/2/jobs", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"total_count":1,"jobs":[{"id":201,"name":"e2e","status":"in_progress","conclusion":null,
			"runner_name":"Windows-X86-CI-1","started_at":"2026-09-06T10:10:30Z"}]}`)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	oldClient := client
	defer func() { client = oldClient }()
	client = github.NewClient(nil)
	client.BaseURL, _ = url.Parse(srv.URL + "/")

	setFixtureRepos()
	config.Github.Refresh = 0
	config.Metrics.WorkflowRunWindowHours = 12

	done := make(chan struct{})
	go func() { getJobsFromGithub(ctx); close(done) }()
	<-done

	running := testutil.ToFloat64(jobStatusGauge.WithLabelValues(
		"o/r", "CI", "2", "e2e", "in_progress", "", "Windows-X86-CI-1"))
	if running != 1 {
		t.Errorf("in-progress job not exported for its runner (got %v)", running)
	}

	finished := testutil.ToFloat64(jobStatusGauge.WithLabelValues(
		"o/r", "CI", "1", "build", "completed", "success", "runner-a"))
	if finished != 1 {
		t.Errorf("completed job not exported (got %v)", finished)
	}
}

func setFixtureRepos() {
	mu.Lock()
	defer mu.Unlock()
	repositories = []string{"o/r"}
	name := "CI"
	id := int64(10)
	workflows = map[string]map[int64]github.Workflow{
		"o/r": {10: github.Workflow{ID: &id, Name: &name}},
	}
}
