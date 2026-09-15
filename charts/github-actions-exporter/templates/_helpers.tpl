{{/* vim: set filetype=mustache: */}}
{{/*
Expand the name of the chart.
*/}}
{{- define "github-actions-exporter.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "github-actions-exporter.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "github-actions-exporter.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "github-actions-exporter.labels" -}}
helm.sh/chart: {{ include "github-actions-exporter.chart" . }}
{{ include "github-actions-exporter.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "github-actions-exporter.selectorLabels" -}}
app.kubernetes.io/name: {{ include "github-actions-exporter.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Maps the metrics.* toggles onto the exporter's METRICS_* env vars.

Each area covers a collector as well as its series: switching one off stops the
API calls it makes, not just the metrics it exports. Anything left unset in
values is omitted here so the binary's own default applies.
*/}}
{{- define "github-actions-exporter.metricsEnv" -}}
{{- $m := .Values.metrics | default dict -}}
{{- $runners := $m.runners | default dict -}}
{{- $runs := $m.workflowRuns | default dict -}}
{{- $jobs := $m.jobs | default dict -}}
{{- if not (kindIs "invalid" $runners.repo) }}METRICS_RUNNERS_REPO: {{ $runners.repo }}
{{ end -}}
{{- if not (kindIs "invalid" $runners.organization) }}METRICS_RUNNERS_ORG: {{ $runners.organization }}
{{ end -}}
{{- if not (kindIs "invalid" $runners.enterprise) }}METRICS_RUNNERS_ENTERPRISE: {{ $runners.enterprise }}
{{ end -}}
{{- if not (kindIs "invalid" $runs.status) }}METRICS_WORKFLOW_RUN_STATUS: {{ $runs.status }}
{{ end -}}
{{- if not (kindIs "invalid" $runs.summary) }}METRICS_WORKFLOW_RUN_SUMMARY: {{ $runs.summary }}
{{ end -}}
{{- if not (kindIs "invalid" $jobs.enabled) }}FETCH_JOB_METRICS: {{ $jobs.enabled }}
{{ end -}}
{{- if not (kindIs "invalid" $jobs.status) }}METRICS_JOB_STATUS: {{ $jobs.status }}
{{ end -}}
{{- if not (kindIs "invalid" $jobs.summary) }}METRICS_JOB_SUMMARY: {{ $jobs.summary }}
{{ end -}}
{{- if not (kindIs "invalid" $m.billable) }}FETCH_WORKFLOW_RUN_USAGE: {{ $m.billable }}
{{ end -}}
{{- if not (kindIs "invalid" $m.runnerLabels) }}METRICS_RUNNER_LABELS: {{ $m.runnerLabels }}
{{ end -}}
{{- end -}}
