# Vue3 前端迁移说明

> 状态：渐进迁移中。旧版原生前端仍在 `public/`，Vue 版入口是 `/vue/`。
> 交接总览见 [`../AGENTS.md`](../AGENTS.md)。

## 1. 为什么是渐进迁移

旧前端 `public/app.js` 约 5,600 行，包含大量直接 DOM 操作、全局 `store`、Android WebView 兼容逻辑和本地 sql.js/IndexedDB 分支。直接全量重写风险高，所以采用：

- 新增 `web/` Vue3 工程
- 构建到 `public/vue/`
- Vue 使用 hash 路由，访问 `/vue/`
- 旧 `/` 继续可用
- 页面逐个迁移，后端 API 不变

## 2. 运行与构建

### 依赖

- Node.js 20+（本机验证过 Node 24）
- Go 1.24+
- `CGO_ENABLED=0`

### 安装与构建

```powershell
npm --prefix web install
npm --prefix web run build
```

构建输出：

```text
web/ → public/vue/
  index.html
  assets/index-<hash>.css
  assets/index-<hash>.js
```

`public/vue/` 已纳入 git，方便直接运行。每次重新构建会产生新的 hash 文件名，提交时使用 `git add -A web public/vue`，确保旧 hash 文件的删除也被提交。

### 本地开发

Vite 代理固定指向 `127.0.0.1:8081`，所以开发时把 Go 跑在 8081：

```powershell
$env:KAOGONG_ADDR = '127.0.0.1:8081'
$env:CGO_ENABLED = '0'
go run ./cmd/kaogong

# 另一个终端
npm --prefix web run dev
```

- Vite 开发地址：`http://127.0.0.1:5173/vue/`
- 生产静态入口：`http://127.0.0.1:8081/vue/`

Vite 配置在 `web/vite.config.ts`：

```ts
base: '/vue/'
build.outDir: '../public/vue'
server.proxy: { '/api': 'http://127.0.0.1:8081' }
```

### Go 静态服务

`internal/api/static.go` 已支持目录 index：

- `/vue/` → `public/vue/index.html`
- `/vue` → 同样返回 Vue index
- 旧 `/` → 未登录跳 `/login.html`，登录后由旧前端接管
- `/api/*` 缺失仍返回 JSON 404，不会误回 HTML

## 3. 目录与职责

```text
web/src/
├── api/client.ts              fetch 封装、JSON、401 事件
├── stores/session.ts          user / ready / login / logout
├── router/index.ts            hash 路由 + 登录守卫
├── components/Icon.vue        内联 SVG 图标
├── views/
│   ├── LoginView.vue
│   ├── HomeView.vue
│   ├── SubjectView.vue
│   ├── PaperListView.vue
│   ├── PracticeView.vue
│   ├── SmartPaperView.vue
│   ├── StatsView.vue
│   ├── WrongView.vue
│   ├── FavoritesView.vue
│   ├── NotesView.vue
│   ├── QuestionDetailView.vue
│   ├── CustomBankView.vue
│   ├── CustomImportView.vue
│   └── AiSettingsView.vue
├── App.vue                    顶部栏 + 底部导航 + RouterView
├── main.ts                    挂载、样式导入
└── style.css                  Vue 壳层补充样式
```

`main.ts` 先引入旧版 `public/style.css`，再引入 `web/src/style.css`。Vue 页面大量复用旧版 CSS 类：

- 布局：`#view`、`#topbar`、`#bottom-nav`、`.card`、`.subject-grid`
- 刷题：`.q-card`、`.q-content`、`.option`、`.q-meta`、`.q-progress-bar`
- 操作：`.btn`、`.action-row`、`.tag`、`.list-item`

新增页面优先复用这些类，避免建立一套平行的设计系统。

## 4. 路由与 API 映射

