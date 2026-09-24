# video_share 第五阶段综合任务书：Vue、Redis、社区业务与工程验证

**文档版本：** 2.0 · 2026-09-22  
**状态：** 主体实施、本地验收与 GitHub main 推送完成；公网生产部署尚未配置。证据与环境边界见 [收尾复核](../../stage5/final-review.md)。
**目标：** 将现有视频社区 MVP 扩展为可分享、可回访、可治理、可演示的视频平台，并形成可复现的测试、性能和故障恢复证据。  
**代码基线：** 本次检查 main 为 cff21bd；实施前重新确认仓库状态。项目根目录为主仓库根目录。
**适用方向：** 以 Go 后端实习或校招项目为主要展示目标，Vue 提供完整可操作的产品界面。

本文件替代同路径上一版任务书。新增 Vue 迁移、Redis、工程验证；修正原生前端路由、刷新会话、观看时长和验收门禁。它是业务与工程任务合同，不是已完成代码或可直接复制编译的代码教程。本文中的新文件路径是拟新增位置，实施时按仓库模块约定建立；既有文件按实际结构修改。

## 1. 范围与交付结果

### 1.1 已有能力，迁移时必须保留

- 用户注册、登录、退出；小黑猫登录/注册界面及现有视觉风格。
- MP4 预签名直传 MinIO、上传确认、MySQL Outbox、Kafka 异步转码、FFmpeg 多清晰度 HLS 和封面。
- 公开发现、关键词搜索、最新/热门排序、视频播放。
- 点赞、收藏、平铺评论、关注、观看历史。
- 我的投稿、修改标题/简介/可见性、删除投稿、转码进度和错误反馈。

这部分是回归基线，不重复计为第五阶段新增成果。完整接口入口见 [router.go](../../../backend/internal/server/router.go)，已有媒体处理说明见 [backend/README.md](../../../backend/README.md)。

### 1.2 本阶段必须交付

| 编号 | 能力 | 用户可观察的结果 |
|---|---|---|
| F01 | Vue 3 前端迁移 | 既有业务在组件化页面中正常运行，保留小黑猫设计 |
| F02 | URL 路由与分享 | 视频、创作者主页可复制链接、直接打开、刷新及前进后退 |
| F03 | 持久登录和设置 | 刷新恢复登录；可改昵称、简介、密码；退出和改密使相应会话失效 |
| F04 | 公开创作者主页 | 展示资料、公开视频、关注/粉丝数量及分页关系列表 |
| F05 | 分区标签与投稿补齐 | 投稿可选分区、最多 5 个标签及公开/私密；旧投稿继续兼容 |
| F06 | 关注流与规则推荐 | 用户能看关注作者作品，视频详情有相关视频 |
| F07 | Redis 实际应用 | 分布式限流、分区缓存、可重建的日榜/周榜 |
| F08 | 回复与通知 | 评论有一层回复、分页；回复、评论、关注和处置结果有站内通知 |
| F09 | 举报和治理 | 举报处理、隐藏/恢复内容、禁用/恢复账号、管理员操作审计 |
| F10 | 数据中心和续播 | 7/30 天趋势、视频表现、播放口径说明及断点续播 |
| F11 | 部署与自动化验证 | 开发启动、构建部署、CI、真实依赖集成测试和浏览器 E2E |
| F12 | 简历证据 | 架构图、故障演练、可复现压测、演示流程和技术决策说明 |

### 1.3 明确后置

邮箱/手机验证码和找回密码、头像上传、第三方/扫码登录、设备管理界面、私信、动态、弹幕、字幕、分 P/合集、定时发布、断点续传、上传自选封面、强制投稿前审核、机器审核、支付/会员/广告/直播/电商、微服务、Elasticsearch、Kubernetes。

本阶段提供登录态改密，不提供忘记密码找回。公开主页先使用昵称首字或既有默认头像；不得在 UI 中展示无法工作的上传头像或找回密码按钮。

## 2. 技术栈与职责

| 层 | 采用方案 | 职责与边界 |
|---|---|---|
| 前端 | Vue 3、TypeScript、Vite、Vue Router、Pinia | 单文件组件、路由、类型、共享状态；不把全部表单状态放 Pinia |
| 播放器 | HLS.js、HTMLVideoElement | 播放、清理媒体资源、进度及有效观看采集 |
| 后端 | Go、Gin、GORM、Goose | 模块化单体、鉴权、事务、迁移；API 与媒体 Worker 独立进程 |
| 数据 | MySQL | 用户、视频、会话、关系、通知、举报、指标的权威来源 |
| 缓存与限流 | Redis、go-redis/v9 | 可重建缓存/榜单和共享限流；不单独保存永久业务关系 |
| 消息 | Kafka、现有 Outbox | 继续服务转码；不使用 Redis Pub/Sub 替代可靠任务投递 |
| 媒体 | MinIO、FFprobe、FFmpeg | 私有对象存储、校验、转码、封面 |
| 测试 | Go testing、Vitest、Vue Test Utils、Playwright | 服务逻辑、真实存储事务、组件与真实浏览器 |
| 部署 | Docker Compose、Nginx | 静态构建、同源代理、HTTPS 与深链回退 |
| CI 与性能 | GitHub Actions、k6 | 自动构建测试、API 负载测试与报告 |

版本策略：Go 跟随 go.mod；Node 使用已验证的 22.22.3 或经兼容测试后的受支持版本，开发、CI、镜像一致。Vue 生态依赖使用互相兼容的版本组合，提交 package-lock.json；Redis/Nginx 固定已验证版本和部署镜像，避免 latest 漂移。TypeScript 必须运行 vue-tsc，不能只用 Vite 构建代替类型检查。

Prometheus/Grafana 属于可选增强；本阶段先交付请求延迟、缓存命中、转码耗时等日志/计数和报告，监控平台不作为新增业务前置条件。

## 3. 部署与信任边界

开发链路：

    浏览器 -> Vite :5173 -> /api 代理 -> Go API :8081
    浏览器 -> 预签名 URL -> MinIO
    Go API -> MySQL / Redis
    Outbox Dispatcher -> Kafka -> Worker -> FFmpeg / MinIO

