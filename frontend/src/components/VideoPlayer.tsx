import { useEffect, useImperativeHandle, useState, type Ref } from 'react'
import { Link, useNavigate } from 'react-router'
import { useMutation, useQuery } from '@tanstack/react-query'
import {
  ChatCircleIcon,
  HeartIcon,
  ShareNetworkIcon,
  UserPlusIcon,
  CheckIcon,
  TrashIcon,
} from '@phosphor-icons/react'
import * as likes from '../api/like'
import * as comments from '../api/comment'
import { useAuth } from '../lib/auth'
import { queryClient } from '../lib/query'
import type { PlayerActions } from '../hooks/usePlayerControls'
import { useFollow } from '../hooks/useSocial'
import { Avatar, Button, EmptyState, Input, Modal, QueryState } from './ui'
import { notify, notifyError } from './ui/toast'
export function VideoActions({
  videoId,
  authorId,
  initialLikes,
  initialLiked,
  actionsRef,
}: {
  actionsRef?: Ref<PlayerActions>
  videoId: number
  authorId: number
  initialLikes: number
  initialLiked?: boolean
}) {
  const auth = useAuth()
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)
  const [changed, setChanged] = useState<{ liked: boolean; count: number } | null>(null)
  useEffect(() => {
    setChanged(null)
  }, [auth.claims?.account_id, auth.isLoggedIn])
  const likedQuery = useQuery({
    queryKey: ['liked', videoId, auth.claims?.account_id],
    queryFn: () => likes.isLiked(videoId),
    enabled: auth.isLoggedIn,
  })
  const liked = likedQuery.data?.is_liked ?? initialLiked ?? false
  const count = changed?.count ?? initialLikes
  useEffect(() => {
    setChanged(null)
  }, [initialLikes])
  const follow = useFollow()
  const followed = follow.isFollowing(authorId)
  const mutation = useMutation({
    mutationFn: () => (liked ? likes.unlike(videoId) : likes.like(videoId)),
    onSuccess: async () => {
      queryClient.setQueryData(['liked', videoId, auth.claims?.account_id], { is_liked: !liked })
      setChanged({ liked: !liked, count: Math.max(0, count + (liked ? -1 : 1)) })
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['liked', videoId] }),
        queryClient.invalidateQueries({ queryKey: ['liked-videos'] }),
        queryClient.invalidateQueries({ queryKey: ['feed'] }),
        queryClient.invalidateQueries({ queryKey: ['video', videoId] }),
        queryClient.invalidateQueries({ queryKey: ['works'] }),
      ])
      setChanged(null)
    },
    onError: notifyError,
  })
  function requireLogin(action: () => void) {
    if (!auth.isLoggedIn) {
      navigate(`/account?redirect=${encodeURIComponent(location.pathname + location.search)}`)
      return
    }
    action()
  }
  useImperativeHandle(actionsRef, () => ({
    like: () => {
      if (!mutation.isPending) requireLogin(() => mutation.mutate())
    },
    comments: () => setOpen(true),
  }))
  async function share() {
    const url = `${location.origin}/video/${videoId}`
    try {
      await navigator.clipboard.writeText(url)
      notify('视频链接已复制')
    } catch {
      window.prompt('复制视频链接', url)
    }
  }
  return (
    <>
      <div className="video-actions">
        <Button
          aria-label={liked ? '取消点赞' : '点赞'}
          aria-pressed={liked}
          className={liked ? 'is-liked' : ''}
          busy={mutation.isPending}
          onClick={() => requireLogin(() => mutation.mutate())}
        >
          <HeartIcon weight={liked ? 'fill' : 'regular'} />
          {count}
        </Button>
        <Button onClick={() => setOpen(true)}>
          <ChatCircleIcon />
          评论
        </Button>
        {auth.claims?.account_id !== authorId && (
          <Button
            busy={follow.isPending}
            onClick={() => requireLogin(() => follow.mutate({ id: authorId, followed }))}
          >
            {followed ? <CheckIcon /> : <UserPlusIcon />}
            {followed ? '已关注' : '关注'}
          </Button>
        )}
        <Button
          onClick={() => {
            void share()
          }}
        >
          <ShareNetworkIcon />
          分享
        </Button>
      </div>
      <CommentDrawer key={videoId} videoId={videoId} open={open} onOpenChange={setOpen} />
    </>
  )
}
function CommentDrawer({
  videoId,
  open,
  onOpenChange,
}: {
  videoId: number
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const auth = useAuth()
  const [content, setContent] = useState('')
  const query = useQuery({
    queryKey: ['comments', videoId],
    queryFn: () => comments.listAll(videoId),
    enabled: open,
  })
  const publish = useMutation({
    mutationFn: () => comments.publish(videoId, content.trim()),
    onSuccess: () => {
      setContent('')
      void queryClient.invalidateQueries({ queryKey: ['comments', videoId] })
      notify('评论已发布')
    },
    onError: notifyError,
  })
  const remove = useMutation({
    mutationFn: comments.remove,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['comments', videoId] })
    },
    onError: notifyError,
  })
  return (
    <Modal
      open={open}
      onOpenChange={onOpenChange}
      title={`评论${query.data ? ` · ${query.data.length}` : ''}`}
      drawer
    >
      <div className="comment-list">
        <QueryState
          pending={query.isPending}
          error={query.error}
          retry={() => {
            void query.refetch()
          }}
        />
        {query.data?.length === 0 && (
          <EmptyState title="说说你的想法" description="来留下第一条评论吧。" />
        )}
        {query.data?.map((comment) => (
          <article className="comment" key={comment.id}>
            <Link to={`/u/${comment.author_id}`}>
              <Avatar name={comment.username} />
            </Link>
            <div>
              <Link to={`/u/${comment.author_id}`}>
                <strong>{comment.username}</strong>
              </Link>
              <p>{comment.content}</p>
              <small className="text-muted">
                {new Date(comment.created_at).toLocaleString('zh-CN')}
              </small>
            </div>
            {auth.claims?.account_id === comment.author_id && (
              <Button
                size="icon"
                variant="ghost"
                aria-label="删除评论"
                busy={remove.isPending}
                onClick={() => {
                  if (window.confirm('确定删除这条评论吗？')) remove.mutate(comment.id)
                }}
              >
                <TrashIcon />
              </Button>
            )}
          </article>
        ))}
      </div>
      {auth.isLoggedIn ? (
        <form
          className="comment-form"
          onSubmit={(e) => {
            e.preventDefault()
            if (content.trim()) publish.mutate()
          }}
        >
          <Input
            aria-label="评论内容"
            placeholder="友善交流，让分享更有温度"
            maxLength={1000}
            value={content}
            onChange={(e) => setContent(e.target.value)}
          />
          <Button
            variant="primary"
            type="submit"
            busy={publish.isPending}
            disabled={!content.trim()}
          >
            发送
          </Button>
        </form>
      ) : (
        <Link className="ui-button" to={`/account?redirect=/video/${videoId}`}>
          登录后发表评论
        </Link>
      )}
    </Modal>
  )
}
