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
			Help: "Job status; value is 1, conclusion carried as label",
		},
		[]string{"repo", "workflow", "run_id", "job_name", "conclusion", "runner_name"},
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

// observedJobRuns tracks run IDs whose jobs have already been observed.
var observedJobRuns = make(map[int64]struct{})

func getAllJobsForRun(ctx context.Context, owner, repo string, runID int64) []*github.WorkflowJob {
	var jobs []*github.WorkflowJob
	opt := &github.ListWorkflowJobsOptions{
		ListOptions: github.ListOptions{PerPage: 200},
	}

	for {
		resp, rr, err := client.Actions.ListWorkflowJobs(ctx, owner, repo, runID, opt)
		if rl_err, ok := err.(*github.RateLimitError); ok {
			log.Printf("ListWorkflowJobs ratelimited. Pausing until %s", rl_err.Rate.Reset.Time.String())
			if !sleepWithContext(ctx, time.Until(rl_err.Rate.Reset.Time)) {
				return jobs
			}
			continue
		} else if err != nil {
			log.Printf("ListWorkflowJobs error for %s/%s run %d: %s", owner, repo, runID, err.Error())
			scrapeErrorsTotal.WithLabelValues("jobs").Inc()
			return jobs
		}
		updateRateLimit(rr)

		jobs = append(jobs, resp.Jobs...)
		if rr.NextPage == 0 {
			break
		}
		opt.Page = rr.NextPage
	}

	return jobs
}

func getJobsFromGithub(ctx context.Context) {
	for {
		start := time.Now()
		jobStatusGauge.Reset()

		repos := getRepositories()
		wfs := getWorkflows()
		currentRunIDs := make(map[int64]struct{})

		for _, repo := range repos {
			r := strings.Split(repo, "/")
			runs := getRecentWorkflowRuns(ctx, r[0], r[1])

			for _, run := range runs {
				conclusion := run.GetConclusion()
				isCompleted := conclusion != "" && conclusion != "in_progress" && conclusion != "queued"
				if !isCompleted {
					continue
				}

				currentRunIDs[*run.ID] = struct{}{}
				workflowName := getWorkflowName(repo, run, wfs)

				jobs := getAllJobsForRun(ctx, r[0], r[1], *run.ID)
				for _, job := range jobs {
					jobName := job.GetName()
					jobConclusion := job.GetConclusion()
					runnerName := job.GetRunnerName()
					runID := strconv.FormatInt(*run.ID, 10)

					jobStatusGauge.WithLabelValues(repo, workflowName, runID, jobName, jobConclusion, runnerName).Set(1)

					if _, seen := observedJobRuns[*run.ID]; !seen {
						if job.CompletedAt != nil && job.StartedAt != nil {
							duration := job.CompletedAt.Time.Sub(job.StartedAt.Time).Seconds()
							if duration >= 0 {
								jobDurationHistogram.WithLabelValues(repo, workflowName, jobName).Observe(duration)
							}
						}

						if job.StartedAt != nil && run.CreatedAt != nil {
							queueDuration := job.StartedAt.Time.Sub(run.CreatedAt.Time).Seconds()
							if queueDuration >= 0 {
								jobQueueDurationHistogram.WithLabelValues(repo, workflowName, jobName).Observe(queueDuration)
							}
						}
					}
				}

				if _, seen := observedJobRuns[*run.ID]; !seen {
					observedJobRuns[*run.ID] = struct{}{}
				}
			}
		}

		for id := range observedJobRuns {
			if _, exists := currentRunIDs[id]; !exists {
				delete(observedJobRuns, id)
			}
		}

		scrapeDurationSeconds.WithLabelValues("jobs").Set(time.Since(start).Seconds())

		if !sleepWithContext(ctx, time.Duration(config.Github.Refresh)*time.Second) {
			return
		}
	}
}
