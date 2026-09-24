# Stage5 架构说明

MySQL 是用户、会话、视频、互动、通知、治理、观看指标与日指标的权威来源。MinIO 保存私有媒体；Kafka 传递 Outbox 转码事件；Worker 执行 FFprobe、FFmpeg 和 HLS 处理。Redis 只保存有限 TTL 的限流状态、分类缓存及可重建榜单候选。

```text
Browser ── Nginx ── Gin API ── MySQL
   │          │          ├──── Redis
   │          │          └──── Outbox ── Kafka ── Worker ── MinIO
   └──────────┴────────────────── 预签名媒体 URL
```

## 关键时序

### 投稿与异步转码

```mermaid
sequenceDiagram
    autonumber
    actor U as 投稿者
    participant API as Gin API
    participant DB as MySQL
    participant S as MinIO
    participant O as Outbox Dispatcher
    participant K as Kafka
    participant W as Worker / FFmpeg
    U->>API: 创建投稿
    API->>DB: 保存 pending 视频
    API-->>U: 返回短时上传 URL
    U->>S: PUT 原始 MP4
    U->>API: 确认上传
    API->>S: 校验对象元数据
    API->>DB: 同事务更新视频并插入 Outbox
    O->>DB: 锁定并领取到期事件
    O->>K: 发布转码事件
    K-->>W: 至少一次投递
    W->>DB: 条件领取转码任务
    W->>S: 读取原片
    W->>W: FFprobe / FFmpeg 生成 HLS 和封面
    W->>S: 写版本化输出对象
    W->>DB: 保存媒体元数据并标记 ready
    W->>K: 提交消费位点
```

### 会话刷新与撤销

```mermaid
sequenceDiagram
    autonumber
    actor B as 浏览器
    participant API as Gin API
    participant DB as MySQL
    B->>API: POST /auth/refresh + HttpOnly Cookie + CSRF
    API->>API: 校验精确 Origin、CSRF 与 Cookie
    API->>DB: 锁定 session family 和 refresh token
    DB-->>API: 当前 token 未轮换且 family 有效
    API->>DB: 原子轮换旧摘要并写入新摘要
    API-->>B: 新访问 JWT + 新 HttpOnly Cookie
    Note over API,DB: 并发重复使用在冲突窗口返回 409；过窗重放撤销该 family
    B->>API: POST /auth/logout + CSRF
    API->>DB: 撤销同一 family
    API-->>B: 清除 Cookie；旧访问 JWT 因 sid 撤销而失效
```

### 评论、通知与指标

```mermaid
sequenceDiagram
    autonumber
    actor U as 登录用户
    participant API as Gin API
    participant DB as MySQL
    U->>API: POST 评论/回复 + request_id
    API->>DB: 开始事务并校验视频、作者和回复目标
    API->>DB: 锁定幂等收据与关联行
    alt request_id 已处理且摘要相同
        DB-->>API: 返回原资源 ID
    else 新请求
        API->>DB: 写评论、通知和计数净变化
        API->>DB: 写 operation_receipt
        API->>DB: 提交事务
    end
    API-->>U: 返回最终评论和通知状态
```

## 一致性与故障恢复

- 视频状态与 Outbox 在同一 MySQL 事务提交。Kafka 和 Worker 按至少一次投递设计，任务条件更新及版本化对象路径控制重复处理。
- 会话族与 refresh 摘要存于 MySQL；刷新时行锁保护轮换，退出与改密撤销由数据库决定。Cookie 写操作要求 CSRF 和精确 Origin。
- 公开列表、关注流、推荐和榜单在返回前查询当前视频、作者与治理状态；Redis 缓存不决定权限。
- 榜单由 MySQL 日指标重建到临时 ZSET，再以锁所有者校验原子发布。Redis 丢失时回源 MySQL，数据库回源限流；容量耗尽返回 503。
- 有效观看依据服务端确认的可见播放墙钟增量；客户端进度只用于断点与完播判断，拖动不会增加观看时长。

## 运行方式

开发期 Vite 复用既有同源代理中间件。Compose 演示使用 Nginx 提供静态资源、页面深链回退和 `/api/` 同源代理。迁移以显式一次性进程执行。CI 分前端类型/测试/构建、Go 单元/race/vet/build、真实 MySQL/Redis/Kafka 集成与浏览器路由 E2E；完整业务浏览器闭环属于 T11 验收。
