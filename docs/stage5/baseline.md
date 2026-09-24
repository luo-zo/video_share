# 第五阶段基线报告（T00）

**采集时间：** 2026-09-22
**采集目录：** 仓库根目录（主仓库，非 worktree）
**对应任务书：** `docs/superpowers/plans/2026-09-22-stage5-feature-expansion.md`（v2.0）

本文件只记录**本次实际执行**得到的观察结果。凡未实际运行的检查，均在末尾“未验证项”中单独列出。

---

## 1. 仓库与分支状态

| 项目 | 实际值 |
|---|---|
| 当前分支 | `main` |
| HEAD | `cff21bd` fix: finalize stage 4 community flows |
| 与远端关系 | 领先 `origin/main` **19 个提交**（未推送） |
| 提交总数 | 21 |
| 已跟踪文件的未提交改动 | **5 个**（**本条已修正**，初版误记为“无”）：`frontend/index.html`、`frontend/package-lock.json`、`frontend/package.json`、`frontend/server.mjs`、`frontend/tests/server.test.mjs` |
| 未跟踪文件 | **16 个**（**本条已修正**）：`docs/stage5/baseline.md`、`docs/stage5/decisions.md`、`docs/superpowers/plans/2026-09-22-stage5-feature-expansion.md`、`frontend/server.d.mts`、`frontend/src/App.vue`、`frontend/src/main.ts`、`frontend/src/api/{index,client,auth,video,community}.ts`、`frontend/src/lib/player.ts`、`frontend/src/router/index.ts`、`frontend/src/stores/auth.ts`、`frontend/tsconfig.json`、`frontend/vite.config.ts` |

`main` 上这份未提交的 Vue 脚手架是**既有在途工作**，不是本次产生的中间产物。本次收口**没有覆盖、删除、reset、checkout、clean 或 stash** 其中任何文件（详见第 11 节）。

任务书声明“代码基线：本次检查 main 为 cff21bd”，**与实际一致**，无须修正基线。

### 1.1 worktree 副本（重要）

```
仓库根目录                                      cff21bd [main]
.worktrees/stage4-community                    b7116bf [feature/stage4-community]
.worktrees/stage5-upgrade                      8452e3a [feature/stage5-upgrade]
```

`.worktrees/stage5-upgrade` 是今日实际完成 T01 的副本，位于 `feature/stage5-upgrade` 分支，含三个提交（`372469e` → `7e6430a` → `8452e3a`），基线同为 `cff21bd`。T01 的修复只能在该分支内进行，第 11 节记录了本轮的收口结果。

`.worktrees/stage4-community` 落后 main 一个提交。`git status` 在其中显示 35 个文件“已修改”，但经 `git diff main --ignore-cr-at-eol` 核验：

- 这 35 个文件的**内容与 main 完全一致**，差异**全部是 CRLF/LF 换行符**造成的假修改；
- 唯一真实差异是 `backend/internal/server/auth_routes_integration_test.go`（main 中已跟踪，worktree 中因基础版本较早而缺失）。

**结论：worktree 中不存在独立的未提交业务改动，无丢失风险。** 但按任务书要求，第五阶段一律在主仓库 `main` 上进行，**不进入、不修改该 worktree**。

### 1.2 换行符问题（环境事实，非缺陷）

`core.autocrlf=true`，工作区为 CRLF。因此 `gofmt -l .` 会列出约 36 个 Go 文件，但 `gofmt -d` 的差异**全部是行尾 `^M`**，没有任何真实的格式或缩进问题（已用 `cat -A` 逐字节确认）。Linux CI 以 LF 检出时 `gofmt` 为干净。

影响：若在 Windows 上直接给 `gofmt -l` 接门禁，会得到大量假失败。处理建议见 `decisions.md` 第 7 节（加 `.gitattributes`）。

---

## 2. 工具版本（实际执行 `--version`）

| 工具 | 实际版本 | 任务书要求 | 状态 |
|---|---|---|---|
| Go | `go1.26.3 windows/amd64` | 跟随 `go.mod`（`go 1.26.3`） | 一致 |
| Node | `v22.22.3` | 使用已验证的 22.22.3 | 一致（但见下） |
| npm | `12.0.1` | — | — |
| Docker | `29.6.2` | — | — |
| Docker Compose | `v5.3.1` | — | — |
| FFmpeg / FFprobe | `9.0.1-essentials_build` (gyan.dev) | 需要 | 一致 |
| Git | `2.51.2.windows.1` | — | — |

**版本风险：** `frontend/package.json` 只声明 `"engines": { "node": ">=20" }`，没有 `.nvmrc`、没有 `.node-version`、没有 CI 版本锁定。本机恰好是 22.22.3，但任务书要求“开发、CI、镜像一致”。T01 需补 `engines`/`.nvmrc`。

---

## 3. 运行中的服务与代码目录

`docker ps` 实际输出（已运行 27 小时，健康）：

| 容器 | 状态 | 端口 |
|---|---|---|
| `video_share_mysql` | Up 27h (healthy) | `0.0.0.0:3308->3306` |
| `video_share_minio` | Up 27h (healthy) | `127.0.0.1:9000-9001` |
| `video_share_kafka` | Up 27h (healthy) | `127.0.0.1:9092` |

**未运行：** API（`curl http://127.0.0.1:8081/healthz` 无响应）、Worker、前端（`http://127.0.0.1:5173` 返回 `000`）。

即：**依赖服务在跑，应用进程没跑。** 既有 PowerShell 端到端脚本（`e2e-transcode.ps1`、`e2e-community.ps1`）都要求 API+Worker 已启动，因此本基线**未能执行**它们。

MySQL 由 `backend/deploy/docker-compose.yml` 提供，`3308` 映射与 `backend/.env.example` 中 `MYSQL_PORT=3308` 一致。API 端口 `8081`（`HTTP_ADDR` 默认 `127.0.0.1:8081`）、前端 `5173`（`server.mjs` 中 `PORT` 默认 5173），与任务书一致。

---

## 4. 数据库实际状态

连接方式：`docker exec` + `.env` 中的账号（仅在命令中临时读取，未写入任何文件或日志）。

- 数据库列表：仅 `video_share`（无遗留测试库）。
- Goose 版本表 `goose_db_version`：`0,1,2,3,4,5` 全部 `is_applied=1`，即**迁移 000001–000005 已全部执行**。
- 现有表（11 张）：

```
users, videos, video_stats, video_likes, video_favorites,
comments, user_follows, watch_histories,
video_transcode_jobs, outbox_events, goose_db_version
```

- 现有数据量：

| 表 | 行数 |
|---|---|
| users | 12 |
| videos | 9 |
| comments | 4 |
| video_stats | 9 |
| user_follows | 3 |
| watch_histories | 2 |
| outbox_events | 10 |

**结论：** 任务书“现有迁移到 000005，本次从 000006 开始”与实际情况**完全一致**，编号可直接顺延，不需要额外顺延。库中有真实业务数据，**后续任何迁移都不得清空或重建业务库**；Down 迁移不得在业务库上随意执行。

---

## 5. 测试基线（本次实际运行结果）

### 5.1 后端单元测试

```
cd backend && go test ./... -count=1
```

结果：**全部通过**，无失败、无跳过。

```
ok  video_share/internal/config      1.318s
ok  video_share/internal/database    0.513s
ok  video_share/internal/engagement  1.075s
ok  video_share/internal/follow      0.396s
ok  video_share/internal/middleware  0.541s
ok  video_share/internal/outbox      1.635s
ok  video_share/internal/server      0.718s
ok  video_share/internal/storage     2.317s
ok  video_share/internal/token       1.919s
ok  video_share/internal/transcode   0.913s
ok  video_share/internal/user        1.961s
ok  video_share/internal/video       0.812s
（cmd/api、cmd/worker、internal/messaging、internal/response、internal/testutil、migrations 无测试文件）
```

`go vet ./...` 与 `go build ./cmd/api ./cmd/worker`：**均通过（exit 0）**。

### 5.2 前端测试

```
cd frontend && npm test   # node --test
```

结果：**100 个用例全部通过**，`fail 0 / skipped 0`。覆盖输入校验、令牌生命周期、API 调用顺序、转码轮询、MP4/HLS 选择、直传失败处理、社区互动客户端、静态文件与安全响应头、API 路由白名单、MinIO 跳转限制、请求限制与代理超时。

### 5.3 集成测试（真实 MySQL）

**不设 `TEST_MYSQL_DSN` 时**：13 个顶层用例（含子用例共 30 条断言组）全部 `SKIP`。这正是任务书要防止的“把跳过当验收通过”。

**设为应用账号 `video_share` 时**：**失败**，根因明确：

```
create test database: Error 1044 (42000):
Access denied for user 'video_share'@'%' to database 'video_share_test_...'
```

即应用账号**没有 `CREATE DATABASE` 权限**，而 `internal/testutil/testdb.go` 需要为每次运行创建并删除隔离测试库。

**设为 `root`（有建库权限）后**：**全部通过**。

```
ok  video_share/internal/database    6.793s
ok  video_share/internal/engagement 39.276s
ok  video_share/internal/follow     18.925s
ok  video_share/internal/messaging   2.084s
ok  video_share/internal/server      3.732s
ok  video_share/internal/transcode   3.313s
ok  video_share/internal/user        4.551s
ok  video_share/internal/video      14.479s
（其余包无集成测试）
```

**Kafka 集成测试**：`TestKafkaPing` 需 `TEST_KAFKA_BROKERS`，缺省时跳过；设为 `127.0.0.1:9092` 后**通过**（0.03s）。

**结论：** 代码本身没有基线缺陷，集成测试失败纯粹是**测试库权限配置**问题。这必须作为 T10/T11 的显式前提写入文档与 CI，否则“集成测试绿灯”不可信。

---

## 6. 已实现能力与回归基线

以下能力代码中存在，且被单元测试覆盖。**“浏览器验证”一栏诚实地反映本次基线没有做的事情**（API/Worker/前端均未启动）。

| 回归路径 | 主要接口 | 单元测试 | 浏览器/端到端验证 |
|---|---|---|---|
| 注册 | `POST /api/v1/auth/register` | 有 | 未验证 |
| 登录 | `POST /api/v1/auth/login` | 有 | 未验证 |
| 重新获取身份 | `GET /api/v1/users/me` | 有 | 未验证 |
| 发现 + 搜索 + 排序 | `GET /api/v1/videos?q=&sort=` | 有 | 未验证 |
| 投稿创建 | `POST /api/v1/videos` | 有 | 未验证 |
| 直传 + 确认 | MinIO 预签名 `PUT` → `POST /videos/:id/complete` | 有 | 未验证 |
| 异步转码 | Outbox → Kafka → Worker → FFmpeg/HLS | 有 | 未验证（Worker 未运行） |
| 播放 | `GET /videos/:id`、`/hls/*path`、`/cover` | 有 | 未验证 |
| 点赞 / 收藏 | `PUT|DELETE /videos/:id/like`、`.../favorite` | 有 | 未验证 |
| 评论 | `GET|POST /videos/:id/comments`、`DELETE /comments/:id` | 有 | 未验证 |
| 观看历史 | `POST /videos/:id/watch`、`GET /users/me/history` | 有 | 未验证 |
| 关注 | `PUT|DELETE /users/:id/follow`、`GET /users/me/follows` | 有 | 未验证 |
| 作者管理 | `GET|PATCH|DELETE /users/me/videos/:id` | 有 | 未验证 |

