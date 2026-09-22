# video_share

用 Go、Gin、MySQL 和 MinIO 编写的在线视频分享平台。目前项目已经能跑通“投稿 → 异步转码 → 播放 → 社区互动”的完整闭环。

## 当前能力

1. 用户注册、登录、JWT 鉴权和个人资料。
2. 登录用户创建投稿并将 MP4 直接上传到 MinIO。
3. 后端确认对象后进入异步转码，由 Worker 生成多清晰度 HLS 和封面。
4. 发现页浏览公开视频，详情页获取临时地址并播放 HLS。
5. 用户查看自己的投稿及处理进度，并编辑标题、简介、可见性或删除投稿。
6. 社区互动：按关键词和排序搜索公开视频、点赞、收藏、评论、观看历史和关注作者。

## 运行顺序

```text
准备 backend/.env
    ↓
启动 MySQL 与 MinIO
    ↓
backend: go run ./cmd/api migrate up
    ↓
backend: go run ./cmd/api
    ↓
frontend: npm.cmd ci
    ↓
frontend: npm.cmd run dev
    ↓
浏览器打开 http://127.0.0.1:5173
```

详细配置和接口见 [后端说明](backend/README.md)，页面使用方法见 [前端说明](frontend/README.md)。设计和实现步骤保存在 `docs/superpowers/`。

## 主要技术

- 后端：Go、Gin、GORM、Goose、JWT。
- 数据：MySQL 保存业务元数据，MinIO 保存 MP4 文件。
- 前端：Vue 3、TypeScript、Vite、Vue Router、Pinia；开发期由 Vite 复用旧 Node 同源代理中间件承担 API 安全边界。
- 本地环境：Docker Compose。
- 验证：Go `testing`、`go vet`、`vue-tsc` 类型检查、Vitest、迁移期保留的 `node:test`、Playwright E2E，以及 `backend/scripts/` 下的端到端验收脚本。

下一阶段适合加入弹幕、通知和推荐，把现有的互动数据用起来。