部署链路：

    浏览器 -> Nginx HTTPS
               ├─ Vue dist 静态资源与页面路由
               └─ /api/v1 -> Go API
    视频上传/分片访问 -> 受控存储域名、短时预签名地址
    MySQL / Redis / Kafka -> 内部网络

- Node 用于前端开发和构建；部署不要求额外运行旧 Node 静态代理服务。
- Vite/Nginx 迁移时逐项保留旧 server.mjs 的同源边界、安全响应头、请求大小限制、媒体跳转处理和敏感文件访问限制。
- 仅应用页面路径回退 index.html；错误 API、缺失静态文件和路径穿越不能返回首页。
- Cookie、Origin 和 CSRF 相关头必须正确经过代理；不可把任意请求 Origin 改写为可信值。
- Cookie Secure 在生产开启；本机 HTTP 仅允许显式开发配置关闭。MinIO 浏览器访问地址必须匹配环境，生产不向浏览器返回容器内部主机名或 localhost。
- 数据库、Redis 和 Kafka 不直接暴露为公开互联网服务。

## 4. 必须遵守的业务规则

### 4.1 兼容性与权限

1. 保留 /api/v1 现有响应信封和错误结构。新增字段采用兼容扩展；旧评论不带 parent_id 时仍是根评论。
2. 新前端要求选择分区；旧客户端省略 category_id 时归入“未分类”，不破坏既有投稿接口。
3. 历史视频迁移为可见，历史评论迁移为根评论。旧指标不伪造为新系统的历史趋势。
4. 公开内容必须同时满足：作者正常、视频 ready、public、治理 visible、未删除。发现、主页、相关推荐、榜单、收藏/历史公开投影和媒体入口使用一致规则。
5. 作者通过专属接口查看自己的私密内容；其他用户及公共缓存不得获取其私密信息。
6. 管理员权限以数据库当前角色为准；JWT 不携带可永久信任的授权快照。路由守卫只控制界面，最终权限由后端判定。

上述条件是阶段最终合同。T07 引入治理字段前，T04–T06 使用已有状态与权限条件；T07 迁移完成后统一补上治理过滤，不提前引用尚不存在的数据库列。

### 4.2 刷新会话与退出

- 登录创建独立 session family；访问 JWT 有效期 15 分钟，携带 sid；刷新令牌使用 32 字节加密安全随机数，数据库只存 SHA-256 摘要。
- family 最长有效期 30 天；轮换不无限延长绝对到期时间。
- 刷新 Cookie 为 host-only、HttpOnly、SameSite=Lax、Path=/api/v1/auth；生产加 Secure。访问 JWT 留在 Pinia 内存，不持久化到浏览器存储。
- 每次需要登录的请求，以及可选登录请求使用的身份，都验证 sid 未撤销、未到期和用户正常。旧无 sid 令牌升级后要求重新登录。
- 刷新事务先锁 family，再校验当前 token，原子标记旧令牌已轮换并创建新令牌；退出锁同一 family 并撤销，改密撤销该用户所有 family。
- 同页面使用 single-flight 合并刷新；跨标签页使用 Web Locks 协调，取得锁后由浏览器发送当前 Cookie。对不支持 Web Locks 的环境保留服务器并发保护。
- 已轮换令牌在 10 秒冲突窗口内重复出现，返回 409 REFRESH_CONFLICT，不清 Cookie、不撤销 family；客户端退避后重试，最多 2 次。窗口外再次使用旧令牌，撤销该 family，返回 401 SESSION_REUSED。
- 非法令牌、已撤销令牌、过期令牌均不能刷新。已退出 family 不享有冲突窗口。
- 丢失成功刷新响应且浏览器没有拿到新 Cookie 时，本版允许要求重新登录；不得为了恢复而在日志或数据库保存明文令牌。
- 网络错误或 5xx 不自动当成用户登出。退出期间的迟到响应不得恢复前端身份；重新附着的已撤销 Cookie 也必须被后端拒绝。
- 登录、刷新、退出和其他 Cookie 驱动写操作验证配置的精确 Origin，并要求同源自定义 CSRF 请求头和正确 Content-Type；CORS 不允许任意来源携带凭证。程序测试显式提供可信 Origin。
- 不统一自动重试所有业务 POST：只对已知在执行业务前被鉴权拒绝的请求刷新后重试一次；超时后的写请求遵循各自幂等规则。

### 4.3 Redis 缓存、榜单和故障策略

首期缓存仅覆盖分区列表与榜单候选 ID；视频详情、播放签名、权限、viewer_state 暂不放共享缓存。公开创作者资料缓存和通知未读计数列为本阶段完成后的可选优化。

| 键/用途 | 规则 |
|---|---|
| video_share:cache:categories:v1 | 启用分区列表；TTL 300 秒加 0–60 秒抖动，修改分区后删除 |
| video_share:rank:day:日期 | 日榜 Sorted Set；存视频 ID 和分数，从 MySQL 指标重建 |
| video_share:rank:week:日期 | 含当天的最近 7 个自然日榜，统计时区 Asia/Shanghai |
| video_share:rate:用途:主体 | Lua 实现原子读判定更新，所有键带有限 TTL |

- 榜单评分初版：有效播放次数 + 3 × max(净点赞, 0) + 5 × max(净收藏, 0) + 2 × max(净评论, 0)。负净变化原样保留在趋势数据中，仅计算榜单时截为 0。
- 每 60 秒重建当前日榜和周榜，临时键写完后原子替换，榜单 TTL 180 秒；旧日期键最多保留 8 天。空榜可回退最新公开视频，不制造虚拟热度。
- Redis 只提供候选 ID；响应前始终从 MySQL 过滤权限并获取当前公开字段。隐藏、删除或转私密之后，即使候选 ID 仍在 Redis 也不能展示。
- 缓存写入在 MySQL 提交成功后进行；缓存失败不回滚已提交业务。删除分区缓存失败时记录并告警，最多在 TTL 内收敛。
- 缓存和榜单读失败：短超时，回源 MySQL，合并热点并发回源，限制回源并发；数据库饱和时返回明确 503。
- 限流读失败：登录/注册使用明确的 503 RATE_LIMIT_UNAVAILABLE；评论/举报可使用更严格的进程内应急限流，同时记录降级状态。应急限流不宣称保持全局配额。
- Redis 可重建，不作为 session family 或互动关系的唯一存储；Redis 故障本身不导致已登录用户失去身份。
- 有效缓存优化必须测量命中率和数据库查询次数；不给“接入 Redis 即性能提升”的无数据结论。

