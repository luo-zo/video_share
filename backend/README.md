# video_share 后端

这是视频分享平台的 Go 后端。第三阶段已经把“上传 MP4 原文件后直接发布”升级为异步媒体处理链路：API 验证上传对象后，在同一个 MySQL 事务中创建转码任务和 Outbox 事件；Dispatcher 把事件可靠投递到 Kafka；独立 Worker 使用 FFprobe/FFmpeg 生成多清晰度 HLS 和封面，上传到私有 MinIO，最后发布视频。

## 已实现的业务

- 注册、登录、JWT 鉴权和当前用户资料。
- 创建投稿，通过短时效预签名 URL 把 MP4 从浏览器直传私有 MinIO。
- 完成上传时校验对象大小和 Content-Type，并返回 `202 processing`。
- MySQL Outbox 与业务状态同事务写入，Kafka 暂时不可用时事件不会丢失。
- Kafka 消费、任务幂等领取、失败重试、最大尝试次数和最终失败状态。
- FFprobe 获取时长、分辨率和音轨；FFmpeg 生成封面以及不高于原视频的 360p、720p、1080p HLS 清晰度。
- 作者可查询 `uploading`、`processing`、`ready`、`failed`、处理进度和失败原因。
- 公开视频列表、详情、HLS 清单代理、分片临时地址和封面临时地址。
- 请求 ID、结构化日志、限流、请求体限制、panic 恢复和统一错误响应。
- Goose 版本化迁移，以及 MySQL、MinIO、Kafka、API、Worker 的 Docker Compose 编排。

评论、点赞、关注、弹幕、搜索、推荐、断点续传和生产环境部署仍属于后续阶段。

## 目录结构

```text
backend/
├── cmd/
│   ├── api/                  # HTTP API、migrate 子命令、Outbox Dispatcher
│   └── worker/               # Kafka 消费和异步转码进程
├── internal/
│   ├── config/               # 环境变量加载与校验
│   ├── database/             # MySQL 连接、连接池和迁移器
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
├── migrations/               # users、videos、转码任务和 outbox 表
├── scripts/e2e-transcode.ps1 # 完整转码链路验收脚本
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
| GET | `/api/v1/videos` | 否 | 已发布视频分页列表 |
| GET | `/api/v1/videos/:id` | 否 | 视频详情及播放入口 |
| GET | `/api/v1/videos/:id/cover` | 否 | 307 跳转到短时效封面地址 |
| GET | `/api/v1/videos/:id/hls/*path` | 否 | 返回重写后的清单或 307 到媒体分片 |

第三阶段新增字段及前端接入细节见 [docs/stage3-frontend-contract.md](docs/stage3-frontend-contract.md)。

## 验证

```powershell
gofmt -l .
go test ./...
go vet ./...
go build ./...
docker compose --env-file .env -f deploy/docker-compose.yml config --quiet
./scripts/e2e-transcode.ps1
```

集成测试需要设置 `TEST_MYSQL_DSN`。测试会创建独立临时数据库并在结束时删除，不会操作业务库。
