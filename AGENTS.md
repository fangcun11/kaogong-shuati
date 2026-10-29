# AGENTS.md — kaogong-shuati 交接说明

> 面向下一个接手开发的 AI agent。项目目录：`D:\gk\kaogong-shuati`。
> 当前主线：Go + SQLite 后端；Vue3 + Vite + TypeScript 前端迁移中；旧版原生前端仍保留。

## 0. 先做什么

1. 读本文件 + [`docs/vue-migration.md`](docs/vue-migration.md) + [`docs/go-backend.md`](docs/go-backend.md)。
2. 检查 `git status --short --branch`，确认没有未提交改动。
3. 运行一次后端测试和前端构建，确认基线：

```powershell
$env:CGO_ENABLED = '0'
go test ./...
npm --prefix web run build
```

4. 启动后端（开发端口建议 8081，Vite 代理已按 8081 配置）：

```powershell
$env:KAOGONG_ADDR = '127.0.0.1:8081'
$env:CGO_ENABLED = '0'
go run ./cmd/kaogong
```

Vue 版访问 `http://127.0.0.1:8081/vue/`；旧版访问 `http://127.0.0.1:8081/`。

管理员账号只在本地 `data/initial-admin.txt`（gitignore，不要提交或输出到日志）；全新部署通过 `KAOGONG_ADMIN_PASSWORD` 创建。

## 1. 不可破坏的约束

- **Node 后端已删除，不能恢复。** Node 只允许用于 Vite 构建、Capacitor/Android 资源构建和现有工具脚本。
- **后端必须保持 Go + SQLite。** 当前使用 `modernc.org/sqlite`，`CGO_ENABLED=0`。
- **不要提交运行时数据或密钥：** `data/**`、`*.db`、`*.key`、`data/initial-admin.txt`、`*.env` 都在 gitignore 中。
- **旧前端必须保持可用。** 旧入口 `public/index.html` + `public/app.js` 是回退路径；迁移页面不要改坏它。
- **移动端优先。** 现有 UI 使用固定顶部栏、固定底部导航、底部 sheet；桌面端只是响应式适配。
- 题库数据版权归来源方，仅供个人学习；不要把采集工具或数据库副本另外分发。

## 2. 项目结构

```text
cmd/kaogong/                 Go 入口：配置、DB、HTTP server
internal/
  api/                       路由、中间件、题库/组卷/记录/自定义/AI handler
  auth/                      Argon2id 密码、session、MCP token
  ai/                        AI agent、加密 key、OpenAI 兼容客户端、技能加载
  mcp/                       MCP Streamable HTTP 传输层（纯标准库，无 SDK 依赖）
  store/                     schema.sql、legacy DB 导入、tiku 分卷重组
  config/                    环境变量
web/                         Vue3 + Vite + TypeScript 源码
public/                      旧版前端 + 构建后的 Vue 静态文件 public/vue/
public/vue/                  Vite 构建产物（已提交，方便直接运行）
app/                         Capacitor Android 工程
docs/                        设计与迁移文档
ref-skills/                  开源技能参考副本（运行时技能库在 DB）
```

数据库运行时为 `data/kaogong.db`（约 233 MB，gitignore）。首次启动会从 `tiku.db.part-*` 重组并导入 `tiku.db`、`materials.db`、`tiku-index.db`。

## 3. 当前完成度

### 后端

已完成：

- 多用户登录、Argon2id、HttpOnly session、`user`/`admin` 权限
- 题库 subjects/categories/papers/chapters/practice/question/materials
- 判分、做题记录、错题自动归档、统计、错题/收藏/笔记
- 智能组卷（模块/子知识点/四源配额、材料组、去重、弱项加权、职测模板）
- 自定义题库导入、批次、练习、图片 HTML
- AI 配置、加密 key、测试、历史、解析、OCR、结构化、申论批改、模型代理
- 技能库
- MCP 只读接口（`/mcp`，`KAOGONG_MCP=1` 开启）：`study_overview` / `list_questions` / `get_question` / `weakness_report` / `compose_practice`，Bearer token 按用户隔离，详见 [`docs/mcp.md`](docs/mcp.md)
- Go 静态服务支持 `/vue/` 目录 index

详细接口参考 [`docs/go-backend.md`](docs/go-backend.md)。

### Vue3 前端

已完成页面：

