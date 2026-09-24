# video_share 后端

这是视频分享平台的 Go 后端。第三阶段已经把“上传 MP4 原文件后直接发布”升级为异步媒体处理链路：API 验证上传对象后，在同一个 MySQL 事务中创建转码任务和 Outbox 事件；Dispatcher 把事件可靠投递到 Kafka；独立 Worker 使用 FFprobe/FFmpeg 生成多清晰度 HLS 和封面，上传到私有 MinIO，最后发布视频。第四阶段在此基础上补齐社区互动：公开搜索发现、作者编辑与删除投稿、点赞 / 收藏、评论、观看历史和关注关系；第五阶段 T05 增加分区标签、关注流和规则相关推荐，T06 增加回复/通知，T07 增加举报治理与审计。

## 已实现的业务

- 注册、登录、JWT 鉴权和当前用户资料。
- 创建投稿，通过短时效预签名 URL 把 MP4 从浏览器直传私有 MinIO。
- 完成上传时校验对象大小和 Content-Type，并返回 `202 processing`。
- MySQL Outbox 与业务状态同事务写入，Kafka 暂时不可用时事件不会丢失。
- Kafka 消费、任务幂等领取、失败重试、最大尝试次数和最终失败状态。
- FFprobe 获取时长、分辨率和音轨；FFmpeg 生成封面以及不高于原视频的 360p、720p、1080p HLS 清晰度。
- 作者可查询 `uploading`、`processing`、`ready`、`failed`、处理进度和失败原因。
- 公开视频列表、详情、HLS 清单代理、分片临时地址和封面临时地址。
- 公开发现：按标题 / 描述关键字搜索，支持 `latest` 与 `popular` 排序。
- 作者可修改标题、简介、可见性、分区和标签，或删除自己的投稿。
- 发现支持分区与多标签筛选；登录用户可查看只包含已关注正常作者公开作品的关注流，详情页提供最多 6 条规则相关推荐。
- 点赞与收藏：复合主键加 `INSERT IGNORE`，重复写入幂等，返回最终状态与总数。
- 评论：发表（1-500 字符）、分页列表、仅作者本人可删除。
- 观看历史：记录播放进度，重复上报覆盖同一条记录。
- 关注关系：关注 / 取关幂等，禁止自关注，支持分页查询已关注用户。
- 举报治理：普通用户提交和查询自己的举报；管理员通过数据库实时角色校验接单、驳回、隐藏/恢复内容、禁用/恢复账号；所有处置写追加式审计并发送结构化通知。
- 请求 ID、结构化日志、限流、请求体限制、panic 恢复和统一错误响应。
- Goose 版本化迁移，以及 MySQL、MinIO、Kafka、API、Worker 的 Docker Compose 编排。

站内通知、规则相关推荐和断点续播已在第五阶段实现。真实全栈浏览器验收与性能结论仍以 T11/T12 的实测记录为准。

## 目录结构

```text
backend/
├── cmd/
│   ├── api/                  # HTTP API、migrate 子命令、Outbox Dispatcher
│   ├── worker/               # Kafka 消费和异步转码进程
│   └── ranking-rebuild/      # Redis 日榜/周榜重建
├── internal/
│   ├── config/               # 环境变量加载与校验
│   ├── database/             # MySQL 连接、连接池和迁移器
│   ├── analytics/            # 有效观看和创作者指标
│   ├── cache/                # Redis 缓存客户端
│   ├── engagement/           # 点赞、收藏、评论和观看历史
│   ├── follow/               # 用户关注关系
│   ├── moderation/           # 举报、处置、管理员审计
│   ├── notification/         # 站内通知
│   ├── ranking/              # 榜单候选与重建
│   ├── session/              # Refresh 会话与撤销
│   ├── taxonomy/             # 分类与标签
│   ├── messaging/            # Kafka 生产者、消费者和事件协议
│   ├── middleware/           # 请求 ID、日志、恢复、鉴权、限流、正文限制
│   ├── outbox/               # 可靠事件投递的仓储与 Dispatcher
│   ├── response/             # 统一成功响应和业务错误码
│   ├── server/               # 依赖组装和 Gin 路由注册
│   ├── storage/              # MinIO 内部访问、上传下载和预签名 URL
│   ├── token/                # JWT 签发与校验
│   ├── transcode/            # 任务状态、FFprobe、清晰度选择、FFmpeg 和重试
│   ├── user/                 # 用户模型、仓储、服务、Handler 和 DTO
│   └── video/                # 视频模型、投稿流程、HLS 输出和接口
├── migrations/               # users、videos、转码任务、互动和 outbox 表
├── scripts/                  # 端到端验收脚本
│   ├── e2e-transcode.ps1     # 完整转码链路验收
│   └── e2e-community.ps1     # 社区互动闭环验收
├── docs/                     # 第三阶段前后端接口约定
├── deploy/docker-compose.yml
├── Dockerfile
├── .env.example
└── go.mod
```