**现有前端形态**：原生 HTML/CSS/JS 单页应用，**没有任何框架、路由或构建步骤**（`package.json` 仅依赖 `hls.js`，`dev` 脚本是 `node server.mjs`）。页面切换靠 `data-app-panel` 的 `hidden` 属性切换，**不是 URL 路由**——因此刷新丢失页面状态、链接无法分享，正是任务书 F02 要解决的问题。

代码库中**搜索不到任何 Redis 或 Vue 的引用**（仅出现在计划文档与 `hls.js` 的第三方产物中），确认 F01/F07 均为全新工作。

---

## 7. 与任务书的差异清单

| # | 任务书要求 | 实际状态 | 差异性质 |
|---|---|---|---|
| D1 | 前端迁移到 Vue 3 + TS + Vite + Router + Pinia | 原生 JS，无构建、无路由 | **待建设**（T01 核心） |
| D2 | Redis 限流/缓存/榜单 | 完全不存在；compose 中无 Redis 服务 | **待建设**（T02） |
| D3 | 迁移从 000006 起 | 已到 000005，编号可直接顺延 | 一致，无差异 |
| D4 | 前端 `npm run typecheck/test/build/test:e2e` | 只有 `npm test`（`node --test`）；无 vue-tsc / Vite / Vitest / Playwright | **待建设** |
| D5 | 集成门禁 `REQUIRE_INTEGRATION_TESTS=true` | 不存在；缺 DSN 时静默跳过 | **待建设**（T10） |
| D6 | CI（`.github/workflows/ci.yml`） | 不存在 `.github/` | **待建设**（T10） |
| D7 | Nginx 配置与前端 Dockerfile | 只有 `backend/Dockerfile`、`backend/deploy/docker-compose.yml`；无 nginx.conf、无前端 Dockerfile | **待建设**（T10） |
| D8 | `docs/stage5/*` | 目录本次新建 | 本次已建立 |
| D9 | Node 版本跨环境锁定 | 仅 `engines: >=20` | 待补齐（T01） |
| D10 | `.env.example` 含全部变量 | 无 Redis、无会话、无 `TEST_MYSQL_DSN`、无 `REQUIRE_INTEGRATION_TESTS`、无 `TRUSTED_PROXIES` 示例值 | 待补齐 |
| D11 | Redis 固定版本 | compose 已固定 `mysql:8.0`、`minio:RELEASE.2025-09-07`、`apache/kafka:4.0.0`，是良好实践 | T02 照此固定 Redis 版本 |

补充观察：任务书允许“Node 使用已验证的 22.22.3”，本机确为 22.22.3，因此 T01 无需换 Node，只需把版本**锁定并写入配置**。

---

## 8. T01 前端迁移的具体影响面

以下路径与行号来自本次实际阅读。

### 8.1 需要**新建**的文件（任务书已列，本次确认落点）

```
frontend/package.json          改：由 node 脚本改为 vite/vue-tsc/vitest/playwright
frontend/vite.config.ts        新
frontend/tsconfig.json         新（含 vue-tsc 严格模式）
frontend/index.html            改：改为 Vite 入口，保留小黑猫 SVG 与结构
frontend/src/main.ts           新（替代 src/main.js）
frontend/src/App.vue           新
frontend/src/router/index.ts   新
frontend/src/stores/auth.ts    新
frontend/src/views/            LoginView / DiscoverView / VideoDetailView / UploadView / ProfileView
frontend/src/components/       AppHeader / BlackCat / VideoCard / VideoPlayer / CommentList
frontend/src/api/              client.ts / auth.ts / video.ts / community.ts
frontend/tests/unit/           迁移 tests/*.mjs
frontend/tests/e2e/            Playwright
```

### 8.2 需要**迁移或改造**的现有文件

| 现有文件 | 行数 | 迁移动作 |
|---|---|---|
| `frontend/src/main.js` | 1126 | 拆分为 App.vue + 各 view。**这是最大的一块**：含约 60 处 `getElementById`，其中 12 处是**模块加载期求值**的常量（`main.js:20-31`），在 SPA 中会全部取到 `null` |
| `frontend/src/auth.js` | 238 | → `api/auth.ts` + `stores/auth.ts`。注意 `request()` 的 10s 超时与 `AbortController`（`auth.js:100-136`）要保留 |
| `frontend/src/video.js` | 245 | → `api/video.ts`，保留所有校验规则（见 8.3） |
| `frontend/src/community.js` | 169 | → `api/community.ts` |
| `frontend/src/detail-view.js` | 182 | → 组件内纯函数，作为可单测的校验/派生逻辑保留 |
| `frontend/src/discover-view.js` | 153 | 同上；`COVER_PREFIX` 白名单（`discover-view.js:9,85-88`）需统一 |
| `frontend/src/profile-view.js` | 214 | 同上 |
| `frontend/src/player.js` | 59 | → `components/VideoPlayer.vue`。`attachVideoSource` 已正确返回 cleanup（`player.js:55`），是迁移的良好基础 |
| `frontend/src/view-kit.js` | 51 | 描述符渲染器，**被 Vue 模板取代后即可删除**；其“绝不用 innerHTML”的约束必须由“禁用 v-html”承接 |
| `frontend/src/cat.js` | 59 | → `components/BlackCat.vue`。**当前 7 个监听器与 2 个 rAF 永不清理**（`cat.js:31-57`），迁移时必须补 `onUnmounted` 清理 |
| `frontend/src/styles.css` | 537 | 直接复用；设计令牌在 `:root`（`--paper/--ink/--lime/--forest` 等），小黑猫主题色为 `--lime: #d8f28e` + 深绿 `--forest: #1b2b23` |
| `frontend/tests/*.mjs` | 8 个文件 | **逐项迁移，不得删除**。任务书明确“不以删除测试换取通过” |
| `frontend/server.mjs` | 316 | 安全代理逻辑需**逐项**移植到 Vite proxy + Nginx（见 8.4）；旧服务不再承担主入口后才可删除 |

### 8.3 必须原样保留的客户端校验（`api/*.ts` 单测对象）

| 字段 | 规则 | 出处 |
|---|---|---|
| 用户名（注册） | `trim().toLowerCase()` 后匹配 `/^[a-z0-9_]{3,32}$/` | `auth.js:6,19` |
| 密码（注册） | ≥ 8 个码点，且 ≤ 72 **UTF-8 字节** | `auth.js:22-26` |
| 昵称 | `trim()` 后 1–64 码点 | `auth.js:17,27-30` |
| 视频标题 | `trim()` 后 1–100 码点 | `video.js:24,45` |
| 视频简介 | `trim()` 后 ≤ 2000 码点 | `video.js:25,53` |
| 可见性 | 仅 `public` / `private` | `video.js:58` |
| 视频文件 | `video/mp4`，或空 MIME 且文件名 `.mp4`；大小 > 0 且 ≤ 500 MiB | `video.js:26-34` |
| 评论 | `trim()` 后 1–500 码点 | `community.js:14-22` |
| 观看上报 | `progress_ms ≤ duration_ms` | `community.js:140-145` |
| ID 白名单 | `/^\d+$/` 且 ≥ 1 | `community.js:33-38`、`video.js:120-125` |

长度按 `Array.from()` 计（Unicode 码点），密码上限按 `TextEncoder` 字节计——这是刻意设计，迁移时不能改成 `.length`。

### 8.4 T01 最大的技术风险：`server.mjs` 的安全边界不能随 Vite 一起丢掉

`server.mjs` 目前承担的不只是转发，还包括：

- **API 路径+方法白名单**（`fixedApiRoutes` + `methodsForAPIPath`，`server.mjs:26-56`）——不在白名单的路径直接 404；
- **HLS 路径穿越防护**（`server.mjs:52-53`：逐段校验非 `.`/`..` 且仅 `[A-Za-z0-9._-]`）；
- **严格 Origin 校验**（`isLocalOrigin`，`server.mjs:100-113`）——写请求必须同源；
- **CSP 与安全响应头**（`securityHeaders`，`server.mjs:72-89`：CSP、`nosniff`、`DENY`、`same-origin`）；
- **响应体上限 1 MiB、请求体上限 32 KiB、upstream 超时 12s**；
- **重定向只允许跳到既定存储源**（`server.mjs:281-290`）。

Vite 的 `server.proxy` **不具备**以上任何一项（尤其没有 Origin 校验、CSP、路径白名单）。因此 T01 必须明确：开发期由谁承担这些约束，生产期由 Nginx 承担（T10）。这一条已写入 `decisions.md`。

### 8.5 迁移期需一并消化的既有小问题（不扩大范围，但不得带入 Vue）

1. `main.js:1107` 的 `setTimeout(..., 900)` 从不清理；
2. `cat.js` 的监听器与 rAF 从不清理；
3. 封面 URL 白名单不一致：`discover-view.js:85-88` 有前缀校验，`main.js:307-310` 与详情页 `poster`（`main.js:714`）没有；
4. 评论校验前后端不一致：`detail-view.js:49` 只查非空，`community.js:14` 查 1–500；
5. `player.js:4` 的 `import('/vendor/hls.mjs')` 依赖 `server.mjs:24` 的映射，Vite 下必须改为 `import('hls.js')`。

---

## 9. 阻塞项与需确认事项

### 9.1 真实阻塞（不影响 T01，但必须在 T10/T11 前解决）

**B1（高）：集成测试需要建库权限。** 应用账号 `video_share` 无法 `CREATE DATABASE`，集成测试必须用有权限的账号。T10 需要决定并文档化：CI 中使用 root/专用测试账号 DSN，或为测试账号显式授予权限。`REQUIRE_INTEGRATION_TESTS=true` 模式下缺 DSN 应直接失败。

**B2（中）：CRLF 导致 `gofmt -l` 假失败。** 需在 T01/T10 补 `.gitattributes`，否则本地把 `gofmt -l` 接门禁会得到几十个假失败。

**B3（中）：`.env.example` 缺测试与 Redis 变量。** T02/T10 补 `REDIS_*`、`TEST_MYSQL_DSN`、`REQUIRE_INTEGRATION_TESTS`、`TRUSTED_PROXIES` 示例。

### 9.2 需要你确认的两个取舍

**Q1：开发期安全边界由谁承担？** Vite dev server 无法等价复刻 `server.mjs` 的 Origin 校验与 CSP。
- 方案 A：开发期接受 Vite 默认代理（仅本机 `127.0.0.1`），把 Origin/CSRF/CSP 的**完整**实现留到 T10 的 Nginx；T01 只保证不把 `server.mjs` 的安全检查删掉却不替代。
- 方案 B：在 Vite 里写自定义中间件复刻白名单与 Origin 校验，成本更高但与现有测试对齐。

**Q2：`docs/stage5/baseline.md` 与 `decisions.md` 是否提交到 git？** 任务书把两者列为 T00 交付文件，但本仓库此前所有计划/设计文档都提交进了版本库。

### 9.3 无阻塞结论

**T01 可以立即开始。** 前端是全新建设，不与后端迁移冲突；后端现有迁移编号（下一号 000006）与任务书规划完全吻合，无需调整后续任务编号。

---

## 10. 未验证项（诚实清单）

以下项目本次**没有**实际执行，不得当作已验证：

