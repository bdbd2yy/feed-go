import SparkMD5 from 'spark-md5'
import * as api from '../api/video'
const CHUNK_SIZE = 5 << 20
async function hash(blob: Blob) {
  const spark = new SparkMD5.ArrayBuffer()
  for (let offset = 0; offset < blob.size; offset += 2 << 20)
    spark.append(await blob.slice(offset, offset + (2 << 20)).arrayBuffer())
  return spark.end()
}
export async function uploadVideoChunked(
  file: File,
  onProgress: (percent: number, stage: string) => void,
) {
  if (!file.size) throw new Error('视频文件不能为空')
  onProgress(0, '校验视频')
  const total = Math.ceil(file.size / CHUNK_SIZE)
  const init = await api.initChunkUpload({
    filename: file.name,
    file_size: file.size,
    chunk_size: CHUNK_SIZE,
    total_chunks: total,
    file_hash: await hash(file),
  })
  const uploaded = new Set(init.uploaded_chunks.filter((i) => i >= 0 && i < total))
  let bytes = [...uploaded].reduce(
    (sum, i) => sum + Math.min(CHUNK_SIZE, file.size - i * CHUNK_SIZE),
    0,
  )
  const pending = Array.from({ length: total }, (_, i) => i).filter((i) => !uploaded.has(i))
  let next = 0
  let failure: unknown
  onProgress(Math.round((bytes / file.size) * 100), '上传视频')
  async function worker() {
    while (next < pending.length && !failure) {
      const index = pending[next++]!
      try {
        const blob = file.slice(index * CHUNK_SIZE, (index + 1) * CHUNK_SIZE)
        const digest = await hash(blob)
        for (let attempt = 0; attempt < 3; attempt++) {
          try {
            await api.uploadChunk(init.upload_id, index, digest, blob)
            break
          } catch (error) {
            if (attempt === 2) throw error
          }
        }
        bytes += blob.size
        onProgress(Math.round((bytes / file.size) * 100), '上传视频')
      } catch (error) {
        failure = error
      }
    }
  }
  await Promise.all(Array.from({ length: Math.min(3, pending.length) }, worker))
  if (failure) throw failure
  onProgress(100, '合并视频')
  return api.completeChunkUpload(init.upload_id)
}