限流初始配置建议：登录每 IP 10 次/分钟，注册每 IP 5 次/10 分钟，评论每用户 10 次/分钟，举报每用户 5 次/10 分钟。返回 429 和 Retry-After；这些是可配置项目默认值，需按压测和真实使用调整。只信任已配置反向代理的客户端 IP 头。

### 4.4 评论、通知与治理

- 界面只有根评论和一层回复。parent_id 指实际被回复评论，root_id 指根评论；回复回复仍显示在同一根评论下。
- 回复目标必须同视频且当前可见。根评论与回复分别分页，默认 20 条，最多 50；顺序固定包含 ID 次级排序。
- 根评论删除或隐藏后，正文显示删除占位；仍可见回复可以保留，但不能再回复该根评论。隐藏单条回复不影响其他回复。
- 通知：新评论 -> 视频作者；回复 -> 被回复者和视频作者，接收人去重并排除操作者；新增关注 -> 被关注者；举报处理 -> 举报者；内容/账号处置 -> 目标用户。
- 通知与业务写入同事务完成。创建评论、举报、管理操作携带 request_id，按操作者/动作/请求 ID 去重；已存在的关注关系不会再次触发通知。
- 通知存结构化事件，不长期复制评论正文；目标失效后显示“内容不可用”。已读操作校验接收人，未读数首版以 MySQL 为准。
- 通知使用前台页面 30 秒轮询，页面隐藏停止；不要求 WebSocket。
- 举报类型 video/comment/user；原因 spam/abuse/copyright/illegal/other，详情最多 500 字。同一举报者同目标只能有一条未关闭举报；并发通过数据库约束或锁保证。
- 举报状态 open -> investigating -> resolved/rejected；允许 open 直接结案。resolved 必须引用对应处置，rejected 必须填写理由；终态重复请求返回原结果，不能重复处罚。
- 内容状态 visible <-> hidden，作者删除为另一维度的终态；恢复治理状态不得复活已删除内容。
- 用户状态沿用 normal/disabled；管理员不能禁用自己或最后一个正常管理员。禁用账号时在同一事务撤销其全部 session family，恢复账号不恢复旧会话，用户须重新登录。
- 所有管理动作写追加式审计：操作者、目标、动作、理由、前后状态、request_id、时间；普通用户无读取权限。
- 本阶段是举报后人工治理，不宣称完成法律合规、自动版权检测或发布前审核。

媒体边界：隐藏内容后阻止新的清单、封面及签名获取；已经发出的 MinIO 分片预签名地址在到期前仍可能可用。记录并验证实际签名有效期，不承诺瞬时撤回已签地址。若未来要求立即撤回，另立任务改为每次媒体访问都校验权限的受控网关。

### 4.5 观看进度、有效时长与统计口径

必须区分四种指标，不能用进度条差值冒充真实观看时长：

| 指标 | 定义 |
|---|---|
| 旧累计播放量 | 保持现有每登录用户/视频一次的计数语义，继续兼容旧字段 |
| 新有效播放次数 | 新观看 session 连续累计获得至少 min(3 秒, 视频时长) 的有效时长，计一次 |
| 有效观看时长 | 播放器实际播放且页面可见时的墙上时间，暂停/等待/拖动不计；不乘倍速 |
| 有效完成播放数 | session 自然结束、位置距结尾不超过 2 秒且累计有效时长达到视频时长 80%，每 session 一次 |

- 以上是演示分析口径，不是广告结算或完整反作弊能力。只统计登录用户，不伪造匿名观众行为。
- POST 创建观看 session；心跳带递增 seq、position_ms、watched_delta_ms、ended。客户端串行发送，单个心跳最多累计 15 秒；seek/waiting/pause/hidden 重置活动采样基准。
- 服务端以媒体元数据校验时长，锁 session 以及用户/视频对应观看记录。接受增量不超过客户端活动增量、服务端经过时间和 15 秒上限中的最小值。
- 对同一用户/视频维护共享计时上限，防止两个标签页同时播放重复累计墙上时间；该上限跨 session 生效。
- 重复或更小 seq 不重复计数；允许序号跳跃但丢失采样不补估时长。错误 duration、负数、超大增量、越界位置必须拒绝或明确裁剪。
- 第一次达到有效播放阈值时，计入该 session 累计有效时长；之后只加已确认增量。session 最长 24 小时，统计归属日期为首次有效播放的 Asia/Shanghai 日期，跨午夜仍归同一 cohort 日期，避免日完成率分子分母错配。
- 新完成率 = 有效完成播放数 / 有效播放次数，分母 0 显示“—”。它可能随倍速和上述定义变化，页面提供口径提示。
- 点赞/收藏/评论/粉丝趋势记录每日净变化，允许负数；累计总数单独读取当前关系和状态。对重复幂等操作不新增指标。
- 新增 daily 指标自上线日起积累；旧数据可展示累计总数，但历史趋势注明“尚未采集”，不从当前总量倒推。
- 旧 /watch 接口继续保存旧进度/累计播放，不写新的 session 分析；Vue 完成迁移后每次播放仅使用新 session 接口，后端在其中同步旧历史和累计播放。
- 断点续播读取本人进度，媒体元数据就绪后只恢复一次；临近结尾从 0 开始，短视频不能产生负 seek。切换视频或退出后迟到请求不能覆盖当前播放状态。
- 用户手动拖到结尾不能凭位置增加有效时长或完成播放；自然播放、暂停、倍速、缓冲、重复心跳和多标签页均必须测试。

## 5. 数据结构规划

现有迁移到 000005。以下编号按本次基线规划；若实施前已有新迁移占用，则顺延，不修改已在其他环境执行的历史迁移。

