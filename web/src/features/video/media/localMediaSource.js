import {
  resolveMediaFormat,
  supportedMediaDescription,
} from './mediaFormats.js'
import { createPlaybackSource } from './mediaPlayback.js'

const SAMPLE_BYTES = 64 * 1024
const METADATA_TIMEOUT_MS = 15_000
const THUMBNAIL_TIMEOUT_MS = 15_000

export const VIDEO_PICKER_OPTIONS = { multiple: false }
export const VIDEO_FORMAT_DESCRIPTION = supportedMediaDescription()

export const SUBTITLE_PICKER_OPTIONS = {
  multiple: false,
  types: [
    {
      description: 'SRT 字幕',
      accept: { 'application/x-subrip': ['.srt'] },
    },
  ],
}

export async function pickFileHandle(options) {
  if (typeof window.showOpenFilePicker !== 'function') {
    return null
  }

  const [handle] = await window.showOpenFilePicker(options)
  return handle || null
}

export function createEphemeralFileHandle(file) {
  return {
    kind: 'file',
    name: file.name,
    getFile: async () => file,
  }
}

export function isPersistentFileHandle(handle) {
  return (
    handle?.kind === 'file' &&
    typeof handle.getFile === 'function' &&
    typeof handle.queryPermission === 'function' &&
    typeof handle.requestPermission === 'function'
  )
}

export async function ensureFileReadPermission(handle) {
  if (!isPersistentFileHandle(handle)) {
    throw new Error('该文件不是可持久化的本地文件句柄')
  }

  const options = { mode: 'read' }
  let permission = await handle.queryPermission(options)
  if (permission === 'prompt') {
    permission = await handle.requestPermission(options)
  }
  if (permission !== 'granted') {
    throw new Error('未获得本地文件读取权限，请再次点击并允许访问')
  }
}

export async function createMediaFingerprint(file) {
  const head = await file.slice(0, SAMPLE_BYTES).arrayBuffer()
  const tailStart = Math.max(0, file.size - SAMPLE_BYTES)
  const tail = await file.slice(tailStart).arrayBuffer()
  return fingerprintParts({
    name: file.name,
    size: file.size,
    lastModified: file.lastModified,
    headBytes: new Uint8Array(head),
    tailBytes: new Uint8Array(tail),
  })
}

export async function fingerprintParts({
  name,
  size,
  lastModified,
  headBytes,
  tailBytes,
}) {
  const metadata = new TextEncoder().encode(
    JSON.stringify({ name, size, lastModified }),
  )
  const payload = new Uint8Array(
    metadata.length + headBytes.length + tailBytes.length,
  )
  payload.set(metadata, 0)
  payload.set(headBytes, metadata.length)
  payload.set(tailBytes, metadata.length + headBytes.length)

  const digest = await globalThis.crypto.subtle.digest('SHA-256', payload)
  return Array.from(new Uint8Array(digest), (byte) =>
    byte.toString(16).padStart(2, '0'),
  ).join('')
}

export async function loadMediaAsset(handle) {
  const file = await handle.getFile()
  const format = resolveMediaFormat(file)

  const [id, videoUrl] = await Promise.all([
    createMediaFingerprint(file),
    Promise.resolve(URL.createObjectURL(file)),
  ])

  try {
    const duration =
      format.metadataAdapter === 'mediabunny'
        ? await loadVideoMetadataWithMediabunny(file)
        : await loadVideoMetadata(videoUrl)
    return {
      asset: {
        id,
        name: file.name,
        size: file.size,
        lastModified: file.lastModified,
        mimeType: file.type || format.fallbackMimeType,
        format: format.id,
        duration,
        source: {
          kind: 'local-file',
          handle,
        },
      },
      file,
      videoUrl,
      playbackSource: createPlaybackSource({
        file,
        url: videoUrl,
        fingerprint: id,
        duration,
      }),
    }
  } catch (error) {
    URL.revokeObjectURL(videoUrl)
    throw error
  }
}

export function revokeMediaUrl(videoUrl) {
  if (videoUrl) URL.revokeObjectURL(videoUrl)
}