| Route | 页面 | 状态 |
|---|---|---|
| `/vue/#/` | 首页：科目 tab、组卷 banner、快捷入口、推广卡、题库行（宽屏两栏） | 可用 |
| `/vue/#/login` | 登录 | 可用 |
| `/vue/#/subject/:name` | 科目/来源分类 | 可用 |
| `/vue/#/source/:subject/:category` | 试卷列表 | 可用 |
| `/vue/#/practice/:subject/:category?` | 普通/试卷/组卷/自定义刷题 | 可用 |
| `/vue/#/papers` | 智能组卷配置、生成后进入答题 | 可用 |
| `/vue/#/stats` | 学习统计 | 可用（新库无记录时为空态） |
| `/vue/#/wrong` | 错题本 | 可用 |
| `/vue/#/favorites` | 收藏列表、取消收藏 | 可用 |
| `/vue/#/notes` | 笔记列表、删除 | 可用 |
| `/vue/#/question/:id` | 题目详情（内置题 + `custom-*` 自定义题） | 可用 |
| `/vue/#/custom` | 自定义批次列表、改名/删除/开始刷题 | 可用 |
| `/vue/#/custom/import` | TXT/Markdown/JSON/Excel/Word/PDF 导入、多文件、预览 | 可用（PDF 扫描页需「识图转写员」） |
| `/vue/#/ai` | AI 智能体列表、管理员编辑、测试 | 页面可用；真实网关未验证 |

刷题页已支持：

- 单选/多选、服务端判分
- 做题记录、收藏、笔记
- 答题卡（题号跳转、当前/正确/错误状态）
- 草稿纸（手写画布，纸面弹层/半透明覆盖两种模式，墨/红/蓝/绿四色，笔/橡皮/撤销/清空，仅本次练习保留）
- 背题模式（直接显示答案/解析，不写记录）
- 结果页（正确率圆环、对/错/未答统计、全部/错题/未答筛选、点题号回看解析、重做本次错题）

## 4. Vue 前端架构

- 构建：`web/`，Vite `base: '/vue/'`，输出 `../public/vue`
- 路由：`createWebHashHistory()`，所以刷新不需要 Go SPA fallback；`public/vue/index.html` 由 `internal/api/static.go` 的目录 index 逻辑提供
- API：`web/src/api/client.ts`，同源 `/api`、`credentials: include`、401 派发 `kg:unauthorized`
- Session：`web/src/stores/session.ts`，路由守卫首次加载 `/api/auth/me`
- 布局：`web/src/App.vue`，包含顶部栏、底部导航、主题切换、退出；≥900px 隐藏底部导航并显示顶部导航
- 样式：`web/src/main.ts` 引入旧版 `public/style.css`，再引入 `web/src/style.css`
- 组件/页面：`web/src/views/*.vue`、`web/src/components/Icon.vue`

页面与 API 对应、迁移细节、已知问题见 [`docs/vue-migration.md`](docs/vue-migration.md)。

## 5. 下一步优先级

建议按下面顺序继续：

### P0 — AI 功能与导入补齐

1. AI 设置页的保存与测试尚未用真实 API key 验证；要检查：
   - 非管理员只读
   - API key 留空不覆盖
   - 测试结果/错误展示
2. 自定义导入已迁移 Excel/Word/PDF（多文件、AI/本地两种解析方式）；PDF 图表题截图裁剪仍未迁移（旧版 `computeFigureCrops`），旧 `.doc` 二进制格式仅旧版支持。

> 自定义题详情已完成（`GET /api/question?id=custom-N`，归属校验；错题/收藏/笔记列表会带自定义题摘要）。
> 自定义导入 E2E 已验证：Excel 3 题、Word+PDF+TXT 9 题、AI 未配 key 时回退本地，均通过（临时库 8092）。

### P1 — 完整刷题体验

- 结果页回看与重做已完成（`ResultPanel.vue`：筛选、题号回看、重做本次错题）
- 主观题（申论/综应）输入与 AI 批改页
- 桌面端答题布局（当前固定底部导航，移动端优先）
- 旧版/Vue 版视觉与交互对拍（Playwright 截图 + 关键流程）

### P2 — 工程化

- Vue 端 E2E 测试（Playwright）：登录、刷题、判分、收藏、笔记、错题、组卷、导入
- API fixture 对拍（Node 旧版已删除，按 `node-backend-final` 行为做 fixture）
- 组件拆分：PracticeView 目前较大，后续拆 `QuestionCard`、`AnswerSheet`、`ResultPanel`
- 统一 toast/error 处理；目前多为页面内 error 文案

## 6. 开发与验证命令

```powershell
# 后端
$env:CGO_ENABLED = '0'
go test ./...
go vet ./...
go build -o kaogong.exe ./cmd/kaogong

# 前端
npm --prefix web install
npm --prefix web run build

# 前端开发服务器（需要 Go 后端已在 8081）
npm --prefix web run dev
```

浏览器手工验证最小链路：

```text
/vue/ 登录
→ 公务员·行测
→ 分类
→ 试卷
→ 选项
→ 提交答案
→ 答题卡 / 背题
→ 完成页
```

