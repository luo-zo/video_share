# Stage 5 收尾复核

复核日期：2026-09-24。复核范围是本地 `main` 工作区，基线 HEAD 为 `3cee3f4964fe224bb37cbd77b90be74c824e12b7`。本轮补充 Outbox 有界发布/重试、Nginx 动态 Docker DNS 与验收断言，并同步文档；未创建提交，也未推送远端。

## F01–F12 验收证据

| 功能 | 状态 | 证据 |
|---|---|---|
| F01 Vue 迁移 | 通过 | `npm run typecheck`、Vitest 20 个文件/92 项、旧 Node 测试 101 项；见 `frontend/README.md`。 |
| F02 路由与分享 | 通过 | Playwright 深链、浏览器历史和分享 URL 测试 7/7；真实 Stage5 浏览器流程 1/1；见 `frontend/tests/e2e/` 和 `acceptance.md`。 |
| F03 会话与设置 | 通过 | Stage5 实际刷新轮换、资料更新、退出及旧 token 拒绝；Go session 集成与竞态测试。 |
| F04 创作者主页 | 通过 | 真实 E2E 公开资料/分享路径；creator、follow 集成测试覆盖隐私和禁用用户。 |
| F05 分区与标签 | 通过 | Stage5 分类投稿；taxonomy 集成测试覆盖迁移、标准化与兼容。 |
| F06 关注流与推荐 | 通过 | follow、video、taxonomy 集成测试和 API 合同；规则说明见 `api-contract.md`。 |
| F07 Redis | 通过 | Redis 限流共享配额集成测试、缓存/榜单测试和故障演练；另用两个完整 API 进程交替发请求，合并配额在第 11 次返回 429，并带 `Retry-After`。 |
| F08 回复与通知 | 通过 | Stage5 回复、通知、已读、幂等 API 流程；engagement 与 notification 集成测试。 |
| F09 举报治理 | 通过 | Stage5 管理处置、隐藏/恢复与审计；moderation 集成测试覆盖事务和并发管理员。 |
| F10 数据与续播 | 通过 | Stage5 有效观看、重复心跳、续播和数据中心流程；analytics 集成测试。 |
| F11 部署与自动化 | 通过（本地隔离部署） | Docker Hub 恢复后，API、Worker、ranking-rebuild、frontend 镜像均从当前工作树重建；Compose 迁移、健康检查、真实浏览器 E2E、Cookie/CSP/CORS/413、API IP 变化后的 Nginx 代理均通过。 |
| F12 简历证据 | 通过 | 压测脚本、原始数据、限制说明、演示和简历映射见 `performance.md`、`demo.md`、`resume-evidence.md`。 |

## 本次依赖修复

- Go 从 1.26.3 升至 1.26.8；`backend/Dockerfile` 默认构建镜像同步升级。
- `github.com/quic-go/quic-go` 从 v0.59.0 升至 v0.59.1。
- Vitest 从 3.2.7 升至 4.1.11，兼容当前 Vite 7；前端依赖审计为 0 项漏洞。
- CI 新增 `npm audit --audit-level=moderate` 与固定版本 `govulncheck`。
- `go mod tidy -diff` 和 Go 格式检查通过。
- Outbox Dispatcher 增加有界 Kafka 发布上下文；`KAFKA_PUBLISH_TIMEOUT_MS` 默认 5000 ms、最大 120000 ms，超时写入 `last_error` 并进入既有退避重试。API 每批最多领取 10 条，租约覆盖整批的发布截止时间，避免故障时批次尾部记录先过期。
- Nginx `/api/` 代理改用 Docker DNS resolver（5 秒缓存），验证了 API 容器 IP 改变时前端不重启仍可继续代理。

## 本机验证结果

以下均为本次真实执行结果：

| 检查 | 结果 |
|---|---|
| `go test ./... -count=1` | 通过 |
| `go test -race ./... -count=1` | 通过 |
| `go test -race ./...`（本轮 Outbox 配置/实现变更后） | 通过；所有非 integration Go 包通过，无竞态报告。 |
| `go test -race -tags integration ./... -count=1 -timeout=8m` | 通过；使用一次性 MySQL，并连接隔离 Redis DB 15、Kafka、MinIO 和本机 FFmpeg/FFprobe；所有测试包通过。 |
| `go vet ./...` 与三个 Go 命令构建 | 通过 |
| `go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...` | 0 个代码可达漏洞；verbose 报告另提示 `golang.org/x/crypto/openpgp` 已弃用的模块级公告 GO-2026-5932，本项目没有调用该包，且公告没有固定版本。 |
| `npm audit --audit-level=moderate` | 0 项漏洞 |
| `npm run typecheck`、`npm test`、`npm run build` | 通过；92 项 Vitest 与 101 项 legacy Node 测试通过。 |
| `npm run test:e2e`，隔离端口 15174 | 7/7 通过。与 `npm ci` 并行时曾因共享 `node_modules` 短暂失败，串行重跑通过。 |
| `docker compose ... build api worker ranking-rebuild frontend` | 通过；Docker Hub 成功拉取 Go 1.26.8 基础镜像，四个当前镜像全部构建完成。 |
| 隔离 `migrate` | 通过；日志 `migrate ok`、`applied:0`，未修改业务数据库。 |
| `backend/scripts/e2e-stage5.ps1`（本轮镜像重建后） | 通过；12 个 API 阶段及真实浏览器 Playwright 1/1 通过；脚本断言 refresh Cookie 的 `HttpOnly` 和 `SameSite=Lax`。 |
| `backend/scripts/e2e-stage5.ps1 -DrillKafkaOutage -SkipBrowser` | 通过；Kafka 停机时记录 pending、`last_error` 和第二次 claim；恢复后发布成功、错误清除、视频 ready。 |
| Nginx 部署专项 | production/isolated `nginx -t` 通过；预留旧 API IP 后容器地址从 `.7` 改为 `.10`，前端保持运行，5 秒 DNS 缓存后同源 API 返回 200；33,000 字节请求为 413；MinIO CORS 与 CSP 均通过。 |
| `docker compose ... config -q` | 通过 |

真实依赖集成测试的临时 MySQL 容器在结束时自动移除；测试密码、Redis 密码及 MinIO 凭证未输出。业务库没有被用于集成测试。

## 已知边界与交接状态

1. 本地阶段功能和验收已完成，但**尚未提交、合并或推送**：`main` 比 `origin/main` 领先 28 个提交，工作树仍含 T01–T12 的既有未提交变更。原要求禁止自行推送，故本轮只更新工作区；是否整理提交/合并由用户决定。
2. 部署验收是 Windows 本机隔离 Compose，不包含公网发布、真实 HTTPS 域名、CDN、长期稳态或生产数据演练；这些不是已验证结论。
3. 性能报告只代表本机小型隔离数据集；Redis 分类回源有约 2 秒 P95 延迟，HLS 只测一个本地合成分片。容量结论和局限见 `performance.md`。
4. E2E 创建的合成身份、举报和审计记录按脚本设计保留在隔离数据库卷中；合成视频只软删除。没有清空或删除任何业务库、原始压测结果或普通开发 Compose 卷。

本地下一步若要形成仓库交付，是在检查最终 diff 后由用户确认是否创建本地提交；远端合并/推送仍需单独授权。