1. **未启动 API / Worker / 前端**，因此没有任何浏览器或 HTTP 端到端验证。
2. **未运行** `backend/scripts/e2e-transcode.ps1` 与 `e2e-community.ps1`（依赖运行中的服务）。
3. **未验证真实转码链路**（FFmpeg 生成 HLS/封面→MinIO）。FFmpeg 二进制存在且版本正确，但链路未跑。
4. **未验证 MinIO 预签名直传**在浏览器中的实际行为。
5. **未运行** `go test -race ./...`（任务书列为 Linux CI 条件项）。
6. **未测量**任何性能指标（任务书要求 T12 才建立基线，此处不预填）。
7. **未验证** `frontend/server.mjs` 的生产行为，仅做静态阅读。
8. **未运行** `docker compose config --quiet`。

---

## 11. T01 收口复核（本轮新增，2026-09-22 夜）

本节记录本轮 T00/T01 收口**实际执行**的复核结果。第 10 节的“未验证项”中，第 1–4 项在本轮已消除（见 11.3），其余仍然成立。

> **本节是「整合前」的历史快照，其中若干判断已被第 12 节取代。**具体为：11.1 的 HEAD 与未提交清单、11.3 第 2 项（`e2e-community.ps1` 无法在本机运行）、11.4 第 1–5 项、11.5 的「`main` 脚手架不完整」。保留原文以便对照当时的判断与证据，**需要引用当前结论时请以第 12 节为准**。

### 11.1 两个工作树与 T01 提交

| 项目 | 实际值 |
|---|---|
| `main` HEAD | `cff21bd`，领先 `origin/main` **19 个提交**（未推送） |
| `main` 未提交改动 | 5 个已跟踪文件 + 16 个未跟踪文件（见第 1 节修正表），本轮**逐字未改** |
| `.worktrees/stage5-upgrade` HEAD | `8452e3a`，分支 `feature/stage5-upgrade` |
| 三个 T01 提交 | `372469e` feat(frontend): migrate application to Vue 3；`7e6430a` fix(frontend): preserve owner and migration flows；`8452e3a` fix(frontend): close dev boundary bypasses |
| 共同基线 | 均为 `cff21bd` |

本轮在 `feature/stage5-upgrade` 内留下**3 个未提交的修复文件**：

```
frontend/vite.config.ts
frontend/tests/unit/vite-boundary.test.ts
frontend/tests/e2e/routes.spec.ts
```

未执行任何 merge、cherry-pick、rebase、commit、push 或 worktree 删除。两个工作树的差异与无损整合方案尚未提交用户授权，因此 `main` 与 `feature/stage5-upgrade` **仍是分离的**。

### 11.2 前端门禁（本轮实际输出）

执行目录：`frontend/`

| 命令 | 结果 |
|---|---|
| `npm ci` | 通过 |
| `npm run typecheck` | 通过（`vue-tsc --noEmit -p tsconfig.json`，无输出） |
| `npm test` | Vitest **11 个文件 / 73 个用例全部通过**；legacy `node --test` **101 通过 / 0 失败 / 0 跳过** |
| `npm run build` | 通过（3.79s） |
| `npm run test:e2e` | **5 通过**（3.2s） |

legacy 用例数由基线的 100 增至 101：本轮按“先补失败测试再做最小修复”为开发期安全边界新增了用例，未删除任何既有测试。

**Playwright 环境注意（重要）：** 本机设有 `HTTP_PROXY/HTTPS_PROXY=http://127.0.0.1:6984`。Playwright 的 `127.0.0.1` 请求会被送到该代理并失败，表现为 `webServer` 超时或全部用例报连接错误。运行前需：

```bash
unset HTTP_PROXY HTTPS_PROXY http_proxy https_proxy ALL_PROXY all_proxy
export NO_PROXY="127.0.0.1,localhost,::1"
```

这是**本机环境配置**，不是代码缺陷，也不影响 CI（CI 无该代理）。

### 11.3 隔离环境真实链路（本节消除第 10 节第 1–4 项）

为避免动到业务库与业务桶，本轮另起一套隔离资源：

| 资源 | 隔离值 | 业务值（未触碰） |
|---|---|---|
| MySQL 库 | `video_share_stage5_verify`（迁移 5/5） | `video_share` |
| MinIO 桶 | `video-share-stage5-verify`（由 `EnsureBucket` 自动创建） | 业务桶 |
| Kafka topic | `video.transcode.requested.stage5verify` | `video.transcode.requested` |
| 消费组 | `video-transcoder-stage5-verify` | `video-transcoder-v1` |
| API / Vite | `127.0.0.1:8081` / `127.0.0.1:5173`（`API_TARGET` 指向 8081） | — |
| 测试媒体 | 2s 320x240 FFmpeg `testsrc`，30387 字节，本地生成 | — |

**未清空、未重建业务库**；业务库、业务桶与业务消费组始终未参与本轮验证。

实际跑通的链路（结果均为实跑）：

1. `backend/scripts/e2e-transcode.ps1` → **exit 0**：注册 → 登录 → 创建 → 预签名 `PUT` 直传 → `complete` → Outbox → Kafka → Worker FFmpeg HLS → `ready 100%`，并校验 master/variant/segment 与封面。
2. `backend/scripts/e2e-community.ps1` **无法在本机运行**（见 11.4 环境限制），等价断言改由一次性 Node 脚本覆盖：**56/56 通过**，含 LIKE 通配符转义（`%`、`_` 不字面匹配）、点赞/收藏幂等、空评论 400、观看去重 `view_count`、自我关注 400、`viewer_state`、私密视频对匿名 404（详情/清单/封面）且搜索不可见、非作者编辑 403、取消互动计数归零、删除后 404。
3. Chromium 驱动真实 Vue 应用：**27/27 通过**，含注册/登录、未选文件与非 MP4 文件的上传失败提示、真实上传→转码→落到详情路由、hls.js 加载清单并播放（`duration=2s`、`currentTime` 前进）、点赞/收藏/评论、作者编辑、Tab+Enter 键盘激活、浏览器前进后退、快速切视频后状态收敛且无残留错误、375px 无横向溢出、`prefers-reduced-motion` 仍可渲染、刷新后端到端仍可匿名播放、重新登录后删除投稿并确认 404。

转码实测：Worker 单次转码约 **620ms**，消费组 `LAG=0`，全程未出现消费失败。

### 11.4 本轮仍未验证 / 环境限制（诚实清单）

1. **`e2e-community.ps1` 未能在本机运行。** 该脚本第 209 行 `Assert-Equal $found.Count 1` 依赖 **PowerShell 7** 的标量 `.Count` 语义；本机只有 PowerShell 5.1 且无 `pwsh`。已逐项证实是脚本的环境依赖，不是产品缺陷（后端搜索本身返回正确结果）。第 11.3 节的 Node 脚本覆盖了它的全部断言，但**没有**等价地验证该 `.ps1` 自身。
2. **Worker 快速失败未修复（超出 T01 前端范围）。** `backend/cmd/worker/main.go` 中任何 `service.Process` 错误或任何 `consumer.Poll` 错误都会让整个 Worker 退出（`os.Exit(1)`）。本轮独立复现两次：一次是消费到库中不存在的任务，一次是 Kafka 组加入 `i/o timeout`。建议单独排期，不在 T01 内夹带。
3. **Node 版本锁定只在 `feature/stage5-upgrade` 完成，`main` 未同步**（第 2 节版本风险）。该分支 `frontend/package.json` 的 `engines` 已是 `22.22.3` 并新增 `frontend/.nvmrc`；`main` 仍是 `>=22.12.0 <23` 且无 `.nvmrc`。两树差异见 11.5；整合前**不要宣称“全仓已锁定”**。
4. **`.gitattributes` 与 `.env.example` 补齐仍未做**（第 9.1 节 B2/B3）。
5. **构建存在 chunk 体积警告：** 懒加载的 `hls.js` chunk 为 592.63 kB（gzip 185.44 kB），主包 `index-*.js` 为 123.69 kB（gzip 47.50 kB）。主包在预算内，播放器 chunk 按需加载，暂不阻塞，但 T12 若要收紧预算需重新评估。
6. **未运行 `go test -race ./...`**（任务书列为 Linux CI 条件项，本机 WSL 未启用）。
7. **未做 T02。** Redis 限流/缓存/榜单一行未动。

### 11.5 两树文件差异与 `main` 脚手架的真实状态（本轮新增，重要）

第 1 节把 `main` 的 16 个未跟踪文件记为“既有在途工作”。本轮进一步核对后，必须如实补充：**`main` 上的这套 Vue 脚手架是不完整的，当前无法通过类型检查，也无法构建或启动。**

**缺失文件（`main` 不存在，只在 `feature/stage5-upgrade`）**

```
frontend/src/views/              ← 5 个视图组件全部缺失
frontend/src/components/         ← 组件目录整体缺失
frontend/.nvmrc
frontend/tests/unit/**           ← 11 个 Vitest 文件
frontend/tests/e2e/routes.spec.ts
```

**在 `main/frontend` 实跑的证据（本轮执行，非推测）**

| 命令 | 实际输出 |
|---|---|
| `npm run typecheck` | **失败**：`src/router/index.ts` 第 34/35/39/45/51 行共 **5 个 `TS2307: Cannot find module '../views/*.vue'`**（DiscoverView / LoginView / VideoDetailView / UploadView / ProfileView） |
| `npm run test:unit` | `No test files found, exiting with code 1`（`include: tests/unit/**/*.test.ts` 无匹配文件） |

因为 `router/index.ts` 静态导入了 5 个不存在的视图，`npm run build` 与 `npm run dev` 同样会失败。**因此 11.2 的门禁数字是在 `feature/stage5-upgrade` 上取得的，不能套用到 `main`。**

**文档与说明已同步到两棵树并校验逐字节一致：** `docs/stage5/baseline.md`、`docs/stage5/decisions.md`、`README.md`、`frontend/README.md`（`diff -q` 全部相等）。

**无损整合方案（仅建议，未执行，等用户授权）：** 两树共同基线为 `cff21bd`，`main` 侧 5 个已跟踪文件有未提交修改、16 个未跟踪文件；`feature/stage5-upgrade` 侧是完备实现。建议按“`main` 本地提交（或 stash）→ 合并 `feature/stage5-upgrade` → 解决 5 个已跟踪文件的冲突”推进，冲突集中判定：`frontend/tests/server.test.mjs` 以分支版为准（含新增边界用例），`frontend/index.html` / `package.json` / `package-lock.json` / `server.mjs` 需逐文件比对取并集。**未经授权不执行任何一步。**

本轮仍未执行任何 `commit` / `merge` / `cherry-pick` / `rebase` / `push` / worktree 删除。

---

## 12. 仓库整合与四项修复（2026-09-22 夜，整合后复核）

用户在确认第 11.5 节的差异与无损整合方案后授权「完整整合」，并明确本轮除整合外还要修：Worker 快速失败、`e2e-community.ps1` 兼容、工程配置补齐、`hls.js` chunk 警告。本节记录整合后的**实际**状态，并取代第 11 节中已过时的判断。

### 12.1 整合实际执行