export function createVideoThumbnail(
  videoUrl,
  duration,
  { width = 320, height = 180, quality = 0.78 } = {},
) {
  return new Promise((resolve) => {
    const video = document.createElement('video')
    let settled = false

    const cleanup = () => {
      window.clearTimeout(timeoutId)
      video.removeEventListener('loadedmetadata', handleMetadata)
      video.removeEventListener('loadeddata', handleLoadedData)
      video.removeEventListener('seeked', handleFrameReady)
      video.removeEventListener('error', handleError)
      video.removeAttribute('src')
      video.load()
    }
    const finish = (thumbnail = '') => {
      if (settled) return
      settled = true
      cleanup()
      resolve(thumbnail)
    }
    const drawFrame = () => {
      try {
        const canvas = document.createElement('canvas')
        const context = canvas.getContext('2d')
        if (!context || !video.videoWidth || !video.videoHeight) {
          finish()
          return
        }

        canvas.width = width
        canvas.height = height
        context.fillStyle = '#080808'
        context.fillRect(0, 0, width, height)

        const scale = Math.max(
          width / video.videoWidth,
          height / video.videoHeight,
        )
        const drawWidth = video.videoWidth * scale
        const drawHeight = video.videoHeight * scale
        context.drawImage(
          video,
          (width - drawWidth) / 2,
          (height - drawHeight) / 2,
          drawWidth,
          drawHeight,
        )
        finish(canvas.toDataURL('image/jpeg', quality))
      } catch {
        finish()
      }
    }
    const handleFrameReady = () => drawFrame()
    const handleLoadedData = () => {
      if (video.currentTime === 0) drawFrame()
    }
    const handleMetadata = () => {
      const safeDuration = Number.isFinite(Number(duration))
        ? Number(duration)
        : video.duration
      const targetTime = Math.min(10, Math.max(0, safeDuration * 0.1))
      if (targetTime > 0.1) {
        video.currentTime = Math.min(targetTime, Math.max(0, safeDuration - 0.1))
      } else if (video.readyState >= 2) {
        drawFrame()
      }
    }
    const handleError = () => finish()
    const timeoutId = window.setTimeout(() => finish(), THUMBNAIL_TIMEOUT_MS)

    video.muted = true
    video.playsInline = true
    video.preload = 'auto'
    video.addEventListener('loadedmetadata', handleMetadata)
    video.addEventListener('loadeddata', handleLoadedData)
    video.addEventListener('seeked', handleFrameReady)
    video.addEventListener('error', handleError)
    video.src = videoUrl
    video.load()
  })
}

async function loadVideoMetadataWithMediabunny(file) {
  const { ALL_FORMATS, BlobSource, Input } = await import('mediabunny')
  const input = new Input({
    formats: ALL_FORMATS,
    source: new BlobSource(file),
  })

  try {
    const videoTrack = await input.getPrimaryVideoTrack()
    if (!videoTrack) throw new Error('视频文件中没有可用画面')

    const duration = Number(await videoTrack.computeDuration())
    if (!Number.isFinite(duration) || duration <= 0) {
      throw new Error('无法读取视频时长')
    }
    return duration
  } finally {
    input.dispose()
  }
}

function loadVideoMetadata(videoUrl) {
  return new Promise((resolve, reject) => {
    const video = document.createElement('video')
    let settled = false

    const finish = (callback, value) => {
      if (settled) return
      settled = true
      window.clearTimeout(timeoutId)
      video.removeEventListener('loadedmetadata', handleLoaded)
      video.removeEventListener('error', handleError)
      callback(value)
    }

    const handleLoaded = () => {
      if (!Number.isFinite(video.duration) || video.duration <= 0) {
        finish(reject, new Error('无法读取视频时长'))
        return
      }
      finish(resolve, video.duration)
    }

    const handleError = () => {
      finish(
        reject,
        new Error('当前浏览器无法读取该视频，请确认容器和编解码格式受支持'),
      )
    }

    const timeoutId = window.setTimeout(() => {
      finish(reject, new Error('读取视频信息超时'))
    }, METADATA_TIMEOUT_MS)

    video.preload = 'metadata'
    video.addEventListener('loadedmetadata', handleLoaded)
    video.addEventListener('error', handleError)
    video.src = videoUrl
    video.load()
  })
}
