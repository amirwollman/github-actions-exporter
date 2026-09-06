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
	runnersStatusGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "github_runner_status",
			Help: "Runner online status (1=online, 0=offline)",
		},
		[]string{"repo", "os", "name", "id"},
	)
	runnersBusyGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "github_runner_busy",
			Help: "Runner busy status (1=busy, 0=idle)",
		},
		[]string{"repo", "os", "name", "id"},
	)
)

// getAllRepoRunners returns the repository's runners and whether the listing
// completed. An incomplete listing must not be published: see publishRunners.
func getAllRepoRunners(ctx context.Context, owner string, repo string) ([]*github.Runner, bool) {
	var runners []*github.Runner
	opt := &github.ListOptions{PerPage: 200}

	for {
		resp, rr, err := client.Actions.ListRunners(ctx, owner, repo, opt)
		if rl_err, ok := err.(*github.RateLimitError); ok {
			log.Printf("ListRunners ratelimited. Pausing until %s", rl_err.Rate.Reset.Time.String())
			if !sleepWithContext(ctx, time.Until(rl_err.Rate.Reset.Time)) {
				return nil, false
			}
			continue
		} else if err != nil {
			log.Printf("ListRunners error for repo %s: %s", repo, err.Error())
			scrapeErrorsTotal.WithLabelValues("runners").Inc()
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

func getRunnersFromGithub(ctx context.Context) {
	for {
		start := time.Now()

		var samples []runnerSample
		complete := true
		for _, repo := range getRepositories() {
			r := strings.Split(repo, "/")

			runners, ok := getAllRepoRunners(ctx, r[0], r[1])
			if !ok {
				complete = false
				break
			}
			for _, runner := range runners {
				samples = append(samples, runnerSample{
					labels: []string{repo, *runner.OS, *runner.Name, strconv.FormatInt(runner.GetID(), 10)},
					online: runner.GetStatus() == "online",
					busy:   runner.GetBusy(),
				})
			}
		}

		if complete {
			publishRunners(runnersStatusGauge, runnersBusyGauge, samples)
		} else {
			log.Printf("Incomplete repo runner listing, keeping previously exported values")
		}

		scrapeDurationSeconds.WithLabelValues("runners").Set(time.Since(start).Seconds())

		if !sleepWithContext(ctx, time.Duration(config.Github.Refresh)*time.Second) {
			return
		}
	}
}
