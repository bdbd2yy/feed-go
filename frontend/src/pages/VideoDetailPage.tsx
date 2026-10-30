import { Link, useParams } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { ArrowLeftIcon, SpeakerHighIcon, SpeakerSlashIcon } from '@phosphor-icons/react'
import * as videos from '../api/video'
import { Avatar, Button, Card, QueryState } from '../components/ui'
import { VideoActions } from '../components/VideoPlayer'
import { usePlayerControls } from '../hooks/usePlayerControls'
export default function VideoDetailPage() {
  const id = Number(useParams().id)
  const { videoRef, actionsRef, muted, setMuted, onVideoClick } = usePlayerControls(true)
  const valid = Number.isSafeInteger(id) && id > 0
  const query = useQuery({
    queryKey: ['video', id],
    queryFn: () => videos.getDetail(id),
    enabled: valid,
  })
  return (
    <div className="page">
      <Link className="back-link" to="/">
        <ArrowLeftIcon />
        返回发现
      </Link>
      <QueryState
        pending={valid && query.isPending}
        error={valid ? query.error : new Error('视频编号无效')}
        retry={
          valid
            ? () => {
                void query.refetch()
              }
            : undefined
        }
      />
      {query.data && (
        <div className="detail-layout">
          <div className="detail-player">
            <video
              ref={videoRef}
              onClick={onVideoClick}
              onDoubleClick={() => actionsRef.current?.like()}
              src={query.data.play_url}
              poster={query.data.cover_url}
              controls
              autoPlay
              muted={muted}
              playsInline
              loop
            />
            <Button
              className="detail-mute"
              size="icon"
              aria-label={muted ? '取消静音' : '静音'}
              onClick={() => setMuted(!muted)}
            >
              {muted ? <SpeakerSlashIcon /> : <SpeakerHighIcon />}
            </Button>
          </div>
          <Card className="detail-info">
            <Link to={`/u/${query.data.author_id}`} className="row">
              <Avatar name={query.data.username} />
              <strong>{query.data.username}</strong>
            </Link>
            <h1>{query.data.title}</h1>
            <p className="text-muted">{query.data.description || '每个瞬间，都有自己的故事。'}</p>
            <small className="text-muted">
              发布于 {new Date(query.data.create_time).toLocaleDateString('zh-CN')}
            </small>
            <VideoActions
              key={id}
              actionsRef={actionsRef}
              videoId={id}
              authorId={query.data.author_id}
              initialLikes={query.data.likes_count}
            />
          </Card>
        </div>
      )}
    </div>
  )
}
