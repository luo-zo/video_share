# Video Share 前端

前端现使用 Vue 3、TypeScript、Vite、Vue Router 和 Pinia。Vite 开发服务器复用旧 `server.mjs` 的 API 安全中间件，因此仍保留路径与方法白名单、写请求 Origin 校验、请求体限制、上游超时和安全响应头；`server.mjs` 只作为迁移期兼容入口。视频使用后端返回的预签名 URL 直传 MinIO，并通过 `hls.js` 播放 HLS。

Node 版本要求 **22.22.3**：交付分支 `feature/stage5-upgrade` 已把 `engines` 收紧到该版本并附带 `.nvmrc`，`main` 尚未同步（见 `docs/stage5/baseline.md` 第 11.5 节）。

## 已实现页面

- 注册、登录和退出。
- 发现页：分页浏览公开视频，支持关键词搜索与排序切换。
- 详情页：兼容旧 MP4 并播放 HLS，显示封面、时长和分辨率；提供点赞、收藏、评论和关注作者，作者可在此编辑或删除自己的投稿。
- 投稿页：校验标题、简介和可见性，展示创建、直传、确认、转码发布四步进度。
- 个人中心：分页查看“我的投稿”“我的收藏”“观看历史”“我的关注”四个标签页。
- 加载、空列表、错误反馈、键盘焦点、响应式布局和减少动画偏好。

JWT 只保存在当前页面内存中，刷新页面后需要重新登录；持久刷新会话留到 T03。“记住我的用户名”只保存用户名，不保存密码或令牌。

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
- 开发期无 Cookie 与 CSRF 令牌，身份是内存 Bearer。
- `server.mjs` 仍被 `vite.config.ts` 导入，**T10 落地前不得删除**。

## 验证

```powershell
npm.cmd run typecheck
npm.cmd test
npm.cmd run build
npm.cmd run test:e2e
```

`npm test` 同时运行 Vitest/Vue Test Utils 用例和迁移期保留的全部 `node:test` 纯函数/代理用例。
`npm run test:e2e` 用 Playwright 覆盖可分享深链，以及搜索、分页在浏览器前进后退中的恢复，
并验证缺失静态资源返回 404；首次运行前需要执行 `npx playwright install chromium`。

`legacy.html` 在开发服务器上会被刻意屏蔽（`isBlockedDevPath`），旧页面只能通过
`npm run legacy:serve` 打开。
