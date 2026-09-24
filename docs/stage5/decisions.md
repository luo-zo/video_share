# 第五阶段决策记录（T00）

**建立时间：** 2026-09-22
**状态：** 本文件记录**已定的取舍**以及**待确认项**。凡标注“待确认”的，实施前需要用户裁决，不擅自默认。

配套事实见 `baseline.md`。本文件中的技术口径直接来自任务书第 2、3、4 节，落实方式是本仓库的实际结构。

---

## 1. 前端：Vue 3 + TypeScript + Vite

### 1.1 版本与工具链

| 项 | 决定 | 理由 |
|---|---|---|
| 框架 | Vue 3（组合式 API + `<script setup>`） | 任务书指定；组合式 API 更利于把现有纯函数直接搬进 `setup` |
| 语言 | TypeScript，**强制 `vue-tsc` 类型检查** | 任务书明确“不能只用 Vite 构建代替类型检查”；Vite 的 esbuild 会静默丢弃类型错误 |
| 构建 | Vite | 任务书指定；替代现有 `server.mjs` 的手工静态服务 |
| 路由 | Vue Router 4，history 模式 | F02 要求可分享 URL、刷新与前进后退可用 |
| 状态 | Pinia，**仅放跨页面共享状态** | 任务书明确“不把全部表单状态放 Pinia”。登录态、通知未读数进 store；表单/分页留在组件内 |
| 单元测试 | Vitest + Vue Test Utils | 承接现有 8 个 `node:test` 文件，逐项迁移 |
| E2E | Playwright | 任务书指定 |
| Node | **锁定 22.22.3**（`engines` + `.nvmrc`） | 本机实测即为 22.22.3；任务书要求开发/CI/镜像一致。当前 `package.json` 只有 `>=20`，需收紧 |

依赖版本以“互相兼容的一组”整体引入并提交 `package-lock.json`，不逐个指定 latest。

### 1.2 必须保持的不变量

1. **保留小黑猫视觉。** 设计令牌已存在于 `frontend/src/styles.css` 的 `:root`：`--lime:#d8f28e`、`--forest:#1b2b23`、`--paper:#f5f4ed`、`--ink:#203b30`，显示字体 `--display` 与等宽 `--mono`。迁移是**搬运 CSS 与 SVG，不是重新设计**。登录页的小黑猫 SVG 位于 `index.html:37-80`，`components/BlackCat.vue` 直接承接。
2. **禁止 `v-html`。** 现有 `view-kit.js:1-2` 刻意声明“服务端/用户文本永不进入 attrs、永不使用 innerHTML”。Vue 下等价约束是**禁止使用 `v-html` 渲染任何用户输入**，并加 ESLint 规则兜底。
3. **校验规则原样保留**（见 `baseline.md` 8.3）：Unicode 码点计数、密码 72 字节上限、文件名 `.mp4` 兜底判断等细节不得简化。
4. **测试只迁移不删除。** 8 个 `tests/*.mjs`（100 用例）逐项转为 Vitest，`npm test` 最终覆盖全部保留用例。
5. **无障碍**：保留 `skip-link`、`aria-live` 错误提示、`:focus-visible` 轮廓、`prefers-reduced-motion` 处理与键盘可达性。焦点管理（现依赖 `byId(`${view}-title`).focus()`）在 Vue 下改用 `nextTick()` + 模板 ref。
6. **清理定时器与监听。** 迁移时必须补上现有代码缺失的清理：`main.js:1107` 的 `setTimeout`、`cat.js:31-57` 的 7 个监听器与 2 个 rAF、`watchReporter` 的媒体事件。`onUnmounted`/`onScopeDispose` 是硬要求。
7. **HLS 清理。** `player.js:55` 已返回 `hls.destroy()`，此模式保留；`import('/vendor/hls.mjs')` 改为 `import('hls.js')`（已是依赖）。**整合后进一步改为按需加载 `import('hls.js/light')`**（唯一导入点：`frontend/src/lib/player.ts` 的 `loadBundledHls`），把 HLS 从主包拆成独立 chunk。**代价（须长期记录）：light 构建不含字幕、备用音轨、EME 与 CMCD。** 若将来要支持字幕等特性，改回 `hls.js` 的那一处导入即可，其余代码不受影响。实测体积：`592.63 kB → 370.93 kB`（gzip `~186 kB → 117.84 kB`，约 −37%），构建不再报 chunk 过大警告。

### 1.3 开发期安全边界的归属（**已定 — 原 Q1**）

**结论：不重写边界，而是让 Vite 直接复用 `server.mjs` 的 `createApiMiddleware`。** 即否决了“开发期退化为 Vite 默认代理”与“双服务并存”两条路——这里复刻的是**同一份实现**，不是同一套规则的第二次抄写。

实现方式（`frontend/vite.config.ts`）：

- `vite.config.ts:5-9` 从 `./server.mjs` 导入 `createApiMiddleware` / `DEFAULT_API_TARGET` / `DEFAULT_STORAGE_ORIGIN`；
- 插件 `devApiBoundary`（`vite.config.ts:116-155`，`apply: 'serve'`）在 `configureServer` 里 `server.middlewares.use(...)`。该 hook 在 Vite 内部静态/转换中间件**之前**执行，因此边界检查先于任何文件服务；
- 命中 API 路径（`/api/`、`/healthz`、`/readyz`，见 `shouldHandleApiPath`）时整段交给 `createApiMiddleware`，中间件返回的 404 信封与旧服务一致。API 路径的方法白名单、写请求 Origin 校验、请求体上限、上游超时、重定向白名单因此**全部保持旧实现**，开发与旧服务不会行为漂移；
- 非 API 路径先过 `isBlockedDevPath` 黑名单（`vite.config.ts:39-67`），再做页面回退判断。

