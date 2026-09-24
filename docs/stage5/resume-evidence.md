# 简历表述与证据映射

只保留能由自己解释且能链接到实现和测试的内容；性能数字须从三次正式复测与原始结果引用，不能猜测或用单次最好成绩。本文下面已填的 T12 数字只对应所列 Windows/Docker 隔离本机和合成小样本，不是生产容量。

| 可写方向（验证后再使用） | 代码 / 证据 | 表述边界 |
|---|---|---|
| Go + Vue 视频社区 | `backend/internal/server/router.go`、`frontend/src/router/index.ts`、`frontend/src/stores/auth.ts`；`backend/scripts/e2e-stage5.ps1` | 描述真实完成的注册/登录、投稿、播放、社区等闭环；不要仅凭页面路由说功能完成 |
| 可轮换会话与 CSRF | `backend/internal/session/`、`backend/internal/middleware/csrf.go`；会话集成与真实浏览器 E2E | 说清 refresh 族在 MySQL 权威持久化、退出撤销和同源 CSRF；不要写“绝对安全” |
| Outbox + Kafka 转码恢复 | `backend/internal/outbox/`、`backend/internal/video/`、`backend/cmd/worker/`；T11 Kafka 停机/恢复、重复消息和 Worker 退出演练 | 可说“本地隔离环境验证 Outbox 有界发布超时、失败记录、再次领取、恢复后发布及 Worker 幂等”；演练的 `attempts` 是 dispatcher claim 次数，不是 broker delivery 次数 |
| Redis 限流/分类缓存/榜单 | `backend/internal/cache/`、`backend/internal/ranking/`、`backend/internal/middleware/redis_ratelimit.go`；缓存过期、Redis outage、榜单重建测试 | Redis 不是永久业务事实来源；榜单请求会再次校验 MySQL 可见性 |
| 举报治理与审计 | `backend/internal/moderation/`、迁移 `000009_moderation.sql`；真实 MySQL 回滚与并发处置集成测试、浏览器 E2E | 是人工举报与治理，不等同版权自动识别、法律合规或平台级风控 |
| 可复现本机性能基线 | `perf/k6/*.js`、`perf/run-stage5-bench.ps1`、[performance.md](performance.md)、`perf-results/` 原始摘要 | 仅写实测 endpoint、环境、VUs、延迟/错误率及限制；不得把分类缓存 QPS 外推成视频吞吐、转码能力或并发用户数 |

2026-09-24 隔离本机复测：5 VU、15 秒热缓存下分类约 47.06 req/s（P95 8.25 ms）、日榜约 45.63 req/s（P95 10.56 ms），三次请求错误率均为 0；10 VU 热读复测三组约 90–94 req/s，P95 中位数约 11–14 ms。单独读取 242,708-byte 的 360p HLS synthetic segment 时，3 VU 的分片 iteration 约 27.04/s、k6 接收约 6.30 MiB/s（3 次全通过）。这些值只作为该小数据集和本机回环路径的可复现演示证据；Redis 不可用时分类回退 P95 约 2.01 秒，属于已知降级延迟。详见 [performance.md](performance.md) 与 [acceptance.md](acceptance.md)。

可用的保守草稿（仅在对应证据仍通过复测后放入简历）：

> 使用 Go、Gin、MySQL、Redis、Kafka 与 Vue 3 构建视频社区；以 MySQL Outbox 驱动异步转码，并通过隔离环境 E2E 和 Worker 故障演练验证恢复路径。Redis 用于限流、分类缓存与可重建榜单，读取时仍由 MySQL 校验内容权限；性能数据按本机测试环境单独记录，不作为生产容量承诺。