| 迁移 | 新增或扩展结构 | 关键约束 |
|---|---|---|
| 000006_sessions_profiles | users.bio/role；session_families、refresh_tokens | token_hash 唯一；family.user_id 外键；family 绝对到期及撤销时间；token 轮换时间 |
| 000007_taxonomy | categories、tags、video_tags；videos.category_id | 分类标识唯一；标签规范化键唯一；video/tag 复合主键；旧数据未分类 |
| 000008_notifications_replies | comments.parent_id/root_id；notifications、operation_receipts | 同视频回复校验；接收人/事件唯一；操作者/动作/request_id 唯一且校验请求摘要 |
| 000009_moderation | videos/comments.moderation_status；reports、moderation_actions | 同举报者同目标唯一活动举报；审计追加；处置和状态同事务 |
| 000010_metrics | watch_sessions、video_daily_metrics、creator_daily_metrics；watch_histories.last_watch_credit_at | session 归属 user/video；seq 去重；日表复合主键；净变化使用有符号整数 |

字段合同：

- session_families：id、user_id、created_at、absolute_expires_at、revoked_at；refresh_tokens：id、family_id、token_hash、created_at、expires_at、rotated_at。
- categories：id、slug、name、enabled、sort_order；tags：id、display_name、normalized_name；video_tags：video_id、tag_id、created_at。
- notifications：id、recipient_id、actor_id(可空)、event_key、type、video_id/comment_id/report_id(可空)、read_at、created_at；索引 recipient_id/read_at/id。
- 000008 中 report_id 先作为可空预留列；举报事件及其关联约束在 000009 创建 reports 后接入，避免迁移依赖尚不存在的表。
- operation_receipts：actor_id、action、request_id、request_hash、resource_id、created_at；重复请求体不一致返回 409，不把它当新操作。
- reports：id、reporter_id、target_type、target_id、reason_code、detail、status、assigned_to、resolution_reason、action_id、created_at、closed_at；多态目标由服务事务校验。
- moderation_actions：id、actor_id、target_type、target_id、action、reason、before_state、after_state、request_id、created_at。
- watch_sessions：id、user_id、video_id、created_at、expires_at、last_seq、last_received_at、credited_ms、qualified_at、completed_at、cohort_date；播放位置仍写 watch_histories。
- video_daily_metrics：video_id、stat_date、effective_views、watch_time_ms、completions、net_likes、net_favorites、net_comments；creator_daily_metrics：creator_id、stat_date、net_followers。

每个迁移有 Up/Down 测试和旧数据回填测试。生产回滚优先应用版本回退和向前修复，不在已有业务数据的库上随意执行删除表的 Down。

## 6. 接口与页面合同

所有分页默认 20、上限 50，除非另有说明；沿用已有响应信封。下表仅列新增与变更部分，其他第四阶段 API 保留。

| 领域 | 接口 | 鉴权与结果 |
|---|---|---|
| 会话 | POST /auth/refresh、POST /auth/logout | Cookie/Origin/CSRF；刷新返回访问 JWT；退出幂等 |
| 设置 | PATCH /users/me；POST /users/me/change-password | 登录；昵称 1–64 字、简介最多 200 字；改密校验旧密码并沿用原密码规则 |
| 主页 | GET /users/:id、/users/:id/videos | 可选鉴权；公开资料、关系计数和公开视频 |
| 关系 | GET /users/:id/followers、/users/:id/follows | 公开正常用户投影；不泄露凭证或会话 |
| 分类 | GET /categories；扩展 GET /videos | category_id、tag、q、sort、page 参数；tags 首版无独立搜索服务 |
| 投稿 | 扩展 POST /videos 和 PATCH /users/me/videos/:id | 接收 category_id、tags、visibility；旧参数继续有效 |
| 分发 | GET /feed/following；GET /videos/:id/related | 前者登录；后者公开，默认最多 6 条，候选不足就少返回 |
| 榜单 | GET /videos/ranking?window=day或week | 最多 50 条；返回生成时间与榜单口径；过滤不可见候选 |
| 评论 | 扩展 POST /videos/:id/comments；GET /comments/:id/replies | 写操作带 parent_id 和 request_id；根评论/回复分页 |
| 通知 | GET /notifications；PATCH /notifications/:id/read；POST /notifications/read-all | 登录；只能处理本人通知，返回未读数 |
| 举报 | POST /reports；GET /users/me/reports | 登录；结构化原因、本人处理进度 |
| 管理 | GET /admin/reports；PATCH /admin/reports/:id | 管理员；接单、驳回、带处置结案，乐观状态检查防止两人重复处理 |
| 治理 | POST /admin/videos/:id/hide或restore；/admin/comments/:id/hide或restore；/admin/users/:id/disable或enable | 管理员；reason、request_id、可选 report_id；全程审计 |
| 审计 | GET /admin/moderation-actions | 管理员分页查看动作记录 |
| 观看 | POST /videos/:id/watch-sessions；POST /watch-sessions/:id/heartbeat | 登录；session 必须属于本人及可访问视频 |
| 数据 | GET /users/me/creator/summary?days=7或30 | 本人；当前累计、日趋势、前 10 个视频及口径版本 |

以上路径全部带 /api/v1 前缀。固定 /videos/ranking、/users/me 与动态 ID 路径需要路由测试，避免被 :id 误解析。

Vue 路由：

    /                         发现页（筛选条件进 query）
    /login                    登录/注册（内部跳转 returnTo 必须校验）
    /video/:id                视频详情及分享
    /creator/:id              公开创作者主页
    /following                关注视频
    /ranking                  日榜/周榜
    /upload                   投稿
    /me                       我的投稿/收藏/历史/关注
    /settings                 账号设置
    /notifications            通知
    /creator-center           创作者数据
    /admin/reports            举报工作台
    /admin/audit              处置记录

分享链接只包含页面 URL，不包含 JWT、临时媒体签名或内部存储地址。通知跳转目标已失效时提示“内容不可用”，不能通过通知绕过权限。

## 7. 实施顺序和任务包

