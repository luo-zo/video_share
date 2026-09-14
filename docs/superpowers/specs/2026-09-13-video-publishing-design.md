# 视频投稿与播放 MVP 设计

## 目标

在现有注册、登录和 JWT 鉴权基础上，完成可实际操作的视频闭环：登录用户创建投稿，浏览器把 MP4 直接上传到 MinIO，后端确认文件后发布；访客可以浏览视频列表、打开详情并播放；登录用户可以查看自己的投稿。

## 架构

- MySQL 保存视频元数据和状态，不保存二进制视频。
- MinIO 保存原始 MP4。后端使用兼容 S3 的 SDK生成短时效上传、播放 URL。
- 浏览器先向 API 创建投稿，再使用预签名 PUT URL 直传 MinIO，最后通知 API确认上传完成。
- 后端保持现有 Handler → Service → Repository 分层，并新增 `storage` 包隔离对象存储实现。
- 前端保持原生 HTML/CSS/JavaScript 和同源 Node 代理，不引入构建工具或框架。

## 数据模型

`videos` 包含 `id`、`user_id`、`title`、`description`、`object_key`、`status`、`file_size`、`content_type`、`created_at`、`updated_at`。状态为 `uploading`、`ready`、`failed`、`deleted`。用户名与视频通过外键关联，公开列表只返回 `ready` 视频。

## API

- `POST /api/v1/videos`：需要 JWT，创建投稿并返回上传 URL。
- `POST /api/v1/videos/:id/complete`：需要 JWT，只允许作者确认；校验 MinIO 对象存在、大小和类型后标记为 ready。
- `GET /api/v1/videos`：公开分页列表，只包含 ready 视频。
- `GET /api/v1/videos/:id`：公开详情，返回短时效播放 URL。
- `GET /api/v1/users/me/videos`：需要 JWT，返回当前用户全部投稿状态。

## 前端界面

登录前保留现有注册/登录页面。登录后进入应用壳，顶部提供“发现”“投稿”“我的投稿”和退出入口。发现页用卡片网格展示已发布视频；投稿页包含标题、简介和 MP4 文件选择，并显示三步上传进度；我的投稿显示状态；详情页使用原生 `<video controls>` 播放。

## 错误与安全边界

- 标题 1–100 字符，简介不超过 2000 字符，只接受 `video/mp4`，文件上限 500 MiB。
- 对象键由后端生成，前端不能指定；更新状态前校验投稿所有者。
- 上传 URL 10 分钟失效，播放 URL 1 小时失效。
- API 响应不返回 MinIO 密钥和数据库内部对象键。
- 前端代理只代理明确允许的方法和视频路由；MinIO 上传 URL由浏览器直接访问。

## 验收

用户能够完成注册、登录、选择 MP4、创建投稿、直传、确认、在发现页看到视频并播放；未登录用户不能投稿；其他用户不能确认某投稿；重复确认保持幂等；失败时显示可读错误；Go 与前端自动化测试通过。