Vite 侧新增的两处，是 `server.mjs` 原本不需要的：

1. **`appType: 'mpa'`**（`vite.config.ts:165`）关掉 Vite 默认的 SPA 全量回退。`spa` 模式会把缺失的 `.js`/`.png` 也回退成 `index.html` 并返回 200，等于把“资源不存在”伪装成“成功”。关掉后，未命中路径由 `shouldFallbackToAppShell`（`vite.config.ts:76-84`）**只对无扩展名的应用页面路径**改写为 `index.html`，其余保持 404。
2. **路径归一化后再比对**（`vite.config.ts:51-62`）：`/src//auth.js`、`//server.mjs`、`/src/auth.js%2F`、反斜杠写法必须先收敛到同一规范路径，否则仅靠字符串精确匹配就能绕过黑名单。归一化包含：重复解码（上限 5 轮，仍不稳定即拒绝）、反斜杠转正斜杠、小写化、空段过滤、`.`/`..` 段拒绝、`/@fs` 前缀拒绝。

黑名单覆盖：构建/配置文件（`server.mjs`、`vite.config.ts`、`package.json`、`tsconfig*.json`、`.nvmrc`、`playwright.config.ts`、`server.d.mts`、`legacy.html`）、**旧实现源码**（`src/*.js`、`vendor/hls.mjs`）、`/dist/`、`/tests/`、`/test-results/`、`/playwright-report/`。

测试对应：`frontend/tests/unit/vite-boundary.test.ts`（归一化绕过、扩展名回退判定）与 `frontend/tests/e2e/routes.spec.ts`（真实 dev server 上缺失资源返回 404）。

**已知局限（必须如实记录，不得声称已完成）**

| ID | 局限 | 影响与处置 |
|---|---|---|
| L1 | 开发期 CSP 放宽：`script-src`/`style-src` 带 `'unsafe-inline'`（`vite.config.ts:93-112`），因为 Vite HMR 是内联注入模块脚本、运行时插 `<style>` | 严格 CSP（`script-src 'self'`、`style-src 'self'`）由 **T10 的 Nginx** 下发；开发期 CSP 不是生产口径 |
| L2 | 开发服务器只绑 `127.0.0.1`（`host: '127.0.0.1'`、`strictPort: true`）。同源 Origin 校验因此退化为对本机来源的校验 | 这是**开发便利性**，不是生产安全边界。`server.mjs` 的 `isLocalOrigin` 同样只面向本机 |
| L3 | **`vite preview` 不提供 SPA 回退**（`appType: 'mpa'` 且 `configureServer` 只在 `serve` 生效） | **不要用 `vite preview` 演示深链**（`/video/1` 会 404）。演示深链请用 `npm run dev`，或用 T10 的 Nginx |
| L4 | T03 前开发期**无 Cookie、无 CSRF 令牌**，身份是内存 Bearer | T03 已落地 Cookie + CSRF + Origin 精确校验；访问 JWT 仍只在内存（见 §3.3） |
| L5 | `server.mjs` 仍被 `vite.config.ts` 导入，**生命周期被延长到 T10** | T10 用 Nginx 取代后，`server.mjs` 与其测试才可删除。**在 T10 落地前不得删除 `server.mjs`** |

**不变量：不允许“删掉 `server.mjs` 却不在任何地方替代这些检查”。** T01 之后，开发期边界的唯一实现是 `server.mjs`；若未来要移除它，必须先有等价实现（T10 的 Nginx 或 Go 后端中间件）并补对应测试。

---

## 2. Redis 首期范围

### 2.1 用途边界

| 用途 | 键 | 规则 |
|---|---|---|
| 分区列表缓存 | `video_share:cache:categories:v1` | TTL 300s + 0–60s 抖动；分区变更后删除 |
| 日榜 | `video_share:rank:day:<日期>` | Sorted Set，成员=视频 ID，值=分数；从 MySQL 指标重建 |
| 周榜 | `video_share:rank:week:<日期>` | 含当天的最近 7 个自然日，时区 `Asia/Shanghai` |
| 分布式限流 | `video_share:rate:<用途>:<主体>` | Lua 原子读-判定-更新；所有键带有限 TTL |

### 2.2 明确**不做**的事（首期）

- **不给视频详情、播放签名、权限判定、`viewer_state` 加共享缓存。** 权限相关一律回源 MySQL。
- **不用 Redis 保存任何永久业务真值。** 关注关系、点赞、收藏、评论、会话 family/refresh token 的权威来源始终是 MySQL。Redis 故障不得让已登录用户失去身份。
- **不用 Redis Pub/Sub 替代 Kafka。** 转码任务投递继续走 MySQL Outbox → Kafka。
- 公开创作者资料缓存、通知未读计数缓存：**列为可选优化**，本阶段不承诺。

### 2.3 榜单口径

分数 = `有效播放次数 + 3 × max(净点赞,0) + 5 × max(净收藏,0) + 2 × max(净评论,0)`。

- 负净变化**原样保留在趋势数据中**，仅计算榜单时截为 0；
- 每 60s 重建当前日榜/周榜，临时键写完后**原子替换**，榜单 TTL 180s；
- 旧日期键最多保留 8 天；
- 空榜**回退到最新公开视频，不制造虚拟热度**；
- **Redis 只提供候选 ID**：响应前必须回 MySQL 过滤权限并取当前公开字段。隐藏/删除/转私密之后，即使 ID 仍在 Redis 也不得展示。

