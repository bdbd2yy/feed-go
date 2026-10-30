import { useEffect, useRef, useState } from 'react'
import { Link, useSearchParams } from 'react-router'
import { useInfiniteQuery } from '@tanstack/react-query'
import {
  ArrowClockwiseIcon,
  ArrowDownIcon,
  ArrowUpIcon,
  ArrowUpRightIcon,
  PlayIcon,
  SparkleIcon,
  SpeakerHighIcon,
  SpeakerSlashIcon,
} from '@phosphor-icons/react'
import * as api from '../api/feed'
import { useAuth } from '../lib/auth'
import { Badge, Button, EmptyState, QueryState, Tabs } from '../components/ui'
import { VideoCard } from '../components/VideoCard'
import { VideoActions } from '../components/VideoPlayer'
import { usePlayerControls, type MuteState } from '../hooks/usePlayerControls'
import type { FeedVideoItem } from '../api/types'
type Cursor = { time: number; likes?: number; id?: number; asOf: number; offset: number }
export default function FeedPage({ popular = false }: { popular?: boolean }) {
  const auth = useAuth()
  const [params] = useSearchParams()
  const [tab, setTab] = useState('recommend')
  const [view, setView] = useState('grid')
  const muteState = useState(true)
  const [active, setActive] = useState(0)
  const lastWheel = useRef(0)
  const q = (params.get('q') || '').toLowerCase()
  const query = useInfiniteQuery({
    queryKey: ['feed', popular, tab, auth.claims?.account_id, auth.isLoggedIn],
    initialPageParam: { time: 0, asOf: 0, offset: 0 } as Cursor,
    enabled: tab !== 'following' || auth.isLoggedIn,
    queryFn: async ({ pageParam: c }) => {
      if (popular) {
        const r = await api.listByPopularity({ limit: 12, as_of: c.asOf, offset: c.offset })
        return {
          items: r.video_list,
          more: r.has_more,
          cursor: { ...c, asOf: r.as_of, offset: r.next_offset },
        }
      }
      if (tab === 'hot') {
        const r = await api.listLikesCount({
          limit: 12,
          likes_count_before: c.likes,
          id_before: c.id,
        })
        return {
          items: r.video_list,
          more: r.has_more,
          cursor: { ...c, likes: r.next_likes_count_before, id: r.next_id_before },
        }
      }
      const r = await (tab === 'following' ? api.listByFollowing : api.listLatest)({
        limit: 12,
        latest_time: c.time,
      })
      return { items: r.video_list, more: r.has_more, cursor: { ...c, time: r.next_time } }
    },
    getNextPageParam: (last) => (last.more ? last.cursor : undefined),
  })
  const seen = new Set<number>()
  const items = (query.data?.pages.flatMap((p) => p.items) || []).filter((v) => {
    if (seen.has(v.id)) return false
    seen.add(v.id)
    return !q || `${v.title} ${v.author.username}`.toLowerCase().includes(q)
  })
  useEffect(() => {
    setActive(0)
  }, [tab, popular, q, view])
  useEffect(() => {
    if (
      view === 'player' &&
      items.length &&
      active >= items.length - 3 &&
      query.hasNextPage &&
      !query.isFetching
    )
      void query.fetchNextPage()
  }, [active, items.length, view, query.hasNextPage, query.isFetching, query.fetchNextPage])
  const content = (
    <>
      <QueryState
        pending={query.isPending && query.fetchStatus !== 'idle'}
        error={query.error}
        retry={() => {
          void query.refetch()
        }}
      />
      {tab === 'following' && !auth.isLoggedIn ? (
        <EmptyState
          title="关注你喜欢的创作者"
          description="登录后，在这里收看他们的新作品。"
          action={
            <Link to="/account" className="ui-button ui-button-primary">
              登录账号
            </Link>
          }
        />
      ) : !query.isPending && !items.length && !query.error ? (
        <EmptyState
          title={q ? '没有找到匹配的视频' : '故事即将开始'}
          description={
            q ? '试试其他关键词，或加载更多视频。' : '这里还没有视频，来分享你的第一个精彩瞬间。'
          }
          action={
            <Link to="/video" className="ui-button ui-button-primary">
              发布视频 <ArrowUpRightIcon />
            </Link>
          }
        />
      ) : null}
      {view === 'grid' || popular ? (
        <div className="video-grid">
          {items.map((video, i) => (
            <VideoCard key={video.id} video={video} index={popular ? i : undefined} />
          ))}
        </div>
      ) : (
        items.length > 0 && (
          <div className="immersive">
            <div
              className="player-stack"
              onWheel={(e) => {
                if (Math.abs(e.deltaY) > 35 && Date.now() - lastWheel.current > 650) {
                  lastWheel.current = Date.now()
                  setActive((i) =>
                    Math.max(0, Math.min(items.length - 1, i + (e.deltaY > 0 ? 1 : -1))),
                  )
                }
              }}
            >
              {items.map((item, i) =>
                Math.abs(i - active) <= 1 ? (
                  <FeedPlayer
                    key={item.id}
                    item={item}
                    muteState={muteState}
                    active={i === active}
                    navigate={(step) =>
                      setActive((index) => Math.max(0, Math.min(items.length - 1, index + step)))
                    }
                  />
                ) : null,
              )}
            </div>
            <div className="player-navigation">
              <Button
                size="icon"
                aria-label="上一个视频"
                disabled={active === 0}
                onClick={() => setActive(active - 1)}
              >
                <ArrowUpIcon />
              </Button>
              <span>
                {active + 1} / {items.length}
              </span>
              <Button
                size="icon"
                aria-label="下一个视频"
                disabled={active >= items.length - 1}
                onClick={() => setActive(active + 1)}
              >
                <ArrowDownIcon />
              </Button>
            </div>
          </div>
        )
      )}
      {query.hasNextPage && (
        <div className="load-more">
          <Button
            busy={query.isFetchingNextPage}
            onClick={() => {
              void query.fetchNextPage()
            }}
          >
            加载更多
          </Button>
        </div>
      )}
    </>
  )
  return (
    <div className="page">
      <div className="page-heading">
        <div>
          <div className="eyebrow">{popular ? 'TRENDING NOW' : 'YOUR DAILY INSPIRATION'}</div>
          <h1>
            {popular ? '正在流行' : '发现每一种精彩'}
            <span className="heading-dot">.</span>
          </h1>
          <p>
            {popular
              ? '看看此刻大家都在喜欢什么。'
              : '有趣的日常、闪光的灵感，还有值得记录的每一刻。'}
          </p>
        </div>
        <Badge>
          <SparkleIcon size={14} />
          {popular ? '人气热榜' : '发现新鲜事'}
        </Badge>
      </div>
      {!popular && !q && (
        <div className="discovery-banner">
          <div>
            <span className="eyebrow">A LITTLE MOMENT. A BIG STORY.</span>
            <h2>
              让平凡的瞬间，
              <br />
              有被看见的可能。
            </h2>
            <Link to="/video">
              分享你的故事 <ArrowUpRightIcon />
            </Link>
          </div>
          <div className="banner-art" aria-hidden="true">
            <span className="art-orbit" />
            <span className="art-disc">
              <PlayIcon size={38} weight="fill" />
            </span>
            <span className="art-tag">生活 · 灵感 · 热爱</span>
            <span className="art-spark">✳</span>
          </div>
        </div>
      )}
      <div className="section-tools">
        <h2>{q ? `搜索：${q}` : popular ? '人气榜单' : '为你发现'}</h2>
        <div className="row">
          {!popular && (
            <Button
              size="small"
              variant="ghost"
              onClick={() => setView(view === 'grid' ? 'player' : 'grid')}
            >
              {view === 'grid' ? '沉浸播放' : '网格浏览'}
            </Button>
          )}
          <Button
            variant="ghost"
            size="small"
            aria-label="刷新视频"
            busy={query.isRefetching}
            onClick={() => {
              void query.refetch()
            }}
          >
            <ArrowClockwiseIcon />
            刷新
          </Button>
        </div>
      </div>
      {popular ? (
        content
      ) : (
        <Tabs
          value={tab}
          onChange={setTab}
          items={[
            { value: 'recommend', label: '最新推荐' },
            { value: 'following', label: '我的关注' },
            { value: 'hot', label: '点赞最多' },
          ]}
        >
          {content}
        </Tabs>
      )}
    </div>
  )
}
function FeedPlayer({
  item,
  active,
  navigate,
  muteState,
}: {
  item: FeedVideoItem
  muteState: MuteState
  active: boolean
  navigate: (step: number) => void
}) {
  const {
    videoRef: ref,
    actionsRef,
    muted,
    setMuted,
    onVideoClick,
  } = usePlayerControls(active, navigate, muteState)
  useEffect(() => {
    const video = ref.current
    if (!video) return
    if (active) void video.play().catch(() => {})
    else video.pause()
  }, [active])
  return (
    <div className="feed-player" hidden={!active}>
      <video
        ref={ref}
        onClick={onVideoClick}
        onDoubleClick={() => actionsRef.current?.like()}
        src={item.play_url}
        poster={item.cover_url}
        muted={muted}
        loop
        playsInline
        controls
        preload={active ? 'auto' : 'metadata'}
      />
      <div className="feed-caption">
        <Link to={`/u/${item.author.id}`}>@{item.author.username}</Link>
        <h2>{item.title}</h2>
        <p>{item.description}</p>
      </div>
      <Button
        className="feed-mute"
        size="icon"
        aria-label={muted ? '取消静音' : '静音'}
        onClick={() => setMuted(!muted)}
      >
        {muted ? <SpeakerSlashIcon /> : <SpeakerHighIcon />}
      </Button>
      <VideoActions
        actionsRef={actionsRef}
        videoId={item.id}
        authorId={item.author.id}
        initialLikes={item.likes_count}
        initialLiked={item.is_liked}
      />
    </div>
  )
}
