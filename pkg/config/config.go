package config

import "github.com/urfave/cli/v2"

var (
	Github struct {
		AppID             int64  `split_words:"true"`
		AppInstallationID int64  `split_words:"true"`
		AppPrivateKey     string `split_words:"true"`
		Token             string
		Refresh           int64
		Repositories      cli.StringSlice
		Organizations     cli.StringSlice
		Workflows         cli.StringSlice
		WorkflowJobs      cli.StringSlice
		APIURL            string
		CacheSizeBytes    int64
	}
	Metrics struct {
		FetchWorkflowRunUsage  bool
		FetchJobMetrics        bool
		WorkflowRunWindowHours int64

		RunnersRepo        bool
		RunnersOrg         bool
		RunnersEnterprise  bool
		WorkflowRunStatus  bool
		WorkflowRunSummary bool
		JobStatus          bool
		JobSummary         bool
		RunnerLabels       bool
	}
	Port           int
	Debug          bool
	EnterpriseName string
	WorkflowFields string
)

func InitConfiguration() []cli.Flag {
	return []cli.Flag{
		&cli.Int64Flag{
			Name:        "app_id",
			Aliases:     []string{"gai"},
			EnvVars:     []string{"GITHUB_APP_ID"},
			Usage:       "Github App Id",
			Destination: &Github.AppID,
		},
		&cli.Int64Flag{
			Name:        "app_installation_id",
			Aliases:     []string{"gii"},
			EnvVars:     []string{"GITHUB_APP_INSTALLATION_ID"},
			Usage:       "Github App Installation Id",
			Destination: &Github.AppInstallationID,
		},
		&cli.StringFlag{
			Name:        "app_private_key",
			Aliases:     []string{"gpk"},
			EnvVars:     []string{"GITHUB_APP_PRIVATE_KEY"},
			Usage:       "Github App Private Key",
			Destination: &Github.AppPrivateKey,
		},
		&cli.IntFlag{
			Name:        "port",
			Aliases:     []string{"p"},
			EnvVars:     []string{"PORT"},
			Value:       9999,
			Usage:       "Exporter port",
			Destination: &Port,
		},
		&cli.StringFlag{
			Name:        "github_token",
			Aliases:     []string{"gt"},
			EnvVars:     []string{"GITHUB_TOKEN"},
			Usage:       "Github Personal Token",
			Destination: &Github.Token,
		},
		&cli.Int64Flag{
			Name:        "github_refresh",
			Aliases:     []string{"gr"},
			EnvVars:     []string{"GITHUB_REFRESH"},
			Value:       300,
			Usage:       "Seconds between collector cycles. Each cycle costs API calls per repo (more with job metrics), so a short interval burns the hourly quota; it cannot make data fresher than the Prometheus scrape interval either",
			Destination: &Github.Refresh,
		},
		&cli.StringFlag{
			Name:        "github_api_url",
			Aliases:     []string{"url"},
			EnvVars:     []string{"GITHUB_API_URL"},
			Value:       "api.github.com",
			Usage:       "Github API URL (primarily designed for Github Enterprise use cases)",
			Destination: &Github.APIURL,
		},
		&cli.StringSliceFlag{
			Name: "github_orgs",
			// github_orgas/GITHUB_ORGAS are the historical upstream spellings,
			// kept so existing deployments keep working. GITHUB_ORGS wins when
			// both are set.
			Aliases:     []string{"go", "github_orgas"},
			EnvVars:     []string{"GITHUB_ORGS", "GITHUB_ORGAS"},
			Usage:       "List all organizations you want get informations. Format <org>,<org2>,<org3> (like test,test2)",
			Destination: &Github.Organizations,
		},
		&cli.StringSliceFlag{
			Name:        "github_repos",
			Aliases:     []string{"grs"},
			EnvVars:     []string{"GITHUB_REPOS"},
			Usage:       "List all repositories you want get informations. Format <org>/<repo>,<org>/<repo2>,<org>/<repo3> (like test/test)",
			Destination: &Github.Repositories,
		},
		&cli.StringSliceFlag{
			Name:        "github_workflows",
			EnvVars:     []string{"GITHUB_WORKFLOWS"},
			Usage:       "Limit workflow-level metrics to these workflow names. Format: <name>,<name2>. Prefix with re: for regex (e.g. re:E2E Tests .*)",
			Destination: &Github.Workflows,
		},
		&cli.StringSliceFlag{
			Name:        "github_workflow_jobs",
			EnvVars:     []string{"GITHUB_WORKFLOW_JOBS"},
			Usage:       "Limit job-level metrics to these job names. Format: <name>,<name2>. Prefix with re: for regex (e.g. re:E2E Tests .*)",
			Destination: &Github.WorkflowJobs,
		},
		&cli.BoolFlag{
			Name:        "debug_profile",
			EnvVars:     []string{"DEBUG_PROFILE"},
			Usage:       "Expose pprof information on /debug/pprof/",
			Destination: &Debug,
		},
		&cli.StringFlag{
			Name:        "enterprise_name",
			EnvVars:     []string{"ENTERPRISE_NAME"},
			Usage:       "Enterprise name. Needed for enterprise endpoints (/enterprises/{ENTERPRISE_NAME}/*)",
			Destination: &EnterpriseName,
			Value:       "",
		},
		&cli.StringFlag{
			Name:        "export_fields",
			EnvVars:     []string{"EXPORT_FIELDS"},
			Usage:       "A comma separated list of fields for workflow metrics that should be exported",
			Value:       "repo,id,node_id,head_branch,head_sha,run_number,run_attempt,workflow_id,workflow,event,status",
			Destination: &WorkflowFields,
		},
		&cli.BoolFlag{
			Name:        "fetch_workflow_run_usage",
			EnvVars:     []string{"FETCH_WORKFLOW_RUN_USAGE"},
			Usage:       "When true, will perform an API call per workflow run to fetch the workflow usage",
			Value:       true,
			Destination: &Metrics.FetchWorkflowRunUsage,
		},
		&cli.BoolFlag{
			Name:        "fetch_job_metrics",
			EnvVars:     []string{"FETCH_JOB_METRICS"},
			Usage:       "When true, will fetch job-level metrics per workflow run (additional API call per run)",
			Value:       false,
			Destination: &Metrics.FetchJobMetrics,
		},
		&cli.Int64Flag{
			Name:        "workflow_run_window_hours",
			EnvVars:     []string{"WORKFLOW_RUN_WINDOW_HOURS"},
			Value:       12,
			Usage:       "Time window in hours for fetching recent workflow runs",
			Destination: &Metrics.WorkflowRunWindowHours,
		},
		&cli.BoolFlag{
			Name:        "metrics_runners_repo",
			EnvVars:     []string{"METRICS_RUNNERS_REPO"},
			Usage:       "Export repository-scoped runner metrics (github_runner_status/busy)",
			Value:       true,
			Destination: &Metrics.RunnersRepo,
		},
		&cli.BoolFlag{
			Name:        "metrics_runners_org",
			EnvVars:     []string{"METRICS_RUNNERS_ORG"},
			Usage:       "Export organization-scoped runner metrics (github_runner_organization_status/busy)",
			Value:       true,
			Destination: &Metrics.RunnersOrg,
		},
		&cli.BoolFlag{
			Name:        "metrics_runners_enterprise",
			EnvVars:     []string{"METRICS_RUNNERS_ENTERPRISE"},
			Usage:       "Export enterprise-scoped runner metrics. Requires ENTERPRISE_NAME",
			Value:       false,
			Destination: &Metrics.RunnersEnterprise,
		},
		&cli.BoolFlag{
			Name:        "metrics_workflow_run_status",
			EnvVars:     []string{"METRICS_WORKFLOW_RUN_STATUS"},
			Usage:       "Export the per-run status gauge (github_workflow_run_status). This is the highest-cardinality metric the exporter produces; see EXPORT_FIELDS",
			Value:       true,
			Destination: &Metrics.WorkflowRunStatus,
		},
		&cli.BoolFlag{
			Name:        "metrics_workflow_run_summary",
			EnvVars:     []string{"METRICS_WORKFLOW_RUN_SUMMARY"},
			Usage:       "Export aggregate run metrics: duration and queue histograms, and github_workflow_runs_total",
			Value:       true,
			Destination: &Metrics.WorkflowRunSummary,
		},
		&cli.BoolFlag{
			Name:        "metrics_job_status",
			EnvVars:     []string{"METRICS_JOB_STATUS"},
			Usage:       "Export the per-job status gauge (github_job_status) and github_job_queue_wait_seconds. Requires FETCH_JOB_METRICS",
			Value:       true,
			Destination: &Metrics.JobStatus,
		},
		&cli.BoolFlag{
			Name:        "metrics_job_summary",
			EnvVars:     []string{"METRICS_JOB_SUMMARY"},
			Usage:       "Export aggregate job metrics: duration and queue histograms, and github_job_conclusions_total. Requires FETCH_JOB_METRICS",
			Value:       true,
			Destination: &Metrics.JobSummary,
		},
		&cli.BoolFlag{
			Name:        "metrics_runner_labels",
			EnvVars:     []string{"METRICS_RUNNER_LABELS"},
			Usage:       "Add a runner_labels label to runner and job metrics, naming the runs-on pool. Turn off to drop that dimension if it costs more series than the capacity insight is worth",
			Value:       true,
			Destination: &Metrics.RunnerLabels,
		},
		&cli.Int64Flag{
			Name:        "github_cache_size_bytes",
			EnvVars:     []string{"GITHUB_CACHE_SIZE_BYTES"},
			Value:       100 * 1024 * 1024,
			Usage:       "Size of Github HTTP cache in bytes",
			Destination: &Github.CacheSizeBytes,
		},
	}
}
