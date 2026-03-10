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

func getAllEnterpriseRunners(ctx context.Context) []*github.Runner {
	var runners []*github.Runner
	opt := &github.ListOptions{PerPage: 200}

	for {
		resp, rr, err := client.Enterprise.ListRunners(ctx, config.EnterpriseName, opt)
		if rl_err, ok := err.(*github.RateLimitError); ok {
			log.Printf("ListRunners ratelimited. Pausing until %s", rl_err.Rate.Reset.Time.String())
			if !sleepWithContext(ctx, time.Until(rl_err.Rate.Reset.Time)) {
				return runners
			}
			continue
		} else if err != nil {
			log.Printf("ListRunners error for enterprise %s: %s", config.EnterpriseName, err.Error())
			scrapeErrorsTotal.WithLabelValues("runners_enterprise").Inc()
			return nil
		}
		updateRateLimit(rr)

		runners = append(runners, resp.Runners...)
		if rr.NextPage == 0 {
			break
		}
		opt.Page = rr.NextPage
	}

	return runners
}

func getRunnersEnterpriseFromGithub(ctx context.Context) {
	if config.EnterpriseName == "" {
		return
	}
	for {
		start := time.Now()
		runnersEnterpriseStatusGauge.Reset()
		runnersEnterpriseBusyGauge.Reset()

		runners := getAllEnterpriseRunners(ctx)

		for _, runner := range runners {
			labels := []string{*runner.OS, *runner.Name, strconv.FormatInt(runner.GetID(), 10)}
			if runner.GetStatus() == "online" {
				runnersEnterpriseStatusGauge.WithLabelValues(labels...).Set(1)
			} else {
				runnersEnterpriseStatusGauge.WithLabelValues(labels...).Set(0)
			}
			if runner.GetBusy() {
				runnersEnterpriseBusyGauge.WithLabelValues(labels...).Set(1)
			} else {
				runnersEnterpriseBusyGauge.WithLabelValues(labels...).Set(0)
			}
		}

		scrapeDurationSeconds.WithLabelValues("runners_enterprise").Set(time.Since(start).Seconds())

		if !sleepWithContext(ctx, time.Duration(config.Github.Refresh)*time.Second) {
			return
		}
	}
}
