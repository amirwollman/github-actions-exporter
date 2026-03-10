package metrics

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/google/go-github/v45/github"

	"github.com/spendesk/github-actions-exporter/pkg/config"
)

var (
	repositories   []string
	workflows      map[string]map[int64]github.Workflow
	mu             sync.RWMutex
	workflowsReady = make(chan struct{})
)

func getRepositories() []string {
	mu.RLock()
	defer mu.RUnlock()
	result := make([]string, len(repositories))
	copy(result, repositories)
	return result
}

func getWorkflows() map[string]map[int64]github.Workflow {
	mu.RLock()
	defer mu.RUnlock()
	return workflows
}

// sleepWithContext blocks for the given duration or until the context is cancelled.
// Returns true if the sleep completed, false if the context was cancelled.
func sleepWithContext(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func getAllReposForOrg(ctx context.Context, orga string) []string {
	var allRepos []string

	opt := &github.RepositoryListByOrgOptions{
		ListOptions: github.ListOptions{
			PerPage: 200,
			Page:    0,
		},
	}
	for {
		reposPage, resp, err := client.Repositories.ListByOrg(ctx, orga, opt)
		if rl_err, ok := err.(*github.RateLimitError); ok {
			log.Printf("ListByOrg ratelimited. Pausing until %s", rl_err.Rate.Reset.Time.String())
			if !sleepWithContext(ctx, time.Until(rl_err.Rate.Reset.Time)) {
				return allRepos
			}
			continue
		} else if err != nil {
			log.Printf("ListByOrg error for %s: %s", orga, err.Error())
			break
		}
		for _, repo := range reposPage {
			allRepos = append(allRepos, *repo.FullName)
		}
		if resp.NextPage == 0 {
			break
		}
		opt.ListOptions.Page = resp.NextPage
	}
	return allRepos
}

func getAllWorkflowsForRepo(ctx context.Context, owner string, repo string) map[int64]github.Workflow {
	res := make(map[int64]github.Workflow)

	opt := &github.ListOptions{
		PerPage: 200,
		Page:    0,
	}

	for {
		workflowsPage, resp, err := client.Actions.ListWorkflows(ctx, owner, repo, opt)
		if rl_err, ok := err.(*github.RateLimitError); ok {
			log.Printf("ListWorkflows ratelimited. Pausing until %s", rl_err.Rate.Reset.Time.String())
			if !sleepWithContext(ctx, time.Until(rl_err.Rate.Reset.Time)) {
				return res
			}
			continue
		} else if err != nil {
			log.Printf("ListWorkflows error for %s: %s", repo, err.Error())
			return res
		}
		for _, w := range workflowsPage.Workflows {
			res[*w.ID] = *w
		}
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	return res
}

func periodicGithubFetcher(ctx context.Context) {
	firstFetch := true
	for {
		var reposToFetch []string
		if len(config.Github.Repositories.Value()) > 0 {
			reposToFetch = config.Github.Repositories.Value()
		} else {
			for _, orga := range config.Github.Organizations.Value() {
				reposToFetch = append(reposToFetch, getAllReposForOrg(ctx, orga)...)
			}
		}

		nonEmptyRepos := make([]string, 0)
		ww := make(map[string]map[int64]github.Workflow)
		for _, repo := range reposToFetch {
			r := strings.Split(repo, "/")
			workflowsForRepo := getAllWorkflowsForRepo(ctx, r[0], r[1])
			if len(workflowsForRepo) == 0 {
				continue
			}
			nonEmptyRepos = append(nonEmptyRepos, repo)
			ww[repo] = workflowsForRepo
			log.Printf("Fetched %d workflows for repository %s", len(ww[repo]), repo)
		}

		mu.Lock()
		repositories = nonEmptyRepos
		workflows = ww
		mu.Unlock()

		if firstFetch {
			close(workflowsReady)
			firstFetch = false
		}

		if !sleepWithContext(ctx, time.Duration(config.Github.Refresh)*5*time.Second) {
			return
		}
	}
}