## 快速启动

环境要求为 Docker Desktop 与 Docker Compose。在 `backend` 目录执行：

```powershell
if (!(Test-Path .env)) { Copy-Item .env.example .env }
```

编辑 `.env`，至少填写随机的 `JWT_SECRET`，并为本地开发设置 MySQL、MinIO 密码。真实密钥不要提交到 Git。

```powershell
[Convert]::ToBase64String([Security.Cryptography.RandomNumberGenerator]::GetBytes(32))
docker compose --env-file .env -f deploy/docker-compose.yml up -d --wait mysql redis minio kafka
docker compose --env-file .env -f deploy/docker-compose.yml run --rm --no-deps migrate
docker compose --env-file .env -f deploy/docker-compose.yml run --rm --no-deps kafka-init
docker compose --env-file .env -f deploy/docker-compose.yml up -d --build api worker
docker compose --env-file .env -f deploy/docker-compose.yml ps
```

迁移和 Kafka 主题初始化是显式步骤；API、Worker 在依赖健康后启动。没有 Worker 时，投稿会一直停在 `processing`。演示 Nginx 与榜单重建进程用 `--profile demo` 启动，完整步骤见 `docs/stage5/deployment.md`。宿主机端口如下：

| 服务 | 地址 |
|---|---|
| API | `http://127.0.0.1:8081` |
| MySQL | `127.0.0.1:3308` |
| MinIO API | `http://127.0.0.1:9000` |
| MinIO 控制台 | `http://127.0.0.1:9001` |
| Kafka | `127.0.0.1:9092` |
| Redis | `127.0.0.1:6379` |

`MINIO_ENDPOINT` 是 API/Worker 的内部访问地址；`MINIO_PUBLIC_ENDPOINT` 是签名 URL 中浏览器可访问的地址。Compose 已分别设置为 `minio:9000` 和 `127.0.0.1:9000`。

也可以在宿主机运行 Go 进程：

```powershell
docker compose --env-file .env -f deploy/docker-compose.yml up -d mysql minio kafka
docker compose --env-file .env -f deploy/docker-compose.yml up -d kafka-init
# T02 Redis（宿主机运行 API 时使用 127.0.0.1:6379）
docker compose --env-file .env -f deploy/docker-compose.yml up -d redis
go run ./cmd/api migrate up
go run ./cmd/api
# 另开一个终端，要求本机已安装 ffmpeg 和 ffprobe
go run ./cmd/worker
```

`kafka-init` 是一次性初始化服务，负责创建 `${KAFKA_TRANSCODE_TOPIC:-video.transcode.requested}`。看到它以退出码 0 结束属于正常状态。

Outbox 发布使用 `KAFKA_PUBLISH_TIMEOUT_MS` 限定单次 Kafka 写入时长，默认 5000 ms，允许范围为 1–120000 ms。API 每批最多领取 10 条，租约覆盖整批的发布截止时间和轮询余量，避免批次后部记录在轮到之前过期。超时会保留待发记录、写入 `last_error` 并按退避时间重试；Kafka 恢复后成功发布会清除上次错误。演示环境可用 `backend/scripts/e2e-stage5.ps1 -DrillKafkaOutage -SkipBrowser` 验证该恢复链路。

