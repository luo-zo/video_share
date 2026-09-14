# 第三阶段前端接入约定

本文件描述异步转码阶段的接口变化，供下一步前端联调使用。全部 JSON 成功响应都包在 `{ "data": ... }` 中。

## 投稿状态

| `status` | 含义 | 前端建议 |
|---|---|---|
| `uploading` | 等待浏览器把原始 MP4 上传到 MinIO | 展示上传进度，成功后调用 complete |
| `processing` | 后端正在排队或转码 | 每 2~5 秒查询作者详情，展示 `processing_progress` |
| `ready` | HLS 和封面已经发布 | 进入公开视频详情页 |
| `failed` | 达到最大重试次数 | 展示 `processing_error`，后续可接重试按钮 |

`deleted` 记录不会出现在作者列表和公开列表中。

## 创建与确认投稿

`POST /api/v1/videos` 的请求保持不变：

```json
{
  "title": "测试视频",
  "description": "第三阶段联调",
  "file_name": "demo.mp4",
  "content_type": "video/mp4",
  "file_size": 123456
}
```

将文件用 `PUT` 上传到 `data.upload_url` 时，必须发送完全相同的 `Content-Type: video/mp4`。上传完成后调用：

```http
POST /api/v1/videos/{id}/complete
Authorization: Bearer <token>
```

成功响应从原来的立即 `ready` 改为 HTTP `202 Accepted`，`data.status` 为 `processing`。重复调用 complete 是幂等的，已经处于 `processing` 或 `ready` 时仍返回当前状态。

## 查询处理进度

作者轮询接口：

```http
GET /api/v1/users/me/videos/{id}
Authorization: Bearer <token>
```

关键字段示例：

```json
{
  "data": {
    "id": 12,
    "status": "processing",
    "processing_progress": 75,
    "processing_error": null,
    "cover_url": null,
    "duration_ms": null,
    "width": null,
    "height": null
  }
}
```

进度是阶段性值，当前可能出现 0、10、20、75、95、100，不保证逐点连续。`failed` 时可向用户显示 `processing_error`；该字段已限制长度，但界面仍应按普通文本渲染。

## 播放与封面

公开视频进入 `ready` 后，`GET /api/v1/videos/{id}` 返回：

```json
{
  "data": {
    "id": 12,
    "status": "ready",
    "play_url": "/api/v1/videos/12/hls/master.m3u8",
    "play_type": "hls",
    "play_expires_in": 0,
    "cover_url": "/api/v1/videos/12/cover",
    "duration_ms": 120000,
    "width": 1920,
    "height": 1080
  }
}
```

HLS 清单 URL 是 API 相对路径。浏览器播放器应以 API 基地址补全，并使用支持 HLS 的播放器；Safari 可原生播放，其他浏览器可接入 hls.js。API 会重写主清单、清晰度清单和分片地址，前端不接触 MinIO 对象键。

`cover_url` 也是 API 相对路径，可直接放入图片请求；接口返回 307 到短时效的 MinIO 签名地址。

迁移前已经发布的旧视频没有 HLS 元数据，详情会临时兼容返回 `play_type: "mp4"` 和原文件签名地址。

## 错误响应

错误仍采用统一结构：

```json
{
  "error": {
    "code": "VIDEO_STATE_CONFLICT",
    "message": "当前视频状态不能执行此操作",
    "request_id": "..."
  }
}
```

前端应按 `error.code` 决定业务行为，用 `request_id` 协助查询服务端日志，不要依赖中文消息做判断。
