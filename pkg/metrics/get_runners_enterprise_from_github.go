package metrics

import (
	"context"
	"log"
	"strconv"
	"time"

	"github.com/spendesk/github-actions-exporter/pkg/config"

	"github.com/google/go-github/v45/github"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	runnersEnterpriseStatusGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "github_runner_enterprise_status",
			Help: "Enterprise runner online status (1=online, 0=offline)",
		},
		[]string{"os", "name", "id"},
	)
	runnersEnterpriseBusyGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "github_runner_enterprise_busy",
			Help: "Enterprise runner busy status (1=busy, 0=idle)",
		},
		[]string{"os", "name", "id"},
	)
)

// getAllEnterpriseRunners returns the enterprise's runners and whether the
// listing completed. An incomplete listing must not be published: see
// publishRunners.
func getAllEnterpriseRunners(ctx context.Context) ([]*github.Runner, bool) {
	var runners []*github.Runner
	opt := &github.ListOptions{PerPage: 200}

	for {
		resp, rr, err := client.Enterprise.ListRunners(ctx, config.EnterpriseName, opt)
		if rl_err, ok := err.(*github.RateLimitError); ok {
			log.Printf("ListRunners ratelimited. Pausing until %s", rl_err.Rate.Reset.Time.String())
			if !sleepWithContext(ctx, time.Until(rl_err.Rate.Reset.Time)) {
				return nil, false
			}
			continue
		} else if err != nil {
			log.Printf("ListRunners error for enterprise %s: %s", config.EnterpriseName, err.Error())
			scrapeErrorsTotal.WithLabelValues("runners_enterprise").Inc()
			return nil, false
		}
		updateRateLimit(rr)

		runners = append(runners, resp.Runners...)
		if rr.NextPage == 0 {
			break
		}
		opt.Page = rr.NextPage
	}

	return runners, true
}

func getRunnersEnterpriseFromGithub(ctx context.Context) {
	if config.EnterpriseName == "" {
		return
	}
	for {
		start := time.Now()

		runners, complete := getAllEnterpriseRunners(ctx)
		if complete {
			samples := make([]runnerSample, 0, len(runners))
			for _, runner := range runners {
				samples = append(samples, runnerSample{
					labels: []string{*runner.OS, *runner.Name, strconv.FormatInt(runner.GetID(), 10)},
					online: runner.GetStatus() == "online",
					busy:   runner.GetBusy(),
				})
			}
			publishRunners(runnersEnterpriseStatusGauge, runnersEnterpriseBusyGauge, samples)
		} else {
			log.Printf("Incomplete enterprise runner listing, keeping previously exported values")
		}

		scrapeDurationSeconds.WithLabelValues("runners_enterprise").Set(time.Since(start).Seconds())

		if !sleepWithContext(ctx, time.Duration(config.Github.Refresh)*time.Second) {
			return
		}
	}
}
