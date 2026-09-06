package metrics

import (
	"context"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/spendesk/github-actions-exporter/pkg/config"

	"github.com/google/go-github/v45/github"
)

// observedRuns tracks completed run IDs that have already been counted by
// the histogram and counter metrics, to avoid double-counting across cycles.
var observedRuns = make(map[int64]struct{})

func getFieldValue(repo string, run github.WorkflowRun, wfs map[string]map[int64]github.Workflow, field string) string {
	switch field {
	case "repo":
		return repo
	case "id":
		return strconv.FormatInt(*run.ID, 10)
	case "node_id":
		return *run.NodeID
	case "head_branch":
		return *run.HeadBranch
	case "head_sha":
		return *run.HeadSHA
	case "run_number":
		return strconv.Itoa(*run.RunNumber)
	case "run_attempt":
		if run.RunAttempt != nil {
			return strconv.Itoa(*run.RunAttempt)
		}
		return "1"
	case "workflow_id":
		return strconv.FormatInt(*run.WorkflowID, 10)
	case "workflow":
		r, exist := wfs[repo]
		if !exist {
			log.Printf("Couldn't fetch repo '%s' from workflow cache.", repo)
			return "unknown"
		}
		w, exist := r[*run.WorkflowID]
		if !exist {
			log.Printf("Couldn't fetch repo '%s', workflow '%d' from workflow cache.", repo, *run.WorkflowID)
			return "unknown"
		}
		return *w.Name
	case "event":
		return *run.Event
	case "status":
		return *run.Status
	case "conclusion":
		return run.GetConclusion()
	}
	log.Printf("Tried to fetch invalid field '%s'", field)
	return ""
}

// workflowFields is the validated EXPORT_FIELDS list, set once by InitMetrics
// and read-only afterwards. It must stay in step with the label set built
// there, so both derive from parseWorkflowFields.
var workflowFields []string

func getRelevantFields(repo string, run *github.WorkflowRun, wfs map[string]map[int64]github.Workflow) []string {
	result := make([]string, len(workflowFields))
	for i, field := range workflowFields {
		result[i] = getFieldValue(repo, *run, wfs, field)
	}
	return result
}

// normalizeWorkflowPhase collapses GitHub's raw status/conclusion vocabulary
// into the four buckets operators actually alert and dashboard on: a run is
// "running" until it completes, and a completed run is "success", "failed"
// or "cancelled". Conclusions without a clear-cut bucket are folded in:
// neutral joins success, and skipped/stale join cancelled since neither
// represents a genuine failure.
func normalizeWorkflowPhase(conclusion string) string {
	switch conclusion {
	case "":
		return "running"
	case "success", "neutral":
		return "success"
	case "cancelled", "skipped", "stale":
		return "cancelled"
	default: // failure, timed_out, action_required, startup_failure, ...
		return "failed"
	}
}

func getWorkflowName(repo string, run *github.WorkflowRun, wfs map[string]map[int64]github.Workflow) string {
	if r, ok := wfs[repo]; ok {
		if w, ok := r[*run.WorkflowID]; ok {
			return *w.Name
		}
	}
	return "unknown"
}

// getRecentWorkflowRuns returns the runs created inside the window and
// whether the listing completed. A partial listing must not be published:
// the gauges are reset before repopulating, so publishing it would drop
// every run the failed page would have carried.
func getRecentWorkflowRuns(ctx context.Context, owner string, repo string) ([]*github.WorkflowRun, bool) {
	windowHours := config.Metrics.WorkflowRunWindowHours
	if windowHours <= 0 {
		windowHours = 12
	}
	windowStart := time.Now().Add(time.Duration(-windowHours) * time.Hour).Format(time.RFC3339)
	opt := &github.ListWorkflowRunsOptions{
		ListOptions: github.ListOptions{PerPage: 200},
		Created:     ">=" + windowStart,
	}

	var runs []*github.WorkflowRun
	for {
		resp, rr, err := client.Actions.ListRepositoryWorkflowRuns(ctx, owner, repo, opt)
		if rl_err, ok := err.(*github.RateLimitError); ok {
			log.Printf("ListRepositoryWorkflowRuns ratelimited. Pausing until %s", rl_err.Rate.Reset.Time.String())
			if !sleepWithContext(ctx, time.Until(rl_err.Rate.Reset.Time)) {
				return nil, false
			}
			continue
		} else if err != nil {
			log.Printf("ListRepositoryWorkflowRuns error for repo %s/%s: %s", owner, repo, err.Error())
			scrapeErrorsTotal.WithLabelValues("workflow_runs").Inc()
			return nil, false
		}
		updateRateLimit(rr)

		runs = append(runs, resp.WorkflowRuns...)
		if rr.NextPage == 0 {
			break
		}
		opt.Page = rr.NextPage
	}

	return runs, true
}