Redis 只承载有限 TTL 的分布式限流状态和分类列表缓存，不保存会话、点赞、收藏、关注或权限真相。登录/注册限流在 Redis 不可用时返回 `503 RATE_LIMIT_UNAVAILABLE`；评论限流退化到有界的进程内令牌桶并记录降级日志。分区列表缓存使用 `video_share:cache:categories:v1`，TTL 为 300 秒加 0–60 秒抖动，失败回源 MySQL。

## 投稿和转码流程

```text
POST /api/v1/videos
        │ 创建 uploading 记录，返回 MinIO upload_url
        ▼
PUT upload_url
        │ 浏览器直传原始 MP4
        ▼
POST /api/v1/videos/:id/complete
        │ 校验对象；一个 MySQL 事务写入 processing + job + outbox
        ▼
Outbox Dispatcher ──► Kafka ──► Worker
                                  │ 下载、探测、生成 HLS/封面、上传
                                  ▼
                          ready 或重试后 failed
                                  │
                                  ▼
GET /api/v1/videos/:id ──► HLS play_url + cover_url
```

消息采用至少一次投递。`idempotency_key`、任务条件更新和已完成任务检查让重复消息不会重复发布同一视频。转码输出使用带版本的确定性对象路径，重试可以安全覆盖未完成输出。

## 接口一览

