import { beforeEach, describe, expect, it, vi } from 'vitest'
vi.mock('../api/video', () => ({
  initChunkUpload: vi.fn(),
  uploadChunk: vi.fn(),
  completeChunkUpload: vi.fn(),
}))
import * as api from '../api/video'
import { uploadVideoChunked } from '../lib/upload'
const size = 5 << 20
beforeEach(() => {
  vi.resetAllMocks()
  vi.mocked(api.initChunkUpload).mockResolvedValue({ upload_id: 'upload', uploaded_chunks: [] })
  vi.mocked(api.uploadChunk).mockResolvedValue({ chunk_index: 0 })
  vi.mocked(api.completeChunkUpload).mockResolvedValue({ url: '/movie.mp4' })
})
describe('chunked video upload', () => {
  it('resumes existing chunks and reports byte-accurate progress', async () => {
    vi.mocked(api.initChunkUpload).mockResolvedValue({
      upload_id: 'upload',
      uploaded_chunks: [0, 2],
    })
    const progress = vi.fn()
    const file = new File([new Uint8Array(size * 2 + 100)], 'movie.mp4')
    await expect(uploadVideoChunked(file, progress)).resolves.toEqual({ url: '/movie.mp4' })
    expect(api.uploadChunk).toHaveBeenCalledTimes(1)
    expect(api.uploadChunk).toHaveBeenCalledWith(
      'upload',
      1,
      expect.stringMatching(/^[a-f0-9]{32}$/),
      expect.any(Blob),
    )
    expect(progress).toHaveBeenCalledWith(100, '合并视频')
  })
  it('retries a failed chunk and completes only once', async () => {
    vi.mocked(api.uploadChunk)
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValue({ chunk_index: 0 })
    await uploadVideoChunked(new File(['movie'], 'movie.mp4'), vi.fn())
    expect(api.uploadChunk).toHaveBeenCalledTimes(2)
    expect(api.completeChunkUpload).toHaveBeenCalledTimes(1)
  })
  it('does not merge after retries are exhausted', async () => {
    vi.mocked(api.uploadChunk).mockRejectedValue(new Error('offline'))
    await expect(uploadVideoChunked(new File(['movie'], 'movie.mp4'), vi.fn())).rejects.toThrow(
      'offline',
    )
    expect(api.uploadChunk).toHaveBeenCalledTimes(3)
    expect(api.completeChunkUpload).not.toHaveBeenCalled()
  })
  it('bounds concurrency at three and drains in-flight work before rejecting', async () => {
    let active = 0
    let max = 0
    vi.mocked(api.uploadChunk).mockImplementation(async (_, index) => {
      active++
      max = Math.max(max, active)
      await new Promise((resolve) => setTimeout(resolve, 5))
      active--
      if (index === 0) throw new Error('offline')
      return { chunk_index: index }
    })
    await expect(
      uploadVideoChunked(new File([new Uint8Array(size * 5)], 'movie.mp4'), vi.fn()),
    ).rejects.toThrow('offline')
    expect(max).toBeLessThanOrEqual(3)
    expect(active).toBe(0)
    expect(api.completeChunkUpload).not.toHaveBeenCalled()
  })
  it('rejects empty files', async () => {
    await expect(uploadVideoChunked(new File([], 'movie.mp4'), vi.fn())).rejects.toThrow('不能为空')
    expect(api.initChunkUpload).not.toHaveBeenCalled()
  })
})
