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
	runnersOrganizationStatusGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "github_runner_organization_status",
			Help: "Organization runner online status (1=online, 0=offline)",
		},
		[]string{"organization", "os", "name", "id"},
	)
	runnersOrganizationBusyGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "github_runner_organization_busy",
			Help: "Organization runner busy status (1=busy, 0=idle)",
		},
		[]string{"organization", "os", "name", "id"},
	)
)

// getAllOrgRunners returns the organization's runners and whether the listing
// completed. An incomplete listing must not be published: see publishRunners.
func getAllOrgRunners(ctx context.Context, orga string) ([]*github.Runner, bool) {
	var runners []*github.Runner
	opt := &github.ListOptions{PerPage: 200}

	for {
		resp, rr, err := client.Actions.ListOrganizationRunners(ctx, orga, opt)
		if rl_err, ok := err.(*github.RateLimitError); ok {
			log.Printf("ListOrganizationRunners ratelimited. Pausing until %s", rl_err.Rate.Reset.Time.String())
			if !sleepWithContext(ctx, time.Until(rl_err.Rate.Reset.Time)) {
				return nil, false
			}
			continue
		} else if err != nil {
			log.Printf("ListOrganizationRunners error for org %s: %s", orga, err.Error())
			scrapeErrorsTotal.WithLabelValues("runners_organization").Inc()
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

func getRunnersOrganizationFromGithub(ctx context.Context) {
	for {
		start := time.Now()

		var samples []runnerSample
		complete := true
		for _, orga := range getOrganizations() {
			runners, ok := getAllOrgRunners(ctx, orga)
			if !ok {
				complete = false
				break
			}
			for _, runner := range runners {
				samples = append(samples, runnerSample{
					labels: []string{orga, *runner.OS, *runner.Name, strconv.FormatInt(runner.GetID(), 10)},
					online: runner.GetStatus() == "online",
					busy:   runner.GetBusy(),
				})
			}
		}

		if complete {
			publishRunners(runnersOrganizationStatusGauge, runnersOrganizationBusyGauge, samples)
		} else {
			log.Printf("Incomplete organization runner listing, keeping previously exported values")
		}

		scrapeDurationSeconds.WithLabelValues("runners_organization").Set(time.Since(start).Seconds())

		if !sleepWithContext(ctx, time.Duration(config.Github.Refresh)*time.Second) {
			return
		}
	}
}