### 2.4 限流初值（可配置默认值，需按压测调整）

| 场景 | 配额 |
|---|---|
| 登录 | 每 IP 10 次/分钟 |
| 注册 | 每 IP 5 次/10 分钟 |
| 评论 | 每用户 10 次/分钟 |
| 举报 | 每用户 5 次/10 分钟 |

返回 429 + `Retry-After`。**只信任已配置反向代理的客户端 IP 头**（沿用 `TRUSTED_PROXIES`，`router.go:35` 已是 `SetTrustedProxies`），未配置代理头时不得伪造 IP。

### 2.5 故障降级（不同接口不同策略，不允许“统一放行”）

| 依赖 | Redis 不可用时 |
|---|---|
| 分区缓存/榜单读 | 短超时 → **回源 MySQL**；合并热点并发回源、限制回源并发；数据库饱和时明确返回 **503** |
| 登录/注册限流 | **503 `RATE_LIMIT_UNAVAILABLE`**（明确拒绝，不静默放行） |
| 评论/举报限流 | 退化为**进程内应急限流**，同时记录降级状态；**不宣称**仍保持全局配额 |
| 缓存写 | 在 MySQL **提交成功后**写；缓存失败**不回滚**已提交业务；删缓存失败则记录+告警，靠 TTL 收敛 |

### 2.6 替换现有实现

现有 `internal/middleware/ratelimit.go` 是**进程内**令牌桶（`golang.org/x/time/rate` + 有界 map），`router.go:57` 用于 register/login。T02 新增 Redis 限流中间件后：

- 保留该进程内限流器作为**应急降级路径**（正是 2.5 表格里评论/举报所需的“进程内应急限流”）；
- 但登录/注册必须走 Redis 全局配额，因为任务书要求“两个 API 实例连接同一 Redis 验证合并后的配额”。

### 2.7 必须实测，不许预填结论

任务书要求测量**命中率**与**数据库查询次数**。禁止在没有对照数据时写出“接入 Redis 即性能提升”。缓存对比（关闭/冷缓存/热缓存）属于 T12。

### 2.8 T02 实施边界（2026-09-23）

- T02 采用可注入数据源的 JSON 缓存组件，不创建 `categories` 表、不新增分类迁移、不注册分类路由。
- `000006_sessions_profiles` 仍由 T03 使用；`000007_taxonomy` 由 T05 创建 `categories`、`tags`、`video_tags` 并接入 `video_share:cache:categories:v1`。
- 缓存未命中或 Redis 不可用时，组件回源注入的数据源；同一键的并发回源通过 singleflight 合并，并受并发槽位限制。
- 登录/注册 Redis 限流不可用时返回 `503 RATE_LIMIT_UNAVAILABLE`；评论限流退化为有界进程内令牌桶并记录降级计数。
- T02 已在 `REQUIRE_INTEGRATION_TESTS=true`、专用 `TEST_REDIS_DB` 下用真实 Redis 验证缓存读写/失效，以及两个独立 Redis 客户端共享同一限流配额；缓存集成键使用唯一测试命名空间，尚未进行性能压测。

### 2.9 T03 会话边界（2026-09-23）

- 访问 JWT 默认 15 分钟并携带 `sid`；旧的无 `sid` JWT 不再通过生产路由鉴权。
- MySQL 的 `session_families` / `refresh_tokens` 是撤销与轮换权威；刷新令牌只存 SHA-256 摘要，浏览器只接收 host-only、HttpOnly、SameSite=Lax Cookie。
- refresh 轮换按 family 加锁；旧令牌在 10 秒内返回 `409 REFRESH_CONFLICT`，窗口外重放撤销整个 family 并返回 `401 SESSION_REUSED`。
- 登录、刷新、退出和资料写入要求精确 `APP_ORIGIN`、双提交 CSRF Cookie/请求头及 JSON Content-Type；网络错误不自动清空前端内存会话。

---

## 3. 会话口径

### 3.1 T03 前历史基线

T03 前是**单一无状态 JWT**：`internal/token/token.go` 的 `Manager` 用 HS256 签发 `{uid, iss, sub, iat, exp}`，`JWT_TTL_SECONDS` 默认 **86400（24 小时）**，**没有服务端会话状态、无法撤销**。前端把 token 只放内存（`auth.js:87`），刷新页面即登出。以下表格保留用于说明本批改动前的差距。

这与任务书 4.2 的差距是结构性的，不是参数调整：

| 任务书要求 | 现状 |
|---|---|
| 访问 JWT 15 分钟 | 24 小时 |
| JWT 携带 `sid` | 无 |
| session family + 刷新令牌轮换 | 不存在 |
| 退出 / 改密可撤销 | **做不到** |
| 刷新 Cookie host-only、HttpOnly、SameSite=Lax、Path=/api/v1/auth | 无 Cookie，token 在内存 |
| 并发刷新冲突窗口（409 `REFRESH_CONFLICT` / 401 `SESSION_REUSED`） | 无 |

### 3.2 T03 的确定口径

