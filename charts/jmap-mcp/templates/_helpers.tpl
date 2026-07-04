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
Name of the Secret holding the attachment URL sealing key.
*/}}
{{- define "jmap-mcp.attachmentSecretName" -}}
{{- if .Values.jmap.attachmentURL.existingSecret.name }}
{{- .Values.jmap.attachmentURL.existingSecret.name }}
{{- else }}
{{- printf "%s-attachment" (include "jmap-mcp.fullname" .) }}
{{- end }}
{{- end }}

{{/*
Key within the attachment Secret.
*/}}
{{- define "jmap-mcp.attachmentSecretKey" -}}
{{- if .Values.jmap.attachmentURL.existingSecret.name }}
{{- .Values.jmap.attachmentURL.existingSecret.key }}
{{- else }}
{{- "secret" }}
{{- end }}
{{- end }}

{{/*
Base64-encoded attachment URL sealing key for the chart-managed Secret.
Precedence: explicit value, then the value already stored in the cluster
(so upgrades do not rotate it), then a freshly generated random key.
*/}}
{{- define "jmap-mcp.attachmentSecretValue" -}}
{{- if .Values.jmap.attachmentURL.secret }}
{{- .Values.jmap.attachmentURL.secret | b64enc }}
{{- else }}
{{- $existing := lookup "v1" "Secret" .Release.Namespace (printf "%s-attachment" (include "jmap-mcp.fullname" .)) }}
{{- if and $existing (index $existing.data "secret") }}
{{- index $existing.data "secret" }}
{{- else }}
{{- randAlphaNum 32 | b64enc }}
{{- end }}
{{- end }}
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