测试会写入 `practice_records`、`favorites`、`notes`。测试后如要保持空库，用只读检查确认后清理对应测试数据；不要删除用户真实数据。

## 7. Git 与发布

- 当前分支 `main`
- 当前上传目标：`origin https://github.com/fangcun11/kaogong-shuati.git`
- 原仓库保留为只读上游：`upstream https://github.com/ERRRC/kaogong-shuati.git`（当前 `fangcun11` 账号没有其写权限）
- `public/vue/` 的构建产物已纳入 Git；每次构建会生成新的 hash 文件名，提交时同时 stage 删除旧文件和新增文件：

```powershell
npm --prefix web run build
git add -A web public/vue
git commit -m "feat: ..."
git push origin main
```

- 不要把 `data/`、`kaogong.exe`、截图、浏览器 profile、API key、管理员密码提交。
- 提交信息建议：`feat: ...` / `fix: ...` / `docs: ...` / `test: ...`。

## 8. 关键文件索引

| 文件 | 用途 |
|---|---|
| `internal/api/server.go` | 路由注册、server 组装 |
| `internal/api/static.go` | 旧版和 `/vue/` 静态文件、目录 index |
| `internal/api/auth_handlers.go` | 登录/登出/me/改密 |
| `internal/api/quiz_handlers.go` | 题库/试卷/练习/判分/题目详情 |
| `internal/api/content_handlers.go` | subjects/categories/stats 等 |
| `internal/api/user_data_handlers.go` | records/favorites/notes/wrong |
| `internal/api/paper_handlers.go` | 智能组卷请求与模板 |
| `internal/api/paper_engine.go` | 组卷抽题引擎 |
| `internal/api/custom_handlers.go` | 自定义题库 |
| `internal/api/ai_handlers.go` | AI API |
| `internal/api/mcp_tools.go` | MCP 五个只读工具（复用现有 handler） |
| `internal/api/mcp_auth.go` | MCP Bearer 校验与用户上下文 |
| `internal/mcp/server.go` | MCP JSON-RPC / Streamable HTTP 传输层 |
| `internal/auth/token.go` | MCP token 签发/校验/吊销 |
| `cmd/kaogong/main.go` | `-mcp-token` / `-mcp-token-revoke` |
| `web/src/views/PracticeView.vue` | 刷题主流程 |
| `web/src/views/CustomImportView.vue` | 自定义导入 |
| `web/src/api/client.ts` | 前端请求封装 |
| `web/src/stores/session.ts` | 前端 session |
| `docs/vue-migration.md` | Vue 迁移详细说明 |

## 9. 已知问题 / 技术债

- 自定义题详情已接入 `GET /api/question?id=custom-*`；接口按 batch 归属校验，跨用户返回 404，材料组成员会借用组内持有者的材料。列表摘要（错题/收藏/笔记）依赖 `customQuestionSummary`，改动列表查询时不要退回只 JOIN `questions`。
- 自定义导入已覆盖 TXT/MD/JSON/Excel（`.xlsx/.xls`）/Word（`.docx`）/PDF（文本层 + 扫描页 OCR），多文件一次导入；PDF 图表题截图裁剪、旧 `.doc` 格式仍未迁移。静态服务必须给 `.mjs` 正确的 `text/javascript` MIME，否则 pdf.js ES module 加载失败（见 `internal/api/static.go` 与 `static_test.go`）。
- AI 真实模型调用、API key 保存回读、错误分支未做真实网关回归。
- MCP 目前只读，没有 SSE/断线续传、写工具和频率限制；工具直接调用现有 handler（`internal/api/mcp_tools.go`），不要复制一套 SQL，否则组卷裁剪、材料补全和归属校验会漂。
- `PracticeView` 的收藏 ID 已按 string 处理，避免自定义 ID 被 `Number()` 变成 NaN；后续改这里不要退回 number。
- `SmartPaperView` 的弱项加权请求字段是简化版，需对照后端 `paper_handlers.go` 校验。
- Vue 使用 hash 路由，外部链接需要带 `#/`；不要改成 history 路由，除非同时给 Go 加 `/vue/*` fallback。
- 旧版 `public/app.js` 仍然很大，迁移时只读参考，不要复制全局 DOM 逻辑到 Vue。

## 10. 完成标准

一个 Vue 功能算完成，至少满足：

1. 页面能从 `/vue/` 访问，刷新可用；
2. 走真实 Go API，不依赖旧版全局变量；
3. 401/错误有可见反馈；
4. 手机宽度（375px）和桌面宽度（1280px）都不溢出；
5. `npm --prefix web run build` 通过；
6. 关键流程用浏览器实际走通；
7. 提交源码 + 更新后的 `public/vue/` 构建产物；
8. 更新本文件或 `docs/vue-migration.md` 的状态。
