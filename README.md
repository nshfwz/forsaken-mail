# 即收即毁 (Go)

独立 Go 项目版本，提供临时邮箱收件与网页/API 查询能力。

## 功能

- SMTP 收件服务（默认 `:25`）
- HTTP API（默认 `:3000`）
- 前端页面（`/`）查看邮件列表与正文（SSE 实时推送，失败自动回退轮询）
- 按邮箱查询、删除单封、清空邮箱 API

## 快速启动

```bash
go mod tidy
go run ./cmd/server
```

访问：

- 前端：`http://127.0.0.1:3000`
- 健康检查：`http://127.0.0.1:3000/api/health`

## API 示例

```bash
curl "http://127.0.0.1:3000/api/mailboxes/demo/messages"
curl "http://127.0.0.1:3000/api/mailboxes/demo/messages/{message_id}"
curl -N "http://127.0.0.1:3000/api/mailboxes/demo/events"
curl -X DELETE "http://127.0.0.1:3000/api/mailboxes/demo/messages/{message_id}"
curl -X DELETE "http://127.0.0.1:3000/api/mailboxes/demo/messages"
curl "http://127.0.0.1:3000/api/messages?email=demo@example.com"
curl "http://127.0.0.1:3000/api/messages/{message_id}?email=demo@example.com"
```

删除响应示例：

```json
{"mailbox":"demo","email":"demo@example.com","id":"abc123","deleted":true}
{"mailbox":"demo","email":"demo@example.com","count":3}
```

## 邮件保留策略

- 内存保存（服务重启即清空）
- 默认每邮箱最多 `200` 封
- 默认过期时间 `24h`（`MESSAGE_TTL_MINUTES=1440`）

### 邮件保存时间说明

- 默认保存时间：`24 小时`
- 配置方式：设置环境变量 `MESSAGE_TTL_MINUTES`
- 示例：`MESSAGE_TTL_MINUTES=60` 表示邮件保存 `60 分钟`
- 清理方式：服务会周期清理过期邮件（约每分钟一次）
- 注意：当前策略不是“查看即删除”，查看邮件不会立刻销毁

## 环境变量

- `HTTP_ADDR`：HTTP 监听地址，默认 `:3000`
- `SMTP_ADDR`：SMTP 监听地址，默认 `:25`
- `MAIL_DOMAIN`：限制收件域名（可选）
- `MAILBOX_BLACKLIST`：邮箱前缀黑名单，逗号分隔
- `BANNED_SENDER_DOMAINS`：拒收发件域名，逗号分隔
- `MAX_MESSAGES_PER_MAILBOX`：每邮箱保留上限，默认 `200`
- `MESSAGE_TTL_MINUTES`：邮件过期分钟数，默认 `1440`
- `MAX_MESSAGE_BYTES`：单封邮件最大字节数，默认 `10485760`

## 构建

```bash
go build -o ./dist/forsaken-mail.exe ./cmd/server
```

## 自动发布（GitHub Actions）

- 推送版本标签会自动构建并发布二进制到 GitHub Release
- 标签格式：`v*`（例如 `v1.0.0`、`v1.1.0-rc1`）
- 发布产物包含 `linux/windows/darwin` 的 `amd64/arm64` 二进制与 `checksums.txt`

示例：

```bash
git tag v1.0.0
git push origin v1.0.0
```

## Docker

```bash
docker build -t tempmail-go .
docker run --name tempmail-go -p 25:25 -p 3000:3000 tempmail-go
```