| 方法 | 路径 | 鉴权 | 说明 |
|---|---|---|---|
| GET | `/healthz` | 否 | 进程存活检查 |
| GET | `/readyz` | 否 | MySQL 可用性检查 |
| POST | `/api/v1/auth/register` | 否 | 注册账号 |
| GET | `/api/v1/auth/csrf` | 否 | 获取 Cookie 驱动写操作的 CSRF token |
| POST | `/api/v1/auth/login` | Origin + CSRF | 登录并返回短时 access token，refresh 只写入 HttpOnly Cookie |
| POST | `/api/v1/auth/refresh` | Origin + CSRF + Cookie | 轮换 refresh token 并返回新的 access token |
| POST | `/api/v1/auth/logout` | 可选 Bearer/refresh Cookie + Origin + CSRF | 撤销当前 session family，幂等清理 Cookie；access JWT 过期时仍可退出 |
| GET | `/api/v1/users/me` | Bearer + sid | 查询当前用户 |
| PATCH | `/api/v1/users/me` | Bearer + Origin + CSRF | 修改昵称和简介 |
| POST | `/api/v1/users/me/change-password` | Bearer + Origin + CSRF | 校验旧密码并修改密码，撤销全部 session family |
| POST | `/api/v1/videos` | Bearer | 创建投稿并获得上传 URL；可选 `category_id`、`tags` |
| POST | `/api/v1/videos/:id/complete` | Bearer | 验证上传并进入异步处理，返回 202 |
| GET | `/api/v1/users/me/videos` | Bearer | 当前用户投稿和处理状态 |
| GET | `/api/v1/users/me/videos/:id` | Bearer | 单个投稿的进度与错误信息 |
| PATCH | `/api/v1/users/me/videos/:id` | Bearer | 修改标题、简介、可见性、分区或标签 |
| DELETE | `/api/v1/users/me/videos/:id` | Bearer | 删除自己的投稿 |
| GET | `/api/v1/videos` | 否 | 已发布视频分页列表，支持 `q`、`sort`、`category_id`、重复 `tag` |
| GET | `/api/v1/videos/ranking` | 否 | 日榜/周榜；`window=day|week`，Redis 候选由 MySQL 当前权限复核 |
| GET | `/api/v1/categories` | 否 | 启用分区列表（Redis 缓存，MySQL 回源） |
| GET | `/api/v1/feed/following` | Bearer | 当前关注作者的公开视频分页列表 |
| GET | `/api/v1/videos/:id/related` | 否 | 最多 6 条规则相关推荐 |
| GET | `/api/v1/videos/:id` | 可选 | 视频详情；带令牌时返回 `viewer_state` |
| GET | `/api/v1/videos/:id/cover` | 否 | 307 跳转到短时效封面地址 |
| GET | `/api/v1/videos/:id/hls/*path` | 否 | 返回重写后的清单或 307 到媒体分片 |
| GET | `/api/v1/videos/:id/comments` | 否 | 根评论分页列表，按时间倒序 |
| GET | `/api/v1/comments/:id/replies` | 否 | 一层回复分页列表 |
| POST | `/api/v1/videos/:id/comments` | Bearer | 发表评论或回复，返回 201；支持 `parent_id`、`request_id` |
| DELETE | `/api/v1/comments/:id` | Bearer | 删除自己的评论 |
| GET | `/api/v1/notifications` | Bearer | 通知分页列表和未读数 |
| PATCH | `/api/v1/notifications/:id/read` | Bearer + Origin + CSRF | 标记本人通知已读 |
| POST | `/api/v1/reports` | Bearer + Origin + CSRF | 提交举报；同 request_id 幂等 |
| GET | `/api/v1/users/me/reports` | Bearer | 查看自己的举报进度 |
| GET | `/api/v1/admin/reports` | Admin | 管理员举报工作台 |
| PATCH | `/api/v1/admin/reports/:id/assign` | Admin + Origin + CSRF | 接单，竞争接单按状态冲突处理 |
| PATCH | `/api/v1/admin/reports/:id` | Admin + Origin + CSRF | 驳回或带处置结案 |
| POST | `/api/v1/admin/videos/:id/:action` | Admin + Origin + CSRF | hide/restore 视频并写审计 |
| POST | `/api/v1/admin/comments/:id/:action` | Admin + Origin + CSRF | hide/restore 评论并写审计 |
| POST | `/api/v1/admin/users/:id/:action` | Admin + Origin + CSRF | disable/enable 账号并写审计 |
| GET | `/api/v1/admin/moderation-actions` | Admin | 分页查看追加式处置审计 |
| POST | `/api/v1/notifications/read-all` | Bearer + Origin + CSRF | 全部标记已读 |
| PUT | `/api/v1/videos/:id/like` | Bearer | 点赞，幂等 |
| DELETE | `/api/v1/videos/:id/like` | Bearer | 取消点赞，幂等 |
| PUT | `/api/v1/videos/:id/favorite` | Bearer | 收藏，幂等 |
| DELETE | `/api/v1/videos/:id/favorite` | Bearer | 取消收藏，幂等 |
| POST | `/api/v1/videos/:id/watch` | Bearer | 上报观看进度 |
| GET | `/api/v1/users/me/favorites` | Bearer | 我的收藏分页列表 |
| GET | `/api/v1/users/me/history` | Bearer | 我的观看历史分页列表 |
| GET | `/api/v1/users/me/follows` | Bearer | 我关注的人分页列表 |
| GET | `/api/v1/users/:id` | 可选 | 公开创作者资料、作品和关系计数；禁用账号按不存在处理 |
| GET | `/api/v1/users/:id/videos` | 否 | 创作者公开视频分页列表 |
| GET | `/api/v1/users/:id/followers` | 否 | 创作者公开粉丝分页列表 |
| GET | `/api/v1/users/:id/follows` | 否 | 创作者公开关注分页列表 |
| PUT | `/api/v1/users/:id/follow` | Bearer | 关注用户，幂等 |
| DELETE | `/api/v1/users/:id/follow` | Bearer | 取消关注，幂等 |

### 请求与响应约定

所有成功响应包裹在 `{"data": ...}` 中，错误响应为 `{"error": {"code", "message", "request_id"}}`。分页查询统一接受 `page`（默认 1）和 `page_size`（默认 12，最大 50），返回 `{items, page, page_size, total}`。