| 项目 | 实际值 |
|---|---|
| `main` 在途脚手架快照提交 | `48ef2a8` chore(frontend): snapshot the in-flight Vue scaffold before integration |
| 合并提交 | `e4d4b89` Merge branch 'feature/stage5-upgrade' into main |
| 合并后补充提交 | `7030fd0` chore: declare LF line endings and document test env vars |
| `main` HEAD | `7030fd0`，领先 `origin/main` **27 个提交**（**未推送**） |
| 冲突处理 | 5 个已跟踪文件按第 11.5 节方案逐文件比对解决（`tests/server.test.mjs` 取并集，`index.html`/`package.json`/`package-lock.json`/`server.mjs` 取并集） |
| worktree | `.worktrees/stage5-upgrade` **保留未删**；未 `push`、未 `rebase`、未 `cherry-pick` |

**11.5 节「`main` 脚手架不完整、无法类型检查与构建」已不成立。**整合后 `main/frontend` 实有 **5 个视图组件、5 个组件、11 个 Vitest 文件、1 个 Playwright spec**，`npm run typecheck` 与 `npm run build` 均通过（见 12.3）。

### 12.2 四项修复（均已实测）

**1) Worker 快速失败** —— `backend/cmd/worker/main.go` 改动，新增 `consume.go`（可测试的消费循环）与 `consume_test.go`。

- 行为变更：`Poll` 的**瞬时错误不再让 Worker 退出**，改为可中断退避后重试；永久性「毒丸」事件在**不提交 offset** 的前提下跳过（保持至少一次语义）；`Commit` 失败仍视为致命（本轮刻意不改，见 12.5）。
- 实测：新二进制在隔离栈上消费真实 Outbox→Kafka→FFmpeg 链路成功转码（视频 23、24，单次约 630–656 ms），并记录其生效 topic 为 `video.transcode.requested.stage5verify`。改动前的旧二进制曾在 `poll Kafka: unable to join group session: i/o timeout` 后直接退出——这是**原地复现**了要修的缺陷。`go test ./...` 中 `video_share/cmd/worker` 通过。

**2) `e2e-community.ps1` 兼容 PowerShell 5.1**

- 修复三处：`Find-Video` 改为 `return ,@(...)`；`Invoke-Api` 的 catch 改读 `$_.ErrorDetails.Message`；请求 `ContentType` 声明 `charset=utf-8`。
- **第 11.4 节第 1 项的诊断有误，此处更正**：真正的依赖站点是 `e2e-community.ps1` 的 **197 / 200 / 201 / 269 / 272 / 299 行**（不是「第 209 行」，也与 185/260 无关）。机制是：无前置逗号时返回值经管道展开，单元素 `@()` 退化成标量，而 **PS 5.1 的标量没有 `.Count`**（取值为 `$null`），断言因此误判——**这与 PS 7 无关**，PS 7 只是恰好也有该属性。
- 实测：**本机 PowerShell 5.1 实跑 exit 0**（video_id=23，author_id=39，viewer_id=40）。11.3 第 2 项与 11.4 第 1 项据此作废。

**3) 工程配置补齐**

- `frontend/.nvmrc`（`22.22.3`，与 `package.json` 的 `engines.node` 一致）已在 `main`；`.gitattributes`（`* text=auto eol=lf`，`*.ps1`/`*.cmd`/`*.bat` 为 CRLF，二进制标记）与 `backend/.env.example` 已补齐。11.4 第 3、4 项作废。
- `frontend/README.md` 中“`main` 尚未同步 `.nvmrc`”的表述已同步更正。

**4) `hls.js` chunk 体积**

- 打包入口由 `hls.js` 改为 `hls.js/light`（`frontend/src/lib/player.ts` 唯一的 bundler 导入点），并新增 `frontend/src/lib/hls-light.d.ts` 提供该子路径的类型声明（hls.js 只发布 `hls.light.mjs`，没有配套 `.d.ts`）。
- 实测：构建产物由整合前的 `assets/hls-*.js` **592.63 kB / gzip 185.44 kB** 变为 `assets/hls.light-Cm5ppQ94.js` **370.93 kB / gzip 117.84 kB**（**−221.70 kB，−37.4%**），**chunk 体积警告消失**。这是真实缩减，**不是把警告关掉**。
- 浏览器实测（main 树，5174）：hls.js 以 MSE 挂载（`video.src` 为 `blob:`），`readyState=4`，`640x360`，`duration=2.103445s`；`play()` 后播放至 `currentTime=2.103445`、`ended=true`，无 console 错误。
- **代价（须长期记录）**：light 构建不含字幕、备用音轨、EME 与 CMCD。若将来要支持字幕等特性，需在 `player.ts` 那一处改回 `hls.js`。11.4 第 5 项作废。

### 12.3 整合后门禁（本轮实跑输出）

前端（执行目录 `frontend/`）：

| 命令 | 实际结果 |
|---|---|
| `npm run typecheck` | 通过（无输出） |
| `npm test` | Vitest **12 文件 / 76 用例全部通过**（整合时为 11/73，见 12.5 第 1 项新增的护栏测试）；legacy `node --test` **101 通过 / 0 失败 / 0 跳过** |
| `npm run build` | 通过（2.88s）；主包 `index-BPn7KPy9.js` 123.69 kB / gzip 47.49 kB，播放器 chunk `hls.light-Cm5ppQ94.js` 370.93 kB；**无 chunk 警告** |
| `E2E_PORT=5273 npm run test:e2e` | **通过：5 passed**（首跑 6.7s，2026-09-23 复跑 6.5s）。日志出现 `[WebServer] ... vite`，说明 Playwright 自起本目录 dev server，未复用 5173 上另一工作树的服务 → 结果归属本树。详见 12.5 第 1 项 |

后端（执行目录 `backend/`）：

| 命令 | 实际结果 |
|---|---|
| `gofmt -l .` | 无输出 |
| `go vet ./...` / `go build ./...` | 通过（无输出） |
| `go test ./...` | 全部 `ok`（含 `video_share/cmd/worker`、`internal/storage`）；`cmd/worker` 消费循环用例 **8/8 通过**（见第 13 节） |
| `go test -race ./...` | **exit 0，全部 `ok`，零竞态报告**（Go 1.26.3 windows/amd64，`CGO_ENABLED=1`，MinGW-W64 gcc 15.2.0）。见 12.5 第 4 项 |

隔离栈真实链路（本节消除第 10 节第 1–4 项；隔离资源同 11.3，业务库/桶/消费组全程未参与）：

| 脚本 | 实际结果 |
|---|---|
| `backend/scripts/e2e-transcode.ps1`（PowerShell 5.1） | **exit 0，连续两次**（video_id=38、39，640x360，duration_ms=2000）；覆盖 注册→登录→创建→预签名 PUT 直传→complete→Outbox→Kafka→Worker FFmpeg HLS→`ready`→master/variant/segment/封面 校验 |
| `backend/scripts/e2e-community.ps1`（PowerShell 5.1） | **exit 0**（video_id=23，author_id=39，viewer_id=40） |

### 12.4 本轮新发现的两个 PowerShell 5.1 陷阱（重要）

**陷阱 A：无 BOM 的 UTF-8 `.ps1` 会被 PS 5.1 按系统 ANSI(936/GBK) 解码，DBCS 解码吞掉行尾换行，代码行被并进注释而静默不执行。**

- 现场：`e2e-transcode.ps1` 在 `HEAD` 上是**纯 ASCII 且无 BOM**（因此一直可跑）。我为其 `-UseBasicParsing` 改动补的**中文注释**触发了该缺陷：`Invoke-WebRequest -Method Put` 整行被并进上一条注释，**PUT 从未执行**，MinIO 中因此没有对象，`complete` 稳定返回 `409 UPLOAD_INCOMPLETE`。
- 侦破线索是**行号偏移**：脚本把 `complete` 的报错定位在「第 73 行」，而 `grep -n` 显示它在**第 75 行**，恰好偏移 2 行 = 被吞掉的 2 个换行。
- 直接证据：让 PS 5.1 以 codepage 936 解码该文件，得到 `ansiLineCount=111`，而文件实际 **112** 行；且解码结果中 `Invoke-WebRequest -Method Put` **不再作为独立行存在**。对照 `e2e-community.ps1`：它有 UTF-8 BOM（`EF BB BF`），解码后**不丢行**，因此一直正常。
- 修复：给 `e2e-transcode.ps1` 补 UTF-8 BOM（`EF BB BF`，CRLF 保持不变），与既有可用的 `e2e-community.ps1` 完全一致。
- **教训：本仓库的 `.ps1` 必须带 UTF-8 BOM**，否则任何非 ASCII 内容都可能静默吃掉相邻代码行。

**陷阱 B：PS 5.1 的 `Invoke-WebRequest` 默认走 IE(mshtml) 响应解析，本机 IE 未初始化时抛 `NullReferenceException`。**

- 绕开方式：`-UseBasicParsing`；若只需取文本（如 HLS playlist），改用 `Invoke-RestMethod`——它内部不走 IE 解析且直接返回字符串。
- 本轮把该修复扩展到 `e2e-transcode.ps1` 的第 7 步（master/variant 用 `Invoke-RestMethod`，segment/封面用 `-UseBasicParsing`），此前因「无证据」暂缓的 4 处现已全部有实证。
- 注意：该异常抛在**请求已经发出之后**——实测中即使抛出 NRE，对象仍已成功落桶。

### 12.5 本轮仍未验证 / 未做（诚实清单）

1. **【已消除，2026-09-23】`main` 的 Playwright e2e 已实跑通过。** 原先 `playwright.config.ts` 的 `baseURL`/`webServer.url` 与 `tests/e2e/routes.spec.ts` 第 49、54 行把 `127.0.0.1:5173` 硬编码，而 5173 被 `.worktrees/stage5-upgrade/frontend` 的开发服务器（PID 53720）占用，`reuseExistingServer: !CI` 会**安静地复用**它——直接跑只会测到另一个工作树。最小修复：`playwright.config.ts` 改为读 `E2E_PORT`（默认 5173）并把 `PORT` 传给 `webServer.env`，使 dev server 与 `baseURL` 落在同一端口；`routes.spec.ts` 改用 Playwright 的 `baseURL` fixture，不再出现第二处硬编码。以 `E2E_PORT=5273` 实跑（该端口原本无人监听）：**5 passed (6.7s)**，日志含 `[WebServer] ... vite`，证明服务器由 Playwright 自起、结果归属本树。`npm run typecheck` 与 `npm test`（Vitest 73 + legacy 101）同时复跑通过。
   **归属证据（排除误测他树）**：另起 `vite --port 5273`（main 目录）做判别性探测——`/src/lib/hls-light.d.ts` 是**只存在于 main** 的新增文件：`5273` 返回 **200**，而 `5173`（工作树服务）返回 **404**；同时 `5273` 上 `/src/main.js`（旧实现，应被黑名单屏蔽）返回 **404**。这证明 5273 上跑的确为本树内容，前述 5 通过成立。
   **护栏测试（防复发）**：新增 `frontend/tests/unit/e2e-server-address.test.ts`（3 用例）。前两个用例以动态 `import` + `E2E_PORT` 实参断言配置真的把端口同时用于 `use.baseURL`、`webServer.url` 与 `webServer.env.PORT`；第三个用例扫描 `tests/e2e/*.spec.ts` 的源文本，禁止出现 `127.0.0.1:<端口>` 字面量。**已验证护栏有牙齿**：对 `HEAD` 版本取同一判据——`routes.spec.ts` 与 `playwright.config.ts` 各有 **2 处**硬编码 origin、`E2E_PORT` 引用 **0 处**（⇒ 会失败）；当前工作副本两者均为 **0 处**硬编码、配置有 **2 处** `E2E_PORT` 引用（⇒ 通过）。
   **残留限制（如实记录）**：不传 `E2E_PORT` 时仍走 5173，`reuseExistingServer` 仍可能复用占用者。在本机请显式指定 `E2E_PORT`；未把默认行为改成“端口被占即报错”，因为那会破坏 README 记录的常规流程。