func getRunUsage(ctx context.Context, owner string, repo string, runId int64) *github.WorkflowRunUsage {
	for {
		resp, rr, err := client.Actions.GetWorkflowRunUsageByID(ctx, owner, repo, runId)
		if rl_err, ok := err.(*github.RateLimitError); ok {
			log.Printf("GetWorkflowRunUsageByID ratelimited. Pausing until %s", rl_err.Rate.Reset.Time.String())
			if !sleepWithContext(ctx, time.Until(rl_err.Rate.Reset.Time)) {
				return nil
			}
			continue
		} else if err != nil {
			log.Printf("GetWorkflowRunUsageByID error for repo %s/%s and runId %d: %s", owner, repo, runId, err.Error())
			scrapeErrorsTotal.WithLabelValues("workflow_runs").Inc()
			return nil
		}
		updateRateLimit(rr)
		return resp
	}
}

func getWorkflowRunsFromGithub(ctx context.Context) {
	for {
		start := time.Now()
		repos := getRepositories()
		wfs := getWorkflows()

		var statusSamples [][]string
		currentRunIDs := make(map[int64]struct{})
		complete := true

		for _, repo := range repos {
			r := strings.Split(repo, "/")
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

				conclusion := run.GetConclusion()
				phase := normalizeWorkflowPhase(conclusion)

				fields := getRelevantFields(repo, run, wfs)
				statusSamples = append(statusSamples, append(fields, conclusion, phase))

				currentRunIDs[*run.ID] = struct{}{}
				isCompleted := conclusion != "" && conclusion != "in_progress" && conclusion != "queued"

				if isCompleted {
					if _, seen := observedRuns[*run.ID]; !seen {
						observedRuns[*run.ID] = struct{}{}

						event := run.GetEvent()

						workflowRunsTotal.WithLabelValues(repo, workflowName, event, conclusion).Inc()

						var durationSeconds float64
						var runUsage *github.WorkflowRunUsage
						if config.Metrics.FetchWorkflowRunUsage {
							runUsage = getRunUsage(ctx, r[0], r[1], *run.ID)
						}
						if runUsage == nil {
							created := run.CreatedAt.Time.Unix()
							updated := run.UpdatedAt.Time.Unix()
							durationSeconds = float64(updated - created)
						} else {
							durationSeconds = float64(runUsage.GetRunDurationMS()) / 1000.0
						}
						workflowRunDurationHistogram.WithLabelValues(repo, workflowName, event).Observe(durationSeconds)

						if run.RunStartedAt != nil {
							queueSeconds := run.RunStartedAt.Time.Sub(run.CreatedAt.Time).Seconds()
							if queueSeconds >= 0 {
								workflowRunQueueDuration.WithLabelValues(repo, workflowName, event).Observe(queueSeconds)
							}
						}
					}
				}
			}
		}

		if complete {
			workflowRunStatusGauge.Reset()
			for _, fields := range statusSamples {
				workflowRunStatusGauge.WithLabelValues(fields...).Set(1)
			}

			// Prune observed run IDs no longer in the current window. Only safe
			// on a complete listing: dropping an ID that a failed page would
			// have carried would let its run be counted twice.
			for id := range observedRuns {
				if _, exists := currentRunIDs[id]; !exists {
					delete(observedRuns, id)
				}
			}
		} else {
			log.Printf("Incomplete workflow run listing, keeping previously exported values")
		}

		scrapeDurationSeconds.WithLabelValues("workflow_runs").Set(time.Since(start).Seconds())

		if !sleepWithContext(ctx, time.Duration(config.Github.Refresh)*time.Second) {
			return
		}
	}
}
