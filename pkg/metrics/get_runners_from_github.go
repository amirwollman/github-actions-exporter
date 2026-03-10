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

func getAllRepoRunners(ctx context.Context, owner string, repo string) []*github.Runner {
	var runners []*github.Runner
	opt := &github.ListOptions{PerPage: 200}

	for {
		resp, rr, err := client.Actions.ListRunners(ctx, owner, repo, opt)
		if rl_err, ok := err.(*github.RateLimitError); ok {
			log.Printf("ListRunners ratelimited. Pausing until %s", rl_err.Rate.Reset.Time.String())
			if !sleepWithContext(ctx, time.Until(rl_err.Rate.Reset.Time)) {
				return runners
			}
			continue
		} else if err != nil {
			log.Printf("ListRunners error for repo %s: %s", repo, err.Error())
			scrapeErrorsTotal.WithLabelValues("runners").Inc()
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

func getRunnersFromGithub(ctx context.Context) {
	for {
		start := time.Now()
		runnersStatusGauge.Reset()
		runnersBusyGauge.Reset()

		repos := getRepositories()
		for _, repo := range repos {
			r := strings.Split(repo, "/")

			runners := getAllRepoRunners(ctx, r[0], r[1])
			for _, runner := range runners {
				labels := []string{repo, *runner.OS, *runner.Name, strconv.FormatInt(runner.GetID(), 10)}
				if runner.GetStatus() == "online" {
					runnersStatusGauge.WithLabelValues(labels...).Set(1)
				} else {
					runnersStatusGauge.WithLabelValues(labels...).Set(0)
				}
				if runner.GetBusy() {
					runnersBusyGauge.WithLabelValues(labels...).Set(1)
				} else {
					runnersBusyGauge.WithLabelValues(labels...).Set(0)
				}
			}
		}

		scrapeDurationSeconds.WithLabelValues("runners").Set(time.Since(start).Seconds())

		if !sleepWithContext(ctx, time.Duration(config.Github.Refresh)*time.Second) {
			return
		}
	}
}
