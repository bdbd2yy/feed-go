import { useEffect, useRef, useState, type FormEvent } from 'react'
import { Link, useParams } from 'react-router'
import { useMutation, useQuery } from '@tanstack/react-query'
import { ArrowClockwiseIcon, PaperPlaneTiltIcon, ChatCircleIcon } from '@phosphor-icons/react'
import * as messages from '../api/message'
import * as accounts from '../api/account'
import { useAuth } from '../lib/auth'
import { queryClient } from '../lib/query'
import { useSocial } from '../hooks/useSocial'
import { Avatar, Button, EmptyState, Input, QueryState } from '../components/ui'
import { notifyError } from '../components/ui/toast'
export default function MessagesPage() {
  const auth = useAuth()
  const { peerId: param } = useParams()
  const peerId = Number(param)
  const valid = Number.isSafeInteger(peerId) && peerId > 0 && peerId !== auth.claims?.account_id
  const social = useSocial()
  const [text, setText] = useState('')
  const end = useRef<HTMLDivElement>(null)
  const people = [
    ...new Map(
      [...(social.following.data?.vloggers || []), ...(social.followers.data?.followers || [])]
        .filter((person) => person.id !== auth.claims?.account_id)
        .map((person) => [person.id, person]),
    ).values(),
  ]
  const peer = useQuery({
    queryKey: ['account', peerId],
    queryFn: () => accounts.findById(peerId),
    enabled: valid,
  })
  const query = useQuery({
    queryKey: ['messages', auth.claims?.account_id, peerId],
    queryFn: () => messages.listMessages(peerId),
    enabled: valid,
    refetchInterval: 15_000,
  })
  const send = useMutation({
    mutationFn: ({ id, content }: { id: number; content: string }) =>
      messages.sendMessage(id, content),
    onSuccess: (_, variables) => {
      if (variables.id === peerId) setText('')
      void queryClient.invalidateQueries({
        queryKey: ['messages', auth.claims?.account_id, variables.id],
      })
    },
    onError: notifyError,
  })
  useEffect(() => {
    setText('')
  }, [peerId])
  useEffect(() => {
    end.current?.scrollIntoView({ block: 'nearest' })
  }, [query.data])
  function submit(e: FormEvent) {
    e.preventDefault()
    if (text.trim() && valid && !send.isPending) send.mutate({ id: peerId, content: text.trim() })
  }
  return (
    <div className="page messages-page">
      <div className="page-heading">
        <div>
          <div className="eyebrow">STAY CONNECTED</div>
          <h1>
            你的私信<span className="heading-dot">.</span>
          </h1>
          <p>从一句问候，开始新的连接。</p>
        </div>
      </div>
      <div className="messages-layout">
        <aside className="contact-panel">
          <h2>
            <ChatCircleIcon size={18} />
            联系人 <span>{people.length}</span>
          </h2>
          <QueryState
            pending={social.followers.isPending || social.following.isPending}
            error={social.followers.error || social.following.error}
            retry={() => {
              void social.followers.refetch()
              void social.following.refetch()
            }}
          />
          {people.map((person) => (
            <Link
              key={person.id}
              className={`person-row ${peerId === person.id ? 'selected' : ''}`}
              to={`/messages/${person.id}`}
            >
              <Avatar name={person.username} src={person.avatar_url} />
              <div>
                <strong>{person.username}</strong>
                <small>点击开始聊天</small>
              </div>
            </Link>
          ))}
          {people.length === 0 && !social.followers.isPending && !social.following.isPending && (
            <p className="contact-hint">关注创作者后，在这里开始聊天。</p>
          )}
        </aside>
        <section className="conversation">
          {valid ? (
            <>
              <header className="conversation-heading">
                <Link to={`/u/${peerId}`} className="row">
                  <Avatar name={peer.data?.username || '用户'} />
                  <strong>{peer.data?.username || `用户 ${peerId}`}</strong>
                </Link>
                <Button
                  size="icon"
                  variant="ghost"
                  aria-label="刷新消息"
                  busy={query.isFetching}
                  onClick={() => {
                    void query.refetch()
                  }}
                >
                  <ArrowClockwiseIcon />
                </Button>
              </header>
              <div className="message-history">
                <QueryState
                  pending={query.isPending}
                  error={query.error}
                  retry={() => {
                    void query.refetch()
                  }}
                />
                {query.data?.messages?.length === 0 && (
                  <EmptyState title="打个招呼吧" description="一段新的对话，从这里开始。" />
                )}
                {[...(query.data?.messages || [])]
                  .sort(
                    (a, b) =>
                      new Date(a.created_at).getTime() - new Date(b.created_at).getTime() ||
                      a.id - b.id,
                  )
                  .map((message) => (
                    <div
                      className={`message ${message.from_id === auth.claims?.account_id ? 'mine' : ''}`}
                      key={message.id}
                    >
                      <p>{message.content}</p>
                      <time>
                        {new Date(message.created_at).toLocaleString('zh-CN', {
                          month: 'numeric',
                          day: 'numeric',
                          hour: '2-digit',
                          minute: '2-digit',
                        })}
                      </time>
                    </div>
                  ))}
                <div ref={end} />
              </div>
              <form className="message-compose" onSubmit={submit}>
                <Input
                  value={text}
                  onChange={(e) => setText(e.target.value)}
                  maxLength={2000}
                  aria-label="私信内容"
                  placeholder="发送一条友善的消息…"
                />
                <Button
                  variant="primary"
                  type="submit"
                  busy={send.isPending}
                  disabled={!text.trim()}
                  aria-label="发送消息"
                >
                  <PaperPlaneTiltIcon />
                  发送
                </Button>
              </form>
            </>
          ) : (
            <EmptyState
              title={param ? '无法打开这段对话' : '让对话发生'}
              description={
                param ? '请选择有效的聊天对象。' : '从左侧选择一位联系人，分享你的想法。'
              }
            />
          )}
        </section>
      </div>
    </div>
  )
}