- 登录创建独立 **session family**；`session_families(id, user_id, created_at, absolute_expires_at, revoked_at)`，family 最长 **30 天**，轮换**不延长**绝对到期。
- `refresh_tokens(id, family_id, token_hash, created_at, expires_at, rotated_at)`；刷新令牌使用 **32 字节密码学安全随机数**，数据库**只存 SHA-256 摘要**。
- 访问 JWT **有效期 15 分钟并携带 `sid`**；`JWT_TTL_SECONDS` 默认值需随之改为 900。
- 刷新事务：**先锁 family** → 校验当前 token → **原子**标记旧令牌已轮换 + 创建新令牌。
- 退出：**锁同一 family** 并撤销。改密：撤销该用户**全部** family。
- 冲突窗口：已轮换令牌在 **10 秒内**重复出现 → **409 `REFRESH_CONFLICT`**，**不清 Cookie、不撤销 family**；客户端退避重试**最多 2 次**。**窗口外**再用旧令牌 → 撤销该 family，**401 `SESSION_REUSED`**。
- 每个需登录请求（以及可选鉴权请求所用的身份）都要校验 `sid` 未撤销、未过期、用户状态正常。**旧的无 `sid` 令牌升级后一律要求重新登录。**
- 访问 JWT 只留 **Pinia 内存**，**不写 localStorage / sessionStorage**。
- 并发：同页面用 **single-flight** 合并刷新；跨标签页用 **Web Locks** 协调，取锁后由浏览器发当前 Cookie；不支持 Web Locks 的环境保留**服务器端并发保护**。
- Cookie 的 `Secure` 生产开启，仅本机 HTTP 显式开发配置可关。
- **网络错误或 5xx 不当作登出。** 退出期间的迟到响应不得恢复前端身份；重新附上的已撤销 Cookie 也必须被后端拒绝。

### 3.3 CSRF 与 Origin

登录、刷新、退出及**其他 Cookie 驱动**的写操作：

- 校验**配置的精确 Origin**；
- 要求**同源自定义 CSRF 请求头** + 正确 `Content-Type`；
- **CORS 不允许任意来源携带凭证**；
- 程序测试显式提供可信 Origin。

注意：这与现有 `server.mjs` 的 `isLocalOrigin`（`server.mjs:100-113`）思路一致，但 T03 之后校验发生在 **Go 后端**，因为刷新/退出是 Cookie 驱动的，不再只是浏览器代理层的职责。

### 3.4 重试策略

**不统一自动重试所有业务 POST。** 只对“已知在执行业务前被鉴权拒绝”的请求刷新后**重试一次**；超时后的写请求遵循各自幂等规则（评论/举报用 `operation_receipts`，见下）。

### 3.5 T03 实现验证

T03 已按上述口径接入 Go 路由与 Vue 客户端。单元测试覆盖 Cookie 属性、CSRF、资料响应、sid 校验、冲突/重放错误映射、前端 single-flight 和退出竞态；真实 MySQL 隔离集成覆盖并发轮换、窗口外重放撤销整族、refresh-only logout 和迁移恢复。未把任何性能结论提前写入简历，性能对比留到 T12。

### 3.6 T04 公开主页边界（2026-09-23）

- 创作者主页只查询 `users.status=normal`；不存在和禁用账号统一返回 `NOT_FOUND`，避免通过计数或分页侧信道确认账号状态。
- 公开作品必须同时满足 `status=ready`、`visibility=public` 和正常作者；私密、处理中、删除作品不进入总数或分页结果。
- 粉丝与关注列表只返回 ID、用户名和昵称，关系两端均过滤禁用账号；`following` 只在主页可选鉴权成功时计算。
- T04 不引入公开资料共享缓存；缓存候选与权限校验留在 T05/T10 的边界内。

---

## 4. 统计口径（观看与指标）

### 4.1 四个指标必须区分

| 指标 | 定义 |
|---|---|
| 旧累计播放量 | 保持**每登录用户/视频一次**的现有语义，继续兼容旧字段（`video_stats.view_count`） |
| 新有效播放次数 | 新观看 session **连续累计**获得至少 `min(3秒, 视频时长)` 有效时长，计一次 |
| 有效观看时长 | 播放器**实际播放且页面可见**时的墙上时间；暂停/等待/拖动**不计**；**不乘倍速** |
| 有效完成播放数 | session 自然结束、位置距结尾 ≤ 2 秒、且累计有效时长 ≥ 视频时长 80%，每 session 一次 |

**绝不能用进度条差值冒充真实观看时长。**

### 4.2 服务端校验规则

- `POST` 创建观看 session；心跳带**递增 `seq`**、`position_ms`、`watched_delta_ms`、`ended`；
- 客户端**串行**发送，单个心跳最多累计 **15 秒**；
- `seek` / `waiting` / `pause` / `hidden` **重置活动采样基准**；
- 服务端以媒体元数据校验时长，锁 session 及用户/视频对应观看记录；
- **接受增量 = min(客户端活动增量, 服务端经过时间, 15 秒上限)**；
- 对同一**用户/视频**维护**共享计时上限**，防止两个标签页重复累计墙上时间（**跨 session 生效**）；
- 重复或更小 `seq` **不重复计数**；允许序号跳跃，但**丢失采样不补估**；
- 错误 duration、负数、超大增量、越界位置必须**拒绝或明确裁剪**；
- session 最长 **24 小时**；归属日期为**首次有效播放**的 `Asia/Shanghai` 日期，跨午夜**归同一 cohort 日期**（避免完成率分子分母错配）；
- 新完成率 = 有效完成播放数 / 有效播放次数；**分母为 0 显示“—”**；它可能随倍速与上述定义变化，页面需提供口径提示。

### 4.3 趋势数据

- 点赞/收藏/评论/粉丝趋势记录**每日净变化，允许负数**；累计总数**单独读取当前关系与状态**；
- 对**重复幂等操作不新增指标**（重放点赞不产生第二条趋势）；
- 新 daily 指标**自上线日起积累**；旧数据可展示累计总数，但历史趋势必须注明**“尚未采集”**，**不从当前总量倒推**。

