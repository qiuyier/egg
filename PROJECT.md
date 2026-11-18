# egg API

基于 Go 1.27 和 Gin 的 HTTP API 骨架。默认使用 Otter v2 内存缓存，可以直接启动。Redis、MySQL、PostgreSQL 和 OTLP 导出按配置启用。

## 启动

```bash
go run ./cmd/api -config configs/config.yaml
```

```bash
curl -i http://localhost:8080/api/v1/ping
curl -fsS http://localhost:8080/livez
curl -fsS http://localhost:8080/readyz
curl -fsS http://localhost:8080/metrics
```

| 路由 | 行为 |
| --- | --- |
| `GET /api/v1/ping` | 示例 API，返回服务名 |
| `GET /livez` | 检查 HTTP 进程是否可响应 |
| `GET /readyz` | 检查启用的远程缓存和数据库。任一检查失败时返回 HTTP 503 |
| `GET /metrics` | 当 `http.metrics=true` 时暴露 Prometheus 指标，包括请求计数、延时和 Go 运行时指标 |

API 使用 `code`、`message`、`request_id` 及可选的 `data`、`trace_id` 字段。访问日志也包含请求 ID，启用追踪后可用 trace ID 关联请求和日志。HTTP 404、405 及尚未写入响应时的处理器 panic 使用同一 JSON 格式。

## 配置

配置优先级从高到低为 `EGG_<SECTION>_<FIELD>` 环境变量、配置文件、内置默认值。文件路径通过 `-config` 或 `EGG_CONFIG` 指定，命令行优先。省略文件路径时使用内置默认值，外加环境变量覆盖。

[完整 YAML 示例](configs/config.yaml)列出所有字段。配置文件支持 `.toml`、`.yaml`、`.yml`、`.json`、`.ini`、`.xml`。各格式使用相同的分组和字段名。时长使用 `250ms`、`5s` 等 Go duration 字符串，零时长写成 `"0s"`。代理地址列表在 INI、XML 和环境变量中以逗号分隔。

```ini
[http]
address = 127.0.0.1:8080
read_timeout = 10s
trusted_proxies = 127.0.0.1,10.0.0.0/24
```

```xml
<config>
  <http>
    <address>127.0.0.1:8080</address>
    <read_timeout>10s</read_timeout>
    <trusted_proxies>127.0.0.1,10.0.0.0/24</trusted_proxies>
  </http>
</config>
```

默认以直接连接的 IP 作为客户端地址。部署在反向代理后时，将代理地址或网段填入 `http.trusted_proxies`。示例 YAML 启用了与 API 共用监听地址的 `/metrics`，可通过 `http.metrics=false` 关闭，或在入口代理限制访问。

## 日志与链路追踪

控制台模式启动时会显示 Egg 标识。交互式终端使用琥珀色，可通过 `log.color=false` 或 `NO_COLOR=1` 关闭标识颜色。JSON 模式和仅写文件时跳过标识。

Zap 支持彩色控制台和 JSON 标准输出。设置 `log.file` 后同时写入 JSON 文件，Lumberjack 按文件大小滚动，可配置备份数量、保留天数和 gzip 压缩。设置 `log.stdout=false` 可仅写文件。

```bash
EGG_LOG_FORMAT=json EGG_LOG_FILE=logs/api.log \
  go run ./cmd/api -config configs/config.yaml
```

追踪使用 OpenTelemetry 和 W3C Trace Context，支持传入的 `traceparent`、`tracestate` 和 `baggage`，通过 OTLP/HTTP 导出到已运行的 Collector 或兼容端点。采样比例适用于本服务新建的根 span，带有父 span 的请求沿用上游采样决策。

```bash
EGG_TRACING_ENABLED=true \
EGG_TRACING_ENDPOINT=http://127.0.0.1:4318/v1/traces \
EGG_TRACING_SAMPLE_RATE=0.25 \
  go run ./cmd/api -config configs/config.yaml
```

省略 `tracing.endpoint` 时使用 OpenTelemetry 的 `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` 等标准配置。业务处理器将 `c.Request.Context()` 传给缓存，以及 `db.WithContext(ctx)`，可把 Redis 和 SQL 操作纳入当前 HTTP trace。追踪默认省略 SQL 查询文本和 Redis 命令内容。进程收到 SIGINT 或 SIGTERM 后先等待 HTTP 请求结束，再关闭缓存、数据库连接池和导出器，刷新待发送的 span。

## 切换缓存和数据库

```bash
docker compose up -d --wait redis mysql postgres
```

并行运行多个项目时，可用 `EGG_HTTP_ADDRESS` 调整 API 地址，用 `EGG_REDIS_PORT`、`EGG_MYSQL_PORT`、`EGG_POSTGRES_PORT` 调整 Compose 映射端口，并同步修改连接地址。

### Redis 和 MySQL

```bash
EGG_CACHE_DRIVER=redis \
EGG_CACHE_REDIS_URL='redis://127.0.0.1:16379/0' \
EGG_DATABASE_DRIVER=mysql \
EGG_DATABASE_DSN='egg:egg@tcp(127.0.0.1:13306)/egg?parseTime=true&loc=UTC' \
  go run ./cmd/api -config configs/config.yaml
```

### Otter 和 PostgreSQL

```bash
EGG_CACHE_DRIVER=otter \
EGG_DATABASE_DRIVER=postgres \
EGG_DATABASE_DSN='postgres://egg:egg@127.0.0.1:15432/egg?sslmode=disable' \
  go run ./cmd/api -config configs/config.yaml
```

- `pkg/cache.Adapter` 提供带 context 的 `Get`、`Set`、`Delete`。未命中返回 `cache.ErrMiss`，TTL 为零表示无到期时间，仍可能受容量淘汰。Otter 限制进程内条目数量。Redis 使用单节点 `redis://` 或 `rediss://` 连接，按 `cache.key_prefix` 隔离键。
- 数据库用 GORM 暴露统一入口，驱动为 `mysql`（MySQL 8）或 `postgres`。连接池、启动探测超时和慢查询阈值可配置。数据库关闭时，传给业务路由的 `DB` 为 nil。手写 SQL 和迁移仍需考虑两个数据库的方言差异。
- Compose 中的账号、口令和关闭 TLS 的 DSN 仅用于本地开发。实际连接凭据通过环境变量注入。

## 接入业务

启动入口在 `cmd/api`，依赖装配和生命周期在 `internal/app`，Gin 路由和中间件在 `internal/httpapi`。在 `app.Options.RegisterV1` 中挂载业务路由，框架将选中的 `cache.Adapter` 和 `*gorm.DB` 注入 `app.Dependencies`。框架的中间件和版本前缀自动应用到这些路由。

自定义缓存实现注册到 `cache.Manager`，将它传入 `app.Options.CustomCaches`，并令 `cache.driver` 与注册名一致。内置名称为 `otter` 和 `redis`。自定义适配器的生命周期由注册方管理，若实现 `Ping(context.Context) error`，则参与启动检查和 readiness，应遵守 context 的截止时间。

## 验证

```bash
go test -race ./...
go vet ./...
```

启动上面的 Compose 服务后，可运行真实后端集成测试。

```bash
EGG_TEST_REDIS_URL='redis://127.0.0.1:16379/0' \
EGG_TEST_MYSQL_DSN='egg:egg@tcp(127.0.0.1:13306)/egg?parseTime=true&loc=UTC' \
EGG_TEST_POSTGRES_DSN='postgres://egg:egg@127.0.0.1:15432/egg?sslmode=disable' \
  go test -race -count=1 ./...
```

停止本地依赖可执行 `docker compose down`。
