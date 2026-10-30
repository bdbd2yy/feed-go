# ShortVideo 前端

React 19 + TypeScript + Vite 8 单页应用，面向 `backend/` 的短视频服务。使用 React Router 7 管理路由、TanStack Query 5 管理服务端数据；UI 由 Base UI、Tailwind CSS 4、Phosphor Icons 和项目自己的基础组件组成。

项目概览见 [根目录 README](../README.md)，整体模块设计见 [项目设计](../feedsystem_video_go项目设计.md)。

## 开发

推荐使用 Node.js 24 和 npm。以下命令在 `frontend/` 目录执行；依赖由 `package-lock.json` 锁定，更新依赖时同步提交该文件。

```sh
npm ci
npm run dev
```

开发服务默认使用 `http://localhost:5173`，通过 `/api` 转发请求到 `http://127.0.0.1:8080`（去掉 `/api` 前缀），并代理 `/static` 视频、封面和头像资源。后端须单独启动，或从项目根目录运行 `./start.sh`。

如果 Docker 前端已占用 5173，可使用 `npm run dev -- --port 5188 --strictPort`。

`VITE_API_BASE` 可在构建时覆盖 API 地址，默认值为 `/api`；设置为跨域地址时，后端需允许对应来源，媒体 URL 也必须可访问。该变量不会自动改变 Vite 的代理目标；代理目标配置位于 `vite.config.ts`。

```sh
npm run format:check
npm test
npm run build
npm run preview
```

`npm run build` 执行 TypeScript 检查并生成 `dist/`；`npm run preview` 用于本地查看构建产物。修改代码后可运行 `npm run format` 统一格式。

Docker 使用 Node.js 24 和 `npm ci` 构建；Nginx 托管 `dist/`，代理 `/api/` 和 `/static/`，并将客户端路由回退到 `index.html`。从项目根目录执行 `docker compose up -d --build frontend` 可重建前端容器。CI 当前执行 `npm ci` 和 `npm run build`，测试和格式检查需在本地运行。

## 结构与组件

- `src/main.tsx`、`src/App.tsx`：应用入口、错误边界、Provider、路由与登录保护。
- `src/components/AppShell.tsx`：导航、搜索、主题切换与页面容器。
- `src/components/ui/`：按钮、输入框、文本域、字段、卡片、徽标、头像、标签页、弹窗、抽屉、通知与加载状态。弹窗和标签页使用 Base UI 的键盘与焦点管理。
- `src/styles/tokens.css`：浅色、深色主题的语义颜色、字体、圆角与阴影。
- `src/styles/components.css`：基础组件样式。
- `src/styles/layout.css`：页面布局、播放器与响应式规则。
- `src/pages/`：路由页面，按需加载。
- `src/hooks/`：React 业务 hooks。
- `src/lib/`：认证、查询缓存、分片上传。
- `src/api/`：后端接口与数据规范化。

组件通过 `Button` 的 `variant`、`size`、`busy`，`Field` 的标签关联，以及 `Modal` 的受控状态保持一致的行为。Tailwind CSS 可用于局部组合，主题色通过语义变量统一管理。新增页面优先复用这些组件，避免重复定义交互样式。

### 组件使用约定

- `Button`：`variant` 为 `primary`、`secondary`、`ghost` 或 `danger`；`size` 为 `default`、`small` 或 `icon`。`busy` 同时显示加载图标并禁用重复操作；表单提交按钮须显式设置 `type="submit"`。
- `Field`：通过渲染函数接收自动生成的 `id`，将它传给 `Input` 或 `Textarea`，保持标签与控件关联。
- `Modal`：使用 `open`、`onOpenChange` 和 `title` 控制；开启 `drawer` 后呈现抽屉。`Tabs` 使用 `value`、`onChange` 和 `items` 控制选择。
- `QueryState`、`EmptyState`：分别呈现加载/错误与空数据状态。写操作结果通过 `notify`、`notifyError` 提示；这些函数位于 `components/ui/toast.tsx`。
- 图标按钮必须设置 `aria-label`。新增颜色优先使用语义主题变量，同时检查浅深色主题；交互变化需要兼顾键盘和减少动画偏好。

```tsx
<Field label="视频标题">
  {(id) => <Input id={id} required value={title} onChange={(e) => setTitle(e.target.value)} />}
</Field>
<Button type="submit" variant="primary" busy={saving}>保存</Button>
```

## 页面与功能

| 路由 | 功能 |
| --- | --- |
| `/`（`/feed` 重定向到 `/`） | 最新推荐、关注流、点赞榜；网格和沉浸播放；本地搜索已加载视频；分页 |
| `/hot` | 人气榜，分页与刷新 |
| `/video` | 视频发布，视频与封面预览 |
| `/video/:id` | 播放、点赞、关注、分享、评论与删除自己的评论 |
| `/account` | 登录、个人作品、喜欢的视频、关注与粉丝 |
| `/account/register` | 注册 |
| `/account/change-password` | 修改密码 |
| `/settings` | 改名、头像、简介、退出登录 |
| `/u/:id` | 用户作品、关注与粉丝、关注操作、私信入口 |
| `/messages`、`/messages/:peerId` | 联系人、最近 50 条消息、发送与定时刷新 |

发现页默认网格浏览，每页 12 条，通过“加载更多”翻页；沉浸模式在距末尾约三条时自动预取，只挂载当前视频及前后相邻视频的播放器。沉浸模式支持方向键切换视频、空格播放/暂停、`M` 静音、`C` 打开评论，以及点击画面播放/暂停、双击点赞；输入框、按钮与弹窗获得焦点时保留其自身键盘交互。搜索只匹配已加载视频的标题或作者。私信每 15 秒刷新当前会话，粉丝/关注列表需登录后查看。

`/video`、`/settings` 和私信页面需要登录；未登录时保存目标地址并跳转 `/account`。

## 认证、缓存与上传

认证沿用 `access_token` 和 `refresh_token` 存储键，401 时共享一次刷新请求并重试。账号切换清除查询缓存。服务端数据由 TanStack Query 管理，默认 30 秒内视为新鲜数据；写操作成功后按查询键使相关列表失效。认证存储变更支持跨标签页同步，主题偏好存储于 `theme`。

视频使用 5 MB 分片，MD5 校验、最多三路并发、每片最多三次尝试；重新提交相同文件时，服务端返回已上传分片以继续上传。失败时等待正在进行的分片完成，防止重试任务重叠。

## 验证范围

`src/__tests__/client.test.ts` 覆盖并发请求共享认证刷新、刷新失败、未登录请求拦截、退出/切换账号期间的迟到响应和刷新后的延迟 401；`src/__tests__/upload.test.ts` 覆盖上传续传、MD5 参数、重试、并发上限及空文件处理。测试使用模拟接口，不依赖正在运行的后端。

浏览器联调时检查：真实视频和封面加载、分页、未登录重定向、登录后的返回地址、点赞关注、评论抽屉、个人资料、私信以及桌面/移动端主题布局。发布链路的端到端验证需准备测试账号、MP4 与封面图片，并启动后端及相关依赖。
