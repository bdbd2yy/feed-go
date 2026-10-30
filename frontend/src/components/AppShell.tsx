import { Suspense, useEffect, useState, type FormEvent } from 'react'
import { Link, NavLink, Outlet, useLocation, useNavigate, useSearchParams } from 'react-router'
import {
  CompassIcon,
  FireIcon,
  PlusIcon,
  ChatCircleIcon,
  UserCircleIcon,
  GearSixIcon,
  MagnifyingGlassIcon,
  MoonIcon,
  SunIcon,
  PlayIcon,
  ArrowUpRightIcon,
} from '@phosphor-icons/react'
import { useAuth } from '../lib/auth'
import { Avatar, Button, QueryState } from './ui'
import { Toaster } from './ui/toast'
const links = [
  { to: '/', label: '发现', icon: CompassIcon },
  { to: '/hot', label: '热门视频', icon: FireIcon },
  { to: '/video', label: '发布视频', icon: PlusIcon },
  { to: '/messages', label: '私信', icon: ChatCircleIcon },
  { to: '/account', label: '个人主页', icon: UserCircleIcon },
]
export default function AppShell() {
  const auth = useAuth()
  const location = useLocation()
  const navigate = useNavigate()
  const [params] = useSearchParams()
  const [search, setSearch] = useState(params.get('q') || '')
  const [dark, setDark] = useState(() => {
    try {
      return localStorage.getItem('theme') === 'dark'
    } catch {
      return false
    }
  })
  useEffect(() => {
    document.documentElement.classList.toggle('dark', dark)
    try {
      localStorage.setItem('theme', dark ? 'dark' : 'light')
    } catch {
      /* Theme remains available for this session. */
    }
  }, [dark])
  useEffect(() => {
    setSearch(params.get('q') || '')
  }, [params])
  useEffect(() => {
    document.getElementById('main')?.scrollTo(0, 0)
  }, [location.pathname])
  function onSearch(e: FormEvent) {
    e.preventDefault()
    navigate(search.trim() ? `/?q=${encodeURIComponent(search.trim())}` : '/')
  }
  const title =
    links.find((link) => link.to === location.pathname)?.label ||
    (location.pathname.startsWith('/video/')
      ? '视频详情'
      : location.pathname.startsWith('/u/')
        ? '创作者主页'
        : '账号与设置')
  return (
    <div className="app-shell">
      <a href="#main" className="skip-link">
        跳到主要内容
      </a>
      <aside className="sidebar">
        <Link to="/" className="brand">
          <span className="brand-mark">
            <PlayIcon weight="fill" size={17} />
          </span>
          ShortVideo<span className="brand-dot">.</span>
        </Link>
        <div className="sidebar-section">你的灵感，从这里开始</div>
        <nav className="main-nav" aria-label="主导航">
          {links.map(({ to, label, icon: Icon }) => (
            <NavLink
              key={to}
              to={to}
              end={to === '/'}
              className={({ isActive }) => `nav-item ${isActive ? 'active' : ''}`}
            >
              <Icon size={20} />
              <span>{label}</span>
              {to === '/video' && <span className="nav-plus">+</span>}
            </NavLink>
          ))}
        </nav>
        <div className="sidebar-note">
          <span className="eyebrow">MAKE SOMETHING GREAT</span>
          <p>
            记录此刻，
            <br />
            分享你的世界。
          </p>
          <Link to="/video">
            发布一个新故事 <ArrowUpRightIcon />
          </Link>
        </div>
        <div className="sidebar-bottom">
          <NavLink to="/settings" className="nav-item">
            <GearSixIcon size={20} />
            设置
          </NavLink>
          <Link to="/account" className="account-mini">
            <Avatar name={auth.claims?.username || '访'} />
            <span>
              <strong>{auth.claims?.username || '访客'}</strong>
              <small>{auth.isLoggedIn ? '查看个人主页' : '登录，发现更多精彩'}</small>
            </span>
            <ArrowUpRightIcon />
          </Link>
        </div>
      </aside>
      <div className="app-body">
        <header className="topbar">
          <div className="breadcrumb">
            探索空间 <span>/</span>
            <strong>{title}</strong>
          </div>
          <form className="search" onSubmit={onSearch}>
            <MagnifyingGlassIcon size={18} />
            <input
              aria-label="搜索视频"
              placeholder="搜索标题或创作者"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
            <kbd>↵</kbd>
          </form>
          <div className="top-actions">
            <Button
              variant="ghost"
              size="icon"
              aria-label={dark ? '切换浅色主题' : '切换深色主题'}
              onClick={() => setDark(!dark)}
            >
              {dark ? <SunIcon size={19} /> : <MoonIcon size={19} />}
            </Button>
            <Link className="ui-button ui-button-primary" to="/video">
              <PlusIcon size={16} />
              发布视频
            </Link>
          </div>
        </header>
        <main id="main" className="main-content" tabIndex={-1}>
          <Suspense fallback={<QueryState pending />}>
            <Outlet />
          </Suspense>
        </main>
        <nav className="mobile-nav" aria-label="移动导航">
          {links.map(({ to, label, icon: Icon }) => (
            <NavLink to={to} end={to === '/'} key={to}>
              <Icon size={21} />
              <span>{label}</span>
            </NavLink>
          ))}
        </nav>
      </div>
      <Toaster />
    </div>
  )
}
