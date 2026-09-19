import {
  attachPlaybackSource,
  createPlaybackSource,
} from '../media/mediaPlayback.js'

const MIME_CANDIDATES = [
  'video/webm;codecs=vp8,opus',
  'video/webm;codecs=vp9,opus',
  'video/webm',
]
const AUDIO_MIME_CANDIDATES = [
  'audio/webm;codecs=opus',
  'audio/webm',
]
const MAX_CLIP_SECONDS = 60
const MAX_CLIP_BYTES = 24 * 1024 * 1024

export function selectRecorderMimeType(
  MediaRecorderClass = globalThis.MediaRecorder,
) {
  if (!MediaRecorderClass) return ''
  if (typeof MediaRecorderClass.isTypeSupported !== 'function') {
    return 'video/webm'
  }
  return (
    MIME_CANDIDATES.find((mime) =>
      MediaRecorderClass.isTypeSupported(mime),
    ) || ''
  )
}

export function selectAudioRecorderMimeType(
  MediaRecorderClass = globalThis.MediaRecorder,
) {
  if (!MediaRecorderClass) return ''
  if (typeof MediaRecorderClass.isTypeSupported !== 'function') {
    return 'audio/webm'
  }
  return (
    AUDIO_MIME_CANDIDATES.find((mime) =>
      MediaRecorderClass.isTypeSupported(mime),
    ) || ''
  )
}

export function validateRecordingRange(startSeconds, endSeconds) {
  const durationSeconds = endSeconds - startSeconds
  if (
    !Number.isFinite(startSeconds) ||
    !Number.isFinite(endSeconds) ||
    startSeconds < 0 ||
    durationSeconds <= 0 ||
    durationSeconds > MAX_CLIP_SECONDS
  ) {
    throw new Error('视频语境时长无效')
  }
  return durationSeconds
}