采用可独立演示的小批交付。每个任务包遵循：先定义失败用例 -> 实现 -> 局部验证 -> 记录验收证据 -> 再进入依赖它的任务。这里只制定任务，不授权部署到外部环境或发布远端代码。

### T00：基线与工具版本

**依赖：** 无。  
**交付文件：** docs/stage5/baseline.md、docs/stage5/decisions.md、前端依赖锁文件与版本配置。

- [x] 记录 main 提交、未提交变化、Node/Go/Docker/FFmpeg 版本，识别当前运行服务的代码目录。
- [x] 运行既有前后端测试，记录实际通过/失败/跳过项，不把旧报告当本次结果。
- [x] 为注册、登录、发现、上传、播放、点赞收藏、评论、关注和作者管理录制或记录现有回归路径。
- [x] 在 decisions.md 记录 Vue/TypeScript、Redis 首期范围、会话、统计和治理口径。
- [x] 固定依赖版本及开发/测试/部署端口；不把旧 .worktrees 目录当第五阶段主项目。

**验收：** 无未解释的基线失败；存在明确的环境说明、已有能力清单和迁移回归表。

### T01：Vue 工程与既有业务迁移

**依赖：** T00。  
**文件：** frontend/package.json、vite.config.ts、tsconfig.json、src/main.ts、App.vue、router/index.ts、stores/auth.ts；views 下的 LoginView.vue、DiscoverView.vue、VideoDetailView.vue、UploadView.vue、ProfileView.vue；components 下的 AppHeader.vue、BlackCat.vue、VideoCard.vue、VideoPlayer.vue、CommentList.vue；api 下的 client.ts、auth.ts、video.ts、community.ts；frontend/tests/unit 与 tests/e2e。

- [x] 建立 Vue 3 + TypeScript + Vite + Router + Pinia、vue-tsc、Vitest/Vue Test Utils 和 Playwright。
- [x] 复用并逐步迁移现有 API 校验逻辑和样式；先登录，再发现/详情，最后投稿/个人中心。
- [x] HLS 实例、事件监听、请求取消、轮询计时器在组件卸载/换视频时清理。
- [x] 建立内部路径导航和 returnTo 校验；视频支持复制页面链接，浏览器返回保留搜索/分页状态。
- [x] Vite 代理 /api，迁移旧 server.mjs 需要的安全/媒体行为；旧服务停止承担主入口后再删除无用实现。
- [x] 纯函数旧 node:test 在迁移期可保留，逐项迁移而不以删除测试换取通过；最终 npm test 覆盖所有保留测试。
- [x] 保留小黑猫、键盘导航、焦点、窄屏和 reduced-motion；Vue v-html 不渲染用户输入。

**验收：** 原有全流程回归通过；深链刷新/前进后退、快速切换视频、上传失败反馈通过；npm run typecheck、test、build 成功。本任务保留原登录语义，持久会话在 T03 验收。

### T02：Redis 连接、分布式限流与故障边界

**依赖：** T00，可在前端迁移完成后实施。  
**文件：** backend/internal/cache/redis.go、cache_test.go、redis_integration_test.go；middleware/redis_ratelimit.go 及对应测试；internal/config、server/router.go；backend/deploy/docker-compose.yml、.env.example。

- [x] 增加 Redis 服务、健康检查、go-redis 客户端、连接/命令超时和关闭处理；凭证从环境读取。
- [x] 实现第 4.3 节限流，Lua 原子执行，所有状态键过期。
- [x] 用两个 API 实例连接同一 Redis，验证合并后的配额，而非只测单进程。隔离 Compose 两 API 进程交替提交 11 次登录请求，前 10 次通过 CSRF guard，合并配额在第 11 次返回 429 和 `Retry-After`。
- [x] 验证 429/Retry-After、不同用户隔离、未配置代理头不能伪造 IP。
- [x] 关闭 Redis，分别验证普通查询、身份、登录限流、评论/举报的预定行为。
- [x] 增加缓存/限流超时和降级计数；Redis 故障不对所有 API 统一报同一个可用性状态。

**验收：** 真实 Redis 集成测试通过；跨实例限流和故障恢复可复现；此任务不改点赞等永久数据写入路径。

### T03：持久会话与账号设置

**依赖：** T01、T02。  
**文件：** 000006 迁移；backend/internal/session 的 model/repository/service/handler/dto 及测试；user、token、middleware、config、server；frontend/stores/auth.ts、api/client.ts、views/SettingsView.vue。

- [x] 实现 family/token 数据结构、事务轮换、sid 校验、退出/改密撤销。
- [x] 前后端落实第 4.2 节冲突窗口、single-flight、跨标签协调、CSRF 和可信代理。
- [x] 资料编辑昵称/简介；改密沿用 bcrypt 输入限制，不擅自改变既有密码规范。
- [x] 测试两个并发刷新、窗口内冲突、窗口外旧 token 重放、过期、退出竞态、改密后旧 JWT 和 Cookie。
- [x] 测试浏览器刷新恢复、网络暂时失败不登出、重试次数上限和重放写请求规则。

**验收：** 退出后旧 sid 的受保护请求立即失败；两标签页正常刷新不互相踢出；生产 Cookie 属性和跨源拒绝测试通过；没有凭证进入日志/浏览器持久存储。

### T04：公开主页与关系展示

**依赖：** T03。  
**文件：** backend/internal/creator 的 dto/repository/service/handler 及测试；follow 查询扩展；frontend/views/CreatorView.vue、components/CreatorCard.vue、api/creator.ts。

- [x] 提供公开资料、公开视频、粉丝/关注分页及计数；计数过滤禁用账号。
- [x] 视频作者名称、评论作者和关注列表跳转公开主页。
- [x] 主页支持关注/取关，操作完成同步当前相关界面状态。
- [x] 测试自己/他人/游客、空作品、私密作品、禁用用户和不存在 ID。

**验收：** 创作者链接可分享；不需要登录也能看公开信息；私密作品/密码散列/会话字段不外泄。

### T05：分区标签、关注流和相关推荐

