# video_share

用 Go、Gin、MySQL 和 MinIO 编写的在线视频分享平台。目前项目已经从“只有账号系统”推进到可实际操作的 MP4 投稿与播放闭环。

## 当前能力

1. 用户注册、登录、JWT 鉴权和个人资料。
2. 登录用户创建投稿并将 MP4 直接上传到 MinIO。
3. 后端确认对象后发布视频。
4. 发现页浏览公开视频，详情页获取临时地址并播放。
5. 用户查看自己的投稿及处理状态。

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
frontend: npm.cmd run dev
    ↓
浏览器打开 http://127.0.0.1:5173
```

详细配置和接口见 [后端说明](backend/README.md)，页面使用方法见 [前端说明](frontend/README.md)。设计和实现步骤保存在 `docs/superpowers/`。

## 主要技术

- 后端：Go、Gin、GORM、Goose、JWT。
- 数据：MySQL 保存业务元数据，MinIO 保存 MP4 文件。
- 前端：原生 HTML、CSS、JavaScript，Node 同源代理。
- 本地环境：Docker Compose。
- 验证：Go `testing`、`go vet`、Node `node:test`。

下一阶段适合加入 FFmpeg 异步转码与封面生成，再逐步实现点赞、评论、关注、弹幕、搜索和推荐。
