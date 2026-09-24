# Stage5 本地部署

本文描述不含真实密钥的 Compose 演示部署。命令均在仓库根目录执行，业务库不会被清空。

## 启动

```powershell
if (!(Test-Path backend/.env)) { Copy-Item backend/.env.example backend/.env }
# 编辑 backend/.env：至少设置随机 JWT_SECRET；公网部署必须替换所有默认凭证。
docker compose --env-file backend/.env -f backend/deploy/docker-compose.yml up -d --wait mysql redis minio kafka
docker compose --env-file backend/.env -f backend/deploy/docker-compose.yml run --rm --no-deps migrate
docker compose --env-file backend/.env -f backend/deploy/docker-compose.yml run --rm --no-deps kafka-init
docker compose --profile demo --env-file backend/.env -f backend/deploy/docker-compose.yml up -d --build api worker ranking-rebuild frontend
docker compose --env-file backend/.env -f backend/deploy/docker-compose.yml ps
```

服务地址：前端 `http://127.0.0.1:5173`，API 经 Nginx 使用同源 `/api/v1`，MySQL `127.0.0.1:3308`，Redis `127.0.0.1:6379`，MinIO `127.0.0.1:9000`，Kafka `127.0.0.1:9092`。`migrate` 和 `kafka-init` 是显式的一次性步骤，退出码 0 是成功。开发模式可以不加 `--profile demo`，使用 Vite 本机开发服务。

### 代理网络下构建

Docker Desktop 的代理选择主要解决镜像拉取；如果 `docker pull` 成功，但 Dockerfile 的 `npm ci`、`go mod download` 或 `apk add` 出现连接重置，需要另外给构建步骤传代理 build args。代理地址必须能从 BuildKit 构建容器访问；不要把 `127.0.0.1:<主机代理端口>` 当成容器可达地址，也不要将代理写进 Dockerfile 的 `ENV`。

```powershell
$proxy = 'http://http.docker.internal:3128' # Docker Desktop 管理的代理；按本机配置替换
docker compose --profile demo --env-file backend/.env -f backend/deploy/docker-compose.yml build --build-arg "HTTP_PROXY=$proxy" --build-arg "HTTPS_PROXY=$proxy" api worker ranking-rebuild frontend
docker compose --profile demo --env-file backend/.env -f backend/deploy/docker-compose.yml up -d api worker ranking-rebuild frontend
```

