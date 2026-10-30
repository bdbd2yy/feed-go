# JWT 是如何在项目里面使用的

- `backend/internal/auth/jwt.go`: 负责 token 本身：定义 Claims、生成和解析 access token。AccountService 登录时会调用它生成 token，再保存到数据库和 Redis
    - `backend/internal/account/service.go:113`: 登录流程
- `backend/internal/middleware/jwt/jwt.go`: 负责 Gin 请求认证：从 Authorization: Bearer ... 取出 token，调用 auth.ParseToken 验证，然后检查它是否仍是该账号当前保存的 token，并把账号信息放进请求上下文

也就是说一个是中间件，一个是jwt的项目级别二次封装


## React 前端的认证入口

- `frontend/src/lib/auth.ts`：管理 `access_token`、`refresh_token` 本地存储，通过 `useSyncExternalStore` 向组件提供登录状态，监听跨标签页的存储变化。
- `frontend/src/api/client.ts`：为请求附加 Bearer Token；401 时共享一次刷新请求，取得新的 access token 后重试原请求，刷新失败则清除认证。
- `frontend/src/lib/query.ts`：监听账号切换事件，取消正在进行的查询并清空 TanStack Query 缓存。
- `frontend/src/App.tsx`：保护发布、设置和私信路由；未登录时跳转登录页，并用 `redirect` 查询参数保存目标地址。

前端解析 JWT 用来展示用户信息和控制页面入口，实际身份校验与权限判断由后端完成。认证刷新行为的单元测试位于 `frontend/src/__tests__/client.test.ts`。
