# video_share 后端

这是视频分享平台的 Go 后端。第三阶段已经把“上传 MP4 原文件后直接发布”升级为异步媒体处理链路：API 验证上传对象后，在同一个 MySQL 事务中创建转码任务和 Outbox 事件；Dispatcher 把事件可靠投递到 Kafka；独立 Worker 使用 FFprobe/FFmpeg 生成多清晰度 HLS 和封面，上传到私有 MinIO，最后发布视频。第四阶段在此基础上补齐社区互动：公开搜索发现、作者编辑与删除投稿、点赞 / 收藏、评论、观看历史和关注关系。

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
- 作者可修改标题、简介和可见性，或删除自己的投稿。
- 点赞与收藏：复合主键加 `INSERT IGNORE`，重复写入幂等，返回最终状态与总数。
- 评论：发表（1-500 字符）、分页列表、仅作者本人可删除。
- 观看历史：记录播放进度，重复上报覆盖同一条记录。
- 关注关系：关注 / 取关幂等，禁止自关注，支持分页查询已关注用户。
- 请求 ID、结构化日志、限流、请求体限制、panic 恢复和统一错误响应。
- Goose 版本化迁移，以及 MySQL、MinIO、Kafka、API、Worker 的 Docker Compose 编排。

弹幕、推荐、断点续传和生产环境部署仍属于后续阶段。

## 目录结构

```text
backend/
├── cmd/
│   ├── api/                  # HTTP API、migrate 子命令、Outbox Dispatcher
│   └── worker/               # Kafka 消费和异步转码进程
├── internal/
│   ├── config/               # 环境变量加载与校验
│   ├── database/             # MySQL 连接、连接池和迁移器
│   ├── engagement/           # 点赞、收藏、评论和观看历史
│   ├── follow/               # 用户关注关系
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
Copy-Item .env.example .env
```

编辑 `.env`，至少填写随机的 `JWT_SECRET`，并为本地开发设置 MySQL、MinIO 密码。真实密钥不要提交到 Git。

```powershell
[Convert]::ToBase64String([Security.Cryptography.RandomNumberGenerator]::GetBytes(32))
docker compose --env-file .env -f deploy/docker-compose.yml up -d --build api worker
docker compose --env-file .env -f deploy/docker-compose.yml ps
```

Compose 会先等待 MySQL、MinIO 和 Kafka 健康，显式创建转码主题，再执行迁移，最后启动 API 和 Worker。不要只启动 API：没有 Worker 时，投稿会一直停在 `processing`。宿主机端口如下：

| 服务 | 地址 |
|---|---|
| API | `http://127.0.0.1:8081` |
| MySQL | `127.0.0.1:3308` |
| MinIO API | `http://127.0.0.1:9000` |
| MinIO 控制台 | `http://127.0.0.1:9001` |
| Kafka | `127.0.0.1:9092` |

`MINIO_ENDPOINT` 是 API/Worker 的内部访问地址；`MINIO_PUBLIC_ENDPOINT` 是签名 URL 中浏览器可访问的地址。Compose 已分别设置为 `minio:9000` 和 `127.0.0.1:9000`。

也可以在宿主机运行 Go 进程：

```powershell
docker compose --env-file .env -f deploy/docker-compose.yml up -d mysql minio kafka
docker compose --env-file .env -f deploy/docker-compose.yml up -d kafka-init
go run ./cmd/api migrate up
go run ./cmd/api
# 另开一个终端，要求本机已安装 ffmpeg 和 ffprobe
go run ./cmd/worker
```