2. **PS 7 兼容性只做了推理，未实跑。** 本机无 `pwsh`。上述三处修复（前置逗号数组、`-UseBasicParsing`/`Invoke-RestMethod`、BOM）在 PS 7 语义下均应成立，但**没有可执行证据**。
3. **`consume` 中 `Commit` 失败仍视为致命**（本轮刻意保留，超出授权范围）。若 Kafka 在提交阶段持续失败，Worker 仍会退出。
4. **【已消除，2026-09-23】`go test -race ./...` 已实跑通过。** 本机 Go 1.26.3 windows/amd64、`CGO_ENABLED=1`、MinGW-W64 gcc 15.2.0，竞态检测器可用，无需 WSL。结果：**exit 0，全部包 `ok`，零竞态报告**（`internal/user` 18.2s 最长）。**范围说明**：未加 `-tags integration`、未设 `TEST_MYSQL_DSN`/`TEST_KAFKA_BROKERS`，因此 MySQL/Kafka 集成用例按 `internal/testutil/testdb.go:26-28` 跳过——本次只覆盖单元与进程内测试，**不含**真实数据库/消息队列的竞态检测，业务库全程未被访问。
5. **未做 T02，也未开始 T03–T12。** Redis 限流/缓存/榜单一行未动。

### 12.6 本轮未提交内容

`main` 工作树当前有 **7 个已跟踪文件被修改**：`backend/cmd/worker/main.go`、`backend/scripts/e2e-community.ps1`、`backend/scripts/e2e-transcode.ps1`、`frontend/README.md`、`frontend/src/lib/player.ts`、`frontend/playwright.config.ts`、`frontend/tests/e2e/routes.spec.ts`；以及 **6 项未跟踪**：`backend/cmd/worker/consume.go`、`backend/cmd/worker/consume_test.go`、`frontend/src/lib/hls-light.d.ts`、`frontend/tests/unit/e2e-server-address.test.ts`、`docs/stage5/`、`docs/superpowers/plans/2026-09-22-stage5-feature-expansion.md`。

以上**尚未 `commit`、未 `push`**，等待授权。

## 13. T01 收口：Kafka 提交位点语义修复（2026-09-23）

第 12.2 节第 1 项引入的消费循环存在一个**真实的丢消息缺陷**，本轮按「先补失败测试，再最小修复」补齐。

### 13.1 缺陷

`consume.go` 的注释原本承诺「不提交偏移，交由 Kafka 重投」，但代码在**同分区后续记录成功时照常提交**。franz-go 的提交是**分区级「下一条待消费」**语义（`internal/messaging/kafka.go:82-87` 调用 `CommitRecords`，即提交 `record.Offset+1`）：

| 记录 | 处理结果 | 提交 offset | 分区位点 | 后果 |
|---|---|---|---|---|
| offset=n | **失败**（不提交） | — | 保持 | 本应重投 |
| offset=n+1 | 成功 | n+1 | 推进到 **n+2** | **offset n 永不再投** |

即 kafka 位点被推过失败点，那条转码任务被**静默丢弃**，与循环自身的承诺相矛盾。`kafka.go:54` 的 `kgo.DisableAutoCommit()` 排除了自动提交绕过修复的可能。

### 13.2 修复（`backend/cmd/worker/consume.go`）

新增 `commitTracker`（`failed map[int32]map[int64]struct{}`，记录每分区**仍未处理成功**的失败偏移集合）。**两处提交点**（成功记录、以及乱码丢弃记录）提交前都查 `blocks(partition, offset)`——只要存在失败偏移 `f <= offset` 就不提交（提交 `o` 会把位点设成 `o+1`，`f <= o` 从此不再重投）。**分区之间互不影响。**

### 13.3 解除阻塞（同日追加）

**第一版把失败记成「每分区最早偏移」，一旦失败该分区位点永不前进——一条瞬时失败即可把分区永久封禁。** 这是一个真实的运维缺陷（用无限重复换不丢消息），按同一指令补测并修复：

- `markRecovered(partition, offset)`：失败的消息**重投并处理成功后**从集合里删除，解除它造成的阻塞；集合空则清掉该分区条目。
- 成功路径的顺序为 **先 `markRecovered`、再 `blocks`、后 `Commit`**——否则重投成功的那一条自己仍被自己的失败记录挡住，永远提交不了。
- 解除是**按偏移**的：重投成功只清掉那一条，更靠后的失败仍拦着它后面的提交，直到它自己也重投成功。

### 13.4 回归测试（`backend/cmd/worker/consume_test.go`，共 8 例）

新增断言助手 `committedPosition(partition)`（= 已提交最大偏移 + 1，即 Kafka 语义下「下一条待消费」）与 `transcodeRecordAt`；`fakeProcessor` 增加 `failTimes`（前 N 次失败、之后成功）以模拟重投痊愈，原有 `failFor`（永久失败）保留用于毒丸。

**不允许越过失败 offset（4 例，全部保留）**
- `TestConsumeDoesNotCommitPastFailedOffsetInSamePartition`（p0：5 失败 / 6 成功 → `committedPosition(0) ≤ 5`）
- `TestConsumeDoesNotCommitPastFailedOffsetWhenLaterRecordIsUnparsable`（5 失败 / 6 乱码 → 同样不得推进）
- `TestConsumeStillCommitsUnaffectedPartition`（p0 失败、p1 成功 → `committedPosition(1) == 10`）
- `TestConsumeSkipsPoisonRecordAndContinues`（毒丸永不痊愈 → 永久阻塞；原断言**恰好断言了** `n+2` 推进这一缺陷，已改为 `committedPosition(0) ≤ 1`）

**恢复场景（2 例，本轮新增）**
- `TestConsumeCommitsFailedOffsetAfterSuccessfulRetry`：批次 `[5 失败, 6 成功]` → `[5 成功]` → `[6 成功]`，最终 `committedPosition(0) == 7`，处理序列 `[5 6 5 6]`。即 **offset=5 可提交、分区不再永久阻塞，且 offset=6 最终也能继续提交**。
- `TestConsumeClearsOnlyTheRetriedOffset`：批次 `[5 失败, 6 失败]` → `[5 成功, 7 成功]` → `[6 成功, 7 成功]`，提交偏移序列 `[5 6 7]`、最终位点 8。即**只解除被重投的那一条**，7 在 6 痊愈前始终被拦下。

**RED 均已复现**：修复前分别报 `partition 0 commit position is 0, want 7 (a successful retry must unblock the partition)` 与 `committed offsets [], want [5 6 7] in that order`；修复后 **8/8 通过**，`gofmt -l ./cmd/worker/` 无输出。

### 13.5 仍保留的代价（必须记录）

**持续失败**的「毒丸」（例如库中已无对应任务、或重投后仍失败）会**一直阻塞该分区**，每次重启都从失败点重投，即用重复处理换取不丢消息——这是至少一次语义的必然取舍。要限制重复需引入显式重试次数或死信队列，**超出本轮授权范围**（对照 12.5 第 3 项：`Commit` 失败仍视为致命，同样刻意保留）。瞬时失败不再落入此列（13.3 已解除）。

### 13.6 本轮门禁（2026-09-23 实跑）

后端（`backend/`）：`go test ./... -count=1` **EXIT=0**（全包 `ok`）；`go vet ./...` **EXIT=0**；`go build ./cmd/api ./cmd/worker` **EXIT=0**；`go test -race ./...` **EXIT=0**（零竞态）；另跑 `go test -race -count=1 ./cmd/worker/ ./internal/outbox/ ./internal/transcode/`（强制不走缓存）**EXIT=0**。

前端（`frontend/`）：`npm run typecheck` **EXIT=0**；`npm test` **通过**（Vitest 12 文件 / **76 用例**、legacy **101 通过 / 0 失败**）；`npm run build` **EXIT=0**（2.50s，`hls.light-Cm5ppQ94.js` 370.93 kB / gzip 117.84 kB，无 chunk 警告）；`E2E_PORT=5273 npm run test:e2e` **5 passed (6.1s)**。

**未做**：`git commit`、`git push`（按要求）。当时尚未开始 T02；后续进度见第 14、15 节。

## 14. T02 Redis 基础设施与分区缓存边界（2026-09-23）

### 14.1 迁移边界

本轮明确采用“可注入数据源缓存组件”方案：**T02 不创建 `categories` 表，也不新增分类迁移或分类路由**。任务书中的 `000006_sessions_profiles` 保留给 T03；`categories`、`tags`、`video_tags` 必须等到 T05 的 `000007_taxonomy` 再创建。T02 只提供 `video_share:cache:categories:v1` 的 JSON 缓存接缝，T05 再把真实 taxonomy repository 接入。

### 14.2 实际改动

- `backend/internal/cache/redis.go`：go-redis 客户端封装、连接/读写/命令超时、Ping、Lua Eval、有限 TTL 写入和关闭。
- `backend/internal/cache/json.go`：通用 JSON 缓存、300 秒基础 TTL + 抖动、singleflight 热点合并、有限回源并发、回源降级、缓存失效和进程内统计。
- `backend/internal/middleware/redis_ratelimit.go`：Lua 原子固定窗口限流、`Retry-After`、Redis 故障拒绝/进程内降级两种策略及诊断计数。
- `backend/internal/server/router.go`：登录/注册使用 Redis 严格拒绝策略；评论使用 Redis 并在故障时退化为有界进程内限流。
- `backend/internal/config/config.go`、`backend/.env.example`：Redis 连接和四类限流默认值与校验。
- `backend/deploy/docker-compose.yml`：固定 `redis:7.4.2-alpine`、健康检查、持久卷和 API 内部地址覆盖。
- `backend/internal/cache/*_test.go`、`backend/internal/middleware/redis_ratelimit*_test.go`：单元、故障、并发、真实 Redis 和跨客户端共享配额测试。

### 14.3 实际验证

- `go test ./... -count=1`：通过。
- `go test -race ./...`：通过。
- `go vet ./...`：通过。
- `go build ./cmd/api ./cmd/worker`：通过。
- `docker compose --env-file .env -f deploy/docker-compose.yml config --quiet`：通过。
- Redis 容器 `redis:7.4.2-alpine`：健康检查通过。
- `TEST_REDIS_ADDR=127.0.0.1:6379 TEST_REDIS_DB=15 REQUIRE_INTEGRATION_TESTS=true go test -race -tags integration ./internal/cache ./internal/middleware -count=1`：通过；覆盖真实 Redis 缓存读写/失效和两个独立客户端共享限流配额。集成辅助在 required 模式下对缺少地址/DB 或连接失败 fail-closed，缓存测试使用唯一测试键。

### 14.4 仍未完成

