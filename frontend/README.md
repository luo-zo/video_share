# Video Share 前端

前端现使用 Vue 3、TypeScript、Vite、Vue Router 和 Pinia。Vite 开发服务器复用旧 `server.mjs` 的 API 安全中间件，因此仍保留路径与方法白名单、写请求 Origin 校验、请求体限制、上游超时和安全响应头；`server.mjs` 只作为迁移期兼容入口。视频使用后端返回的预签名 URL 直传 MinIO，并通过 `hls.js` 播放 HLS。

Node 版本要求 **22.22.3**：`package.json` 的 `engines.node` 与本目录的 `.nvmrc` 均已锁定该版本，两者一致。

## 已实现页面

- 注册、登录和退出。
- 发现页：分页浏览公开视频，支持关键词、分区和多标签筛选与排序切换。
- 关注流：登录后只展示已关注正常创作者的公开作品，支持分页。
- 详情页：兼容旧 MP4 并播放 HLS，显示封面、时长和分辨率；提供点赞、收藏、评论和关注作者，作者可在此编辑或删除自己的投稿。
- 投稿页：校验标题、简介、分区、标签和可见性，展示创建、直传、确认、转码发布四步进度。
- 个人中心：分页查看“我的投稿”“我的收藏”“观看历史”“我的关注”四个标签页。
- 账号设置：编辑昵称/简介、修改密码；刷新页面通过 HttpOnly refresh Cookie 恢复登录。
- 公开创作者主页：`/creator/:id` 展示正常账号资料、公开视频、粉丝/关注分页；作者名和关注列表可跳转，已登录用户可关注/取关。
- 视频详情页显示最多 6 条后端规则相关推荐；推荐和关注流的权限过滤由后端负责，前端不把 Redis 候选当作权限来源。
- 评论支持根评论和一层回复；通知页提供 30 秒轮询、隐藏页面暂停、单条/全部已读，并在退出时清空迟到结果。
- 举报入口支持视频和评论；普通用户可查看自己的处理进度，管理员角色可打开举报工作台与追加式处置审计页。
- 播放页接入服务器确认的观看会话：按墙钟播放时间上报心跳，服务端去重、限制跨标签页重复计时，并在有效观看达到门槛后更新统计；登录用户可断点续播。
- 创作者数据中心：`/creator-center` 展示累计指标、近 7/30 天有效播放趋势和 Top 10 公开作品；数据来自 MySQL 日指标，不使用前端虚构数据。
- 榜单页：`/ranking` 展示按 MySQL 日指标重建的日榜/周榜；每次响应携带快照生成时间，空榜、缓存回源和分页状态都有明确反馈。
- 加载、空列表、错误反馈、键盘焦点、响应式布局和减少动画偏好。

访问 JWT 只保存在当前页面内存中，刷新页面由 HttpOnly refresh Cookie 轮换恢复；“记住我的用户名”只保存用户名，不保存密码或令牌。

## 启动

先确保 Go 后端运行于 `http://127.0.0.1:8081`，MinIO 运行于 `http://127.0.0.1:9000`，然后在 `frontend` 目录执行：

```powershell
npm.cmd ci
npm.cmd run dev
```

打开 <http://127.0.0.1:5173>，依次完成注册、登录、投稿、查看“我的投稿”、打开发现页和播放视频。

后端或对象存储使用其他地址时，可覆盖开发服务器配置：

```powershell
$env:API_TARGET = 'http://127.0.0.1:8082'
$env:STORAGE_ORIGIN = 'http://127.0.0.1:9100'
npm.cmd run dev
```

`STORAGE_ORIGIN` 会进入页面的 Content Security Policy，必须与后端生成的 MinIO 预签名 URL 来源一致。

迁移期间如需核对旧页面，可运行 `npm.cmd run legacy:serve`。这个命令加载保留的
`legacy.html` 与白名单内的旧 JavaScript 入口；Vue 开发入口仍由 `npm.cmd run dev` 提供。

## 开发期安全边界

`vite.config.ts` 的 `devApiBoundary` 插件在 `configureServer` 里直接调用 `server.mjs` 导出的
`createApiMiddleware`，所以方法白名单、Origin 校验、体限制、超时和重定向白名单**只有一份实现**，
不会随迁移漂移。Vite 侧额外补了两点：

- `appType: 'mpa'` 关闭 Vite 默认的 SPA 全量回退：未命中的 `.js`/`.png` 保持 404，不再伪装成首页；
  页面回退只对无扩展名的应用路径生效。
- 比对黑名单前先做路径归一化（重复解码、反斜杠、大小写、空段、`.`/`..`、`/@fs`），
  避免 `/src//auth.js` 这类写法绕过。

已知局限（详见 `docs/stage5/decisions.md` 第 1.3 节）：

- 开发期 CSP 为让 Vite HMR 工作而保留 `'unsafe-inline'`；严格 CSP 由 T10 的 Nginx 下发。
- 开发服务器只绑 `127.0.0.1`，是开发便利性而非生产安全边界。
- **`npm run preview` 不提供页面回退**，深链（如 `/video/1`）会 404。演示深链请用 `npm run dev`。
- refresh Cookie 只在同源 API 下发送；登录、刷新、退出和设置写操作由后端精确 Origin、JSON Content-Type 与双提交 CSRF 令牌保护。
- `server.mjs` 仍被 `vite.config.ts` 导入，**T10 落地前不得删除**。

## 验证

```powershell
npm.cmd run typecheck
npm.cmd test
npm.cmd run build
npm.cmd run test:e2e
```

`npm test` 同时运行 Vitest/Vue Test Utils 用例和迁移期保留的全部 `node:test` 纯函数/代理用例。
`npm run test:e2e` 用 Playwright 覆盖可分享深链、日榜/周榜切换，以及搜索、分页在浏览器前进后退中的恢复，
创作者主页公开深链和缺失静态资源返回 404；首次运行前需要执行 `npx playwright install chromium`。

5173 若已被别的开发服务器占用，`reuseExistingServer` 会安静地复用它，跑出来的结果就属于那个
目录而不是本目录。本机请显式换一个空闲端口，Playwright 会自己拉起本目录的 dev server：

```powershell
$env:E2E_PORT = '5273'
npm.cmd run test:e2e
```

`legacy.html` 在开发服务器上会被刻意屏蔽（`isBlockedDevPath`），旧页面只能通过
`npm run legacy:serve` 打开。

## 观看统计与数据中心

播放会话接口由后端创建 24 小时有效的会话 ID。前端只在页面可见且视频实际播放时按墙钟采样，暂停、缓冲、拖动、切后台和卸载会先发送一次边界心跳；心跳带单调递增序号，服务端对重复序号幂等处理，并以用户/视频历史记录限制多标签页重复计时。旧的 `/videos/:id/watch` 兼容接口仍保留，但 Vue 详情页使用新的 watch-session 接口。

创作者数据中心需要登录，页面可切换 7 天和 30 天窗口。有效播放、观看时长和完播只展示服务端已确认的指标；本项目未把这些指标包装成未经压测的性能承诺。
