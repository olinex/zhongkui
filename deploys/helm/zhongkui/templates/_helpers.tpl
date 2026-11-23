{{/*
基础命名与标签
*/}}
{{- define "zhongkui.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "zhongkui.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "zhongkui.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "zhongkui.labels" -}}
helm.sh/chart: {{ include "zhongkui.chart" .root }}
app.kubernetes.io/name: {{ include "zhongkui.name" .root }}
app.kubernetes.io/instance: {{ .root.Release.Name }}
app.kubernetes.io/component: {{ .component }}
app.kubernetes.io/version: {{ .root.Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .root.Release.Service }}
{{- end -}}

{{- define "zhongkui.selectorLabels" -}}
app.kubernetes.io/name: {{ include "zhongkui.name" .root }}
app.kubernetes.io/instance: {{ .root.Release.Name }}
app.kubernetes.io/component: {{ .component }}
{{- end -}}

{{- define "zhongkui.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "zhongkui.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{- define "zhongkui.secretName" -}}
{{- if .Values.secrets.name -}}
{{- .Values.secrets.name -}}
{{- else if .Values.secrets.create -}}
{{- printf "%s-secret" (include "zhongkui.fullname" .) -}}
{{- else -}}
{{- fail "secrets.name is required when secrets.create=false" -}}
{{- end -}}
{{- end -}}

{{- define "zhongkui.configMapName" -}}
{{- printf "%s-config" (include "zhongkui.fullname" .) -}}
{{- end -}}

{{/*
镜像：tag 为空时取 Chart.appVersion，保证 OCI 安装锁定到对应发布版本
*/}}
{{- define "zhongkui.coreImage" -}}
{{- printf "%s:%s" .Values.image.repository (default .Chart.AppVersion .Values.image.tag) -}}
{{- end -}}

{{- define "zhongkui.webImage" -}}
{{- printf "%s:%s" .Values.webImage.repository (default .Chart.AppVersion .Values.webImage.tag) -}}
{{- end -}}

{{/*
完整 config.toml：字段与 `zki create default-config` 对齐。
敏感值以占位符写入，运行时由 ZKI_* 环境变量覆盖。
*/}}
{{- define "zhongkui.config" -}}
{{- $fullname := include "zhongkui.fullname" . -}}
{{- $cryptoMount := .Values.crypto.mountPath -}}
{{- $tlsEnabled := .Values.grpc.tls.enabled -}}
{{- $tlsDomain := default (printf "%s-grpc" $fullname) .Values.grpc.tls.domain -}}
[crypto]
source = 'config'
parent_secret_key = '<from-secret>'
password_book_path = {{ printf "%s/%s" $cryptoMount .Values.crypto.passwordBookFile | toJson }}

[api]
host = '0.0.0.0'
port = 8080
frontendBaseURL = {{ .Values.config.api.frontendBaseURL | toJson }}
internalBaseURL = {{ printf "http://%s-api:%v" $fullname .Values.api.service.port | toJson }}
trustedProxies = {{ .Values.config.api.trustedProxies | toJson }}

[api.namespace]
headerName = 'X-Namespace'
queryName = 'namespace'
namespace = 'namespace'

[api.auth]
issuer = 'zhongkui'
subject = 'api'
audience = ['web']
tokenPrefix = 'Bearer:'
tokenTimeout = '1h0m0s'
tokenRefreshTimeout = '24h0m0s'
headerName = 'X-Jwtoken'
accessKeyCount = 3

[api.auth.cookie]
name = 'jwtoken'
domain = {{ .Values.config.api.cookieDomain | toJson }}
path = '/'
secure = {{ .Values.config.api.cookieSecure }}
httpOnly = false

[api.secure]
frameOptions = 'DENY'
contentSecurityPolicy = "default-src 'self'; connect-src *; font-src *; script-src-elem * 'unsafe-inline'; img-src * data:; style-src * 'unsafe-inline';"
xssProtection = '1; mode=block'
strictTransportSecurity = 'max-age=31536000; includeSubDomains; preload'
referrerPolicy = 'strict-origin'
contentTypeOptions = 'nosniff'
permissionsPolicy = 'geolocation=(),midi=(),sync-xhr=(),microphone=(),camera=(),magnetometer=(),gyroscope=(),fullscreen=(self),payment=()'

[api.secure.cors]
allowOrigins = {{ .Values.config.api.corsAllowOrigins | toJson }}
allowCredentials = true
allowHeaders = ['Content-Type', 'Content-Length', 'Accept', 'Accept-Encoding', 'X-CSRF-Token', 'Authorization', 'Cache-Control', 'X-Jwtoken', 'X-Namespace']
allowMethods = ['GET', 'POST', 'PUT', 'DELETE', 'OPTIONS']

[api.passwordReset]
tokenTTL = '30m0s'
rateLimit = 5
rateLimitWindow = '1h0m0s'

[capture.resource]
namespace = {{ .Values.config.capture.namespace | toJson }}
name = {{ .Values.config.capture.name | toJson }}
version = {{ .Values.config.capture.version | toJson }}

[capture.watch]
intervalTimeout = '1m0s'
tolerateRatio = 0.5
maxRetries = 5

[capture.feedback]
intervalTimeout = '5s'
tolerateRatio = 0.5
maxRetries = 5

[capture.upload]
dumpDir = '/tmp/zhongkui'
encoderLevel = 2
intervalTimeout = '1m0s'
packetBufferSize = '95 MiB'
packetCount = 1000000
maxRetries = 5

[pipeline]
concurrency = -1
taskCheckInterval = '1s'
healthCheckInterval = '15s'
delayedTaskCheckInterval = '5s'
shutdownTimeout = '10s'
queues = { critical = 6, default = 3, low = 1 }
strictPriority = false

[pipeline.scheduler]
location = 'Asia/Shanghai'
syncInterval = '1m0s'
cursorCheckInterval = '1m0s'
heartbeatInterval = '10s'
forceFlush = true

[pipeline.scheduler.tasks.'periodic-report:plan:dispatch']
cronSpec = ''
interval = '1m0s'
queue = 'default'
timeout = '1m0s'
maxRetry = 1
enabled = true
backlogEnabled = false
backlogWindow = '0s'
backlogLimit = 1
retention = '1h0m0s'
lookback = '5m0s'
payload = {}

[pipeline.scheduler.tasks.'periodic-report:run:cleanup']
cronSpec = '0 3 * * *'
interval = '0s'
queue = 'default'
timeout = '30m0s'
maxRetry = 1
enabled = true
backlogEnabled = false
backlogWindow = '0s'
backlogLimit = 100
retention = '168h0m0s'
lookback = '5m0s'
payload = {}

[pipeline.scheduler.tasks.'tag:event:cleanup']
cronSpec = '0 * * * *'
interval = '0s'
queue = 'default'
timeout = '5m0s'
maxRetry = 3
enabled = true
backlogEnabled = false
backlogWindow = '0s'
backlogLimit = 1
retention = '1h0m0s'
lookback = '5m0s'
payload = { retention = '168h' }

[pipeline.scheduler.tasks.'tag:event:detect']
cronSpec = ''
interval = '1m0s'
queue = 'default'
timeout = '5m0s'
maxRetry = 3
enabled = true
backlogEnabled = true
backlogWindow = '1h0m0s'
backlogLimit = 60
retention = '1h0m0s'
lookback = '5m0s'
payload = {}

[pipeline.scheduler.tasks.'tag:event:suppress']
cronSpec = ''
interval = '1m0s'
queue = 'default'
timeout = '5m0s'
maxRetry = 3
enabled = true
backlogEnabled = true
backlogWindow = '1h0m0s'
backlogLimit = 60
retention = '1h0m0s'
lookback = '5m0s'
payload = {}

[pipeline.custom]
tasks = {}

[pipeline.custom.processors.ipv4-tcp]
batchSize = 1000
flushInterval = '5s'
maxOutOrderPacketSize = 256
maxReassemblerSize = 1024
noticeTimeout = '10s'
pollInterval = '1m'
pollOffset = '1m'
replayWindow = '1h'
saveInterval = '30s'
vNodeCount = 128

[pipeline.custom.processors.ipv6-tcp]
batchSize = 1000
flushInterval = '5s'
maxOutOrderPacketSize = 256
maxReassemblerSize = 1024
noticeTimeout = '10s'
pollInterval = '1m'
pollOffset = '1m'
replayWindow = '1h'
saveInterval = '30s'
vNodeCount = 128

[zap.default]
level = {{ .Values.config.logLevel | toJson }}
development = false
disableCaller = false
disableStacktrace = false
encoding = {{ .Values.config.logEncoding | toJson }}
outputPaths = ['stdout']
errorOutputPaths = ['stderr']

[zap.capture]
level = {{ .Values.config.logLevel | toJson }}
development = false
disableCaller = false
disableStacktrace = false
encoding = {{ .Values.config.logEncoding | toJson }}
outputPaths = ['stdout']
errorOutputPaths = ['stderr']

[zap.api]
level = {{ .Values.config.logLevel | toJson }}
development = false
disableCaller = false
disableStacktrace = false
encoding = {{ .Values.config.logEncoding | toJson }}
outputPaths = ['stdout']
errorOutputPaths = ['stderr']

[zap.database]
level = {{ .Values.config.logLevel | toJson }}
development = false
disableCaller = false
disableStacktrace = false
encoding = {{ .Values.config.logEncoding | toJson }}
outputPaths = ['stdout']
errorOutputPaths = ['stderr']

[zap.etcd]
level = {{ .Values.config.logLevel | toJson }}
development = false
disableCaller = false
disableStacktrace = false
encoding = {{ .Values.config.logEncoding | toJson }}
outputPaths = ['stdout']
errorOutputPaths = ['stderr']

[zap.grpc]
level = {{ .Values.config.logLevel | toJson }}
development = false
disableCaller = false
disableStacktrace = false
encoding = {{ .Values.config.logEncoding | toJson }}
outputPaths = ['stdout']
errorOutputPaths = ['stderr']

[zap.pipeline]
level = {{ .Values.config.logLevel | toJson }}
development = false
disableCaller = false
disableStacktrace = false
encoding = {{ .Values.config.logEncoding | toJson }}
outputPaths = ['stdout']
errorOutputPaths = ['stderr']

[database.core]
driver = 'postgres'
dsn = 'host=postgresql port=5432 user=postgres password=<from-secret> dbname=zhongkui sslmode=disable TimeZone=Asia/Shanghai'

[database.core.meta]
skipDefaultTransaction = false
fullSaveAssociations = false
dryRun = false
prepareStmt = false
disableAutomaticPing = false
disableForeignKeyConstraintWhenMigrating = false
ignoreRelationshipsWhenMigrating = false
disableNestedTransaction = false
allowGlobalUpdate = false
queryFields = false
createBatchSize = 0
translateError = true
propagateUnscoped = false

[database.raw]
driver = 'clickhouse'
dsn = 'clickhouse://default:<from-secret>@clickhouse:9000/zhongkui'

[database.raw.meta]
skipDefaultTransaction = false
fullSaveAssociations = false
dryRun = false
prepareStmt = false
disableAutomaticPing = false
disableForeignKeyConstraintWhenMigrating = false
ignoreRelationshipsWhenMigrating = false
disableNestedTransaction = false
allowGlobalUpdate = false
queryFields = false
createBatchSize = 0
translateError = true
propagateUnscoped = false

[redis.cache]
type = 'normal'
masterName = ''
sentinelAddrs = []
sentinelUsername = ''
sentinelPassword = ''
routeByLatency = false
routeRandomly = true
replicaOnly = true
useDisconnectedReplicas = false
network = 'tcp'
addr = {{ .Values.config.redis.cacheAddr | toJson }}
username = ''
password = '<from-secret>'
clientName = ''
db = 0
poolSize = 10
minIdleConns = 3
maxIdleConns = 5
maxActiveConns = 10
maxRetries = 5
minRetryBackoff = '8ms'
maxRetryBackoff = '512ms'
dialTimeout = '5s'
readTimeout = '3s'
writeTimeout = '3s'
contextTimeoutEnabled = true

[redis.pipeline]
type = 'normal'
masterName = ''
sentinelAddrs = []
sentinelUsername = ''
sentinelPassword = ''
routeByLatency = false
routeRandomly = true
replicaOnly = true
useDisconnectedReplicas = false
network = 'tcp'
addr = {{ .Values.config.redis.pipelineAddr | toJson }}
username = ''
password = '<from-secret>'
clientName = ''
db = 1
poolSize = 10
minIdleConns = 3
maxIdleConns = 5
maxActiveConns = 10
maxRetries = 5
minRetryBackoff = '8ms'
maxRetryBackoff = '512ms'
dialTimeout = '5s'
readTimeout = '3s'
writeTimeout = '3s'
contextTimeoutEnabled = true

[etcd]
endpoints = {{ .Values.config.etcd.endpoints | toJson }}
username = ''
password = ''
autoSyncInterval = '0s'
dialTimeout = '30s'
dialKeepAliveTime = '1m0s'
dialKeepAliveTimeout = '30s'
maxCallSendMsgSize = 2097152
maxCallRecvMsgSize = 4294967296
rejectOldCluster = true

[etcd.tls]
enabled = false
domain = ''
cert = ''
certFilePath = ''

[grpc.server.reporter]
host = '0.0.0.0'
port = {{ .Values.grpc.service.port }}

[grpc.server.reporter.tls]
enabled = {{ $tlsEnabled }}
cert = ''
key = ''
{{- if $tlsEnabled }}
certFilePath = {{ printf "%s/reporter-cert.pem" $cryptoMount | toJson }}
keyFilePath = {{ printf "%s/reporter-key.pem" $cryptoMount | toJson }}
{{- else }}
certFilePath = ''
keyFilePath = ''
{{- end }}

[grpc.server.reporter.auth]
accessKeyEnabled = true

[grpc.server.reporter.keepalive]
time = '30s'
timeout = '5s'
minTime = '5s'
maxConnectionAge = '0s'
maxConnectionAgeGrace = '5s'
maxConnectionIdle = '5m0s'
permitWithoutStream = true

[grpc.server.reporter.health]
enabled = true
serviceName = 'healthcheck'

[grpc.client.reporter]
host = {{ printf "%s-grpc" $fullname | toJson }}
port = {{ .Values.grpc.service.port }}
serviceConfig = {}

[grpc.client.reporter.tls]
enabled = {{ $tlsEnabled }}
domain = {{ $tlsDomain | toJson }}
cert = ''
{{- if $tlsEnabled }}
certFilePath = {{ printf "%s/reporter-cert.pem" $cryptoMount | toJson }}
{{- else }}
certFilePath = ''
{{- end }}

[grpc.client.reporter.auth]
enabled = true
authorize = 'accesskeySecret'
accessKeyID = '<CHANGE_ME>'
accessKeySecret = '<CHANGE_ME>'

[grpc.client.reporter.keepalive]
time = '1m0s'
timeout = '20s'
permitWithoutStream = true

[s3]
endpoint = {{ .Values.config.s3.endpoint | toJson }}
accessKey = '<from-secret>'
secretKey = '<from-secret>'
bucket = {{ .Values.config.s3.bucket | toJson }}
region = {{ .Values.config.s3.region | toJson }}
useSSL = {{ .Values.config.s3.useSSL }}
streamingUpload = {{ .Values.config.s3.streamingUpload }}
objectCacheDir = ''
objectCacheMemoryThresholdMB = 8
objectCacheTTL = '1h0m0s'

[smtp]
enabled = {{ .Values.config.smtp.enabled }}
host = {{ .Values.config.smtp.host | toJson }}
port = {{ .Values.config.smtp.port }}
username = {{ .Values.config.smtp.username | toJson }}
password = '<from-secret>'
from = {{ .Values.config.smtp.from | toJson }}

[health]
enabled = {{ .Values.config.health.enabled }}
host = '0.0.0.0'
port = {{ .Values.config.health.port }}
readyTimeout = '3s'

[health.registry]
enabled = true
keyPrefix = 'services/'
ttl = '15s'

[tagengine]
s3cryptoMaxBytes = 8388608
{{- end -}}

{{/*
公共环境变量：配置覆盖 + Secret
*/}}
{{- define "zhongkui.coreEnv" -}}
- name: ZKI_ENVIRONMENT
  value: {{ .Values.environment | quote }}
- name: ZKI_CRYPTO_PARENT_SECRET_KEY
  valueFrom:
    secretKeyRef:
      name: {{ .Values.crypto.existingSecret }}
      key: {{ .Values.crypto.parentSecretKeyKey }}
- name: ZKI_DATABASE_CORE_DSN
  valueFrom:
    secretKeyRef:
      name: {{ include "zhongkui.secretName" . }}
      key: core-dsn
- name: ZKI_DATABASE_RAW_DSN
  valueFrom:
    secretKeyRef:
      name: {{ include "zhongkui.secretName" . }}
      key: raw-dsn
- name: ZKI_REDIS_CACHE_ADDR
  value: {{ .Values.config.redis.cacheAddr | quote }}
- name: ZKI_REDIS_CACHE_PASSWORD
  valueFrom:
    secretKeyRef:
      name: {{ include "zhongkui.secretName" . }}
      key: redis-cache-password
- name: ZKI_REDIS_PIPELINE_ADDR
  value: {{ .Values.config.redis.pipelineAddr | quote }}
- name: ZKI_REDIS_PIPELINE_PASSWORD
  valueFrom:
    secretKeyRef:
      name: {{ include "zhongkui.secretName" . }}
      key: redis-pipeline-password
- name: ZKI_ETCD_ENDPOINTS
  value: {{ join "," .Values.config.etcd.endpoints | quote }}
- name: ZKI_S3_ENDPOINT
  value: {{ .Values.config.s3.endpoint | quote }}
- name: ZKI_S3_ACCESSKEY
  valueFrom:
    secretKeyRef:
      name: {{ include "zhongkui.secretName" . }}
      key: s3-access-key
- name: ZKI_S3_SECRETKEY
  valueFrom:
    secretKeyRef:
      name: {{ include "zhongkui.secretName" . }}
      key: s3-secret-key
- name: ZKI_S3_BUCKET
  value: {{ .Values.config.s3.bucket | quote }}
- name: ZKI_S3_USESSL
  value: {{ .Values.config.s3.useSSL | quote }}
- name: ZKI_API_SECURE_CORS_ALLOWORIGINS
  value: {{ join "," .Values.config.api.corsAllowOrigins | quote }}
{{- if .Values.config.smtp.enabled }}
- name: ZKI_SMTP_PASSWORD
  valueFrom:
    secretKeyRef:
      name: {{ include "zhongkui.secretName" . }}
      key: smtp-password
{{- end }}
{{- end -}}

{{/*
OTel 环境变量（仅 otel.enabled=true 时注入）
*/}}
{{- define "zhongkui.otelEnv" -}}
{{- $root := .root -}}
{{- $service := .service -}}
{{- if $root.Values.otel.enabled -}}
{{- if and (eq $root.Values.otel.provider "otlp") (not $root.Values.otel.endpoint) -}}
{{- fail "otel.endpoint is required when otel.enabled=true and otel.provider=otlp" -}}
{{- end -}}
{{- $names := $root.Values.otel.serviceNames | default dict -}}
{{- $serviceName := index $names $service | default (printf "%s-%s" (include "zhongkui.fullname" $root) $service) -}}
- name: OTEL_ENABLED
  value: "true"
- name: OTEL_PROVIDER
  value: {{ $root.Values.otel.provider | quote }}
- name: OTEL_OTLP_PROTOCOL
  value: {{ $root.Values.otel.protocol | quote }}
- name: OTEL_EXPORTER_OTLP_ENDPOINT
  value: {{ $root.Values.otel.endpoint | quote }}
- name: OTEL_EXPORTER_OTLP_HEADERS
  valueFrom:
    secretKeyRef:
      name: {{ include "zhongkui.secretName" $root }}
      key: otel-headers
      optional: true
- name: OTEL_EXPORTER_OTLP_COMPRESSION
  value: {{ $root.Values.otel.compression | quote }}
- name: OTEL_EXPORTER_OTLP_INSECURE
  value: {{ $root.Values.otel.insecure | quote }}
- name: OTEL_SAMPLER_TYPE
  value: {{ $root.Values.otel.samplerType | quote }}
- name: OTEL_SAMPLER_RATIO
  value: {{ $root.Values.otel.samplerRatio | quote }}
- name: OTEL_TAIL_ON_ERROR
  value: {{ $root.Values.otel.tailOnError | quote }}
{{- if $root.Values.otel.resourceAttributes }}
- name: OTEL_RESOURCE_ATTRIBUTES
  value: {{ $root.Values.otel.resourceAttributes | quote }}
{{- end }}
- name: OTEL_SERVICE_NAME
  value: {{ $serviceName | quote }}
{{- end -}}
{{- end -}}

{{/*
卷与挂载
*/}}
{{- define "zhongkui.configVolume" -}}
- name: config
  configMap:
    name: {{ include "zhongkui.configMapName" . }}
{{- end -}}

{{- define "zhongkui.configVolumeMount" -}}
- name: config
  mountPath: /etc/zhongkui/config.toml
  subPath: config.toml
  readOnly: true
{{- end -}}

{{- define "zhongkui.cryptoVolume" -}}
- name: crypto
  secret:
    secretName: {{ .Values.crypto.existingSecret }}
    items:
      - key: {{ .Values.crypto.passwordBookFile }}
        path: {{ .Values.crypto.passwordBookFile }}
      {{- if .Values.grpc.tls.enabled }}
      - key: reporter-cert.pem
        path: reporter-cert.pem
      - key: reporter-key.pem
        path: reporter-key.pem
      {{- end }}
{{- end -}}

{{- define "zhongkui.cryptoVolumeMount" -}}
- name: crypto
  mountPath: {{ .Values.crypto.mountPath }}
  readOnly: true
{{- end -}}

{{/*
容器探针
*/}}
{{- define "zhongkui.probes" -}}
startupProbe:
  httpGet:
    path: /startupz
    port: health
  periodSeconds: 5
  failureThreshold: 30
readinessProbe:
  httpGet:
    path: /readyz
    port: health
  periodSeconds: 10
  failureThreshold: 3
livenessProbe:
  httpGet:
    path: /healthz
    port: health
  periodSeconds: 20
  failureThreshold: 3
{{- end -}}

{{/*
迁移 Job 环境：仅依赖 DB 与密码本
*/}}
{{- define "zhongkui.migrationEnv" -}}
- name: ZKI_ENVIRONMENT
  value: {{ .Values.environment | quote }}
- name: ZKI_CRYPTO_PARENT_SECRET_KEY
  valueFrom:
    secretKeyRef:
      name: {{ .Values.crypto.existingSecret }}
      key: {{ .Values.crypto.parentSecretKeyKey }}
- name: ZKI_DATABASE_CORE_DSN
  value: {{ .Values.config.database.coreDSN | quote }}
- name: ZKI_DATABASE_RAW_DSN
  value: {{ .Values.config.database.rawDSN | quote }}
- name: ZKI_REDIS_CACHE_ADDR
  value: {{ .Values.config.redis.cacheAddr | quote }}
- name: ZKI_REDIS_PIPELINE_ADDR
  value: {{ .Values.config.redis.pipelineAddr | quote }}
- name: ZKI_ETCD_ENDPOINTS
  value: {{ join "," .Values.config.etcd.endpoints | quote }}
- name: ZKI_S3_ENDPOINT
  value: {{ .Values.config.s3.endpoint | quote }}
- name: ZKI_S3_BUCKET
  value: {{ .Values.config.s3.bucket | quote }}
{{- end -}}