**依赖：** T02、T04。  
**文件：** 000007 迁移；backend/internal/taxonomy 的 model/repository/service/handler/dto 及测试；video 查询/写入模块；frontend/views/FollowingView.vue、components/CategoryFilter.vue、TagInput.vue；api/taxonomy.ts。

- [x] 预置未分类、生活、知识、科技、游戏、音乐等演示分区；分类启用状态和展示顺序落库。
- [x] 标签去首尾空白、Unicode 规范化和大小写标准化，1–20 个字符、最多 5 个，事务去重；测试中文、重音字符和并发新建。
- [x] 投稿与编辑支持分区、标签、visibility；旧请求不带新增字段仍成功。
- [x] 发现页支持筛选；关注流仅展示当前关注的正常作者公开作品。
- [x] 相关推荐按共同标签数、同分区优先、旧累计播放/点赞、发布时间、ID 排序，明确为规则推荐。
- [x] 分区实现第 4.3 节缓存，测试命中、失效、冷启动并发和 Redis 故障回源。

**验收：** 旧视频和旧客户端回归通过；相关推荐最多 6 条且无自身/私密/重复内容；测试数据不足时正确展示少量或空状态。

### T06：评论回复、通知和未读状态

**依赖：** T03、T04、T05（保证 000007 先于 000008 执行）。  
**文件：** 000008 迁移；backend/internal/notification 的 model/repository/service/handler/dto 及测试；engagement/follow 事务扩展；frontend/views/NotificationsView.vue、components/CommentThread.vue、stores/notifications.ts、api/notification.ts。

- [x] 按第 4.4 节实现两层展示、独立分页、删除占位和同视频约束。
- [x] 为 POST 幂等请求建立 operation_receipts，保存请求摘要和资源 ID；冲突请求体返回 409。
- [x] 评论/关注、通知、必要计数同事务提交；定义固定锁顺序，死锁只对有幂等键的操作有限重试。
- [x] 通知接收人去重、自通知排除、所有权校验、未读/全部已读、目标失效提示。
- [x] 前台轮询、隐藏暂停；logout 时清空通知并忽略迟到结果。

**验收：** A 评论、B 回复、C 为视频作者的通知矩阵全部匹配；重放请求不重复评论/通知；通知读取失败不会把未读数无提示地改为 0。

### T07：举报、管理角色、处置与审计

**依赖：** T03、T06。  
**文件：** 000009 迁移；backend/internal/moderation 的 model/repository/service/handler/dto 及测试；middleware/require_admin.go；API CLI 管理员引导子命令；video/engagement/follow 公共查询；frontend/views/admin/ReportsView.vue、AuditView.vue、components/ReportDialog.vue、api/moderation.ts。

- [x] 普通用户可举报并查看自己的处理进度；管理员工作台接单、查看目标、填写原因、驳回或带处置结案。
- [x] 提供显式 CLI 命令给已有用户授予/撤回管理员角色；无默认管理员密码、无公开自助提权路由。
- [x] 执行隐藏/恢复、禁用/恢复、通知、计数调整和审计事务；禁止恢复已删内容。
- [x] 对所有公开查询及 HLS/封面入口接入统一可见性条件，补缓存命中下的权限测试。
- [x] 测试越权、自禁用、最后管理员保护、重复处置、两管理员同时接单和失败回滚。
- [x] 演示关闭内容后新媒体请求被阻止，测量已签名对象访问的残余有效窗口。

**验收：** 普通用户直接请求管理 API 为 403；每个成功处置有唯一审计记录和正确状态；隐藏内容从发现/主页/推荐/关注/历史等入口统一消失或显示不可用。

### T08：有效观看、数据中心与断点续播

**依赖：** T05、T06、T07。  
**文件：** 000010 迁移；backend/internal/analytics 的 model/repository/service/handler/dto 及测试；creator 汇总；engagement/follow 事务指标；frontend/composables/useWatchSession.ts、VideoPlayer.vue、views/CreatorDashboardView.vue、api/analytics.ts。

- [x] 完整落实第 4.5 节 session/seq/时钟上限；采用可注入时钟测试，不依赖测试真实等待。
- [x] 点赞/收藏/评论/粉丝发生真实关系变化时写同事务净增量；上线前旧状态继续可取消且趋势允许负数。
- [x] 汇总 7/30 天当前总数、日趋势、Top 10；不同指标日期和旧/新口径在 API 元数据及页面说明。
- [x] 断点在 loadedmetadata 后恢复一次；短视频、临近结束、切视频、退出和请求乱序有用例。
- [x] 测试自然播放、拖到结尾、暂停/缓冲、隐藏标签、多倍速、多标签、重放心跳、session 过期、跨午夜和零分母。
- [x] 首版用表格、进度条与轻量图形展示；无需增加大型 BI 服务。

**验收：** 拖动进度不增加有效时长，重复 seq 不重复计数；完成率有明确分母且不超过 100%；无历史采集数据不显示虚构趋势；播放历史能正确续播。

### T09：Redis 日榜/周榜与重建

**依赖：** T02、T05、T07、T08。  
**文件：** backend/internal/ranking 的 repository/service/handler/rebuild 及测试；cmd 下榜单重建命令；frontend/views/RankingView.vue、api/ranking.ts；Redis 集成测试。

- [x] 从 MySQL 日指标计算分数，ZSET 批量写临时键，原子替换；用明确时区生成日期。
- [x] 采用单独、可重复执行的重建进程/命令，60 秒周期；多进程时使用带所有者校验与超时的构建锁或明确单实例约束。
- [x] 相同分数使用固定次级顺序；分页只承诺同一生成快照下稳定，重新生成时间随响应返回。
- [x] 命中榜单后仍从数据库过滤候选，补足到请求数量或确实无更多内容；结果空时展示合规空状态。
- [x] 缓存丢失、Redis 重启、过期、重建中崩溃分别测试，旧完整榜或 MySQL 回源可用。
- [x] 压测榜单命中/未命中，记录 SQL 次数及可见性过滤成本。

**验收：** Redis 清空后可完全从 MySQL 重建；隐藏视频不因榜单缓存出现；没有只写 Redis 导致永久热度数据丢失的路径。

### T10：部署、CI 与文档

