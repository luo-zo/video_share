# Stage5 API 合同索引

所有业务路径带 `/api/v1`。成功响应为 `{"data": ...}`，错误响应为 `{"error":{"code","message","request_id"}}`。具体字段、分页参数与错误码见 [后端接口说明](../../backend/README.md)。

| 能力 | 主要路径 | 权限与数据来源 |
|---|---|---|
| 会话 | `/auth/csrf`、`/auth/refresh`、`/auth/logout` | Refresh 在 HttpOnly Cookie；写请求校验 Origin 和 CSRF；MySQL 保存会话族及撤销状态 |
| 账号设置 | `PATCH /users/me`、`POST /users/me/change-password` | 只修改本人；改密撤销会话族 |
| 创作者主页 | `GET /users/:id`、`/videos`、`/followers`、`/follows` | 只公开正常作者和 ready/public/visible 作品 |
| 分类与标签 | `GET /categories`、投稿及编辑中的 `category_id`、`tags` | MySQL 权威，Redis 缓存启用分类列表 |
| 关注流与推荐 | `GET /feed/following`、`GET /videos/:id/related` | 候选返回前重新验证当前可见性 |
| 回复与通知 | `POST /videos/:id/comments`、`GET /comments/:id/replies`、`/notifications` | 一层回复；写入、通知和计数同事务；`request_id` 幂等 |
| 治理与审计 | `/reports`、`/admin/reports`、`/admin/moderation-actions` | 管理员每次从 MySQL 校验；处置与审计同事务 |
| 有效观看 | `/videos/:id/watch-sessions`、`/watch-sessions/:id/heartbeat` | 服务端墙钟增量、`seq` 幂等、跨标签页上限 |
| 榜单 | `GET /videos/ranking?window=day|week` | Redis 候选，MySQL 当前权限复核；缺失时回源 |

管理员直接治理动作可选传 `report_id`；传入时必须关联同一目标，且该关联计入 `request_id` 的幂等摘要。相同请求 ID 换举报关联返回 409；并发把目标设为相同状态时审计保留真实的前后状态，不重复向目标所有者发送状态变化通知。

客户端应区分 HTTP 成功与业务成功。`401` 按会话规则处理，`409` 展示冲突，`429` 遵循 `Retry-After`，`503 SERVICE_UNAVAILABLE` 提示稍后重试。观看进度字段不代表有效观看时长。
