import { useState, type FormEvent } from 'react'
import { Link, useNavigate } from 'react-router'
import { SignOutIcon, ShieldCheckIcon, UserCircleIcon } from '@phosphor-icons/react'
import { useQuery } from '@tanstack/react-query'
import * as account from '../api/account'
import { auth, useAuth } from '../lib/auth'
import { queryClient } from '../lib/query'
import { Avatar, Button, Card, Field, Input, Textarea, QueryState } from '../components/ui'
import { notify, notifyError } from '../components/ui/toast'
export default function SettingsPage() {
  const session = useAuth()
  const navigate = useNavigate()
  const [busy, setBusy] = useState(false)
  const [username, setUsername] = useState(session.claims?.username || '')
  const [bio, setBio] = useState<string | null>(null)
  const [avatar, setAvatar] = useState<File | null>(null)
  const id = Number(session.claims?.account_id)
  const profile = useQuery({
    queryKey: ['account', id],
    queryFn: () => account.findById(id),
    enabled: !!id,
  })
  async function save(e: FormEvent) {
    e.preventDefault()
    if (busy) return
    setBusy(true)
    try {
      if (username.trim() !== session.claims?.username) {
        const res = await account.rename(username.trim())
        if (res.refresh_token) auth.setTokens(res.token, res.refresh_token)
        else auth.setToken(res.token)
      }
      let url = profile.data?.avatar_url
      if (avatar) url = (await account.uploadAvatar(avatar)).avatar_url
      await account.updateProfile({
        ...(url ? { avatar_url: url } : {}),
        bio: bio ?? profile.data?.bio ?? '',
      })
      await queryClient.invalidateQueries({ queryKey: ['account'] })
      notify('个人资料已保存')
    } catch (error) {
      notifyError(error)
    } finally {
      setBusy(false)
    }
  }
  async function logout() {
    if (busy) return
    setBusy(true)
    try {
      await account.logout()
      notify('已退出登录')
    } catch (error) {
      notifyError(error)
    } finally {
      auth.clearTokens()
      queryClient.clear()
      navigate('/account')
      setBusy(false)
    }
  }
  return (
    <div className="page settings-page">
      <div className="page-heading">
        <div>
          <div className="eyebrow">YOUR PREFERENCES</div>
          <h1>
            账号设置<span className="heading-dot">.</span>
          </h1>
          <p>让这个空间，更像你自己。</p>
        </div>
      </div>
      <Card>
        <div className="card-heading">
          <UserCircleIcon size={20} />
          <h2>个人资料</h2>
        </div>
        <QueryState
          pending={profile.isPending}
          error={profile.error}
          retry={() => {
            void profile.refetch()
          }}
        />
        <form className="form-stack" onSubmit={save}>
          <fieldset disabled={busy || profile.isPending || profile.isError} className="form-stack">
            <div className="row">
              <Avatar large name={username || '我'} src={profile.data?.avatar_url} />
              <Field label="更换头像" hint={avatar?.name}>
                {(fieldId) => (
                  <Input
                    id={fieldId}
                    type="file"
                    accept="image/jpeg,image/png,image/webp"
                    onChange={(e) => setAvatar(e.target.files?.[0] || null)}
                  />
                )}
              </Field>
            </div>
            <Field label="用户名">
              {(fieldId) => (
                <Input
                  id={fieldId}
                  required
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                />
              )}
            </Field>
            <Field label="个人简介">
              {(fieldId) => (
                <Textarea
                  id={fieldId}
                  maxLength={500}
                  value={bio ?? profile.data?.bio ?? ''}
                  onChange={(e) => setBio(e.target.value)}
                  placeholder="介绍一下自己，让更多人认识你"
                />
              )}
            </Field>
          </fieldset>
          <div className="form-actions">
            <Button
              type="submit"
              variant="primary"
              busy={busy}
              disabled={profile.isPending || profile.isError}
            >
              保存资料
            </Button>
          </div>
        </form>
      </Card>
      <Card>
        <div className="card-heading">
          <ShieldCheckIcon size={20} />
          <h2>账号安全</h2>
        </div>
        <div className="setting-row">
          <div>
            <strong>登录密码</strong>
            <p className="text-muted text-xs">定期更新密码，保护你的账号。</p>
          </div>
          <Link to="/account/change-password" className="ui-button">
            修改密码
          </Link>
        </div>
        <div className="setting-row">
          <div>
            <strong>退出账号</strong>
            <p className="text-muted text-xs">退出后可以随时重新登录。</p>
          </div>
          <Button
            variant="danger"
            busy={busy}
            onClick={() => {
              void logout()
            }}
          >
            <SignOutIcon />
            退出登录
          </Button>
        </div>
      </Card>
    </div>
  )
}