### 4.4 新旧接口并存

`POST /videos/:id/watch` **继续保存旧进度与累计播放，不写新的 session 分析**。Vue 迁移完成后每次播放**只用新 session 接口**，由后端在 session 流程中**同步**旧历史与累计播放。这样旧客户端不破坏，新前端不双写。

### 4.5 断点续播

读取**本人**进度；媒体元数据（`loadedmetadata`）就绪后**只恢复一次**；**临近结尾从 0 开始**，短视频不得产生负 seek；切换视频或退出后**迟到请求不得覆盖**当前播放状态。

### 4.6 必须测试的场景

自然播放、手动拖到结尾（**不得**凭位置增加有效时长或完成播放）、暂停、缓冲、倍速、重复心跳、多标签页、session 过期、跨午夜、零分母。

---

## 5. 治理与一致性口径

### 5.1 公开内容统一规则

公开内容必须**同时**满足：**作者正常 + 视频 `ready` + `public` + 治理 `visible` + 未删除**。

此规则统一应用于：**发现页、创作者主页、相关推荐、榜单、收藏/历史公开投影、媒体入口（HLS/封面）**。

分阶段落地（任务书 4.1 的明确要求）：
- **T07 之前**：使用**已有**状态与权限条件（`status`、`visibility`、`users.status`）；
- **T07 迁移完成后**：统一补上 `moderation_status` 过滤；
- **不提前引用尚不存在的数据库列。**

作者通过**专属接口**查看自己的私密内容；其他用户与**公共缓存**均不得获取。

### 5.2 缓存与权限的关系（关键安全不变式）

**缓存候选返回前仍须验证当前权限。** 隐藏、删除或转私密之后，即使候选 ID 仍在 Redis，也必须从响应中消失。这条是 T07 验收的一部分（“补缓存命中下的权限测试”）。

### 5.3 评论与回复

- 界面只有**根评论 + 一层回复**；`parent_id` = 实际被回复的评论，`root_id` = 根评论；回复回复仍显示在**同一根评论**下；
- 回复目标必须**同视频且当前可见**；
- 根评论与回复**分别分页**，默认 20、最多 50，顺序**固定包含 ID 次级排序**（保证稳定）；
- 根评论删除/隐藏后正文显示**删除占位**，可见回复可保留，但**不能再回复该根评论**；隐藏单条回复**不影响**其他回复。

### 5.4 通知

- 触发矩阵：新评论→视频作者；回复→被回复者 **和** 视频作者（**去重并排除操作者**）；新增关注→被关注者；举报处理→举报者；内容/账号处置→目标用户；
- 通知与业务写入**同事务**完成；
- 创建评论、举报、管理操作携带 `request_id`，按**操作者/动作/请求 ID 去重**；`operation_receipts` 保存请求摘要，**重复请求体不一致返回 409**（不当作新操作）；
- 已存在的关注关系**不再触发通知**；
- 通知**存结构化事件，不长期复制评论正文**；目标失效显示**“内容不可用”**；
- 已读操作校验**接收人**；未读计数首版**以 MySQL 为准**（不用 Redis，见 2.2）；
- 轮询 **30 秒**，**页面隐藏即停止**；不要求 WebSocket。

### 5.5 举报与处置

- 举报类型 `video/comment/user`；原因 `spam/abuse/copyright/illegal/other`；详情**最多 500 字**；
- **同一举报者同目标只能有一条未关闭举报**，并发用数据库约束或锁保证；
- 状态机 `open → investigating → resolved/rejected`，允许 `open` **直接结案**；
- `resolved` **必须引用对应处置**；`rejected` **必须填写理由**；
- **终态重复请求返回原结果，不能重复处罚**；
- 内容状态 `visible ↔ hidden`，作者删除是**另一维度**的终态；**恢复治理状态不得复活已删除内容**；
- 用户状态沿用 `normal/disabled`；管理员**不能禁用自己或最后一个正常管理员**；禁用账号时**同一事务撤销其全部 session family**；**恢复账号不恢复旧会话**，须重新登录；
- 所有管理动作写**追加式审计**（操作者、目标、动作、理由、前后状态、`request_id`、时间）；普通用户**无读取权限**。

### 5.6 管理员角色

- 权限以**数据库当前角色为准**；**JWT 不携带可永久信任的授权快照**；
- 前端路由守卫**只控制界面**，最终权限由**后端**判定；
- 通过**显式 CLI 子命令**授予/撤回管理员；**无默认管理员密码、无公开自助提权路由**。

### 5.7 媒体边界（必须如实记录，不夸大）

隐藏内容后**阻止新的**清单、封面及签名获取；但**已经发出的 MinIO 分片预签名地址在到期前仍可能可用**。

- 必须**记录并验证实际签名有效期**；
- **不承诺瞬时撤回已签地址**；
- 若未来要求立即撤回，另立任务改为“每次媒体访问都校验权限的受控网关”。

### 5.8 本阶段的边界声明

本阶段是**举报后人工治理**。**不宣称**完成法律合规、自动版权检测或发布前审核。

---

## 6. 迁移与数据兼容口径

1. **编号顺延，不改历史。** 现有已到 `000005`，新迁移从 `000006` 开始。**绝不修改已在环境执行过的迁移文件**（`backend/README.md:195` 已确立此规则：迁移一经提交即冻结）。
2. **不清空业务库。** 当前库有 12 用户 / 9 视频 / 4 评论等真实数据。Down 迁移**不在有业务数据的库上执行**；生产回滚优先“版本回退 + 向前修复”。
3. **每个迁移要有 Up/Down 测试和旧数据回填测试**（`000005_backfill_community_data.sql` 是既有范例）。
4. **兼容旧接口：**
   - 旧客户端省略 `category_id` → 归入**“未分类”**，不破坏既有投稿接口；
   - 旧评论不带 `parent_id` → 仍是**根评论**；
   - 新增字段一律**兼容扩展**，保留 `/api/v1` 现有响应信封与错误结构；
   - 历史视频迁移为**可见**。
