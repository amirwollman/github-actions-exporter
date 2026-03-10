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
	workflowBillGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "github_workflow_usage_seconds",
			Help: "Number of billable seconds used by a specific workflow during the current billing cycle. Any job re-runs are also included in the usage. Only apply to workflows in private repositories that use GitHub-hosted runners.",
		},
		[]string{"repo", "id", "node_id", "name", "state", "os"},
	)
)

func getBillableFromGithub(ctx context.Context) {
	for {
		start := time.Now()
		workflowBillGauge.Reset()

		repos := getRepositories()
		wfs := getWorkflows()
		for _, repo := range repos {
			for k, v := range wfs[repo] {
				r := strings.Split(repo, "/")

				for {
					resp, rr, err := client.Actions.GetWorkflowUsageByID(ctx, r[0], r[1], k)
					if rl_err, ok := err.(*github.RateLimitError); ok {
						log.Printf("GetWorkflowUsageByID ratelimited. Pausing until %s", rl_err.Rate.Reset.Time.String())
						if !sleepWithContext(ctx, time.Until(rl_err.Rate.Reset.Time)) {
							return
						}
						continue
					} else if err != nil {
						log.Printf("GetWorkflowUsageByID error for %s: %s", repo, err.Error())
						scrapeErrorsTotal.WithLabelValues("billable").Inc()
						break
					}
					updateRateLimit(rr)
					workflowBillGauge.WithLabelValues(repo, strconv.FormatInt(*v.ID, 10), *v.NodeID, *v.Name, *v.State, "MACOS").Set(float64(resp.GetBillable().MacOS.GetTotalMS()) / 1000)
					workflowBillGauge.WithLabelValues(repo, strconv.FormatInt(*v.ID, 10), *v.NodeID, *v.Name, *v.State, "WINDOWS").Set(float64(resp.GetBillable().Windows.GetTotalMS()) / 1000)
					workflowBillGauge.WithLabelValues(repo, strconv.FormatInt(*v.ID, 10), *v.NodeID, *v.Name, *v.State, "UBUNTU").Set(float64(resp.GetBillable().Ubuntu.GetTotalMS()) / 1000)
					break
				}

			}
		}

		scrapeDurationSeconds.WithLabelValues("billable").Set(time.Since(start).Seconds())

		if !sleepWithContext(ctx, time.Duration(config.Github.Refresh)*5*time.Second) {
			return
		}
	}
}