**依赖：** T01–T09。  
**文件：** frontend/Dockerfile、backend/deploy/nginx.conf、docker-compose.yml 及演示配置；.github/workflows/ci.yml；docs/stage5/deployment.md、api-contract.md、architecture.md；相关 README。

- [x] Vue 构建为 dist，Nginx 提供静态/深链/API 代理；媒体域名、CORS、Cookie、CSP、请求限制逐项演示验证。当前 API/Worker/ranking/frontend 镜像已全部重建；Nginx API IP 变化后代理仍返回 200，33,000 字节请求返回 413，MinIO CORS、CSP 和 refresh Cookie 属性均已验证。
- [x] Compose 编排 MySQL、Redis、MinIO、Kafka、API、Worker、前端服务，分开开发测试与演示配置。
- [x] 迁移作为显式步骤单独执行；备份恢复演练只操作一次性测试数据，验证账号和视频元数据可恢复。
- [x] CI 分前端类型/单元/构建、Go 单元/vet/build、真实依赖集成、浏览器 E2E；Linux runner 可用条件下增加 Go race 检查。
- [x] 配置测试所需 MySQL/Redis/Kafka/MinIO 与 FFmpeg；环境缺失明确失败，不把跳过当验收通过。
- [x] 文档提供环境变量名称、示例非秘密值、启动停止命令、媒体公共地址、管理员引导和故障定位。
- [x] 在线演示发布作为交付后的独立操作；可先完成本地同等部署验收，不要求本任务购买服务器。

**验收：** 新环境按 README 能启动；深链刷新不 404；CI 可重复运行；仓库和构建产物无真实密码、令牌或私人媒体。

### T11：端到端与故障演练

**依赖：** T10。  
**文件：** backend/scripts/e2e-stage5.ps1、frontend/tests/e2e/stage5.spec.ts、docs/stage5/acceptance.md、failure-drills.md。

- [x] 创建隔离作者/观众/管理员测试身份，完成注册—登录—刷新—资料—投稿—转码—发现—分享—互动—通知—举报—处置—恢复—续播—退出。
- [x] 保留既有转码/社区脚本并完成兼容回归；新增 PowerShell 脚本在 Windows PowerShell 5.1 实际解析执行验证，使用 ASCII 或 UTF-8 BOM。
- [x] 演练 Kafka 暂停再恢复，观察 Outbox 待发记录、重试与发布结果。已观测到 pending、`last_error`、第二次 claim、恢复发布和清除错误；单次发布默认 5000 ms 超时由 `KAFKA_PUBLISH_TIMEOUT_MS` 配置。
- [x] 演练消息重复、Worker 中途退出与租约/任务恢复，确认不会重复发布或产生失控重试；Outbox claim 租约覆盖整批的有界发布时长。
- [x] 演练 Redis 不可用、热点过期、榜单重建、数据库事务失败和两管理员并发处置。
- [x] 记录每次演练的步骤、期望、实际、日志、修复和复测；发现基线缺陷时建立修复记录，不修改期望掩盖失败。
- [x] 只清理本次脚本标识的测试数据，不对主库执行全库清空。

**验收：** 核心路径实际运行通过；每个故障有恢复证据；高风险越权、数据丢失、重复写入问题没有未解决项。

### T12：压测、项目展示与简历材料

**依赖：** T11。  
**文件：** perf/k6/categories.js、ranking.js、engagement.js、hls-segment.js、run-stage5-bench.ps1；docs/stage5/performance.md、demo.md、resume-evidence.md、decisions.md、acceptance.md。

- [x] 固定测试环境：CPU/内存、镜像版本、数据规模、网络、并发模式、预热、持续时间、代码提交、随机性说明和错误定义。
- [x] 先 smoke，再基准负载，再递增压力；API 基线各持续条件至少重复 3 次。对相同隔离样本分别记录冷、热及 Redis 不可用时的 MySQL 回退；项目没有人为添加 cache-disable 开关，报告明确区分 Redis 故障等待和纯冷缓存。
- [x] 测量吞吐、P50/P95/P99、错误率、Redis 命中率、MySQL 全局 Questions 差值、CPU/内存；同时验证内容正确性和权限。
- [x] API 负载与 HLS 分片读取分开复测；报告限定本机/样片含义，不把 API QPS 宣称为整个平台或转码能力。
- [x] 转码单独记录固定样片时长/分辨率、排队耗时、claim-to-completion 和 Worker 并行配置。
- [x] 建立固定演示环境基线；自动化目标只要求请求无错误及业务 checks 通过，延迟结果保持为测量数据、不伪装成生产 SLO，也不预写“支持十万用户”“提升 90%”。
- [x] 准备约 6 分钟演示：小黑猫登录、投稿转码、主页分发、互动通知、治理、Redis 故障回退与性能证据。
- [x] 简历材料映射“实际实现 -> 测试或报告 -> 设计取舍”，只引用能由实现、原始结果和限制说明支撑的内容。

**验收：** 本地复跑命令与三组原始 k6 结果已记录；当前环境下基线、递增压力及 HLS 单分片读取全部通过。未测量的长稳态、公网/CDN 行为和 FFmpeg 纯处理吞吐仍明确列为限制，不写成既成能力。

## 8. 验收命令合同

以下是实施后必须提供的命令入口；目前缺少的脚本由对应任务实现。本次编写任务书不运行这些尚未完成的功能验收。

前端目录执行：

    npm ci
    npm run typecheck
    npm test
    npm run build
    npm run test:e2e

后端目录执行：

    go test ./... -count=1
    go vet ./...
    go build ./cmd/api ./cmd/worker
    go test -tags=integration ./... -count=1

Linux CI 具备 C 编译器和 CGO 条件时：

    go test -race ./... -count=1

仓库根目录执行：

    powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\backend\scripts\e2e-stage5.ps1

集成门禁：CI 与最终验收必须提供真实 TEST_MYSQL_DSN、测试 Redis/Kafka/MinIO 配置。增加 REQUIRE_INTEGRATION_TESTS=true 模式，关键依赖配置缺失或无法连接直接失败；普通开发模式允许明确跳过，但报告列为未验证。仅测试命令退出码 0 不足以证明数据库或媒体路径已运行。

