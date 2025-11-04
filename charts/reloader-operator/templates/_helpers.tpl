{{/*
Expand the name of the chart.
*/}}
{{- define "reloader-operator.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "reloader-operator.fullname" -}}
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
{{- define "reloader-operator.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "reloader-operator.labels" -}}
helm.sh/chart: {{ include "reloader-operator.chart" . }}
{{ include "reloader-operator.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- with .Values.commonLabels }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "reloader-operator.selectorLabels" -}}
app.kubernetes.io/name: {{ include "reloader-operator.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "reloader-operator.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "reloader-operator.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Create the name of the ClusterRole to use
*/}}
{{- define "reloader-operator.clusterRoleName" -}}
{{- printf "%s-manager-role" (include "reloader-operator.fullname" .) }}
{{- end }}

{{/*
Create the name of the ClusterRoleBinding to use
*/}}
{{- define "reloader-operator.clusterRoleBindingName" -}}
{{- printf "%s-manager-rolebinding" (include "reloader-operator.fullname" .) }}
{{- end }}

{{/*
Image name
*/}}
{{- define "reloader-operator.image" -}}
{{- $tag := .Values.image.tag | default .Chart.AppVersion }}
{{- printf "%s:%s" .Values.image.repository $tag }}
{{- end }}

{{/*
Create common annotations
*/}}
{{- define "reloader-operator.annotations" -}}
{{- with .Values.commonAnnotations }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Namespace list for watchNamespaces
*/}}
{{- define "reloader-operator.watchNamespaces" -}}
{{- if .Values.operator.watchNamespaces }}
{{- join "," .Values.operator.watchNamespaces }}
{{- end }}
{{- end }}

{{/*
Leader election config
*/}}
{{- define "reloader-operator.leaderElectionEnabled" -}}
{{- .Values.operator.leaderElection.enabled | toString }}
{{- end }}

{{/*
ServiceMonitor namespace
*/}}
{{- define "reloader-operator.serviceMonitorNamespace" -}}
{{- if .Values.serviceMonitor.namespace }}
{{- .Values.serviceMonitor.namespace }}
{{- else }}
{{- .Release.Namespace }}
{{- end }}
{{- end }}