不需要代理的网络仍可直接使用前面的 `up -d --build` 命令。Docker 的预定义代理 build args 不会像 `ENV` 那样写入最终运行容器；详见 [Docker build proxy arguments](https://docs.docker.com/build/building/variables/#proxy-arguments)。

### 隔离 Compose 冒烟验收

不要把演示栈直接启动到正在使用的开发 Compose 项目：固定容器名和默认端口会冲突，API/Worker 也可能处理原数据库中的任务。仓库提供 `backend/deploy/docker-compose.isolated-smoke.yml`：它移除固定容器名、映射到 `15173/18081/13308/16379/19000/19001/19092`，并通过独立 Compose 项目名创建单独网络和数据卷。该配置挂载仅供 smoke 使用的 `frontend/deploy/nginx.isolated-smoke.conf`，CSP 仅允许隔离 MinIO 的 19000 端口；生产 `nginx.conf` 仍只允许常规 9000 端口。服务配置只从非秘密的 `backend/.env.example` 读取，其 MySQL、Redis、MinIO 和 JWT 凭证由以下当前 PowerShell 会话生成。

```powershell
$project = "stage5-smoke-$([guid]::NewGuid().ToString('N').Substring(0,8))"
$env:STAGE5_SMOKE_MYSQL_ROOT_PASSWORD = [guid]::NewGuid().ToString('N')
$env:STAGE5_SMOKE_MYSQL_PASSWORD = [guid]::NewGuid().ToString('N')
$env:STAGE5_SMOKE_MINIO_USER = 'smoke' + [guid]::NewGuid().ToString('N').Substring(0,12)
$env:STAGE5_SMOKE_MINIO_PASSWORD = [guid]::NewGuid().ToString('N')
$env:STAGE5_SMOKE_REDIS_PASSWORD = [guid]::NewGuid().ToString('N')
$env:STAGE5_SMOKE_JWT_SECRET = [guid]::NewGuid().ToString('N') + [guid]::NewGuid().ToString('N')
$env:STAGE5_SMOKE_APP_ORIGIN = 'http://127.0.0.1:15173'
$composeArgs = @('--profile','demo','--project-name',$project,'--env-file','backend/.env.example',
  '-f','backend/deploy/docker-compose.yml','-f','backend/deploy/docker-compose.isolated-smoke.yml')
$proxy = 'http://http.docker.internal:3128' # 若未使用 Docker Desktop 代理，去掉 build 命令中的两个参数
docker compose @composeArgs build --build-arg "HTTP_PROXY=$proxy" --build-arg "HTTPS_PROXY=$proxy" api worker ranking-rebuild frontend
docker compose @composeArgs up -d --wait mysql redis minio kafka
docker compose @composeArgs run --rm --no-deps kafka-init
docker compose @composeArgs run --rm --no-deps migrate
docker compose @composeArgs up -d --wait api worker ranking-rebuild frontend
docker compose @composeArgs ps
Invoke-WebRequest http://127.0.0.1:15173/healthz -UseBasicParsing
Invoke-WebRequest http://127.0.0.1:18081/readyz -UseBasicParsing
```

浏览器从 `http://127.0.0.1:15173/` 验收。Compose 项目名必须在整个会话中保持相同；端口已占用时不要复用其他项目或改动其容器，先查明占用者。若在新 PowerShell 窗口停止，先把 `$project` 设为启动时生成并记录的**准确项目名**，然后重建参数并停止：

```powershell
$project = 'stage5-smoke-替换为启动时的项目名'
$composeArgs = @('--profile','demo','--project-name',$project,'--env-file','backend/.env.example',
  '-f','backend/deploy/docker-compose.yml','-f','backend/deploy/docker-compose.isolated-smoke.yml')
docker compose @composeArgs down
```

`down` 会保留该项目数据卷。只有确认 `$project` 指向本次隔离项目且无需保留演示数据时，才对该项目单独加 `-v` 删除卷。不要对默认开发项目使用 `down -v`。

首次创建普通账号后，从 `backend` 目录运行 `go run ./cmd/api admin grant <username>` 显式授予演示管理员；不要预置管理员密码或把管理员账号写进镜像。迁移与授予角色前建议先备份业务库，绝不在业务库上运行集成测试。

停止但保留数据：

```powershell
docker compose --env-file backend/.env -f backend/deploy/docker-compose.yml down
```

仅在明确使用一次性演示数据时删除卷；不要对业务环境执行 `down -v`。

## 配置边界

- `MYSQL_*`、`REDIS_*`、`KAFKA_*`、`MINIO_*` 分别控制服务连接；Compose 内部会把 API/Worker 的地址覆盖为服务名。
- `.env.example` 的账号口令都是本地演示默认值，不能用于公网。Compose 只将端口绑定到 `127.0.0.1`；它不是带 TLS 的公网配置。
- `MINIO_ENDPOINT` 是容器内访问地址，`MINIO_PUBLIC_ENDPOINT` 必须是浏览器可访问的地址。
- `APP_ORIGIN` 必须与浏览器地址完全一致；启用 HTTPS 时同时设置 `COOKIE_SECURE=true`。
- `KAFKA_PUBLISH_TIMEOUT_MS` 限制 API Outbox Dispatcher 单次 Kafka 发布时长，默认 5000 ms、最大 120000 ms；API 每批最多领取 10 条，租约覆盖这批发布的最坏时长与轮询余量。超时记录失败并延迟重试，不删除 MySQL 中的 Outbox 记录。
- Nginx 的 `/api/` upstream 使用 Docker 内置 DNS `127.0.0.11` 和 5 秒解析缓存。API 容器替换导致地址变化时，无需靠重启前端刷新旧 upstream IP。
- 现有 Nginx CSP 的媒体白名单只覆盖本地 `127.0.0.1:9000` / `localhost:9000`。更换媒体域名时，必须同步修改 `frontend/deploy/nginx.conf` 的 `img-src`、`connect-src`、`media-src`，以及 MinIO CORS 允许的 Origin，并重新构建前端镜像。视频直传使用 MinIO 签名 URL，不通过 Nginx 的 32 KiB API 请求体限制。
- `TEST_MYSQL_DSN` 只能指向一次性测试服务器，CI 以 `REQUIRE_INTEGRATION_TESTS=true` 防止依赖缺失时悄悄跳过。
- 真实密码、JWT、签名 URL 和私人媒体只放 `.env` 或 CI secret，不进入 Git、镜像或日志。

## 隔离备份恢复演练

在 `backend` 目录设置 `TEST_MYSQL_DSN` 为有建库权限的**测试** MySQL 连接（可参照 `.env.example`），并确保 `mysql`、`mysqldump` 在 PATH 中：

```powershell
$env:REQUIRE_INTEGRATION_TESTS = 'true'
go test -tags integration ./internal/database -run '^TestAccountAndVideoMetadataBackupRestore$' -count=1 -v
```

测试自动新建两个唯一命名的库：源库迁移并写入合成账号和视频元数据；`mysqldump` 备份、`mysql` 导入另一个库；校验账号、视频外键关系和迁移记录，然后只删除这两个测试库及临时 dump。它不会连接或重建业务库。MinIO 对象不在 MySQL dump 中，真实灾备必须另行备份对象存储。CI 的真实依赖任务也运行此测试。

## 浏览器与 HTTP 验收

启动演示栈后，用浏览器打开 `http://127.0.0.1:5173/` 和 `http://127.0.0.1:5173/ranking`，在深链页面刷新，确认仍返回应用页面而不是 404；打开开发者工具检查 `/api/v1` 请求为同源，登录后的 refresh Cookie 为 HttpOnly、SameSite=Lax，控制台无 CSP/CORS 错误。用大于 32 KiB 的 JSON 请求检查 Nginx 返回 413；视频文件应经预签名 MinIO URL 直传。MinIO 的预检请求可单独检查：

```powershell
curl.exe -i -X OPTIONS http://127.0.0.1:9000/video-share/example -H 'Origin: http://127.0.0.1:5173' -H 'Access-Control-Request-Method: PUT' -H 'Access-Control-Request-Headers: content-type'
```

预期包含 `Access-Control-Allow-Origin: http://127.0.0.1:5173`。不要用真实私有媒体 URL 作为演示日志或截图素材。

## 故障定位

```powershell
docker compose --env-file backend/.env -f backend/deploy/docker-compose.yml logs --tail=100 api worker ranking-rebuild frontend
Invoke-WebRequest http://127.0.0.1:5173/healthz -UseBasicParsing
Invoke-WebRequest http://127.0.0.1:8081/readyz -UseBasicParsing
```

如果 API 未启动，先检查 `migrate`、`kafka-init`、MySQL、Redis、MinIO 健康状态。榜单 Redis 故障时会回源 MySQL；登录/注册限流故障关闭并返回 503，评论/举报限流按配置降级为有界本地限流。