- `GET /api/v1/videos`：`q` 为标题 / 描述关键字（去空白后最长 50 个 Unicode 字符），`sort` 取 `latest`（默认）或 `popular`；`category_id` 和重复 `tag` 用于筛选。未发布、私密、删除或禁用作者的视频不出现在结果中。
- `GET /api/v1/videos/:id`：使用可选鉴权。匿名请求返回详情但省略 `viewer_state`；携带有效 Bearer 令牌时返回 `viewer_state.liked / favorited / following_author`。无效或过期令牌按匿名处理，不返回 401。
- `PATCH /api/v1/users/me/videos/:id`：请求体字段均为可选，`title`、`description`、`visibility`（`public` 或 `private`）、`category_id`、`tags` 只更新显式提供的字段；标签传数组，最多 5 个，每个 1–20 个字符。仅作者本人可修改，否则 403。
- `DELETE /api/v1/users/me/videos/:id`：仅作者本人可删除，返回 `{"id": ...}`。
- `PUT / DELETE /api/v1/videos/:id/like` 与 `.../favorite`：请求体为空，返回 `{"active": bool, "count": n}`，即写入后的最终状态。重复调用结果一致。
- `POST /api/v1/videos/:id/comments`：请求体 `{"content": "...", "parent_id": 123, "request_id": "..."}`；`parent_id` 省略表示根评论，回复目标必须属于同一公开视频且未删除。`request_id` 重放返回原评论，重用同一 ID 但正文不同返回 409 `IDEMPOTENCY_CONFLICT`。内容去空白后长度需为 1-500，超限返回 400。
- `GET /api/v1/comments/:id/replies`：返回该根评论的一层回复，默认 20、最多 50；删除的评论使用“评论已删除”占位，不复制原正文到通知。
- 评论、回复和新增关注通知在同一 MySQL 事务内写入，接收人去重并排除操作者；`GET /api/v1/notifications` 的 `unread_count` 以 MySQL 为准，已读接口只允许操作本人通知。
- `POST /api/v1/videos/:id/watch`：请求体 `{"progress_ms": n, "duration_ms": n}`；`progress_ms` 不得大于 `duration_ms`，否则 400。反复上报同一视频只保留最新进度。
- `PUT / DELETE /api/v1/users/:id/follow`：请求体为空，返回 `{"following": bool}`。关注自己返回 400 `SELF_FOLLOW`，目标用户不存在或已禁用返回 404。
- `GET /api/v1/users/:id`：只返回正常账号的公开资料、公开作品数、正常账号关系计数和当前 viewer 的 `following`；不会返回密码散列、状态、角色或会话字段。退出请求带有效 access JWT 时按 `sid` 撤销，access 已过期时按 refresh Cookie 撤销。
- `GET /api/v1/categories`：返回预置的未分类、生活、知识、科技、游戏、音乐等启用分区。标签会做 NFC、去空白、小写和事务去重，每个 1–20 字符，单次最多 5 个。
- `GET /api/v1/feed/following`：必须登录，只返回当前用户关注的正常作者的 ready/public 作品；`GET /api/v1/videos/:id/related` 按同分区、共同标签数、累计播放/点赞、发布时间和 ID 固定排序，最多 6 条，候选仍由 MySQL 复核可见性。
- `GET /api/v1/videos/ranking`：默认 `window=day`，支持 `week`；分页默认 20、最多 50，返回 `generated_at`。评分为 `effective_views + 3*max(net_likes,0) + 5*max(net_favorites,0) + 2*max(net_comments,0)`，同分按视频 ID 降序；Redis 不可用时从 MySQL 日指标回源。

稳定业务错误码包括 `INVALID_PARAMETER`、`UNAUTHORIZED`、`FORBIDDEN`、`COMMENT_FORBIDDEN`、`SELF_FOLLOW`、`NOT_FOUND`、`RATE_LIMITED`、`INTERNAL_ERROR` 等。`object_key` 等存储内部字段不会出现在任何响应中。

