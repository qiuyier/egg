# egg

`egg` 是通用的 Go API 脚手架，用于创建并运行独立的 Gin 项目。[框架运行说明](PROJECT.md)包含配置、缓存、数据库和追踪的使用方式。

## 安装 CLI

使用 Go 1.27.0 或更高版本的工具链安装。在任意目录执行下列命令。

```bash
go install github.com/qiuyier/egg/cmd/egg@v0.1.0
egg version
```

如果 shell 找不到 `egg`，将 `go env GOBIN` 对应的目录加入 `PATH`。如果 `GOBIN` 为空，安装目录默认为 `$(go env GOPATH)/bin`。

安装最新发布版本时，将 `@v0.1.0` 换成 `@latest`。

## 创建并运行项目

在准备存放新项目的目录里运行以下命令。把示例模块路径换成项目实际使用的路径。默认模板提供通用的 HTTP 服务和基础设施，业务路由通过 `RegisterV1` 接入。

```bash
egg new my-api -module github.com/your-org/my-api
cd my-api
egg run -config configs/config.yaml
```

| 命令 | 用途 |
| --- | --- |
| `egg new <目录>` | 创建独立的通用 Gin API 项目 |
| `egg api <目录>` | `egg new` 的别名 |
| `egg run [入口] [参数]` | 在项目根目录启动指定的 Go 入口，默认使用 `./cmd/api` |
| `egg version` | 显示启动标识、CLI 和 Go 版本、系统架构、CPU 核心数、Go 路径和编译器。有 Git 构建信息时也显示提交和提交时间 |
| `egg help [命令]` | 查看命令用法 |

自定义入口放在应用参数前，例如 `egg run ./cmd/server -config configs/local.yaml`。入口在项目根目录时，用 `egg run . -config configs/local.yaml`。

CLI 内置安装时的模板。省略 `-module` 时使用 `example.com/<项目名>`。目标目录必须尚未存在，父目录需已存在。项目名默认取目标目录名，可用 `-name my-api` 指定。项目名以小写字母开头，可使用小写字母、数字和连字符，最多 30 个字符。

如果需要直接从工作区生成，也可以在模板仓库根目录运行原有命令，`-out` 指向模板外的新目录。

```bash
go run ./tools/new-project -out ../my-api -module github.com/your-org/my-api
```

生成器会换成新项目的 Go 模块和包导入路径，并设置独立的服务名、环境变量前缀、Prometheus 指标名称、Redis 键前缀和本地 Compose 项目名称。例如 `my-api` 使用 `MY_API_HTTP_ADDRESS` 和 `my-api:`。

每次生成都是一份独立源码。生成器复制框架代码、测试、锁定依赖和示例配置。Git 历史、`.env`、本地配置和日志保留在各自目录。需要版本管理时，在新项目目录运行 `git init`。

新项目的 `README.md` 说明如何运行 API、切换 Redis 和数据库以及接入业务路由。修改本地模板后，在仓库根目录运行 `go install ./cmd/egg`，后续生成的项目会使用更新后的模板。已有项目可以按需单独合并框架更新。

## 可选示例

显式传入 `-example inventory` 可以在通用模板上附加库存示例，用于学习业务路由、SQL 迁移、事务和 OpenAPI 的接入方式。在源码仓库根目录安装开发版 CLI，再到准备存放示例的目录生成项目。

```bash
go install ./cmd/egg
cd ..
egg new inventory-demo -example inventory
cd inventory-demo
```

按生成项目里的 `INVENTORY.md` 启动 PostgreSQL、执行迁移并试用接口。[库存示例说明](examples/inventory/README.md)

## 许可证

本项目采用 [Apache License 2.0](LICENSE)。
