# MCP 只读接口

> 状态：2026-10 实现，默认关闭。外部 agent（Qoder、Claude Desktop、Cursor 等）可以通过
> `/mcp` 查询学习数据、组练习题；它不替代应用内置的 AI 智能体。

## 边界

- **只读**：不交卷、不写记录、不改题库、不碰 AI 配置与用户管理。
- **按用户隔离**：token 绑定一个用户，工具只能看到该用户自己的记录。
- **不替代内置 AI**：解析、批改、OCR 仍在应用内走 `internal/ai`；MCP 是给外部 agent 的入口。

## 启用与签发

```powershell
$env:KAOGONG_MCP = '1'
$env:CGO_ENABLED = '0'

# 签发一个只读 token，输出只显示一次
go run ./cmd/kaogong -mcp-token laptop

# 默认 180 天，可改
go run ./cmd/kaogong -mcp-token laptop -mcp-token-days 30

# 按 id 吊销（签发时会打印 #id）
go run ./cmd/kaogong -mcp-token-revoke 3
```

token 形如 `kgm_...`，数据库只存 SHA-256 哈希和 `last_used_at`。签发对象是
`KAOGONG_ADMIN_USER`（默认 `admin`）；若该用户不存在则取第一个管理员。

## 客户端配置

支持 HTTP MCP 的客户端：

```json
{
  "mcpServers": {
    "kaogong": {
      "type": "http",
      "url": "http://127.0.0.1:8080/mcp",
      "headers": {
        "Authorization": "Bearer kgm_xxx"
      }
    }
  }
}
```

只支持 stdio 的客户端用 `mcp-remote` 桥接：

```json
{
  "mcpServers": {
    "kaogong": {
      "command": "npx",
      "args": [
        "-y", "mcp-remote",
        "http://127.0.0.1:8080/mcp",
        "--header", "Authorization: Bearer kgm_xxx"
      ]
    }
  }
}
```

远程访问时把地址换成 HTTPS 隧道域名；token 不要放进前端或提交到仓库。

## 工具

| 工具 | 参数 | 返回 |
|---|---|---|
| `study_overview` | `subject?` | 各科目题量/已做、累计正确率、错题数、近 7 天 |
| `list_questions` | `state`（`wrong`/`favorite`/`unseen`）、`subject?`、`limit?`、`offset?` | 题目摘要、总数、`hasMore` |
| `get_question` | `id`（数字或 `custom-<id>`）、`includeAnswer?` | 题干、选项、材料、图片绝对 URL、答案与解析 |
| `weakness_report` | `subject?`、`minAttempts?`、`limit?` | 按正确率升序的章节列表 |
| `compose_practice` | `subject`、`n?`、`group?`、`chapters?`、`difficulty?`、`mock?` | 一组题（题干/选项），**不含答案**、不落库 |

约定：

- 返回体是 MCP `content[0].text` 里的 **JSON 字符串**，兼容只读文本的客户端。
- `compose_practice` 故意不给答案和解析，避免 agent 提前剧透；需要讲解时逐题调用 `get_question`。
- `get_question` 默认带答案，`includeAnswer=false` 只取题干，适合让用户先作答。
- `list_questions` 的 `unseen` 表示从未出现在该用户做题记录里的内置题；自定义题不参与。

## 协议形态

- 传输：Streamable HTTP，**无状态**，不签发 `Mcp-Session-Id`。
- 只接受 `POST /mcp`，返回 `application/json`；不提供 SSE 流和断线续传。
- 支持 `initialize`、`tools/list`、`tools/call`、`ping`；通知（无 `id`）返回 `202`。
- 协议版本：客户端请求 `2024-11-05` / `2025-03-26` / `2025-06-18` / `2026-07-28` 时原样协商，其他版本回落到 `2025-06-18`。
- HTTP 状态：未启用 `404`，缺 token/无效 token `401`，非 `POST` `405`，请求体上限 1 MiB。
- 未使用官方 Go SDK：其当前版本要求 Go 1.25，而本项目目标 Go 1.24；本实现只依赖标准库。

## 安全

- `Authorization: Bearer kgm_...`；不复用网页 cookie。
- 带 `Authorization` 的请求跳过 CSRF Origin 检查（bearer 不是环境凭证），其余请求规则不变。
- 建议只监听 `127.0.0.1`，或放在带 HTTPS 的隧道/反代后面。
- 暂无调用频率限制和工具级审计；仅沿用 HTTP 访问日志，不要把它直接暴露到公网。

## 代码与测试

| 文件 | 作用 |
|---|---|
| `internal/mcp/server.go` | 无依赖的 JSON-RPC / Streamable HTTP 传输层 |
| `internal/api/mcp_tools.go` | 五个工具，复用现有 handler 的结果与业务规则 |
| `internal/api/mcp_auth.go` | Bearer 校验、用户上下文、图片绝对 URL 的 base |
| `internal/auth/token.go` | token 签发/校验/吊销 |
| `internal/mcp/server_test.go` | 协议单测 |
| `internal/api/mcp_test.go` | 握手、五个工具、隔离、过期/吊销、CSRF 旁路 |

```powershell
$env:CGO_ENABLED = '0'
go test ./internal/mcp ./internal/api ./internal/auth
```

快速手工验证：

```bash
curl -s -X POST http://127.0.0.1:8080/mcp \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"curl","version":"1"}}}'
```
