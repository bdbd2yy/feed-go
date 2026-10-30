import { useState, type FormEvent } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router'
import { ArrowRightIcon, PlayIcon } from '@phosphor-icons/react'
import * as account from '../api/account'
import { auth, useAuth } from '../lib/auth'
import { Button, Card, Field, Input } from '../components/ui'
import { notify, notifyError } from '../components/ui/toast'
import { ProfilePage } from './ProfilePage'
export default function AuthPage({ mode = 'login' }: { mode?: 'login' | 'register' | 'password' }) {
  const session = useAuth()
  const navigate = useNavigate()
  const [params] = useSearchParams()
  const [busy, setBusy] = useState(false)
  const [username, setUsername] = useState(session.claims?.username || '')
  const [password, setPassword] = useState('')
  const [oldPassword, setOldPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const title =
    mode === 'login' ? '欢迎回来' : mode === 'register' ? '开启你的创作之旅' : '修改密码'
  async function submit(e: FormEvent) {
    e.preventDefault()
    if (busy) return
    if (mode !== 'login' && password !== confirm) {
      notify('两次输入的密码不一致', true)
      return
    }
    setBusy(true)
    try {
      if (mode === 'login') {
        const res = await account.login(username.trim(), password)
        auth.setTokens(res.token, res.refresh_token || '')
        const redirect = params.get('redirect')
        navigate(redirect?.startsWith('/') && !redirect.startsWith('//') ? redirect : '/account', {
          replace: true,
        })
        notify('登录成功')
      } else if (mode === 'register') {
        await account.register(username.trim(), password)
        notify('注册成功，请登录')
        navigate('/account')
      } else {
        await account.changePassword(username.trim(), oldPassword, password)
        auth.clearTokens()
        notify('密码已修改，请重新登录')
        navigate('/account')
      }
    } catch (error) {
      notifyError(error)
    } finally {
      setBusy(false)
    }
  }
  if (mode === 'login' && session.isLoggedIn) return <ProfilePage mine />
  return (
    <div className="auth-layout">
      <div className="auth-story">
        <span className="eyebrow">SHORTVIDEO / YOUR CREATIVE SPACE</span>
        <h1>
          每一个瞬间，
          <br />
          都值得被看见<span>.</span>
        </h1>
        <p>
          发现灵感，分享热爱。
          <br />
          在这里，找到与你同频的人。
        </p>
        <div className="auth-art" aria-hidden="true">
          <PlayIcon weight="fill" size={56} />
        </div>
        <span className="text-muted text-xs">记录生活的无限可能</span>
      </div>
      <div className="auth-form-wrap">
        <Card className="auth-card">
          <span className="eyebrow">
            {mode === 'login' ? 'GOOD TO SEE YOU AGAIN' : 'A NEW CHAPTER'}
          </span>
          <h2>{title}</h2>
          <p className="text-muted">
            {mode === 'login'
              ? '登录账号，继续探索你的精彩世界。'
              : mode === 'register'
                ? '创建账号，记录和分享你的精彩瞬间。'
                : '使用旧密码验证身份，设置新的登录密码。'}
          </p>
          <form className="form-stack" onSubmit={submit}>
            <Field label="用户名">
              {(id) => (
                <Input
                  id={id}
                  autoComplete="username"
                  required
                  maxLength={64}
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  placeholder="输入你的用户名"
                />
              )}
            </Field>
            {mode === 'password' && (
              <Field label="当前密码">
                {(id) => (
                  <Input
                    id={id}
                    type="password"
                    autoComplete="current-password"
                    required
                    value={oldPassword}
                    onChange={(e) => setOldPassword(e.target.value)}
                  />
                )}
              </Field>
            )}
            <Field label={mode === 'password' ? '新密码' : '密码'}>
              {(id) => (
                <Input
                  id={id}
                  type="password"
                  autoComplete={mode === 'login' ? 'current-password' : 'new-password'}
                  required
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  placeholder="输入密码"
                />
              )}
            </Field>
            {mode !== 'login' && (
              <Field label="确认密码">
                {(id) => (
                  <Input
                    id={id}
                    type="password"
                    autoComplete="new-password"
                    required
                    value={confirm}
                    onChange={(e) => setConfirm(e.target.value)}
                    placeholder="再次输入密码"
                  />
                )}
              </Field>
            )}
            <Button variant="primary" type="submit" busy={busy}>
              {mode === 'login' ? '登录账号' : mode === 'register' ? '创建账号' : '保存新密码'}
              <ArrowRightIcon />
            </Button>
          </form>
          <div className="auth-footer">
            {mode === 'login' ? (
              <>
                还没有账号？<Link to="/account/register">立即注册</Link>
                <Link className="password-link" to="/account/change-password">
                  修改密码
                </Link>
              </>
            ) : (
              <Link to="/account">返回登录</Link>
            )}
          </div>
        </Card>
      </div>
    </div>
  )
}
