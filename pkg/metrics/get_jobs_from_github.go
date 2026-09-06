package metrics

import (
	"context"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/spendesk/github-actions-exporter/pkg/config"

	"github.com/google/go-github/v45/github"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	jobStatusGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "github_job_status",
			Help: "Job status; value is 1. status is queued/in_progress/completed, conclusion is set once completed, runner_name names the runner executing it",
		},
		[]string{"repo", "workflow", "run_id", "job_name", "status", "conclusion", "runner_name"},
	)

	jobDurationHistogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "github_job_duration_seconds",
			Help:    "Duration of completed jobs in seconds",
			Buckets: []float64{5, 10, 30, 60, 120, 300, 600, 1200},
		},
		[]string{"repo", "workflow", "job_name"},
	)

	jobQueueDurationHistogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "github_job_queue_duration_seconds",
			Help:    "Time a job waited for a runner before starting",
			Buckets: []float64{1, 5, 10, 30, 60, 120, 300},
		},
		[]string{"repo", "workflow", "job_name"},
	)
)

// jobSample is one job's exported state, kept so a run's jobs can be rendered
// once and replayed on later cycles.
type jobSample struct {
	repo       string
	workflow   string
	runID      string
	jobName    string
	status     string
	conclusion string
	runnerName string
}

// completedJobs caches the jobs of runs that have finished, keyed by run ID.
// A finished run's jobs never change, but the gauge is reset every cycle, so
// without a cache repopulating it meant re-listing every run in the window on
// every pass — several hundred API calls per cycle against a 5000/hour quota,
// which is enough to exhaust it and stall every other collector on the
// rate-limit pause. Entries are dropped when the run leaves the window.
var completedJobs = make(map[int64][]jobSample)

// getAllJobsForRun returns a run's jobs and whether the listing completed.
// A partial listing must not be cached: it would be replayed as the run's
// jobs for as long as the run stays in the window.
func getAllJobsForRun(ctx context.Context, owner, repo string, runID int64) ([]*github.WorkflowJob, bool) {
	var jobs []*github.WorkflowJob
	opt := &github.ListWorkflowJobsOptions{
		ListOptions: github.ListOptions{PerPage: 200},
	}

	for {
		resp, rr, err := client.Actions.ListWorkflowJobs(ctx, owner, repo, runID, opt)
		if rl_err, ok := err.(*github.RateLimitError); ok {
			log.Printf("ListWorkflowJobs ratelimited. Pausing until %s", rl_err.Rate.Reset.Time.String())
			if !sleepWithContext(ctx, time.Until(rl_err.Rate.Reset.Time)) {
				return nil, false
			}
			continue
		} else if err != nil {
			log.Printf("ListWorkflowJobs error for %s/%s run %d: %s", owner, repo, runID, err.Error())
			scrapeErrorsTotal.WithLabelValues("jobs").Inc()
			return nil, false
		}
		updateRateLimit(rr)

		jobs = append(jobs, resp.Jobs...)
		if rr.NextPage == 0 {
			break
		}
		opt.Page = rr.NextPage
	}

	return jobs, true
}

func getJobsFromGithub(ctx context.Context) {
	for {
		start := time.Now()

		repos := getRepositories()
		wfs := getWorkflows()

		var samples []jobSample
		currentRunIDs := make(map[int64]struct{})
		complete := true

		for _, repo := range repos {
			r := strings.Split(repo, "/")

			// Includes runs that are still queued or executing: those are the
			// ones that say which job a runner is working on right now, and
			// they are the reason this collector cannot serve everything from
			// the cache.
			runs, ok := getRecentWorkflowRuns(ctx, r[0], r[1])
			if !ok {
				complete = false
				break
			}

			for _, run := range runs {
				workflowName := getWorkflowName(repo, run, wfs)
				if !isWorkflowAllowed(workflowName) {
					continue
				}

				runID := *run.ID
				currentRunIDs[runID] = struct{}{}

				if cached, ok := completedJobs[runID]; ok {
					samples = append(samples, cached...)
					continue
				}

				jobs, ok := getAllJobsForRun(ctx, r[0], r[1], runID)
				if !ok {
					complete = false
					continue
				}

				completed := run.GetStatus() == "completed"
				runSamples := make([]jobSample, 0, len(jobs))
				for _, job := range jobs {
					jobName := job.GetName()
					if !isJobAllowed(jobName) {
						continue
					}

					runSamples = append(runSamples, jobSample{
						repo:       repo,
						workflow:   workflowName,
						runID:      strconv.FormatInt(runID, 10),
						jobName:    jobName,
						status:     job.GetStatus(),
						conclusion: job.GetConclusion(),
						runnerName: job.GetRunnerName(),
					})

					if completed {
						observeJobDurations(repo, workflowName, jobName, run, job)
					}
				}

				samples = append(samples, runSamples...)

				// Caching only finished runs is also what keeps the histograms
				// honest: a finished run is fetched exactly once, so each job
				// is observed exactly once.
				if completed {
					completedJobs[runID] = runSamples
				}
			}
		}

		if complete {
			jobStatusGauge.Reset()
			for _, s := range samples {
				jobStatusGauge.WithLabelValues(s.repo, s.workflow, s.runID, s.jobName, s.status, s.conclusion, s.runnerName).Set(1)
			}

			for id := range completedJobs {
				if _, exists := currentRunIDs[id]; !exists {
					delete(completedJobs, id)
				}
			}
		} else {
			log.Printf("Incomplete job listing, keeping previously exported values")
		}

		scrapeDurationSeconds.WithLabelValues("jobs").Set(time.Since(start).Seconds())

		if !sleepWithContext(ctx, time.Duration(config.Github.Refresh)*time.Second) {
			return
		}
	}
}

func observeJobDurations(repo, workflow, jobName string, run *github.WorkflowRun, job *github.WorkflowJob) {
	if job.CompletedAt != nil && job.StartedAt != nil {
		if d := job.CompletedAt.Time.Sub(job.StartedAt.Time).Seconds(); d >= 0 {
			jobDurationHistogram.WithLabelValues(repo, workflow, jobName).Observe(d)
		}
	}

	if job.StartedAt != nil && run.CreatedAt != nil {
		if d := job.StartedAt.Time.Sub(run.CreatedAt.Time).Seconds(); d >= 0 {
			jobQueueDurationHistogram.WithLabelValues(repo, workflow, jobName).Observe(d)
		}
	}
}
