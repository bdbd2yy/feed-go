import { Link } from 'react-router'
import { HeartIcon, PlayIcon } from '@phosphor-icons/react'
import { Avatar } from './ui'
import type { FeedVideoItem, Video } from '../api/types'
export function VideoCard({ video, index }: { video: Video | FeedVideoItem; index?: number }) {
  const name = 'author' in video ? video.author.username : video.username
  return (
    <Link to={`/video/${video.id}`} className="video-card">
      <div className="video-cover">
        {video.cover_url ? (
          <img src={video.cover_url} alt="" loading="lazy" />
        ) : (
          <div className="cover-placeholder">
            <PlayIcon size={36} weight="thin" />
          </div>
        )}
        <span className="cover-play">
          <PlayIcon weight="fill" size={18} />
        </span>
        {index !== undefined && <span className="rank">{String(index + 1).padStart(2, '0')}</span>}
        <span className="cover-likes">
          <HeartIcon size={13} />
          {video.likes_count.toLocaleString()}
        </span>
      </div>
      <h3>{video.title}</h3>
      <div className="video-card-meta">
        <Avatar name={name} />
        <span>{name}</span>
        <span className="meta-dot">·</span>
        <span>短视频</span>
      </div>
    </Link>
  )
}