`kafka-init` 是一次性初始化服务，负责创建 `${KAFKA_TRANSCODE_TOPIC:-video.transcode.requested}`。看到它以退出码 0 结束属于正常状态。

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
| POST | `/api/v1/auth/login` | 否 | 登录并返回 access token |
| GET | `/api/v1/users/me` | Bearer | 查询当前用户 |
| POST | `/api/v1/videos` | Bearer | 创建投稿并获得上传 URL |
| POST | `/api/v1/videos/:id/complete` | Bearer | 验证上传并进入异步处理，返回 202 |
| GET | `/api/v1/users/me/videos` | Bearer | 当前用户投稿和处理状态 |
| GET | `/api/v1/users/me/videos/:id` | Bearer | 单个投稿的进度与错误信息 |
| PATCH | `/api/v1/users/me/videos/:id` | Bearer | 修改标题、简介或可见性 |
| DELETE | `/api/v1/users/me/videos/:id` | Bearer | 删除自己的投稿 |
| GET | `/api/v1/videos` | 否 | 已发布视频分页列表，支持 `q`、`sort` |
| GET | `/api/v1/videos/:id` | 可选 | 视频详情；带令牌时返回 `viewer_state` |
| GET | `/api/v1/videos/:id/cover` | 否 | 307 跳转到短时效封面地址 |
| GET | `/api/v1/videos/:id/hls/*path` | 否 | 返回重写后的清单或 307 到媒体分片 |
| GET | `/api/v1/videos/:id/comments` | 否 | 评论分页列表，按时间倒序 |
| POST | `/api/v1/videos/:id/comments` | Bearer | 发表评论，返回 201 |
| DELETE | `/api/v1/comments/:id` | Bearer | 删除自己的评论 |
| PUT | `/api/v1/videos/:id/like` | Bearer | 点赞，幂等 |
| DELETE | `/api/v1/videos/:id/like` | Bearer | 取消点赞，幂等 |
| PUT | `/api/v1/videos/:id/favorite` | Bearer | 收藏，幂等 |
| DELETE | `/api/v1/videos/:id/favorite` | Bearer | 取消收藏，幂等 |
| POST | `/api/v1/videos/:id/watch` | Bearer | 上报观看进度 |
| GET | `/api/v1/users/me/favorites` | Bearer | 我的收藏分页列表 |
| GET | `/api/v1/users/me/history` | Bearer | 我的观看历史分页列表 |
| GET | `/api/v1/users/me/follows` | Bearer | 我关注的人分页列表 |
| PUT | `/api/v1/users/:id/follow` | Bearer | 关注用户，幂等 |
| DELETE | `/api/v1/users/:id/follow` | Bearer | 取消关注，幂等 |

### 请求与响应约定

所有成功响应包裹在 `{"data": ...}` 中，错误响应为 `{"error": {"code", "message", "request_id"}}`。分页查询统一接受 `page`（默认 1）和 `page_size`（默认 12，最大 50），返回 `{items, page, page_size, total}`。

- `GET /api/v1/videos`：`q` 为标题 / 描述关键字（去空白后最长 100 字符），`sort` 取 `latest`（默认）或 `popular`。未发布的视频不出现在结果中。
- `GET /api/v1/videos/:id`：使用可选鉴权。匿名请求返回详情但省略 `viewer_state`；携带有效 Bearer 令牌时返回 `viewer_state.liked / favorited / following_author`。无效或过期令牌按匿名处理，不返回 401。
- `PATCH /api/v1/users/me/videos/:id`：请求体字段均为可选，`title`、`description`、`visibility`（`public` 或 `private`）只更新显式提供的字段。仅作者本人可修改，否则 403。
- `DELETE /api/v1/users/me/videos/:id`：仅作者本人可删除，返回 `{"id": ...}`。
- `PUT / DELETE /api/v1/videos/:id/like` 与 `.../favorite`：请求体为空，返回 `{"active": bool, "count": n}`，即写入后的最终状态。重复调用结果一致。
- `POST /api/v1/videos/:id/comments`：请求体 `{"content": "..."}`，内容去空白后长度需为 1-500，超限返回 400。
- `POST /api/v1/videos/:id/watch`：请求体 `{"progress_ms": n, "duration_ms": n}`；`progress_ms` 不得大于 `duration_ms`，否则 400。反复上报同一视频只保留最新进度。
- `PUT / DELETE /api/v1/users/:id/follow`：请求体为空，返回 `{"following": bool}`。关注自己返回 400 `SELF_FOLLOW`，目标用户不存在或已禁用返回 404。

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

`scripts/e2e-community.ps1` 需要 api 与 worker 已经启动，会创建临时用户和投稿，覆盖搜索、点赞、收藏、评论、观看历史与关注，并在结束时清理自己创建的数据。可用 `-BaseUrl` 覆盖默认地址 `http://127.0.0.1:8081`。

集成测试需要设置 `TEST_MYSQL_DSN`。测试会创建独立临时数据库并在结束时删除，不会操作业务库。

数据库结构通过 Goose 版本化迁移管理。第四阶段的表由 `000004_add_community_schema.sql`（`video_stats`、`video_likes`、`video_favorites`、`comments`、`user_follows`、`watch_histories`，以及 `videos.visibility` / `videos.published_at` 与发现索引）引入，`000005_backfill_community_data.sql` 回填历史数据。迁移文件按序号幂等执行：

```powershell
go run ./cmd/api migrate up
```

重复执行 `migrate up` 不会重复建表。迁移文件一经提交即冻结，后续结构变更必须新增迁移文件，而不是修改已有文件。
