{{/*
Expand the name of the chart.
*/}}
{{- define "demo-app.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "demo-app.fullname" -}}
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
Common labels
*/}}
{{- define "demo-app.labels" -}}
helm.sh/chart: {{ include "demo-app.chart" . }}
{{ include "demo-app.selectorLabels" . }}
app.kubernetes.io/version: {{ (first .Values.containers).image.tag | default .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "demo-app.selectorLabels" -}}
app.kubernetes.io/name: {{ include "demo-app.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Chart label
*/}}
{{- define "demo-app.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Render a liveness or readiness probe block.
Usage: {{ include "demo-app.probe" (dict "probe" .livenessProbe "key" "livenessProbe") }}
*/}}
{{- define "demo-app.probe" -}}
{{- $p := .probe -}}
{{- if $p.enabled }}
{{ .key }}:
  {{- if eq $p.type "grpc" }}
  grpc:
    port: {{ $p.grpc.port }}
    {{- if $p.grpc.service }}
    service: {{ $p.grpc.service }}
    {{- end }}
  {{- else if eq $p.type "tcpSocket" }}
  tcpSocket:
    port: {{ $p.tcpSocket.port }}
  {{- else }}
  httpGet:
    path: {{ $p.httpGet.path }}
    port: {{ $p.httpGet.port }}
  {{- end }}
  initialDelaySeconds: {{ $p.initialDelaySeconds }}
  periodSeconds: {{ $p.periodSeconds }}
  failureThreshold: {{ $p.failureThreshold }}
{{- end }}
{{- end }}

{{/*
ServiceAccount name
*/}}
{{- define "demo-app.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "demo-app.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}
