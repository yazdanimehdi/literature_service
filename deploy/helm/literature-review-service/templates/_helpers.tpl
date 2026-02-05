{{/*
Expand the name of the chart.
*/}}
{{- define "literature-review-service.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "literature-review-service.fullname" -}}
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
{{- define "literature-review-service.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "literature-review-service.labels" -}}
helm.sh/chart: {{ include "literature-review-service.chart" . }}
{{ include "literature-review-service.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: helixir
{{- end }}

{{/*
Selector labels
*/}}
{{- define "literature-review-service.selectorLabels" -}}
app.kubernetes.io/name: {{ include "literature-review-service.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Server selector labels
*/}}
{{- define "literature-review-service.serverSelectorLabels" -}}
{{ include "literature-review-service.selectorLabels" . }}
app.kubernetes.io/component: server
{{- end }}

{{/*
Worker selector labels
*/}}
{{- define "literature-review-service.workerSelectorLabels" -}}
{{ include "literature-review-service.selectorLabels" . }}
app.kubernetes.io/component: worker
{{- end }}

{{/*
Service account name
*/}}
{{- define "literature-review-service.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "literature-review-service.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Image reference
*/}}
{{- define "literature-review-service.image" -}}
{{- $tag := .Values.image.tag | default .Chart.AppVersion }}
{{- printf "%s:%s" .Values.image.repository $tag }}
{{- end }}

{{/*
Secret name
*/}}
{{- define "literature-review-service.secretName" -}}
{{- if .Values.externalSecrets.enabled }}
{{- .Values.externalSecrets.target.name | default (printf "%s-secrets" (include "literature-review-service.fullname" .)) }}
{{- else }}
{{- printf "%s-secrets" (include "literature-review-service.fullname" .) }}
{{- end }}
{{- end }}

{{/*
PostgreSQL host
*/}}
{{- define "literature-review-service.postgresql.host" -}}
{{- if .Values.postgresql.enabled }}
{{- printf "%s-postgresql" (include "literature-review-service.fullname" .) }}
{{- else }}
{{- .Values.secrets.database.host }}
{{- end }}
{{- end }}

{{/*
PostgreSQL port
*/}}
{{- define "literature-review-service.postgresql.port" -}}
{{- if .Values.postgresql.enabled }}
{{- "5432" }}
{{- else }}
{{- .Values.secrets.database.port | default "5432" | toString }}
{{- end }}
{{- end }}

{{/*
PostgreSQL database
*/}}
{{- define "literature-review-service.postgresql.database" -}}
{{- if .Values.postgresql.enabled }}
{{- .Values.postgresql.auth.database }}
{{- else }}
{{- .Values.secrets.database.name }}
{{- end }}
{{- end }}