测试矩阵：

| 场景 | 必须验证的结果 |
|---|---|
| 旧用户/视频/评论迁移 | 数据保留、旧 API 合同仍可用 |
| 两个标签页刷新、退出竞态 | 不误恢复已退出会话，不无限刷新 |
| 越权访问他人私密作品/通知/后台 | 明确拒绝，无敏感数据返回 |
| 同一点赞/关注/评论请求重试 | 关系、计数、通知都不重复 |
| 同视频跨标签心跳和 seek | 有效时长不翻倍，位置跳跃不当观看 |
| 隐藏内容且缓存命中 | 新页面/API 不泄露；旧签名窗口有准确说明 |
| Redis 关闭 | 各接口遵循独立降级策略，无整体无提示放行 |
| Kafka 暂停、重复消费、Worker 崩溃 | 任务状态可解释、恢复可验证 |
| 手机宽度/键盘/减少动画 | 核心操作可用，焦点与错误提示清晰 |

## 9. 里程碑与交付门槛

| 里程碑 | 包含任务 | 放行条件 |
|---|---|---|
| M0 基线 | T00 | 已有功能与环境可复现 |
| M1 前端与基础设施 | T01、T02 | Vue 等价迁移，Redis 限流和故障策略验证 |
| M2 账号与分发 | T03、T04、T05 | 会话、资料、主页、分类与推荐可演示 |
| M3 互动与治理 | T06、T07 | 回复、通知、举报、权限和审计闭环 |
| M4 指标与榜单 | T08、T09 | 统计口径一致，续播正确，榜单可重建 |
| M5 工程与展示 | T10、T11、T12 | CI、部署、真实 E2E、故障报告及性能证据 |

先完成一个里程碑再扩大功能范围。工作量按验收结果管理，不在缺乏实施速度基线时承诺固定日期；M1 完成后用实际迁移耗时估算余量。

每个里程碑至少交付：功能说明、代码变更、相关迁移、测试结果、演示步骤、已知限制。保持可构建的提交粒度；提交/合并按届时用户授权和仓库约定执行。

## 10. 简历材料的证据边界

完成并验证后，可围绕以下三条写项目成果：

1. 媒体处理可靠性：MySQL Outbox 与 Kafka 驱动转码，幂等领取、重试及 Worker 故障恢复；链接实际演练。
2. 社区一致性：事务维护互动关系、计数和通知，Redis 提供跨实例限流与可重建榜单；链接集成测试和缓存对比。
3. 身份与治理：刷新会话轮换/撤销、服务器权限验证、举报处置与审计；链接浏览器 E2E。

示例表达仅作为完成后的写作方向：

> 实现 Go + Vue 视频社区，覆盖投稿转码、公开分发、互动通知与举报治理；通过可复现故障演练验证消息重试及会话撤销，并对 Redis 缓存进行同条件性能对比。

最终简历补充真实测量数据、自己的职责和设计取舍。功能未完成时不得直接使用该示例宣称已实现；本地测试并发不等于真实用户规模，也不自动构成生产高可用能力。

## 11. 最终交付清单

功能与任务对应关系：

| 功能 | 实施任务 |
|---|---|
| F01 Vue 迁移 | T01 |
| F02 路由分享 | T01、T04 |
| F03 会话与设置 | T03 |
| F04 创作者主页 | T04 |
| F05 分区与投稿 | T05 |
| F06 关注流与推荐 | T05 |
| F07 Redis | T02、T05、T09 |
| F08 回复通知 | T06、T07 |
| F09 举报治理 | T07 |
| F10 数据与续播 | T08 |
| F11 部署验证 | T10、T11 |
| F12 简历证据 | T12 |

T00 为全部任务的基线准备；验收时将每项功能链接到实际测试结果与演示步骤。

- [x] Vue 既有功能等价迁移与新增页面。
- [x] Redis 真实用途、限流配额、缓存策略、榜单重建。
- [x] 五组版本化迁移、旧数据兼容与数据恢复记录。
- [x] API 合同、会话/指标/治理状态规则与权限矩阵。
- [x] CI、单元、真实依赖集成、浏览器 E2E、故障演练。
- [x] Compose/Nginx 环境、README、管理员准备与演示步骤。隔离 Compose 全镜像重建、迁移、健康检查、真实浏览器和部署边界验证通过。
- [x] 压测脚本、原始结果、性能分析和局限。
- [x] 架构图、转码/刷新/互动时序图、设计决策和简历证据。
- [x] 所有重大缺陷已修复复测；Stage 5 已推送至 `origin/main`。公网 HTTPS/生产数据/长稳态未测、性能仅代表本机隔离样本等范围限制见收尾复核。
- [x] 第 1.2 节 F01–F12 逐条对照验收，不能用“页面存在”代替业务通过；见 [收尾复核](../../stage5/final-review.md)。

## 12. 设计参考

以下为选型和设计参考，不是本项目的实测结果；实施锁版本时再次核验兼容性。

- [Vue 官方快速开始](https://vuejs.org/guide/quick-start)
- [Vue TypeScript 与类型检查](https://vuejs.org/guide/typescript/overview)
- [Vue 测试建议](https://vuejs.org/guide/scaling-up/testing)
- [Vue Router](https://router.vuejs.org/guide/) 与 [Pinia](https://pinia.vuejs.org/introduction)
- [Redis Go 客户端](https://redis.io/docs/latest/integrate/go-redis/)
- [Redis Cache Aside](https://redis.io/docs/latest/develop/use-cases/cache-aside/) 与 [分布式限流](https://redis.io/docs/latest/develop/use-cases/rate-limiter/)
- [OWASP 会话管理](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html)
- [OWASP CSRF 防护](https://cheatsheetseries.owasp.org/cheatsheets/Cross-Site_Request_Forgery_Prevention_Cheat_Sheet.html)
- [GitHub Go CI](https://docs.github.com/en/actions/tutorials/build-and-test-code/go)
- [k6 API 压测](https://grafana.com/docs/k6/latest/testing-guides/api-load-testing/)
