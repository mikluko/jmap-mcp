{{/*
Expand the name of the chart.
*/}}
{{- define "jmap-mcp.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "jmap-mcp.fullname" -}}
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
{{- define "jmap-mcp.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "jmap-mcp.labels" -}}
helm.sh/chart: {{ include "jmap-mcp.chart" . }}
{{ include "jmap-mcp.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "jmap-mcp.selectorLabels" -}}
app.kubernetes.io/name: {{ include "jmap-mcp.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "jmap-mcp.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "jmap-mcp.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Create the name of the secret to use
*/}}
{{- define "jmap-mcp.secretName" -}}
{{- if .Values.jmap.existingSecret.name }}
{{- .Values.jmap.existingSecret.name }}
{{- else }}
{{- include "jmap-mcp.fullname" . }}
{{- end }}
{{- end }}

{{/*
Key for session URL in the secret
*/}}
{{- define "jmap-mcp.secretSessionURLKey" -}}
{{- if .Values.jmap.existingSecret.name }}
{{- .Values.jmap.existingSecret.sessionURLKey | default "session-url" }}
{{- else -}}
session-url
{{- end -}}
{{- end }}

{{/*
Key for auth token in the secret
*/}}
{{- define "jmap-mcp.secretAuthTokenKey" -}}
{{- if .Values.jmap.existingSecret.name }}
{{- .Values.jmap.existingSecret.authTokenKey | default "auth-token" }}
{{- else -}}
auth-token
{{- end -}}
{{- end }}