5. **旧指标不伪造。** 不从当前总量倒推历史趋势。
6. **路由冲突风险：** 固定路径 `/videos/ranking`、`/users/me` 与动态 `:id` 路径**必须写路由测试**，避免被 `:id` 误解析（`router.go:79-82` 已有 `/videos/:id`，新增 `/videos/ranking` 时顺序/冲突需专门验证）。

---

## 7. 工程环境口径

1. **端口：** 前端 `5173`、后端 `8081`（与现状及任务书一致，不改）。
2. **换行符（B2）：** 建议新增 `.gitattributes`，对 `*.go`、`*.sql`、`*.ts`、`*.vue` 声明 `eol=lf`，并在 CI 用 LF 检出，使 `gofmt -l`、prettier 在本地与 CI 结果一致。当前 Windows 工作区的 CRLF 会让 `gofmt -l` 产生约 36 个假失败。
3. **集成测试权限（B1，T10 已定）：** 应用账号 `video_share` **无 `CREATE DATABASE` 权限**。本地与 CI 当前明确使用各自一次性 MySQL 服务的 root DSN，`testutil.MySQLDSN` 不设默认业务库，只按进程 ID 和计数器创建唯一测试库，并在清理时只删除该库；备份演练创建两个这样的库。绝不把生产 root DSN 填入测试环境。以后可改用仅有测试库权限的专用账号，但需先验证建库/删库授权。`REQUIRE_INTEGRATION_TESTS=true` 让关键依赖缺失时直接失败；普通开发模式跳过的用例必须报告为“未验证”。
4. **Redis / Nginx 固定版本**（不用 `latest`），沿用 compose 现有做法（`mysql:8.0`、`minio:RELEASE.2025-09-07T16-13-09Z`、`apache/kafka:4.0.0`）。
5. **不引入**任务书明确后置的技术：微服务、Elasticsearch、Kubernetes、Redis Pub/Sub 替代 Kafka、Prometheus/Grafana（可选增强）。
6. **不提交密钥。** `.gitignore` 已忽略 `.env`、`.claude/`、`.worktrees/`。注意 `.claude/settings.local.json` 中已出现历史命令含数据库口令，该目录已忽略，但**不得复制该口令到任何提交内容或文档**。
7. **不擅自推送远端、不发布、不执行破坏性操作。** 整合前 `main` 领先 `origin/main` 19 个提交且未推送；**整合后（2026-09-22 夜）`main` 领先 27 个提交仍未推送**（整合提交 `48ef2a8` / `e4d4b89` / `7030fd0`，HEAD `7030fd0`）。保持现状，提交时机由用户决定。
8. **`.ps1` 必须带 UTF-8 BOM（2026-09-22 新增）。** Windows PowerShell 5.1 对**无 BOM** 的 UTF-8 `.ps1` 按系统 ANSI 代码页（本机 936/GBK）解码；GBK 解码器会吞掉行终止符，把后续代码行**并进注释**，导致该行**静默不执行**。症状是报错行号与实际行号不符（偏离量＝被吞掉的换行数）。本仓库的交付脚本 `backend/scripts/e2e-community.ps1` 与 `e2e-transcode.ps1` 现均为「UTF-8 BOM + CRLF」。指纹：把同一文件按 ANSI 读出的行数比按 UTF-8 少，且缺失的正是紧跟在中文注释之后的那一行。
9. **PowerShell 5.1 的响应解析陷阱。** `Invoke-WebRequest` 默认走 IE(mshtml) 响应解析器，本机 IE 未初始化时抛 `NullReferenceException`——**该异常在请求已经发出之后才抛**（对象可能已上传成功）。规则：一律加 `-UseBasicParsing`；**文本响应（HLS 播放列表）用 `Invoke-RestMethod`**，因为它自带 basic parsing 并直接返回文本，避免 `.Content` 变成 `byte[]`（`-notmatch "#EXTM3U"` 会因此失败）。另：`-PassThru` 在 5.1 必须与 `-OutFile` 同时使用，否则抛 `WebCmdletOutFileMissingException`；管道里只命中一个元素的数组会退化成标量，而 5.1 的标量没有 `.Count`（取值为 `$null`），函数需 `return ,@(...)` 包一层。
10. **Kafka 提交位点是分区级「下一条」语义，失败分区不得继续推进（2026-09-23 新增，T01 收口）。** franz-go 的 `CommitRecords(record)` 提交的是 `record.Offset+1`，即把该分区位点设成 `n+2`。因此**同分区里失败 offset=n 之后，offset=n+1 即使成功也绝不能提交**，否则 offset=n 永不再投——静默丢一条转码任务，与 `consume.go` 自身「不提交偏移，交由 Kafka 重投」的承诺矛盾。`internal/messaging/kafka.go:54` 的 `kgo.DisableAutoCommit()` 排除了自动提交绕过。规则：`cmd/worker` 用 `commitTracker` 记每分区**仍未处理成功**的失败偏移集合，**两处提交点**（成功记录、乱码丢弃记录）提交前都查 `blocks(partition, offset)`——存在失败偏移 `f <= offset` 即不提交；**阻塞按分区**，一个分区失败不拖住其他分区。
    **阻塞可解除（同日追加）**：失败的消息**重投并处理成功后**调 `markRecovered` 从集合删除，分区恢复提交；成功路径顺序必须是 **`markRecovered` → `blocks` → `Commit`**，否则重投成功的那一条会被自己的失败记录挡住。**第一版曾把失败记成「每分区最早偏移」而永久封禁分区**，一条瞬时失败即导致无限重复——已修正。
    **仍保留的代价（有意）**：**持续失败**的「毒丸」（库中已无对应任务等）会一直阻塞该分区，每次重启重投——即用重复处理换不丢消息（至少一次）。要限制重复需引入显式重试上限或死信队列，**不在 T01 范围**。回归测试：`backend/cmd/worker/consume_test.go` 共 **8 例**（4 例禁止越过失败偏移 + 2 例恢复场景 + 2 例其他）。

