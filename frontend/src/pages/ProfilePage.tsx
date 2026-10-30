import { useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import {
  ChatCircleIcon,
  GearSixIcon,
  PlusIcon,
  UserPlusIcon,
  CheckIcon,
} from '@phosphor-icons/react'
import * as accounts from '../api/account'
import * as videos from '../api/video'
import * as likes from '../api/like'
import { useAuth } from '../lib/auth'
import { useFollow, useSocial } from '../hooks/useSocial'
import { Avatar, Badge, Button, Card, EmptyState, Modal, QueryState, Tabs } from '../components/ui'
import { VideoCard } from '../components/VideoCard'
export function ProfilePage({ mine = false }: { mine?: boolean }) {
  const auth = useAuth()
  const params = useParams()
  const navigate = useNavigate()
  const id = mine ? Number(auth.claims?.account_id) : Number(params.id)
  const valid = Number.isSafeInteger(id) && id > 0
  const own = id === auth.claims?.account_id
  const [tab, setTab] = useState('works')
  const [list, setList] = useState<'followers' | 'following' | null>(null)
  useEffect(() => {
    setTab('works')
    setList(null)
  }, [id])
  const account = useQuery({
    queryKey: ['account', id],
    queryFn: () => accounts.findById(id),
    enabled: valid,
  })
  const works = useQuery({
    queryKey: ['works', id],
    queryFn: () => videos.listByAuthorId(id),
    enabled: valid,
  })
  const liked = useQuery({
    queryKey: ['liked-videos', id],
    queryFn: likes.listMyLikedVideos,
    enabled: own && tab === 'likes',
  })
  const social = useSocial(id)
  const follow = useFollow()
  const name = account.data?.username || (own ? auth.claims?.username : '') || '创作者'
  const selected = list === 'followers' ? social.followers : social.following
  const query = tab === 'likes' ? liked : works
  return (
    <div className="page">
      <QueryState
        pending={valid && account.isPending}
        error={valid ? account.error : new Error('用户编号无效')}
        retry={
          valid
            ? () => {
                void account.refetch()
              }
            : undefined
        }
      />
      {valid && (
        <>
          <Card className="profile-card">
            <div className="profile-banner" />
            <div className="profile-main">
              <Avatar name={name} src={account.data?.avatar_url} large />
              <div className="profile-title">
                <div className="row">
                  <h1>{name}</h1>
                  <Badge>{own ? '我的空间' : '创作者'}</Badge>
                </div>
                <p className="text-muted">
                  {account.data?.bio || '用镜头记录生活，用分享连接彼此。'}
                </p>
                <span className="text-muted text-xs">ShortVideo ID · {id}</span>
              </div>
              <div className="row profile-actions">
                {own ? (
                  <>
                    <Link to="/settings" className="ui-button">
                      <GearSixIcon />
                      编辑资料
                    </Link>
                    <Link to="/video" className="ui-button ui-button-primary">
                      <PlusIcon />
                      发布视频
                    </Link>
                  </>
                ) : (
                  <>
                    <Button
                      variant="primary"
                      busy={follow.isPending}
                      onClick={() => {
                        if (!auth.isLoggedIn) navigate(`/account?redirect=/u/${id}`)
                        else follow.mutate({ id, followed: follow.isFollowing(id) })
                      }}
                    >
                      {follow.isFollowing(id) ? <CheckIcon /> : <UserPlusIcon />}
                      {follow.isFollowing(id) ? '已关注' : '关注'}
                    </Button>
                    <Link className="ui-button" to={`/messages/${id}`}>
                      <ChatCircleIcon />
                      私信
                    </Link>
                  </>
                )}
              </div>
            </div>
            <div className="profile-stats">
              <span>
                <strong>{works.data?.length ?? '—'}</strong>作品
              </span>
              <button onClick={() => setList('following')}>
                <strong>{social.following.data?.vloggers.length ?? '—'}</strong>关注
              </button>
              <button onClick={() => setList('followers')}>
                <strong>{social.followers.data?.followers.length ?? '—'}</strong>粉丝
              </button>
            </div>
          </Card>
          <Tabs
            value={tab}
            onChange={setTab}
            items={[
              { value: 'works', label: '发布的作品' },
              ...(own ? [{ value: 'likes', label: '喜欢的视频' }] : []),
            ]}
          >
            <QueryState
              pending={query.isPending}
              error={query.error}
              retry={() => {
                void query.refetch()
              }}
            />
            <div className="video-grid">
              {query.data?.map((video) => (
                <VideoCard key={video.id} video={video} />
              ))}
            </div>
            {query.data?.length === 0 && (
              <EmptyState
                title={tab === 'likes' ? '把喜欢留在这里' : '还没有发布作品'}
                description={
                  tab === 'likes' ? '点赞的视频会出现在这里。' : '每个故事，都从第一个镜头开始。'
                }
                action={
                  own && tab === 'works' ? (
                    <Link to="/video" className="ui-button ui-button-primary">
                      发布第一个视频
                    </Link>
                  ) : undefined
                }
              />
            )}
          </Tabs>
        </>
      )}
      <Modal
        open={list !== null}
        onOpenChange={(open) => {
          if (!open) setList(null)
        }}
        title={list === 'followers' ? '粉丝' : '关注'}
      >
        {!auth.isLoggedIn ? (
          <EmptyState
            title="登录后查看"
            action={
              <Link to="/account" className="ui-button">
                登录账号
              </Link>
            }
          />
        ) : (
          <>
            <QueryState
              pending={selected.isPending}
              error={selected.error}
              retry={() => {
                void selected.refetch()
              }}
            />
            {(list === 'followers'
              ? social.followers.data?.followers
              : social.following.data?.vloggers
            )?.map((person) => (
              <Link
                onClick={() => setList(null)}
                to={`/u/${person.id}`}
                className="person-row"
                key={person.id}
              >
                <Avatar name={person.username} src={person.avatar_url} />
                <span>{person.username}</span>
              </Link>
            ))}
          </>
        )}
      </Modal>
    </div>
  )
}