第三阶段新增字段及前端接入细节见 [docs/stage3-frontend-contract.md](docs/stage3-frontend-contract.md)；第四阶段互动设计见 [community-interactions-design.md](../docs/superpowers/specs/2026-09-14-community-interactions-design.md)。

## 验证

```powershell
gofmt -l .
go test ./...
go vet ./...
go build ./...
docker compose --env-file .env -f deploy/docker-compose.yml config --quiet
./scripts/e2e-transcode.ps1
./scripts/e2e-community.ps1
```

`scripts/e2e-community.ps1` 需要 api 与 worker 已经启动，会创建临时用户和投稿，覆盖搜索、点赞、收藏、评论、观看历史与关注，并在结束或中途失败时尽力取消可逆互动、软删除自己创建的视频。项目目前没有删除账号和观看记录的接口，因此临时测试账号与观看记录会保留。可用 `-BaseUrl` 覆盖默认地址 `http://127.0.0.1:8081`；复用已有视频时，脚本会通过 `-VideoOwnerToken` 读取真实作者身份用于关注和状态断言。

集成测试需要设置 `TEST_MYSQL_DSN`。测试会创建独立临时数据库并在结束时删除，不会操作业务库。

榜单重建命令在 `backend` 目录运行：

```powershell
go run ./cmd/ranking-rebuild             # 常驻，每 60 秒重建 day + week
$env:RANKING_REBUILD_ONCE = "1"
go run ./cmd/ranking-rebuild day        # 一次性重建日榜
```

命令使用 Redis owner/TTL 锁和临时 ZSET 原子替换；MySQL 日指标是永久来源。若 Redis 清空，公开榜单读取会回源 MySQL，下一次重建可完全恢复快照。

T03 会话配置：访问 JWT 默认 15 分钟；刷新令牌只通过 host-only、HttpOnly、SameSite=Lax Cookie 传输，生产环境将 `APP_ORIGIN` 配置为 HTTPS 并设置 `COOKIE_SECURE=true`。浏览器访问令牌只保存在 Pinia 内存，不写入 localStorage/sessionStorage。

T02 Redis 集成测试需要设置 `TEST_REDIS_ADDR`、专用的 `TEST_REDIS_DB`（不要使用业务库所在的 DB）和可选的 `TEST_REDIS_PASSWORD`，并使用 `-tags integration` 运行。设置 `REQUIRE_INTEGRATION_TESTS=true` 后，缺少地址、DB 或连接失败会直接失败；未设置时才允许本地开发跳过：

```powershell
$env:TEST_REDIS_ADDR = "127.0.0.1:6379"
$env:TEST_REDIS_DB = "15"
$env:REQUIRE_INTEGRATION_TESTS = "true"
go test -race -tags integration ./internal/cache ./internal/middleware -count=1
```

缓存集成用例使用唯一测试键，限流用例使用唯一 subject；仍应让 `TEST_REDIS_DB` 指向一次性或专用 Redis 数据库，避免与开发业务数据共享。

数据库结构通过 Goose 版本化迁移管理。第四阶段的表由 `000004_add_community_schema.sql`（`video_stats`、`video_likes`、`video_favorites`、`comments`、`user_follows`、`watch_histories`，以及 `videos.visibility` / `videos.published_at` 与发现索引）引入，`000005_backfill_community_data.sql` 回填历史数据。迁移文件按序号幂等执行：

```powershell
go run ./cmd/api migrate up
```

重复执行 `migrate up` 不会重复建表。迁移文件一经提交即冻结，后续结构变更必须新增迁移文件，而不是修改已有文件。

管理员角色只能通过显式 CLI 调整：`go run ./cmd/api admin grant <username>` 或 `go run ./cmd/api admin revoke <username>`。撤回最后一个正常管理员会失败；JWT 不保存永久角色快照，管理 API 每次请求都会查询当前数据库角色。