---

## 8. 待确认项汇总

| ID | 事项 | 影响任务 | 默认动作 |
|---|---|---|---|
| Q1 | 开发期安全边界由 Vite 还是自定义中间件承担（§1.3） | T01 | **已定**：Vite 复用 `server.mjs` 的 `createApiMiddleware`（`devApiBoundary` 插件），另加 `appType: 'mpa'` 与路径归一化；局限 L1–L5 见 §1.3 |
| Q2 | `docs/stage5/*.md` 是否提交 git | T00 | 先留在工作区，等用户决定 |
| Q3 | 集成测试用专用账号还是 root（§7.3） | T10、T11 | **已定**：仅本地/CI 一次性 MySQL 服务使用 root DSN，唯一测试库自动创建/删除；生产环境禁止使用 |

除上述三项外，第 1–7 节记录的口径均直接来自任务书，视为已定。

## 9. T05 分类、标签与推荐决策（2026-09-23）

- `000007_taxonomy.sql` 只做增量迁移；历史视频的 `category_id` 默认指向固定的“未分类”记录，不清空业务数据。
- 标签在服务层先做 Unicode NFC、去首尾空白、Unicode 小写和去重；每个标签 1–20 个 Unicode 字符，单次最多 5 个。数据库唯一键负责并发下最终去重。
- Redis 仅缓存启用分类列表，使用既有 `video_share:cache:categories:v1`、300 秒 TTL + 抖动和 MySQL 回源；分类缓存不是权限来源。
- 关注流和相关推荐的候选 ID/查询结果都必须在 MySQL 侧再次应用正常作者、ready、public 条件；相关推荐按同分区、共同标签数、累计播放/点赞、发布时间和 ID 固定排序，最多返回 6 条。
- 旧投稿/编辑请求不带新增字段时保持兼容；旧投稿落入未分类。分类管理与榜单缓存不在 T05 提前扩展，分别留给治理和 T09。

## 10. T06 回复、通知与幂等决策（2026-09-23）

- `000008_notifications_replies.sql` 只做加法：历史评论 `parent_id/root_id` 回填为根评论；通知和收据以 MySQL 为权威，Redis 不参与一致性。
- `parent_id` 必须指向同一公开视频中未删除的评论；回复的 `root_id` 固定为根评论，回复回复仍归入同一线程；删除评论在服务响应中显示“评论已删除”占位。
- 评论和关注写请求使用操作者 + action + request_id 唯一收据，收据保存 SHA-256 请求摘要与资源 ID；同 ID 不同摘要返回 409，不重复关系、计数或通知。
- 通知只保存事件类型和目标 ID，接收人去重、排除操作者；评论/回复/关注通知与业务写入在同一数据库事务内提交。未读数读取 MySQL，前台 30 秒轮询，页面隐藏暂停。
- 通知写操作在前端使旧轮询 generation 失效，并在成功后重新读取全局未读数；首屏之外的未读通知不会被当前页数量覆盖。

## 11. T07 举报与治理决策（2026-09-23）

- `moderation_status` 是视频/评论的公开可见性开关；公开查询、关注流、相关推荐、个人收藏/历史、评论和媒体签名入口统一要求 `visible`，MySQL 是最终权限来源。
- 举报活动唯一键采用 `reporter_id + target_type + target_id`，结案时清空；因此同一目标可在前一举报结案后再次举报，但活动期间不会重复打开。
- 管理员权限不写入 JWT 快照。`RequireAdmin` 每次请求读取数据库当前 `role/status`；CLI 负责显式授予/撤回，最后一个正常管理员受保护，管理员不能禁用自己。
- 接单先锁举报行再锁操作收据，固定锁顺序让并发接单表现为一个成功和一个状态冲突；处置、状态迁移、评论计数、审计和通知在一个 MySQL 事务提交。
- T10 真实并发门禁补充：结案和带 `report_id` 的直接处置一律先锁举报主键，再查幂等收据，最后锁目标；创建举报先非锁定查询活动键对应 ID，再按主键锁定复核，避免次级索引→主键与结案的主键→次级索引死锁。直接处置的收据摘要包含 `report_id`，关联目标不一致拒绝；并发已把目标设为相同状态时只记录如实的无状态变化审计，不重复向目标所有者发状态变化通知。
- 同 actor `report.create` / `report.resolve` 的缺失收据在 InnoDB RR 下仍可能持有相邻间隙锁。治理写事务保留 MySQL 默认隔离级别，仅对 1213 做最多 4 次额外整事务重试；所有副作用均在事务内，非 1213 或请求已取消不重试。该机制是有界恢复，不保证无限争用下永不返回失败。
- 禁用用户在同一事务撤销其所有未撤销 `session_families`；刷新时仍检查 `users.status`，作为治理事务与刷新并发之间的第二道防线。
- 用户禁用/启用与 CLI 管理员撤回均先按 `id ASC` 锁定管理员集合，再锁定目标用户；最后一个正常管理员不能被并发移除。
- 000009 的新增列、索引和表使用 information_schema 存在性检查与 `CREATE TABLE IF NOT EXISTS`，允许 MySQL 已提交部分 DDL、Goose 版本未记录时安全重试。
- 举报入口沿用 Redis 固定窗口 5 次/10 分钟，key 按用户而不是 IP；Redis 故障回退有界本地限流，登录/注册仍保持故障关闭策略。
- 恢复已删除视频/评论直接拒绝；已发出的 MinIO 预签名地址不能被本迁移瞬时撤回，残余有效窗口在 T11 实测记录。