- T02 尚未提交 Git，未推送远端。
- 未创建 taxonomy 表、分类接口或标签业务；这些属于 T05。
- 尚未对 T02 做正式压测，因此不宣称 QPS、命中率或数据库查询下降；性能对比留到 T12。
- 真实 MySQL/Kafka 集成竞态仍受 `TEST_MYSQL_DSN`/`TEST_KAFKA_BROKERS` 门控，未因 Redis 验证而自动变为已验证。
- 本轮未改变 MySQL/Kafka 集成测试的跳过策略；`REQUIRE_INTEGRATION_TESTS` 的 fail-closed 辅助已用于 Redis T02 用例，T10 仍需统一覆盖全部外部依赖。

## 15. T03 持久会话与账号设置（2026-09-23）

### 15.1 实际改动

- `backend/migrations/000006_sessions_profiles.sql`：为用户增加 `bio`/`role`，创建 session family 与 refresh token 表；refresh token 仅保存 SHA-256，family 具有绝对到期和撤销时间。
- `backend/internal/session/`：实现加密随机 refresh token、JWT `sid`、事务轮换、窗口内冲突、窗口外重放撤销整族、退出撤销和会话校验；并提供 Cookie/CSRF/资料/改密处理器。
- `backend/internal/middleware/auth.go`、`csrf.go`：受保护路由强制 sid 和活动 family，Cookie 驱动写操作精确 Origin + 双提交 CSRF + JSON Content-Type。
- `backend/internal/user/`、`token/`、`config/`、`server/router.go`：账号资料、改密事务撤销全部 family、15 分钟访问令牌及会话配置和新路由。
- `frontend/src/api/auth.ts`、`client.ts`、`stores/auth.ts`、`main.ts`、`views/SettingsView.vue`：访问令牌只在内存，HttpOnly refresh Cookie 恢复；single-flight、Web Locks、冲突重试上限、资料编辑和改密界面。

### 15.2 测试与实际结果

- `go test ./... -count=1`：通过。
- `go test -tags integration ./internal/session ./internal/database ./internal/server -count=1`：通过；使用 root 仅为创建隔离测试库，测试结束自动删除，未连接业务库。覆盖两并发刷新一成功一冲突、窗口外旧 token 重放撤销整族、迁移回滚/部分 ALTER 恢复、sid 受保护路由和请求体上限。
- `go test -race -tags integration ./... -count=1`：此前 T03 单独收口时通过；本轮加入 T04 后并行全包重跑在本机 MySQL 11 分钟超时，详见 §16.3，不能把本轮全包写成通过。
- `go test -race ./... -count=1`、`go vet ./...`、`go build ./cmd/api ./cmd/worker`：均通过；无竞态报告。
- `npm run typecheck`：通过。
- `npx vitest run`：14 个文件、82 个用例通过。
- `E2E_PORT=5275 npm run test:e2e`：6 passed；登录 E2E 已覆盖 CSRF/refresh 启动链路 mock，并新增公开创作者深链。
- T03 退出竞态补测：前端 4 个会话用例通过；真实 MySQL session/server 集成（race）通过，覆盖有效 access 无 refresh cookie、过期 access 按 refresh cookie 退出和旧 JWT 立即失效。
- `npm run build`：通过；Settings 懒加载 chunk 及 hls.light chunk 生成，无构建错误。

### 15.3 未验证与限制

- 尚未把 T02/T03 工作区提交或推送；不执行远端操作。
- MySQL 集成测试使用本机 root 权限创建临时库；默认 `video_share` 业务用户没有 `CREATE DATABASE` 权限，部署环境需按任务书 §7.3 单独授予测试库权限或使用专用测试账号。
- 未做真实浏览器连接 Go API 的全栈业务演示（当前 Playwright 仍使用路由 mock）；性能数字留到 T12，不提前声称。

## 16. T04 公开创作者主页与关系展示（2026-09-23）

### 16.1 实际改动

- `backend/internal/creator/`：新增公开资料投影、关系计数、公开视频分页和禁用/不存在统一 404；不返回密码、状态、角色或会话字段。
- `backend/internal/video/repository.go`：新增 `ListPublicByUser`，分页前应用 ready/public/normal-author 过滤。
- `backend/internal/follow/`：扩展粉丝与关注公开分页查询，双方均过滤禁用账号。
- `backend/internal/server/router.go`：注册 `/users/:id`、`/users/:id/videos`、`/users/:id/followers`、`/users/:id/follows`，主页使用可选鉴权计算关注状态。
- `frontend/src/api/creator.ts`、`views/CreatorView.vue`、`components/CreatorCard.vue`：增加公开主页、分页关系列表、关注/取关与作者链接；Vite/旧代理加入动态公开主页白名单。

### 16.2 测试与实际结果

- `go test -race ./... -count=1`、`go vet ./...`、`go build ./cmd/api ./cmd/worker`：通过。
- `go test -race -tags integration ./internal/creator ./internal/follow ./internal/video -count=1`：通过；真实 MySQL 覆盖公开/私密/处理中作品过滤、禁用作者、正常关系计数及粉丝/关注分页。
- `go test -race -tags integration ./internal/server ./internal/session -count=1`：通过；覆盖 logout 两种身份来源及旧 JWT 失效。
- `npm test`：14 个 Vitest 文件 82 个用例，加 93 个迁移期 node:test 用例，全部通过。
- `npm run typecheck`、`npm run build`：通过。
- `E2E_PORT=5275 npm run test:e2e`：6 passed，新增公开创作者深链。

### 16.3 未验证与限制

- `go test -race -tags integration ./... -count=1` 本轮全包并行重跑在本机 MySQL 11 分钟超时，database/engagement/follow/user/video 包被测试进程杀掉；已拆分执行并通过 T03/T04 相关包，不能把这次全包结果写成通过。
- T04 尚未提交或推送；没有公开资料缓存，也未提前声称性能提升。

## 17. T05 分区、标签、关注流和相关推荐（2026-09-23）

### 17.1 实际改动

- `backend/migrations/000007_taxonomy.sql`：新增 `categories`、`tags`、`video_tags`，为旧视频增加 `category_id` 并预置未分类、生活、知识、科技、游戏、音乐；历史投稿默认归入未分类。
- `backend/internal/taxonomy/`：分类/标签模型、Unicode NFC + 小写标准化、1–20 字符与最多 5 个标签限制、事务去重/替换、分类 JSON 缓存接入和回源降级。
- `backend/internal/video/`：投稿/编辑兼容 `category_id`、`tags`；发现支持分类与多标签筛选；新增关注流与最多 6 条规则相关推荐。公开查询仍在 MySQL 侧校验 ready/public/正常作者，Redis 不保存权限真相。
- `backend/internal/server/router.go`、`frontend/server.mjs`：新增 `/api/v1/categories`、`/api/v1/feed/following`、`/api/v1/videos/:id/related` 白名单与路由。
- `frontend/src/api/taxonomy.ts`、`video.ts`、`DiscoverView.vue`、`UploadView.vue`、`FollowingView.vue`、`VideoDetailView.vue`：接入分类筛选、标签输入、关注流、相关推荐和投稿字段；作者编辑表单也会回填并提交分区/标签。

### 17.2 实际验证

- 在隔离 MySQL 测试库执行 `go run ./cmd/api migrate up`：`000006_sessions_profiles.sql`、`000007_taxonomy.sql` 均成功应用；未清理或重建业务库。
- `go test -race -tags integration ./internal/taxonomy -count=1`：通过；覆盖真实迁移、Unicode 标签去重、分类/标签关联、分类筛选、关注流权限过滤和相关推荐排序。
- `go test -race -tags integration ./internal/video ./internal/creator ./internal/follow -count=1`：通过。
- `go test -race ./... -count=1`、`go vet ./...`、`go build ./cmd/api ./cmd/worker`：通过。
- `npm run typecheck`、`npm test`（Vitest 14 文件/82 用例，legacy 101/101）、`npm run build`：通过。
- `node --test tests/server.test.mjs`：通过，新增分类、关注流、相关推荐代理白名单断言。
- `$env:E2E_PORT='5275'; npm run test:e2e`：6/6 通过；这些用例仍以路由 mock 为主，不能等同真实全栈浏览器验收。
- T05 收口复审发现的作者编辑缺口已补齐；详情页定向单测 4/4 及完整前端门禁重新通过。

### 17.3 未验证与限制

- T05 尚未提交或推送；未执行任何远端操作。
- 分类目前只有预置数据和读取缓存，没有管理员分类维护接口；分类修改/失效接缝已保留给后续治理或运维命令。
- 相关推荐是规则排序，不是机器学习推荐；本轮未做 QPS、缓存命中率或 SQL 次数性能结论。
- 真实 API + Vite + Worker 的浏览器直传/转码/HLS 全链路仍留到 T10/T11；本轮 E2E 不伪装成真实依赖验收。

## 18. T06 评论回复、通知与幂等收据（2026-09-23）

### 18.1 实际改动

- `backend/migrations/000008_notifications_replies.sql`：为评论增加 `parent_id/root_id`，新增 `notifications` 与 `operation_receipts`，旧评论回填为根评论。
- `backend/internal/engagement/`：一层回复校验、根评论/回复分页、删除占位、评论 request_id 重放与冲突检测；评论、回复通知与必要计数在同一事务完成。
- `backend/internal/follow/`：新增关注通知和 request_id 幂等收据，已存在关系不会重复通知。
- `backend/internal/notification/`：结构化通知模型、MySQL 未读计数、单条/全部已读和所有权校验。
- `frontend/src/api/notification.ts`、`stores/notifications.ts`、`views/NotificationsView.vue`、`components/CommentThread.vue`：回复交互、通知 30 秒轮询、页面隐藏暂停和退出清空迟到结果。

### 18.2 实际验证

- `go run ./cmd/api migrate up`：本地业务库仅追加应用 `000008_notifications_replies.sql`，未清库。
- `go test ./... -count=1`、`go test -race ./... -count=1`、`go vet ./...`、`go build ./cmd/api ./cmd/worker`：通过。
- `go test -race -tags integration ./internal/engagement ./internal/follow -count=1`：通过；覆盖评论重放/冲突、同视频回复、通知接收人矩阵、关注通知重放。
- `go test -race -tags integration ./internal/database -run 'TestCommunitySchemaConstraintsAndBackfill|TestSessionMigrationRecoversPartiallyAppliedUserAlter' -count=1`：通过；迁移回滚/部分应用恢复测试已按 000006–000008 顺延。
- `npm test`：Vitest 16 文件/85 用例，legacy 101/101；`npm run typecheck`、`npm run build`、`node --test tests/server.test.mjs` 均通过；使用隔离端口 `E2E_PORT=5183` 的 Playwright E2E 6/6（仍为路由 mock）。默认 5173 若已有其他工作树服务会被复用，不能作为本工作树证据。

### 18.3 未验证与限制

- T06 尚未提交或推送；未执行远端操作。
- 通知目标失效时前端只展示结构化事件和“打开相关视频”入口，治理处置字段属于 T07；未复制评论正文。
- 评论删除后的根列表由服务层映射为“评论已删除”占位，历史底层正文仍保存在软删除行中，后续治理审计可用。
- 未宣称 WebSocket、跨区域顺序或性能提升；轮询与 SQL 查询性能留到 T12。

## 19. T07 举报、管理处置与审计（2026-09-23）

### 19.1 实际改动

