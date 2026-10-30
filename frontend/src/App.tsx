import { lazy, Suspense } from 'react'
import { Navigate, Outlet, Route, Routes, useLocation } from 'react-router'
import AppShell from './components/AppShell'
import { EmptyState, QueryState } from './components/ui'
import { useAuth } from './lib/auth'
const FeedPage = lazy(() => import('./pages/FeedPage'))
const AuthPage = lazy(() => import('./pages/AuthPage'))
const PublishPage = lazy(() => import('./pages/PublishPage'))
const SettingsPage = lazy(() => import('./pages/SettingsPage'))
const MessagesPage = lazy(() => import('./pages/MessagesPage'))
const VideoDetailPage = lazy(() => import('./pages/VideoDetailPage'))
const ProfilePage = lazy(() =>
  import('./pages/ProfilePage').then((module) => ({ default: module.ProfilePage })),
)
function Protected() {
  const auth = useAuth()
  const location = useLocation()
  return auth.isLoggedIn ? (
    <Outlet />
  ) : (
    <Navigate
      replace
      to={`/account?redirect=${encodeURIComponent(location.pathname + location.search)}`}
    />
  )
}
export default function App() {
  return (
    <Suspense fallback={<QueryState pending />}>
      <Routes>
        <Route element={<AppShell />}>
          <Route index element={<FeedPage key="discover" />} />
          <Route path="feed" element={<Navigate to="/" replace />} />
          <Route path="hot" element={<FeedPage key="popular" popular />} />
          <Route path="video/:id" element={<VideoDetailPage />} />
          <Route path="account" element={<AuthPage />} />
          <Route path="account/register" element={<AuthPage mode="register" />} />
          <Route path="account/change-password" element={<AuthPage mode="password" />} />
          <Route path="u/:id" element={<ProfilePage />} />
          <Route element={<Protected />}>
            <Route path="video" element={<PublishPage />} />
            <Route path="settings" element={<SettingsPage />} />
            <Route path="messages" element={<MessagesPage />} />
            <Route path="messages/:peerId" element={<MessagesPage />} />
          </Route>
          <Route
            path="*"
            element={
              <EmptyState
                title="页面没有找到"
                description="换个方向，继续探索精彩。"
                action={
                  <a href="/" className="ui-button">
                    返回首页
                  </a>
                }
              />
            }
          />
        </Route>
      </Routes>
    </Suspense>
  )
}
