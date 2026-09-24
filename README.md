# video_share

基于 Go、Vue 3、MySQL、Redis、Kafka、MinIO 和 FFmpeg 的视频社区。已有投稿、异步转码、播放和互动链路；第五阶段增加会话、创作者主页、分类推荐、通知治理、有效观看及榜单。真实浏览器全链路与故障演练的验收证据见后续 T11 记录。

## 当前能力

1. 用户注册、登录、JWT 鉴权和个人资料。
2. 登录用户创建投稿并将 MP4 直接上传到 MinIO。
3. 后端确认对象后进入异步转码，由 Worker 生成多清晰度 HLS 和封面。
4. 发现页浏览公开视频，详情页获取临时地址并播放 HLS。
5. 用户查看自己的投稿及处理进度，并编辑标题、简介、可见性或删除投稿。
6. 社区互动：按关键词和排序搜索公开视频、点赞、收藏、评论、观看历史和关注作者。

## 运行顺序

在仓库根目录准备 `backend/.env`（保留已有配置，不覆盖业务密钥），然后执行：

```powershell
docker compose --env-file backend/.env -f backend/deploy/docker-compose.yml up -d --wait mysql redis minio kafka
docker compose --env-file backend/.env -f backend/deploy/docker-compose.yml run --rm --no-deps kafka-init
```

在 `backend` 目录先执行 `go run ./cmd/api migrate up`；之后分别开三个终端运行 `go run ./cmd/api`、`go run ./cmd/worker`、`go run ./cmd/ranking-rebuild`。在 `frontend` 目录运行 `npm.cmd ci` 和 `npm.cmd run dev`，浏览器打开 `http://127.0.0.1:5173`。容器化演示改用下方部署说明；迁移始终是显式步骤。

详细配置和接口见 [后端说明](backend/README.md)，页面使用方法见 [前端说明](frontend/README.md)，Compose 演示步骤见 [部署说明](docs/stage5/deployment.md)。设计和实现步骤保存在 `docs/superpowers/`。

## 主要技术

- 后端：Go、Gin、GORM、Goose、短时访问 JWT 与 MySQL 会话族。
- 数据：MySQL 保存业务真相，Redis 保存有期限流、分类缓存及可重建榜单，MinIO 保存媒体。
- 异步媒体：MySQL Outbox、Kafka、独立 Worker、FFmpeg/FFprobe、HLS。
- 前端：Vue 3、TypeScript、Vite、Vue Router、Pinia；开发期由 Vite 复用旧 Node 同源代理中间件承担 API 安全边界。
- 本地环境：Docker Compose。
- 验证：Go `testing`、`go vet`、`vue-tsc` 类型检查、Vitest、迁移期保留的 `node:test`、Playwright E2E，以及 `backend/scripts/` 下的端到端验收脚本。

当前阶段的部署、CI、真实依赖集成与演示结果以 `docs/stage5/` 的实际验证记录为准。
