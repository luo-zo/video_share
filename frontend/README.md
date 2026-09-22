# Video Share 前端

前端现使用 Vue 3、TypeScript、Vite、Vue Router 和 Pinia。Vite 开发服务器复用旧 `server.mjs` 的 API 安全中间件，因此仍保留路径与方法白名单、写请求 Origin 校验、请求体限制、上游超时和安全响应头；`server.mjs` 只作为迁移期兼容入口。视频使用后端返回的预签名 URL 直传 MinIO，并通过 `hls.js` 播放 HLS。

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

后端或对象存储使用其他地址时，可覆盖本地服务器配置：

```powershell
$env:API_TARGET = 'http://127.0.0.1:8082'
$env:STORAGE_ORIGIN = 'http://127.0.0.1:9100'
npm.cmd run dev
```

`STORAGE_ORIGIN` 会进入页面的 Content Security Policy，必须与后端生成的 MinIO 预签名 URL 来源一致。

迁移期间如需核对旧页面，可运行 `npm.cmd run legacy:serve`。这个命令加载保留的
`legacy.html` 与白名单内的旧 JavaScript 入口；Vue 开发入口仍由 `npm.cmd run dev` 提供。

## 验证

```powershell
npm.cmd run typecheck
npm.cmd test
npm.cmd run build
npm.cmd run test:e2e
```

`npm test` 同时运行 Vitest/Vue Test Utils 用例和迁移期保留的全部 `node:test` 纯函数/代理用例。Playwright 覆盖可分享深链以及搜索、分页在浏览器前进后退中的恢复；首次运行前需要执行 `npx playwright install chromium`。