- `backend/migrations/000009_moderation.sql`：为视频/评论增加 `moderation_status`，新增 `reports` 与 `moderation_actions`，活动举报唯一键、管理员/目标/审计索引及外键；新增 DDL 通过 information_schema 存在性检查和 `CREATE TABLE IF NOT EXISTS` 支持部分提交后的重试恢复，Down 同样可重入。
- `backend/internal/moderation/`：举报创建与本人列表、管理员实时角色校验、并发接单、驳回/带处置结案、视频/评论隐藏恢复、账号禁用恢复、追加审计和结构化通知；处置写入与状态/计数在同一事务。
- `backend/internal/middleware/require_admin.go`、`cmd/api/main.go`：管理 API 每次查询当前数据库角色；显式 `admin grant/revoke <username>` CLI，撤回最后正常管理员会拒绝。
- `/reports` 按用户 ID 使用 Redis 固定窗口限流（默认 5 次/10 分钟）；Redis 故障时降级到有界进程内限流，避免举报入口无限写入。
- 禁用账号在同一治理事务撤销全部 `session_families`，刷新仓储再次校验数据库账号状态；管理员集合按稳定顺序加锁，保护最后一个正常管理员。
- `video`、`engagement`、`creator` 公开/互动/历史查询统一过滤治理隐藏状态；HLS/封面复用公开详情条件，隐藏视频不会继续签发新媒体地址，创作者公开作品计数也不包含隐藏视频。
- `frontend/src/api/moderation.ts`、`ReportDialog.vue`、`ReportsView.vue`、`AdminReportsView.vue`、`AuditView.vue`：举报弹窗、本人进度、管理员工作台和审计页；代理白名单转发幂等请求头。

### 19.2 实际验证

- `go run ./cmd/api migrate up`：本地业务库仅追加应用 `000009_moderation.sql`，未清库。
- `go test -race -tags integration ./internal/moderation ./internal/session ./internal/database -count=1`：通过；真实 MySQL 覆盖举报重放/活动重复、两管理员并发接单一胜一冲突、并发禁用管理员的最后管理员保护、禁用账号会话族撤销、刷新拒绝、处置通知、隐藏恢复、评论计数调整、失败回滚、已删除内容不可恢复，以及 000009 部分 DDL 应用后的迁移恢复。
- `go test -race -tags integration ./... -count=1`：通过；所有真实依赖集成包通过，测试使用独立临时库并在清理时删除，未触碰业务数据库。
- `go test -race -tags integration ./internal/server -run TestReportRouteFallsBackToPerUserRateLimitWhenRedisIsUnavailable -count=1`：通过；真实路由验证首个举报成功、Redis 不可用时同一用户第二次请求返回 429。
- `go test -race ./... -count=1`、`go vet ./...`、`go build ./cmd/api ./cmd/worker`：通过。
- `npm test`：Vitest 17 文件/87 用例，legacy 101/101；`npm run typecheck`、`npm run build`、`node --test tests/server.test.mjs`：通过。
- 使用隔离端口 `E2E_PORT=5184 npm run test:e2e`：6/6 通过；仍是路由 mock，不等同真实 API 全链路。

### 19.3 未验证与限制

- T07 尚未提交或推送；未执行任何远端操作。
- 管理工作台当前以结构化目标 ID 展示，不自动抓取目标正文/媒体；真实浏览器举报—处置闭环留给 T11 全栈 E2E。
- 隐藏内容不再进入公开发现、主页、相关推荐、关注流、收藏/历史、评论和 HLS/封面新签名；已发出的对象存储预签名地址仍受其原到期时间约束。
- 未宣称性能提升、残余签名窗口或压测数字；这些留到 T11/T12 实测。

## 20. T08 有效观看、创作者数据中心与断点续播（2026-09-23）

### 20.1 实际改动

- `backend/migrations/000010_metrics.sql`：为观看历史增加有效观看累计与最近计时点，新增观看会话、视频日指标和创作者日指标；新增列采用 information_schema 检查，Down 可重入。
- `backend/internal/analytics/`：实现观看会话创建、序号幂等心跳、每次最多 15 秒与跨标签页共享计时上限、3 秒有效观看门槛、80% 完播条件、历史进度和创作者汇总查询。
- `backend/internal/engagement/`、`internal/follow/`：点赞、收藏、评论和关注的真实关系变更与视频/创作者日指标在同一事务写入。
- `frontend/src/composables/useWatchSession.ts`、`components/VideoPlayer.vue`：按可见且实际播放的墙钟时间采样，暂停/缓冲/拖动/切后台/卸载边界心跳，序号递增并清理所有监听器；播放页按服务端 resume 位置断点续播。
- `frontend/src/views/CreatorDashboardView.vue`、`api/analytics.ts`：新增登录后的 `/creator-center` 数据中心，展示 7/30 天趋势和公开作品 Top 10，不生成虚假数据。

### 20.2 实际验证

- T08 后端观看会话集成测试通过：重复 seq、暂停标签空心跳不阻塞活跃标签、跨标签共享计时、有效播放/完播、跨上海午夜 cohort、断点重置、隐藏视频拒绝、创作者趋势和 Top 10（`go test -race -tags integration ./internal/analytics -run TestWatchSessionCreditsOnceAndSharesCapAcrossTabs -count=1`）。
- `go test -race -tags integration ./internal/analytics ./internal/database ./internal/engagement ./internal/follow -count=1`：通过；覆盖 000010 部分 DDL 恢复以及关系净指标事务写入。测试使用 root 仅创建隔离临时库，未连接业务库。
- 后端最终门禁：`go test -race ./... -count=1`、`go vet ./...`、`go build ./cmd/api ./cmd/worker` 均 EXIT=0；此前发现的 cache 并发测试调度抖动已通过等待所有调用者进入并发区间修正。
- 前端新增 analytics API 与 watch-session 单元测试；`npm run typecheck` EXIT=0，`npm test` 通过（Vitest 19 文件/90 用例、legacy 101/101），`npm run build` EXIT=0，隔离端口 `E2E_PORT=5187 npm run test:e2e` 6/6 通过。E2E 仍为路由 mock，不等同真实 API 全链路。

### 20.3 未验证与限制

- T08 尚未提交或推送；未执行远端操作。
- 旧 `/videos/:id/watch` 兼容接口仍保留，Vue 播放页已迁移到 watch-session；真实 API、Vite、MySQL、Redis、MinIO、Kafka 的浏览器全链路验收仍留在 T11。
- 当前只记录有效观看指标，不宣称 QPS、命中率或观看时长提升；压测与故障演练留到 T12。

## 21. T09 Redis 日榜/周榜与可重建快照（2026-09-23）

### 21.1 实际改动

- `backend/internal/ranking/`：按 Asia/Shanghai 自然日聚合 `video_daily_metrics`，评分固定为 `effective_views + 3*max(net_likes,0) + 5*max(net_favorites,0) + 2*max(net_comments,0)`；同分按视频 ID 降序。
- Redis ZSET 使用 `video_share:rank:{day|week}:YYYY-MM-DD`，构建写入带 owner 的临时键，完成后 `RENAME` 原子替换；锁带 owner 校验和 30 秒 TTL，快照 TTL 180 秒，日期键保留不超过 8 天。
- 榜单读取只把 Redis 当候选 ID/分数来源，随后在 MySQL 重新校验正常作者、ready、public、visible，并填充分页；Redis 丢失、过期或不可用时回源 MySQL 日指标，MySQL 是永久权威来源。
- `backend/cmd/ranking-rebuild`：默认每 60 秒重建日榜和周榜，可用 `day`/`week` 限定窗口；`RANKING_REBUILD_ONCE=1` 适合一次性任务或测试。
- `GET /api/v1/videos/ranking?window=day|week&page=&page_size=` 与前端 `/ranking` 日榜/周榜页；响应返回 `generated_at`，空榜显示合规空状态。

### 21.2 已验证

- `go test ./internal/ranking ./internal/cache`：通过；覆盖上海日期边界、初版评分、同分次序、Redis 锁 owner、ZSET 临时键原子替换、回源拥塞错误映射和超大分页拒绝。
- `go test -race -tags integration ./internal/ranking -count=1`：通过（真实 MySQL/Redis）；覆盖隐藏/私密候选过滤、快照过期与 Redis 清空回源、空榜/无指标时最新公开视频零分兜底、锁冲突、原子重建和 Redis 故障保留旧快照。
- `go test ./... -count=1`、`go test -race ./... -count=1`、`go vet ./...`：通过；后者覆盖本轮新增榜单回源并发保护。
- `go build ./cmd/api ./cmd/worker ./cmd/ranking-rebuild`：通过；路由接入和榜单重建命令编译通过。
- `npm run typecheck`、`npm test`（Vitest 20 文件/92 用例，legacy 101/101）、`npm run build`：通过；榜单 API 参数边界和页面构建通过。
- `$env:E2E_PORT='5192'; npm run test:e2e`：7/7 通过；新增真实浏览器榜单日榜/周榜切换用例。用例仍使用 API 路由 mock，不等同真实全栈浏览器验收。

### 21.3 未验证与限制

- 上述集成测试依赖专用 MySQL DSN 与 Redis DB 15；普通开发未设置这些变量时仍会跳过，不能把跳过写成真实依赖已验证。
- Redis 命中路径按 200 个候选批量回查 MySQL，回源并发槽位当前为 4；超过容量明确返回 503。尚未在 T12 压测中测定其 QPS、SQL 次数或命中率。
- 尚未运行 T12 的命中/未命中压测，因此不宣称缓存命中率、SQL 次数或性能提升；真实浏览器全链路仍属于 T11。

## 22. T10 部署、CI 与文档（2026-09-23–24，本地隔离 Compose 验收通过）

### 22.1 已实现

- `backend/Dockerfile` 增加独立榜单重建运行镜像，Go 构建镜像固定到 1.26.3；`frontend/Dockerfile` 将 Vue 构建产物交给 Nginx，`frontend/deploy/nginx.conf` 配置静态资源、深链回退、`/api/` 同源代理、安全头、CSP 和 32 KiB API 请求体限制。
- Compose 增加 `demo` profile 的前端与榜单重建服务；MySQL、Redis、MinIO、Kafka、API、Worker 保持原有数据卷。迁移改为显式执行，未调用 `down -v` 或清空业务库。
- `.github/workflows/ci.yml` 分为前端类型/单元/构建、浏览器路由 E2E、Go 单元/race/vet/build 和真实依赖集成任务；集成任务配置 MySQL、Redis、Kafka、MinIO、FFmpeg 与 MySQL 客户端，强制缺依赖失败。
- 新增真实 MinIO 预签名上传/读取、FFmpeg/FFprobe 处理和两个一次性 MySQL 库间 `mysqldump`/`mysql` 备份恢复集成用例；更新部署、接口、架构和 README。
- 真实 MySQL 并发门禁暴露了创建举报与结案更新之间的 `active_key` 次级索引 ↔ 主键锁顺序死锁；创建逻辑改为先非锁定读取 ID、再按主键锁定并复核状态。原测试连续失败，修复后连续 5 次通过。测试夹具的会话族 ID 也改为符合 `CHAR(36)` 的 UUID。
- 复审又发现带 `report_id` 的直接处置与举报结案在举报行/目标行间形成反向锁；新增真实并发用例在修复前连续复现 1213。两条路径现统一先锁举报行、再查幂等收据、再锁目标行；收据摘要包含 `report_id`，同请求 ID 换关联返回冲突。目标已由并发管理员设为相同状态时记录真实 `before_state=after_state` 的审计，但不重复发目标状态变化通知，也不把 SQL no-op 误报成目标不存在。
- 同一管理员既举报又结案时，`report.create` 与 `report.resolve` 的缺失收据会触发 MySQL 间隙锁反向等待；新增同 actor 用例在修复前真实复现 1213。四类治理写事务现仅对 MySQL 1213 进行最多 4 次额外的整事务重试（10/20/40/80 ms）；审计、通知、计数及收据全部在同一事务内，失败尝试不会部分提交，超时或其他错误不重试。
- 复审发现旧 Compose MySQL 健康检查以 exec 形式传入字面量 `$MYSQL_ROOT_PASSWORD`，且 `mysqladmin ping` 对认证失败也可能返回成功；现改为容器 shell 展开密码后执行 `mysql SELECT 1`，实际认证探针已在运行中的 MySQL 容器验证通过。