| Vue route | 页面 | 主要 API |
|---|---|---|
| `#/login` | 登录 | `POST /api/auth/login`、`GET /api/auth/me` |
| `#/` | 首页 | `GET /api/subjects`、`GET /api/records/stats`、`GET /api/custom/batches` |
| `#/subject/:name` | 来源分类 | `GET /api/categories?subject=` |
| `#/source/:subject/:category` | 试卷列表 | `GET /api/papers?subject=&category=` |
| `#/practice/:subject/:category?` | 练习 | `GET /api/practice`、`POST /api/check`、`POST /api/records` |
| `#/practice/:subject?paperId=` | 整套试卷刷题 | `GET /api/papers/{id}` |
| `#/practice/:subject?generated=1` | 智能组卷结果 | `POST /api/paper/generate` → sessionStorage |
| `#/practice/:subject?batchId=` | 自定义题练习 | `GET /api/custom/practice` |
| `#/papers` | 智能组卷配置 | `POST /api/paper/generate` |
| `#/stats` | 统计 | `GET /api/records/stats` |
| `#/wrong` | 错题本 | `GET /api/records/wrong` |
| `#/favorites` | 收藏 | `GET /api/favorites`、`DELETE /api/favorites` |
| `#/notes` | 笔记 | `GET /api/notes`、`DELETE /api/notes` |
| `#/question/:id` | 题目详情 | `GET /api/question?id=` |
| `#/custom` | 自定义批次 | `GET /api/custom/batches`、`PUT/DELETE /api/custom/batch` |
| `#/custom/import` | 文本导入 | `POST /api/custom/import` |
| `#/ai` | AI 设置 | `GET /api/ai/agents`、`PUT /api/ai/agents/{id}`、`POST /api/ai/agents/{id}/test` |

## 5. 关键实现细节

### API client

`web/src/api/client.ts`：

- 所有请求带 `credentials: 'include'`
- 有 body 时默认 `Content-Type: application/json`
- 响应先按 text 读，再尝试 JSON
- 非 2xx 抛 `ApiError(status, message)`
- 401 时派发 `window` 事件 `kg:unauthorized`
- `App.vue` 监听该事件，跳转 `#/login`，不依赖每个页面重复处理

### Session

`web/src/stores/session.ts`：

- `session.user` 为 `null | User`
- 首次路由导航调用 `/api/auth/me`
- 登录成功后 reload session
- 退出时即使 API 失败也清本地 session

### 刷题数据流

`PracticeView.vue` 支持 4 种来源：

1. **普通/分类练习**：`GET /api/practice?subject=&n=15&group=`
2. **整套试卷**：`GET /api/papers/{id}`，返回 `{name, questions}`
3. **智能组卷**：`SmartPaperView` 生成后写入 `sessionStorage['kg_generated_paper']`，再跳到 `?generated=1`；不依赖后端持久化
4. **自定义批次**：`GET /api/custom/practice?batch_id=&count=50`

提交：

```text
POST /api/check { questionId, selected }
POST /api/records { questionId, subject, chapter, type, selected, correct, costMs }
```

`subject` 对普通题是真实科目，对自定义题是 `"自定义"`。`check.ok === null` 表示没有标准答案，不应当当作正确。

### 答题卡/背题/结果

- `answers` 以题目 id 的字符串为 key 保存 `{ selected, check, costMs }`
- 答题卡状态：`current` / `todo` / `correct` / `wrong` / `answered`
- 背题模式不选答案、不提交、不写记录，直接高亮正确项并显示解析
- 结果页统计由 `answers` 派生，不依赖后端汇总

### 自定义导入

`CustomImportView.vue` 通过 Vite 直接引入：

```ts
import { parseTxt } from '../../../public/lib/custom-parser.js'
```

这是旧版共享的纯浏览器 ES module，复用它的文本解析。当前支持：

- 粘贴文本
- `.txt` / `.md`
- `.json`（数组或 `{questions: []}`）

导入字段：

```json
{
  "name": "批次名",
  "subject": "行测",
  "questions": [
    {
      "prompt": "题干",
      "material": "",
      "options": ["A. 选项一", "B. 选项二"],
      "answer": "B",
      "answer_index": 1,
      "analysis": "解析"
    }
  ]
}
```