## 12. T08 有效观看与创作者指标决策（2026-09-23）

- 观看时长只接受服务端确认的可见播放墙钟增量；客户端 `position_ms` 仅用于进度和完播判断，不能用拖动差值或 `playbackRate` 铸造观看时长。
- 每个播放标签页持有独立 24 小时 `watch_sessions`，序号重复/乱序幂等；同一用户/视频的 `watch_histories.last_watch_credit_at` 作为跨标签页共享上限，MySQL 行锁保证并发下只计一次。
- 有效播放在单会话累计达到 `min(3s, duration)` 时计数一次；完播要求 ended、接近视频尾部且有效观看达到 80%。旧总播放字段继续兼容，同时写入可重建的日指标。
- 日指标按上海自然日写入视频和创作者两级表；关系变更与指标在同一事务内完成，历史趋势不从当前总量倒推。
- 断点续播只返回当前用户自己的 `watch_histories` 进度，接近尾部的进度视为已看完并从 0 开始；视频/作者当前不可见时会话创建和心跳均拒绝。
- 数据中心只展示正常账号的公开 ready 视频和 MySQL 汇总；前端只负责呈现，不把本地播放事件当作统计成功，也不提前写性能结论。

## 13. T09 榜单快照决策（2026-09-23）

- 榜单评分遵循任务书初版合同：有效播放次数为主，正向净点赞/收藏/评论分别按 3/5/2 加权；负净变化仍保留在趋势表，只在榜单评分时按 0 处理。观看时长与完播不另行偷偷改变分数。
- Redis ZSET 只保存按日期命名的候选 ID 与分数。构建顺序是 MySQL 聚合 → 临时 ZSET → 设置有限 TTL → `RENAME` 原子替换；构建失败不覆盖旧完整快照。分布式锁保存随机 owner，释放使用 compare-and-delete Lua，锁过期后可恢复。
- 读取先取 Redis 候选，再用 MySQL 当前视频/作者状态过滤并补足页面；Redis 不是权限来源，也不保存永久热度事实。缓存丢失时直接以 MySQL 日指标重建排序。
- CLI 默认常驻每 60 秒重建 day/week；`RANKING_REBUILD_ONCE=1` 用于 cron/验收单次执行，避免测试进程常驻。
- Redis 命中读取按固定批次回查可见性，避免一次性把整榜候选和 `IN` 参数加载进内存；Redis 缺失或不可用时以 4 个并发槽位限制 MySQL 聚合回源，槽位耗尽返回 503。
- 日榜/周榜没有可见指标候选时，回退到 MySQL 最新公开 ready/public/visible/正常作者视频，分数保持 0；该兜底仍受当前权限过滤，Redis 不产生虚假热度。

## 14. T12 本机基线口径（2026-09-24）

- 压测工具固定为官方 `grafana/k6:2.3.0` 容器，不安装全局 k6、不使用 `latest`。脚本目标固定为本机隔离 API；runner 拒绝普通开发端口与非 `stage5-smoke-*` Compose 项目。
- 每个 VU 请求后暂停 100 ms，避免低延迟本机接口在固定 VU 模式中无上限自旋。5 VU × 15 秒的数值因此代表约 50 req/s 的有界本机基线，不是压力极限。
- “冷”只删除本轮分类或榜单的精确版本化 key 后测一个请求；榜单读取的缓存缺失会经 MySQL 回源。不得把单次冷样本报告成 P95/P99。
- “热”分类先读一次填充 JSON cache；“热”榜单通过现有 rebuild CLI 生成 ZSET 与 metadata 后再读。Redis outage 是停用隔离 Redis 的回退测量，不是对业务代码增加 cache-disable 开关。
- 用户权限通过响应内容断言（榜单每项必须 `ready/public`）和计时外匿名评论写入 401 探针核验。主性能 workload 是只读，避免插入无法精确回收的大量互动数据。
- MySQL Questions 与 Redis keyspace hit/miss 是全实例差值，容器内健康检查、后台榜单任务与其它并发请求可能共同贡献；CPU/内存来自约 1 秒一次的 Docker 采样。报告只称观察指标，不称 handler 的精确 SQL 计数或宿主机 profile。
- 每种持续条件重复三次保留原始 k6 JSON；异常结果不通过删除或平均处理掩盖。未测的整段连续播放、长稳态、极限并发及生产硬件能力不外推。
- API 请求和媒体分片读取分别测量；本地 HLS 签名 URL 指向 `127.0.0.1:19000`，所以独立媒体 k6 容器使用 host networking 并限制在 5 VU 内，避免在 Docker bridge 中把无法解析的宿主 loopback 错判为媒体接口故障。媒体样本只取合成视频的一个固定 MPEG-TS 分片，不能代替完整播放/公网或 CDN 测量。
