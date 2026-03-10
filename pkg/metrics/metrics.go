package metrics

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"runtime"
	"strings"

	"github.com/spendesk/github-actions-exporter/pkg/config"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/die-net/lrucache"
	"github.com/google/go-github/v45/github"
	"github.com/gregjones/httpcache"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/oauth2"
)

var (
	client *github.Client
	err    error

	workflowRunStatusGauge *prometheus.GaugeVec

	workflowRunDurationHistogram *prometheus.HistogramVec
	workflowRunQueueDuration     *prometheus.HistogramVec
	workflowRunsTotal            *prometheus.CounterVec

	buildInfoGauge        *prometheus.GaugeVec
	scrapeErrorsTotal     *prometheus.CounterVec
	scrapeDurationSeconds *prometheus.GaugeVec
	apiRateLimitRemaining *prometheus.GaugeVec
	apiRateLimitLimit     *prometheus.GaugeVec

	Version string
)

func InitMetrics(ctx context.Context, version string) {
	Version = version

	statusLabels := append(strings.Split(config.WorkflowFields, ","), "conclusion")
	workflowRunStatusGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "github_workflow_run_status",
			Help: "Workflow run status; value is always 1, state carried in conclusion label",
		},
		statusLabels,
	)

	workflowRunDurationHistogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "github_workflow_run_duration_seconds",
			Help:    "Duration of completed workflow runs in seconds",
			Buckets: []float64{10, 30, 60, 120, 300, 600, 1200, 1800, 3600},
		},
		[]string{"repo", "workflow", "event"},
	)

	workflowRunQueueDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "github_workflow_run_queue_duration_seconds",
			Help:    "Time from workflow run creation to first execution start",
			Buckets: []float64{5, 10, 30, 60, 120, 300, 600},
		},
		[]string{"repo", "workflow", "event"},
	)

	workflowRunsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "github_workflow_runs_total",
			Help: "Total number of completed workflow runs observed",
		},
		[]string{"repo", "workflow", "event", "conclusion"},
	)

	buildInfoGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "github_exporter_build_info",
			Help: "Build information about the exporter",
		},
		[]string{"version", "goversion"},
	)

	scrapeErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "github_exporter_scrape_errors_total",
			Help: "Total number of errors encountered during GitHub API scrapes",
		},
		[]string{"collector"},
	)

	scrapeDurationSeconds = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "github_exporter_scrape_duration_seconds",
			Help: "Duration of the last scrape cycle per collector",
		},
		[]string{"collector"},
	)

	apiRateLimitRemaining = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "github_exporter_api_rate_limit_remaining",
			Help: "GitHub API rate limit remaining requests",
		},
		[]string{"resource"},
	)

	apiRateLimitLimit = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "github_exporter_api_rate_limit_limit",
			Help: "GitHub API rate limit maximum requests",
		},
		[]string{"resource"},
	)

	prometheus.MustRegister(runnersStatusGauge)
	prometheus.MustRegister(runnersBusyGauge)
	prometheus.MustRegister(runnersOrganizationStatusGauge)
	prometheus.MustRegister(runnersOrganizationBusyGauge)
	prometheus.MustRegister(runnersEnterpriseStatusGauge)
	prometheus.MustRegister(runnersEnterpriseBusyGauge)
	prometheus.MustRegister(workflowRunStatusGauge)
	prometheus.MustRegister(workflowRunDurationHistogram)
	prometheus.MustRegister(workflowRunQueueDuration)
	prometheus.MustRegister(workflowRunsTotal)
	prometheus.MustRegister(workflowBillGauge)
	prometheus.MustRegister(buildInfoGauge)
	prometheus.MustRegister(scrapeErrorsTotal)
	prometheus.MustRegister(scrapeDurationSeconds)
	prometheus.MustRegister(apiRateLimitRemaining)
	prometheus.MustRegister(apiRateLimitLimit)

	buildInfoGauge.WithLabelValues(version, runtime.Version()).Set(1)

	client, err = NewClient()
	if err != nil {
		log.Fatalln("Error: Client creation failed." + err.Error())
	}

	go periodicGithubFetcher(ctx)

	<-workflowsReady

	go getBillableFromGithub(ctx)
	go getRunnersFromGithub(ctx)
	go getRunnersOrganizationFromGithub(ctx)
	go getWorkflowRunsFromGithub(ctx)
	go getRunnersEnterpriseFromGithub(ctx)

	if config.Metrics.FetchJobMetrics {
		prometheus.MustRegister(jobStatusGauge)
		prometheus.MustRegister(jobDurationHistogram)
		prometheus.MustRegister(jobQueueDurationHistogram)
		go getJobsFromGithub(ctx)
	}
}

func updateRateLimit(resp *github.Response) {
	if resp == nil {
		return
	}
	apiRateLimitRemaining.WithLabelValues("core").Set(float64(resp.Rate.Remaining))
	apiRateLimitLimit.WithLabelValues("core").Set(float64(resp.Rate.Limit))
}

// NewClient creates a Github Client
func NewClient() (*github.Client, error) {
	var (
		httpClient      *http.Client
		client          *github.Client
		cachedTransport *httpcache.Transport
	)

	cache := lrucache.New(config.Github.CacheSizeBytes, 0)
	cachedTransport = httpcache.NewTransport(cache)

	if len(config.Github.Token) > 0 {
		log.Printf("authenticating with Github Token")
		ctx := context.Background()
		ctx = context.WithValue(ctx, "HTTPClient", cachedTransport.Client())
		httpClient = oauth2.NewClient(ctx, oauth2.StaticTokenSource(&oauth2.Token{AccessToken: config.Github.Token}))
	} else {
		log.Printf("authenticating with Github App")
		transport, err := ghinstallation.NewKeyFromFile(cachedTransport, config.Github.AppID, config.Github.AppInstallationID, config.Github.AppPrivateKey)
		if err != nil {
			return nil, fmt.Errorf("authentication failed: %v", err)
		}
		if config.Github.APIURL != "api.github.com" {
			githubAPIURL, err := getEnterpriseApiUrl(config.Github.APIURL)
			if err != nil {
				return nil, fmt.Errorf("enterprise url incorrect: %v", err)
			}
			transport.BaseURL = githubAPIURL
		}
		httpClient = &http.Client{Transport: transport}
	}

	if config.Github.APIURL != "api.github.com" {
		var err error
		client, err = github.NewEnterpriseClient(config.Github.APIURL, config.Github.APIURL, httpClient)
		if err != nil {
			return nil, fmt.Errorf("enterprise client creation failed: %v", err)
		}
	} else {
		client = github.NewClient(httpClient)
	}

	return client, nil
}

func getEnterpriseApiUrl(baseURL string) (string, error) {
	baseEndpoint, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	if !strings.HasSuffix(baseEndpoint.Path, "/") {
		baseEndpoint.Path += "/"
	}
	if !strings.HasSuffix(baseEndpoint.Path, "/api/v3/") &&
		!strings.HasPrefix(baseEndpoint.Host, "api.") &&
		!strings.Contains(baseEndpoint.Host, ".api.") {
		baseEndpoint.Path += "api/v3/"
	}

	return fmt.Sprintf("%s://%s%s", baseEndpoint.Scheme, baseEndpoint.Host, strings.TrimSuffix(baseEndpoint.Path, "/")), nil
}