Excel/Word/PDF 仍走旧版路径；不要假装 Vue 已支持。

### AI 设置

`AiSettingsView.vue` 读取 `/api/ai/agents`，管理员才能保存/测试。API key 输入框为空时不会覆盖已保存 key；后端返回的是 `api_key_masked`，不回传明文。

## 6. 当前完成度与缺口

### 已完成

- 登录、会话、退出
- 首页/科目卡/快捷入口
- 来源分类 → 试卷列表 → 试卷刷题
- 普通/分类/自定义/组卷四种练习来源
- 单选/多选判分
- 做题记录
- 收藏/取消收藏
- 笔记添加/查看/删除
- 错题列表
- 题目详情（普通题）
- 学习统计
- 智能组卷配置 + 生成
- 自定义批次列表、改名/删除、练习
- 文本/JSON 导入
- AI 智能体列表/编辑/测试 UI
- 答题卡/背题/完成页

### 未完成

- `custom-*` 题目详情：后端 `GET /api/question` 目前拒绝 custom id；从收藏/错题点自定义题会提示“暂不支持”
- Excel/Word/PDF 导入
- 主观题输入和 AI 批改页面（申论/综应）
- 结果页题目列表、错题回顾、单题重做
- 桌面端答题排版优化
- AI 真实网关回归
- Vue 专用 E2E 测试
- 旧版/新版完整视觉对拍

## 7. 手工回归清单

每次改刷题/记录相关代码，至少走：

```text
1. 登录
2. 首页 5 张科目/自定义卡
3. 行测 → 来源分类 → 试卷列表 → 打开试卷
4. 选答案 → 提交 → 正确/错误高亮 → 下一题
5. 答题卡打开、跳题、关闭
6. 背题模式开关、答案/解析显示
7. 最后一题完成 → 结果页统计
8. 收藏 → 收藏页可见 → 取消收藏
9. 笔记 → 笔记页可见 → 删除
10. 错题 → 题目详情
11. 智能组卷 → 生成 → 进入答题
12. 自定义批次 → 刷题
```

自动构建检查：

```powershell
$env:CGO_ENABLED = '0'
go test ./...
npm --prefix web run build
```

浏览器验证建议用 Playwright/Chromium，账号从本地 `data/initial-admin.txt` 读取；不要把凭据写入脚本或提交。

## 8. 测试数据清理

测试可能写入：

- `practice_records`
- `favorites`
- `notes`
- `custom_batches` / `custom_questions`

清理前先确认只删除自己创建的测试数据；不要清空真实用户记录。当前本地数据库曾用于验证，交付前应检查 `practice_records` 等表为空或只包含预期数据。

## 9. 新增一个 Vue 页面的步骤

1. 在 `web/src/views/` 新建 `XxxView.vue`
2. 在 `web/src/router/index.ts` 加路由（记得 `meta.title`）
3. 用 `api()`/`post()` 调 Go API，不要直接 `fetch`
4. 复用 `.card`、`.btn`、`.list-item`、`.q-*` 等现有样式
5. 用 `npm --prefix web run build` 构建
6. 启动 Go 在 8081，浏览器走 `/vue/#/...`
7. 手机 375px + 桌面 1280px 截图检查
8. `git add -A web public/vue` 提交
9. 更新 `AGENTS.md` 完成度或本文件

## 10. 代码风格与限制

- TypeScript strict，但旧共享 parser 是未类型化 JS，用 `@ts-expect-error` 引入
- 不引入 Pinia/Router 之外的重量级状态库；当前 session 用 reactive store 足够
- 不把业务数据塞进 URL；组卷结果用 sessionStorage，因为后端没有持久化生成卷
- 使用 hash 路由，避免 Go 需要为 `/vue/*` 做 SPA fallback；如果改 history 路由，必须同时改 `internal/api/static.go`
- 不在 Vue 中复刻旧版全局 `store` 和 `$()` DOM 选择器
- 不用 `v-html` 渲染不可信内容；目前题干/解析来自本地题库，属于受信数据