export async function recordContextClip({
  videoUrl,
  playbackSource,
  startSeconds,
  endSeconds,
  onProgress,
  signal,
  documentRef = globalThis.document,
  MediaRecorderClass = globalThis.MediaRecorder,
}) {
  const durationSeconds = validateRecordingRange(startSeconds, endSeconds)
  if (!videoUrl || !documentRef || !MediaRecorderClass) {
    throw new Error('当前浏览器不支持视频语境录制')
  }
  const mimeType = selectRecorderMimeType(MediaRecorderClass)
  if (!mimeType) {
    throw new Error('当前浏览器不支持 WebM 录制')
  }

  const video = documentRef.createElement('video')
  let stream
  let recorder
  let audioRecorder
  let timeoutId
  let progressId
  let abortHandler
  let playbackAttachment
  const chunks = []
  const audioChunks = []

  video.preload = 'auto'
  video.playsInline = true
  video.muted = true
  video.style.position = 'fixed'
  video.style.width = '1px'
  video.style.height = '1px'
  video.style.left = '-10000px'
  video.style.top = '0'
  video.style.opacity = '0'
  video.style.pointerEvents = 'none'
  documentRef.body.append(video)

  try {
    playbackAttachment = await attachPlaybackSource(
      video,
      playbackSource || createPlaybackSource({ url: videoUrl }),
      { signal },
    )
    await waitForMediaEvent(video, 'loadedmetadata', 15_000, signal)
    await waitForMediaEvent(video, 'loadeddata', 15_000, signal)
    if (endSeconds > video.duration + 0.1) {
      throw new Error('视频语境超出源视频范围')
    }
    if (Math.abs(video.currentTime - startSeconds) > 0.01) {
      video.currentTime = startSeconds
      await waitForMediaEvent(video, 'seeked', 15_000, signal)
    }

    const capture = video.captureStream || video.mozCaptureStream
    if (typeof capture !== 'function') {
      throw new Error('当前浏览器不支持视频语境录制')
    }
    stream = capture.call(video)
    if (!stream.getVideoTracks().length) {
      throw new Error('无法读取视频轨道')
    }
    if (!stream.getAudioTracks().length) {
      throw new Error('无法读取视频原声音轨')
    }
    const audioMimeType = selectAudioRecorderMimeType(MediaRecorderClass)
    if (!audioMimeType) {
      throw new Error('当前浏览器不支持原声音频录制')
    }

    recorder = new MediaRecorderClass(stream, {
      mimeType,
      videoBitsPerSecond: 2_500_000,
      audioBitsPerSecond: 96_000,
    })
    audioRecorder = new MediaRecorderClass(
      new MediaStream(stream.getAudioTracks()),
      {
        mimeType: audioMimeType,
        audioBitsPerSecond: 96_000,
      },
    )
    const videoStopped = recorderStopped(recorder, chunks, '视频语境录制失败')
    const audioStopped = recorderStopped(
      audioRecorder,
      audioChunks,
      '视频原声录制失败',
    )

    abortHandler = () => {
      if (recorder?.state !== 'inactive') recorder.stop()
      if (audioRecorder?.state !== 'inactive') audioRecorder.stop()
      video.pause()
    }
    signal?.addEventListener('abort', abortHandler, { once: true })
    recorder.start(1_000)
    audioRecorder.start(1_000)
    await video.play()

    progressId = globalThis.setInterval(() => {
      const elapsed = Math.max(0, video.currentTime - startSeconds)
      onProgress?.(Math.min(durationSeconds, elapsed), durationSeconds)
      if (video.currentTime >= endSeconds - 0.03 || video.ended) {
        if (recorder.state !== 'inactive') recorder.stop()
        if (audioRecorder.state !== 'inactive') audioRecorder.stop()
        video.pause()
      }
    }, 100)
    timeoutId = globalThis.setTimeout(() => {
      if (recorder.state !== 'inactive') recorder.stop()
      if (audioRecorder.state !== 'inactive') audioRecorder.stop()
      video.pause()
    }, (durationSeconds + 10) * 1_000)

    await Promise.all([videoStopped, audioStopped])
    if (signal?.aborted) throw abortError()

    const blob = new Blob(chunks, { type: recorder.mimeType || mimeType })
    const audioBlob = new Blob(audioChunks, {
      type: audioRecorder.mimeType || audioMimeType,
    })
    if (blob.size <= 0) throw new Error('视频语境录制结果为空')
    if (blob.size > MAX_CLIP_BYTES) throw new Error('视频语境文件过大')
    if (audioBlob.size <= 0) throw new Error('视频原声录制结果为空')
    onProgress?.(durationSeconds, durationSeconds)
    return {
      blob,
      audioBlob,
      mimeType: blob.type || mimeType,
      audioMimeType: audioBlob.type || audioMimeType,
      durationSeconds,
    }
  } finally {
    globalThis.clearTimeout(timeoutId)
    globalThis.clearInterval(progressId)
    signal?.removeEventListener('abort', abortHandler)
    if (recorder?.state !== 'inactive') recorder.stop()
    if (audioRecorder?.state !== 'inactive') audioRecorder.stop()
    video.pause()
    stream?.getTracks().forEach((track) => track.stop())
    playbackAttachment?.destroy()
    video.remove()
  }
}

function recorderStopped(recorder, chunks, errorMessage) {
  return new Promise((resolve, reject) => {
    recorder.addEventListener('dataavailable', (event) => {
      if (event.data?.size > 0) chunks.push(event.data)
    })
    recorder.addEventListener('stop', resolve, { once: true })
    recorder.addEventListener(
      'error',
      () => reject(new Error(errorMessage)),
      { once: true },
    )
  })
}

function waitForMediaEvent(media, eventName, timeoutMs, signal) {
  if (eventName === 'loadedmetadata' && media.readyState >= 1) {
    return Promise.resolve()
  }
  if (eventName === 'loadeddata' && media.readyState >= 2) {
    return Promise.resolve()
  }
  return new Promise((resolve, reject) => {
    const cleanup = () => {
      globalThis.clearTimeout(timeoutId)
      media.removeEventListener(eventName, handleEvent)
      media.removeEventListener('error', handleError)
      signal?.removeEventListener('abort', handleAbort)
    }
    const handleEvent = () => {
      cleanup()
      resolve()
    }
    const handleError = () => {
      cleanup()
      reject(new Error('读取源视频失败'))
    }
    const handleAbort = () => {
      cleanup()
      reject(abortError())
    }
    const timeoutId = globalThis.setTimeout(() => {
      cleanup()
      reject(new Error('读取源视频超时'))
    }, timeoutMs)

    media.addEventListener(eventName, handleEvent, { once: true })
    media.addEventListener('error', handleError, { once: true })
    signal?.addEventListener('abort', handleAbort, { once: true })
  })
}

function abortError() {
  return new DOMException('视频语境录制已取消', 'AbortError')
}
