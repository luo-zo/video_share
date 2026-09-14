# Video Share 前端

前端使用原生 HTML、CSS 和 JavaScript。Node 本地服务器负责静态文件和受限制的同源 API 代理；视频文件使用后端返回的预签名 URL 直接上传到 MinIO，Chrome 等浏览器通过本地安装的 `hls.js` 播放第三阶段生成的 HLS。

## 已实现页面

- 注册、登录和退出。
- 发现页：分页查看已发布视频。
- 详情页：兼容旧 MP4，并播放第三阶段多清晰度 HLS，显示封面、时长和分辨率。
- 投稿页：校验标题、简介和文件，展示创建、直传、确认、转码发布四步进度。
- 我的投稿：分页查看全部投稿、处理进度和转码失败原因；新投稿会自动轮询直至完成。
- 加载、空列表、错误反馈、键盘焦点、响应式布局和减少动画偏好。

JWT 只保存在当前页面内存中，刷新页面后需要重新登录。“记住我的用户名”只保存用户名，不保存密码或令牌。

## 启动

先确保 Go 后端运行于 `http://127.0.0.1:8081`，MinIO 运行于 `http://127.0.0.1:9000`，然后在 `frontend` 目录执行：

```powershell
npm.cmd install
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

## 验证

```powershell
node --check server.mjs
node --check src/main.js
npm.cmd test
```

测试覆盖输入校验、令牌生命周期、视频 API 调用顺序、异步状态轮询、MP4/HLS 播放选择、直传失败处理、静态文件、安全响应头、API 路由白名单、MinIO 安全跳转、请求限制和代理超时。
