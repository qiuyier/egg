# 库存 API 示例

在 `egg new <目录> -example inventory` 生成的项目中执行下面的命令。示例使用 PostgreSQL，新增商品时库存为零，同一 SKU 只能创建一次。

## 迁移并启动

```bash
docker compose up -d --wait postgres
export EGG_DATABASE_DRIVER=postgres
export EGG_DATABASE_DSN='postgres://egg:egg@127.0.0.1:15432/egg?sslmode=disable'
GOOSE_DRIVER=postgres GOOSE_DBSTRING="$EGG_DATABASE_DSN" \
  go run github.com/pressly/goose/v3/cmd/goose@v3.30.0 -dir migrations up
egg run -config configs/config.yaml
```

本地 DSN 对应 Compose 中的开发数据库。如果修改了映射端口，同步修改 DSN。部署时，用部署环境中的数据库连接信息单独执行迁移，再启动应用。

## 试用接口

创建商品。用返回的 `data.id` 替换下面示例中的 `1`。

```bash
curl -fsS http://localhost:8080/api/v1/items \
  -H 'Content-Type: application/json' \
  -d '{"sku":"SKU-001","name":"USB-C 线"}'

curl -fsS 'http://localhost:8080/api/v1/items?page=1&per_page=20'

curl -fsS -X PATCH http://localhost:8080/api/v1/items/1/stock \
  -H 'Content-Type: application/json' \
  -d '{"delta":10}'
```

正数 `delta` 增加库存，负数减少库存。更新在事务内锁定商品行。调整后库存超出范围时返回 HTTP 409，商品不存在时返回 HTTP 404。浏览器接入、鉴权和限流可按项目部署环境配置。[OpenAPI 定义](openapi.yaml)
