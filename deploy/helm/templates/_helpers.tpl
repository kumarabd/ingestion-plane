{{/*
Expand the name of the chart.
*/}}
{{- define "ingestion-plane.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "ingestion-plane.fullname" -}}
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
{{- define "ingestion-plane.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "ingestion-plane.labels" -}}
helm.sh/chart: {{ include "ingestion-plane.chart" . }}
{{ include "ingestion-plane.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "ingestion-plane.selectorLabels" -}}
app.kubernetes.io/name: {{ include "ingestion-plane.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "ingestion-plane.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "ingestion-plane.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Gateway labels
*/}}
{{- define "ingestion-plane.gateway.labels" -}}
{{ include "ingestion-plane.labels" . }}
app.kubernetes.io/component: gateway
{{- end }}

{{/*
Gateway selector labels
*/}}
{{- define "ingestion-plane.gateway.selectorLabels" -}}
{{ include "ingestion-plane.selectorLabels" . }}
app.kubernetes.io/component: gateway
{{- end }}

{{/*
Miner labels
*/}}
{{- define "ingestion-plane.miner.labels" -}}
{{ include "ingestion-plane.labels" . }}
app.kubernetes.io/component: miner
{{- end }}

{{/*
Miner selector labels
*/}}
{{- define "ingestion-plane.miner.selectorLabels" -}}
{{ include "ingestion-plane.selectorLabels" . }}
app.kubernetes.io/component: miner
{{- end }}

{{/*
Sampler labels
*/}}
{{- define "ingestion-plane.sampler.labels" -}}
{{ include "ingestion-plane.labels" . }}
app.kubernetes.io/component: sampler
{{- end }}

{{/*
Sampler selector labels
*/}}
{{- define "ingestion-plane.sampler.selectorLabels" -}}
{{ include "ingestion-plane.selectorLabels" . }}
app.kubernetes.io/component: sampler
{{- end }}

{{/*
IndexFeed labels
*/}}
{{- define "ingestion-plane.indexfeed.labels" -}}
{{ include "ingestion-plane.labels" . }}
app.kubernetes.io/component: indexfeed
{{- end }}

{{/*
IndexFeed selector labels
*/}}
{{- define "ingestion-plane.indexfeed.selectorLabels" -}}
{{ include "ingestion-plane.selectorLabels" . }}
app.kubernetes.io/component: indexfeed
{{- end }}

{{/*
Planner labels
*/}}
{{- define "ingestion-plane.planner.labels" -}}
{{ include "ingestion-plane.labels" . }}
app.kubernetes.io/component: planner
{{- end }}

{{/*
Planner selector labels
*/}}
{{- define "ingestion-plane.planner.selectorLabels" -}}
{{ include "ingestion-plane.selectorLabels" . }}
app.kubernetes.io/component: planner
{{- end }}

{{/*
Image pull secrets
*/}}
{{- define "ingestion-plane.imagePullSecrets" -}}
{{- if .Values.global.imagePullSecrets }}
imagePullSecrets:
{{- range .Values.global.imagePullSecrets }}
  - name: {{ . }}
{{- end }}
{{- end }}
{{- end }}

{{/*
PostgreSQL host
*/}}
{{- define "ingestion-plane.postgresql.host" -}}
{{- if .Values.postgresql.enabled }}
{{- printf "%s-postgresql" (include "ingestion-plane.fullname" .) }}
{{- else }}
{{- .Values.postgresql.externalHost }}
{{- end }}
{{- end }}

{{/*
Redis host
*/}}
{{- define "ingestion-plane.redis.host" -}}
{{- if .Values.redis.enabled }}
{{- printf "%s-redis-master" (include "ingestion-plane.fullname" .) }}
{{- else }}
{{- .Values.redis.externalHost }}
{{- end }}
{{- end }}

{{/*
Qdrant host
*/}}
{{- define "ingestion-plane.qdrant.host" -}}
{{- if .Values.qdrant.enabled }}
{{- printf "%s-qdrant" (include "ingestion-plane.fullname" .) }}
{{- else }}
{{- .Values.qdrant.externalHost }}
{{- end }}
{{- end }}