### 22.2 本机实际验证

- 2026-09-24 收尾复验：在 `REQUIRE_INTEGRATION_TESTS=true` 并显式配置本机 MySQL、Redis DB 15、Kafka、MinIO、FFmpeg/FFprobe 后，`go test -race -tags integration ./... -count=1 -timeout=8m` EXIT=0；所有带集成标签的依赖包均通过，没有因缺少依赖而跳过。慢包为 engagement 142.553s、database 75.893s、follow 83.055s、moderation 35.507s、video 56.618s。测试辅助函数创建/清理唯一命名的临时 MySQL 库，不操作业务库。
- 同日后端普通门禁复跑：`go test ./... -count=1`、`go test -race ./... -count=1`、`go vet ./...`、`go build ./cmd/api ./cmd/worker ./cmd/ranking-rebuild` 均 EXIT=0。
- 同日前端门禁复跑：`npm run typecheck` EXIT=0；`npm test` 中 Vitest 20 文件/92 用例及 legacy 101/101 全通过；`npm run build` EXIT=0；`$env:E2E_PORT='5194'; npm run test:e2e` 7/7 通过。E2E 使用 API 路由 mock。
- Docker Desktop 截图中 Docker Desktop proxy 为 System proxy、Containers proxy 为 Same as host proxy。Windows 系统代理路径访问 `auth.docker.io` 返回 200、`registry-1.docker.io/v2/` 返回预期 401；`docker pull nginx:1.29.1-alpine`、`golang:1.26.3-alpine`、`node:22.22.3-alpine`、`alpine:3.22` 均通过。首次 Compose build 的镜像拉取已通过，但构建步骤中的 `npm ci` 因 ECONNRESET 失败；加入 `HTTP_PROXY`/`HTTPS_PROXY=http://http.docker.internal:3128` 的临时 build args 后，`api`、`worker`、`ranking-rebuild`、`frontend` 四个镜像全部构建成功，未把代理写入 Dockerfile 或运行镜像。
- 对构建出的 Nginx 镜像做隔离冒烟：用临时 Node API stub 满足 `api:8081` 上游解析，容器映射至 5194；`/healthz`、`/`、`/ranking` 返回 200，`/api/` 能到达 stub，CSP/`X-Content-Type-Options`/`X-Frame-Options`/`Referrer-Policy` 响应头存在，33 KB API 请求返回 413。临时 stub/Nginx 容器已清理；该项不是完整 Go API 全链路验收。
- Docker 镜像构建的 `npm ci` 报告 2 个 moderate 依赖漏洞；本轮未执行 `npm audit fix --force`，待单独审查依赖路径和可升级范围。
- `backend/deploy/docker-compose.isolated-smoke.yml` 为本地完整 Compose 验收清除固定容器名、映射独立端口、采用项目隔离卷，并显式使用 `.env.example` 与进程内随机测试凭证；普通演示 Compose 未改变。隔离栈另挂载 `frontend/deploy/nginx.isolated-smoke.conf` 放行自身 MinIO 19000 端口，不放宽生产 Nginx 的 CSP。
- 2026-09-24 完整隔离栈实跑：四个应用镜像构建成功；独立 MySQL/Redis/MinIO/Kafka 健康，Kafka topic 初始化成功，迁移 `000001`–`000010` 全部成功；API `/readyz` 为 `ready`，API、前端健康检查通过，Worker 与榜单重建进程保持运行。隔离栈映射前端 15173、API 18081、MySQL 13308、Redis 16379、MinIO 19000/19001、Kafka 19092，使用 `stage5-smoke-20260924` 自有网络和数据卷。
- 同一真实栈经 Nginx 验证 `/`、`/ranking`、`/login?returnTo=/`、`/healthz` 为 200；公开视频列表代理到真实 Go API 为 200；34 KB JSON 请求为 413；CSP、`nosniff`、`DENY`、Referrer-Policy 响应头存在，隔离 CSP 允许 MinIO 19000。MinIO 19000 的浏览器 Origin 预检为 204，返回精确的 `Access-Control-Allow-Origin: http://127.0.0.1:15173`。
- 本机浏览器直接打开隔离栈 `/`，随后直达 `/ranking` 并刷新；刷新后 URL 仍为 `/ranking`，Vue 榜单页正常渲染“尚无数据”空状态。此为路由/深链浏览器烟测，不等同 T11 的业务闭环 E2E。
- 经 Nginx 用一次性合成账号实测注册 201、获取 CSRF、同源登录 200、刷新 Cookie 同时有 `HttpOnly` 和 `SameSite=Lax`、登出 204。未使用现存业务账号或媒体。当前隔离栈为方便后续浏览器验收仍运行；没有停止、重启或迁移既有开发服务与业务数据卷。
- `go test -race -tags integration ./... -count=1`：修复死锁后全包通过，增加备份恢复用例后又全包重跑一次通过；真实依赖为本地 MySQL/Redis/Kafka/MinIO/FFmpeg。备份恢复新用例单独运行通过：恢复 1 个账号、1 条视频元数据和 11 条已应用迁移记录，dump 为 34,165 字节；测试库由唯一名称辅助函数清理。
- `go test ./... -count=1`、`go test -race ./... -count=1`、`go vet ./...`、`go build ./cmd/api ./cmd/worker ./cmd/ranking-rebuild`：通过。
- `npm run typecheck`、`npm test`（Vitest 20 文件/92 用例，legacy 101/101）、`npm run build`：通过；隔离端口 `E2E_PORT=5194 npm run test:e2e` 为 7/7，通过的是 API 模拟路由用例。
- `docker compose --profile demo --env-file backend/.env -f backend/deploy/docker-compose.yml config --quiet` 与 `git diff --check`：退出码 0。对运行中的 MinIO 发起本地 Origin/PUT 预检，返回 204 且 `Access-Control-Allow-Origin` 精确为 `http://127.0.0.1:5173`。
- 新增带 `report_id` 直接处置与结案并发用例：修复前真实 MySQL 连续 10 次失败，固定举报→收据→目标锁序后连续 10 次通过；再增加不带关联的直接处置竞争，合并场景连续 5 次通过。原创建举报/结案竞态复测结果见下方最终门禁。
- 同 actor 创建举报/结案竞态在修复前触发真实 1213；加入有界整事务重试后，`TestModerationReportLifecycleAndVisibility` 以真实 MySQL、`-race -count=10` 通过。重试单测覆盖成功、重试上限、非 1213 不重试和上下文取消。

### 22.3 未验证与阻塞

- 本轮验证了真实 Go API 代理与 cookie/CORS HTTP 契约，但未用浏览器做完整业务流程；注册—投稿—转码—互动—治理闭环、Worker/Kafka/Redis 故障演练仍属 T11。
- CI 工作流文件已建立，但因未提交/推送，本轮没有 GitHub Actions 运行证据；Linux runner 与 MariaDB 客户端兼容性仍待远端执行确认。
- 真实浏览器注册—投稿—转码—互动—治理闭环、故障演练和压测仍按依赖属于 T11/T12，不用当前模拟 E2E 或单次备份测试代替。

## 23. T10/T11 部署与故障验收收口（2026-09-24）

本节是上述按阶段记录之后的最新本地证据；若与旧章节中的“待验证”冲突，以本节和 [final-review.md](final-review.md) 的最终复核为准。没有改写旧记录中的历史状态。

### 23.1 Docker Hub 与镜像

- 用户确认 `docker pull golang:1.26.8-alpine` 成功；Docker 本机镜像 digest 为 `sha256:8ac98ca534ac3f51e1f420a1dd2c15e74c75cfa0f23f3ad27eb5d7236c349a0c`。
- 在 `stage5-smoke-20260924` 隔离项目中从当前工作树构建 `api`、`worker`、`ranking-rebuild`、`frontend`，随后显式运行迁移（`applied:0`）并只替换隔离应用容器。开发 Compose、业务数据库和既有卷未被重建或清理。

### 23.2 Outbox 故障路径修复和复测

- `KAFKA_PUBLISH_TIMEOUT_MS` 默认 5000 ms，允许范围 1–120000 ms；超时写入 `last_error` 并沿用指数退避重试。API 每次最多领取 10 条，claim 租约覆盖有界发布的整批最坏时长，防止批次尾部记录在轮到前过期。
- `go test ./internal/outbox ./internal/config -count=1`、`go test -race ./...`、`go vet ./...` 通过；新增测试覆盖发布截止时间、错误持久化、未来重试时间及 lease 覆盖完整批次时间。
- `e2e-stage5.ps1 -DrillKafkaOutage -SkipBrowser` 实际停掉隔离 Kafka 后观察到 `status=1`、`attempts=2`、`last_error` 非空；恢复 broker 后该事件为 `status=2`、`attempts=4`、`last_error` 清空，Worker 完成转码。`attempts` 是领取计数而非 Kafka delivery 次数。

### 23.3 Nginx、浏览器和部署边界

- production 与 isolated Nginx 配置都通过 `nginx -t`；`/api/` 使用 Docker DNS resolver `127.0.0.11`、5 秒缓存。占用原 API 地址后重建 API，容器 IP 从 `172.20.0.7` 变为 `172.20.0.10`；保持 Nginx 前端进程不变，等待 resolver 缓存后同源分类接口仍返回 200。
- 33,000 字节 API 请求返回 413；MinIO 对 `http://127.0.0.1:15173` 的预检返回相应 `Access-Control-Allow-Origin`；CSP 头存在。最终 Stage5 E2E 对 refresh `Set-Cookie` 断言 `HttpOnly`、`SameSite=Lax`，12 个 API 阶段和真实 Playwright 1/1 均通过。
- 两个 API 进程共同使用隔离 Redis 的登录限流测试：前 10 个交替请求到达 CSRF guard（403），第 11 个跨进程返回 429 并带 `Retry-After`。
- Stage 5 已提交为 `91dcdf99c9d11f2ccb3c6686ef5bdd6ef4ab5642` 并快进推送至 `origin/main`。GitHub Actions 四个 job 详情页显示成功，汇总页当时仍为 In progress，见 [运行记录](https://github.com/luo-zo/video_share/actions/runs/35993930550)。本地隔离 Compose 已验收，但未发布到公网；HTTPS 域名/CDN、生产数据、长时间稳态和生产容量不在本地实测范围。
