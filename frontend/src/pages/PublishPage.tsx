import { useEffect, useState, type FormEvent } from 'react'
import { Link } from 'react-router'
import { CheckCircleIcon, CloudArrowUpIcon, ImageIcon, FilmSlateIcon } from '@phosphor-icons/react'
import * as videos from '../api/video'
import type { Video } from '../api/types'
import { uploadVideoChunked } from '../lib/upload'
import { queryClient } from '../lib/query'
import { Badge, Button, Card, Field, Input, Textarea } from '../components/ui'
import { notify, notifyError } from '../components/ui/toast'
function usePreview(file: File | null) {
  const [url, setUrl] = useState('')
  useEffect(() => {
    if (!file) {
      setUrl('')
      return
    }
    const value = URL.createObjectURL(file)
    setUrl(value)
    return () => URL.revokeObjectURL(value)
  }, [file])
  return url
}
export default function PublishPage() {
  const [title, setTitle] = useState('')
  const [description, setDescription] = useState('')
  const [file, setFile] = useState<File | null>(null)
  const [cover, setCover] = useState<File | null>(null)
  const [busy, setBusy] = useState(false)
  const [progress, setProgress] = useState(0)
  const [stage, setStage] = useState('')
  const [published, setPublished] = useState<Video | null>(null)
  const videoUrl = usePreview(file)
  const coverUrl = usePreview(cover)
  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    if (busy || !file || !cover || !title.trim()) return
    const form = e.currentTarget
    setBusy(true)
    setPublished(null)
    try {
      const result = await uploadVideoChunked(file, (percent, next) => {
        setProgress(percent)
        setStage(next)
      })
      setStage('上传封面')
      const image = await videos.uploadCover(cover)
      const play_url = result.url || result.play_url || ''
      const cover_url = image.url || image.cover_url || ''
      if (!play_url || !cover_url) throw new Error('上传结果缺少文件地址，请重试')
      setStage('发布视频')
      const video = await videos.publishVideo({
        title: title.trim(),
        description: description.trim(),
        play_url,
        cover_url,
      })
      setPublished(video)
      setFile(null)
      setCover(null)
      setTitle('')
      setDescription('')
      form.reset()
      void queryClient.invalidateQueries({ queryKey: ['feed'] })
      void queryClient.invalidateQueries({ queryKey: ['works'] })
      notify('视频已发布')
    } catch (error) {
      notifyError(error)
    } finally {
      setBusy(false)
    }
  }
  return (
    <div className="page">
      <div className="page-heading">
        <div>
          <div className="eyebrow">CREATOR STUDIO</div>
          <h1>
            分享你的故事<span className="heading-dot">.</span>
          </h1>
          <p>生活中的闪光时刻，值得被更多人看见。</p>
        </div>
        <Badge>创作空间</Badge>
      </div>
      {published && (
        <div role="status" className="success-banner">
          <CheckCircleIcon size={20} />
          发布成功！<Link to={`/video/${published.id}`}>查看视频 →</Link>
        </div>
      )}
      <form onSubmit={submit} className="publish-layout">
        <Card>
          <div className="card-heading">
            <FilmSlateIcon />
            <h2>视频信息</h2>
          </div>
          <fieldset disabled={busy} className="form-stack">
            <Field label="视频标题">
              {(id) => (
                <Input
                  id={id}
                  required
                  maxLength={100}
                  placeholder="给你的故事起个名字"
                  value={title}
                  onChange={(e) => setTitle(e.target.value)}
                />
              )}
            </Field>
            <Field label="视频简介" hint="分享视频背后的故事，让大家更了解你的创作。">
              {(id) => (
                <Textarea
                  id={id}
                  maxLength={2000}
                  placeholder="这一刻，你想说些什么？"
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                />
              )}
            </Field>
            <div className="file-grid">
              <Field
                label="视频文件"
                hint={file ? `${(file.size / 1024 / 1024).toFixed(1)} MB` : '支持 MP4 格式'}
              >
                {(id) => (
                  <label className="upload-zone" htmlFor={id}>
                    <CloudArrowUpIcon size={30} />
                    <strong>{file?.name || '选择视频文件'}</strong>
                    <span>点击选择，开始创作</span>
                    <input
                      id={id}
                      type="file"
                      accept="video/mp4"
                      required
                      onChange={(e) => setFile(e.target.files?.[0] || null)}
                    />
                  </label>
                )}
              </Field>
              <Field label="视频封面" hint="支持 JPG、PNG、WebP">
                {(id) => (
                  <label className="upload-zone" htmlFor={id}>
                    {coverUrl ? <img src={coverUrl} alt="封面预览" /> : <ImageIcon size={30} />}
                    <strong>{cover?.name || '选择封面图片'}</strong>
                    <span>用一张图片吸引目光</span>
                    <input
                      id={id}
                      type="file"
                      accept="image/jpeg,image/png,image/webp"
                      required
                      onChange={(e) => setCover(e.target.files?.[0] || null)}
                    />
                  </label>
                )}
              </Field>
            </div>
          </fieldset>
          {busy && (
            <div role="status" className="upload-progress">
              <div className="row">
                <span>{stage}</span>
                <strong>{progress}%</strong>
              </div>
              <progress value={progress} max={100} aria-label="上传进度" />
            </div>
          )}
          <div className="form-actions">
            <span className="text-muted text-xs">分享原创，传递美好。</span>
            <Button
              type="submit"
              variant="primary"
              busy={busy}
              disabled={!file || !cover || !title.trim()}
            >
              <CloudArrowUpIcon />
              发布视频
            </Button>
          </div>
        </Card>
        <div>
          <Card className="preview-card">
            <div className="card-heading">
              <h2>作品预览</h2>
              <Badge>PREVIEW</Badge>
            </div>
            <div className="publish-preview">
              {videoUrl ? (
                <video controls src={videoUrl} poster={coverUrl || undefined} />
              ) : (
                <div>
                  <FilmSlateIcon size={42} weight="thin" />
                  <p>你的下一个精彩故事</p>
                </div>
              )}
            </div>
            <h3>{title || '在这里，开启你的创作'}</h3>
            <p className="text-muted text-xs">
              {description || '选择视频和封面后，预览你的作品。'}
            </p>
          </Card>
          <p className="publish-tip">
            好的故事无需复杂的开场。
            <br />
            真实，就是最好的表达。
          </p>
        </div>
      </form>
    </div>
  )
}
