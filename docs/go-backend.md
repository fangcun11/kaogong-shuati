# Go 后端迁移说明

> 旧 Node 后端已打标签 `node-backend-final` 并从工作树删除。当前运行路径为 Go + SQLite。

## 环境

- Go 1.24+
- SQLite 由 `modernc.org/sqlite` 提供，纯 Go，无需 CGO

## 启动

```powershell
$env:KAOGONG_ADMIN_USER = 'admin'
$env:KAOGONG_ADMIN_PASSWORD = '请换成至少 8 位的强密码'
$env:KAOGONG_ADDR = '127.0.0.1:8080'
$env:CGO_ENABLED = '0'
go run ./cmd/kaogong
```

首次启动会自动：

1. 创建 `data/kaogong.db`
2. 必要时从 `tiku.db.part-*` 重组 `tiku.db`，再从 `tiku.db`、`materials.db`、`tiku-index.db` 导入题库内容
3. 创建第一个管理员账号

本工作区当前的初始管理员凭据写在 `data/initial-admin.txt`（gitignore，不进入仓库）。首次登录后请立即修改密码，或在正式部署时删除该文件；全新部署则通过 `KAOGONG_ADMIN_PASSWORD` 创建管理员，不会自动生成该文件。

也可以用 `-import` 强制重新导入内容。

## 已迁移接口

- `POST /api/auth/login`
- `POST /api/auth/logout`
- `GET /api/auth/me`
- `POST /api/auth/password`
- 管理员：`GET/POST /api/admin/users`，`PATCH /api/admin/users/{id}`
- 题库：subjects、categories、papers、chapters、practice、question、materials
- 记录：`POST /api/check`、`POST /api/records`、stats、recent、wrong、wrong/groups
- 收藏、笔记、整理
- 智能组卷：`POST /api/paper/generate`（基础版，按模块配额抽题）
- 自定义题库：导入、批次、题目编辑、材料分组、练习、判分
- AI：配置、测试、历史、解析、OCR、题目结构化、申论批改、材料读取
- 技能库：列表、导入、删除、URL 拉取
- Vue3 迁移版：`web/` 构建到 `public/vue/`，访问 `/vue/`；当前已完成登录、首页、科目分类和基础 AppShell

## 账号与权限

- 角色：`user`、`admin`
- 密码：Argon2id
- 会话：随机 token 的 SHA-256 哈希存 SQLite，HttpOnly Cookie
- 所有用户数据表都带 `user_id`；查询强制按当前 session 用户过滤
- 管理员接口使用 `requireAdmin` 中间件

## 与旧 Node 版的差异

- 智能组卷已按模块/子知识点配额、国考/联考省考/独立命题/选调四类题源配额、资料分析材料整组、组去重与跨源去重、模块/子题型顺序和弱项加权实现；支持 `chapters`、`types`、`perSource`。
- 职测 A/B/C 类与山东按对应模板组卷，B/C 的「数量分析/综合分析」已接入。
- 自定义题已支持题干/材料图片的 HTML 附加渲染，按题量截断时会保留完整材料组；富文本排版仍是简化版。
- 旧版依赖本机 Chrome 的 PDF 材料 OCR 未移植；优先使用 `materials.db` 中已采集的材料。
- 错题自动归档、错题分页 `hasMore`、一键整理来源分类、练习的年份/难度/模拟卷/材料组过滤和统计的时间范围过滤已补齐。
- AI 图片 OCR 已支持格式校验和 OCR→文本结构化的降级；仍没有浏览器 PDF 渲染提取链路。
- 本地模式（Android 离线）不经过 Go 后端，保持原有 sql.js/IndexedDB 逻辑。

## 测试

```powershell
$env:CGO_ENABLED = '0'
go test ./...
```

## 安全默认值

- 默认只监听 `127.0.0.1`
- 请求体有大小限制
- 修改类请求校验同源 Origin
- 登录失败限流
- 第一个管理员没有默认密码，必须通过环境变量创建
- 生产环境请设置 `KAOGONG_SECRET`（至少 32 字节随机值），用于加密 AI API key
