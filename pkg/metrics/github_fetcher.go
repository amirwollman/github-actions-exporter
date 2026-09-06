package metrics

import (
	"context"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/go-github/v45/github"

	"github.com/spendesk/github-actions-exporter/pkg/config"
)

// expandCommaSlice takes a StringSlice (which may contain comma-separated values
// when set via env var) and returns individual trimmed strings.
func expandCommaSlice(slice []string) []string {
	var out []string
	for _, s := range slice {
		for _, part := range strings.Split(s, ",") {
			if trimmed := strings.TrimSpace(part); trimmed != "" {
				out = append(out, trimmed)
			}
		}
	}
	return out
}

// patternMatches returns true if name matches pattern. If pattern starts with "re:",
// the rest is treated as a regex; otherwise exact match is used.
func patternMatches(pattern, name string) bool {
	if strings.HasPrefix(pattern, "re:") {
		re, err := regexp.Compile(strings.TrimPrefix(pattern, "re:"))
		if err != nil {
			log.Printf("invalid regex pattern %q: %v", pattern, err)
			return false
		}
		return re.MatchString(name)
	}
	return pattern == name
}

var (
	repositories   []string
	organizations  []string
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

// getOrganizations returns every organization the exporter should fetch
// org-level runners for: those explicitly configured via GITHUB_ORGS, plus
// the owner of every configured/discovered repository, so org-level runners
// are included even when only GITHUB_REPOS is set.
func getOrganizations() []string {
	mu.RLock()
	defer mu.RUnlock()
	result := make([]string, len(organizations))
	copy(result, organizations)
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

func isWorkflowAllowed(name string) bool {
	allowed := expandCommaSlice(config.Github.Workflows.Value())
	if len(allowed) == 0 {
		return true
	}
	for _, w := range allowed {
		if patternMatches(w, name) {
			return true
		}
	}
	return false
}

func isJobAllowed(name string) bool {
	allowed := expandCommaSlice(config.Github.WorkflowJobs.Value())
	if len(allowed) == 0 {
		return true
	}
	for _, j := range allowed {
		if patternMatches(j, name) {
			return true
		}
	}
	return false
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
		explicitOrgs := expandCommaSlice(config.Github.Organizations.Value())
		repos := expandCommaSlice(config.Github.Repositories.Value())
		if len(repos) > 0 {
			reposToFetch = repos
		} else {
			for _, orga := range explicitOrgs {
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

		// Org-level runners are relevant for every organization that owns a
		// configured/discovered repository, not just those explicitly listed
		// in GITHUB_ORGS, so they are included even when only GITHUB_REPOS
		// is set.
		orgSet := make(map[string]struct{})
		for _, orga := range explicitOrgs {
			orgSet[orga] = struct{}{}
		}
		for _, repo := range reposToFetch {
			if owner := strings.SplitN(repo, "/", 2)[0]; owner != "" {
				orgSet[owner] = struct{}{}
			}
		}
		orgsToFetch := make([]string, 0, len(orgSet))
		for orga := range orgSet {
			orgsToFetch = append(orgsToFetch, orga)
		}

		mu.Lock()
		repositories = nonEmptyRepos
		organizations = orgsToFetch
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
