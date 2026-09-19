import { useCallback, useEffect, useRef, useState } from 'react'
import { attachPlaybackSource } from '../media/mediaPlayback.js'
import { debugLog, snapshotVideo } from '../../diagnostics/backendLogger.js'

// 浏览器自动播放策略要求“最近一次用户手势”才允许带声播放。
// 首次导入视频时，metadata 到达的时机距离最近一次点击已过去几秒，Chrome 会
// 拒掉 video.play() 并抛 NotAllowedError。这里加一层降级：先尝试正常带声播放，
// 若被拒则切到静音继续播放，再在 playing 事件里尝试恢复音量。这样用户不需要
// 二次点击就能看到画面动起来。
async function autoplayWithFallback(video, log) {
  try {
    await video.play()
    return 'played'
  } catch (reason) {
    if (reason?.name !== 'NotAllowedError') throw reason
    log?.('play:autoplay-blocked', { name: reason.name, message: reason.message })
    const previousVolume = video.volume
    video.muted = true
    try {
      await video.play()
    } catch (mutedReason) {
      video.muted = false
      throw mutedReason
    }
    const restore = () => {
      video.muted = false
      video.volume = previousVolume
      video.removeEventListener('playing', restore)
    }
    video.addEventListener('playing', restore, { once: true })
    return 'played-muted'
  }
}

export function useVideoPlayback({
  playbackSource,
  initialTime = 0,
  onProgress,
  onPlaybackEvent,
  onPrepareProgress,
}) {
  const videoRef = useRef(null)
  const progressCallbackRef = useRef(onProgress)
  const eventCallbackRef = useRef(onPlaybackEvent)
  const prepareCallbackRef = useRef(onPrepareProgress)
  const initialTimeRef = useRef(initialTime)
  const [currentTime, setCurrentTime] = useState(initialTime)
  const [duration, setDuration] = useState(0)
  const [isPlaying, setIsPlaying] = useState(false)
  const [isPreparing, setIsPreparing] = useState(false)
  const [codecPath, setCodecPath] = useState(null)
  const [error, setError] = useState('')

  useEffect(() => {
    progressCallbackRef.current = onProgress
  }, [onProgress])

  useEffect(() => {
    eventCallbackRef.current = onPlaybackEvent
  }, [onPlaybackEvent])

  useEffect(() => {
    prepareCallbackRef.current = onPrepareProgress
  }, [onPrepareProgress])

  useEffect(() => {
    const video = videoRef.current
    if (!video || !playbackSource) return undefined

    const startTime = initialTimeRef.current
    const controller = new AbortController()
    let attachment = null
    let cancelled = false
    let metadataHandled = false
    let pollTimer = null
    debugLog('effect:start', {
      fingerprint: playbackSource.fingerprint,
      initialTime: startTime,
      snap: snapshotVideo(video),
    })
    setCurrentTime(startTime)
    setDuration(0)
    setIsPlaying(false)
    setIsPreparing(true)
    setCodecPath(null)
    setError('')

    const emitProgress = () => {
      const nextTime = Number.isFinite(video.currentTime) ? video.currentTime : 0
      const nextDuration = Number.isFinite(video.duration) ? video.duration : 0
      setCurrentTime(nextTime)
      setDuration(nextDuration)
      progressCallbackRef.current?.({
        currentTime: nextTime,
        duration: nextDuration,
      })
    }

    const emitEvent = (type) => {
      eventCallbackRef.current?.({
        type,
        currentTime: Number.isFinite(video.currentTime) ? video.currentTime : 0,
      })
    }

    const handleMetadata = () => {
      metadataHandled = true
      const nextDuration = Number.isFinite(video.duration) ? video.duration : 0
      setDuration(nextDuration)
      if (startTime > 0 && (nextDuration <= 0 || startTime < nextDuration)) {
        video.currentTime = startTime
      }
      video.muted = false
      video.volume = 1
      emitProgress()
      debugLog('metadata', { snap: snapshotVideo(video) })
      autoplayWithFallback(video, debugLog).then((outcome) => {
        debugLog('play:resolved', { outcome, snap: snapshotVideo(video) })
      }).catch((reason) => {
        setIsPlaying(false)
        debugLog('play:rejected', {
          name: reason?.name,
          message: reason?.message,
          snap: snapshotVideo(video),
        })
      })
    }

    const handlePlay = () => {
      setIsPlaying(true)
      emitEvent('play')
    }
    const handlePause = () => {
      setIsPlaying(false)
      emitEvent('pause')
    }
    const handleEnded = () => {
      setIsPlaying(false)
      emitProgress()
      emitEvent('ended')
    }
    const handleError = () => {
      if (!attachment) return
      setIsPlaying(false)
      setError('当前浏览器无法播放该视频源文件')
      emitEvent('error')
    }

    video.addEventListener('loadedmetadata', handleMetadata)
    video.addEventListener('durationchange', emitProgress)
    video.addEventListener('timeupdate', emitProgress)
    video.addEventListener('play', handlePlay)
    video.addEventListener('pause', handlePause)
    video.addEventListener('ended', handleEnded)
    video.addEventListener('error', handleError)

    attachPlaybackSource(video, playbackSource, {
      signal: controller.signal,
      onPrepareProgress: (info) => prepareCallbackRef.current?.(info),
    })
      .then((result) => {
        if (cancelled) {
          try { result?.destroy?.() } catch { /* ignore */ }
          debugLog('attach:late-destroy', { mode: result?.mode })
          return
        }
        attachment = result
        setCodecPath(result.codecPath)
        setIsPreparing(false)
        debugLog('attach:ok', { mode: result.mode, snap: snapshotVideo(video) })
        pollTimer = window.setInterval(() => {
          debugLog('poll', { snap: snapshotVideo(video) })
        }, 2000)
        if (!metadataHandled && video.readyState >= 1) handleMetadata()
      })
      .catch((reason) => {
        if (reason.name === 'AbortError') return
        setIsPreparing(false)
        setIsPlaying(false)
        setError(reason.message || '视频加载失败')
        emitEvent('error')
        debugLog('attach:error', {
          name: reason?.name,
          message: reason?.message,
        })
      })

    return () => {
      cancelled = true
      controller.abort()
      if (pollTimer) window.clearInterval(pollTimer)
      debugLog('effect:cleanup', { snap: snapshotVideo(video) })
      attachment?.destroy()
      video.removeEventListener('loadedmetadata', handleMetadata)
      video.removeEventListener('durationchange', emitProgress)
      video.removeEventListener('timeupdate', emitProgress)
      video.removeEventListener('play', handlePlay)
      video.removeEventListener('pause', handlePause)
      video.removeEventListener('ended', handleEnded)
      video.removeEventListener('error', handleError)
    }
  }, [playbackSource])

  const togglePlayback = useCallback(() => {
    const video = videoRef.current
    if (!video) return
    if (video.paused || video.ended) {
      video.play().catch(() => {
        setError('浏览器阻止了播放，请再次点击播放按钮')
      })
    } else {
      video.pause()
    }
  }, [])

  const pausePlayback = useCallback(() => {
    videoRef.current?.pause()
  }, [])

  const seekTo = useCallback((seconds) => {
    const video = videoRef.current
    if (!video) return
    const max = Number.isFinite(video.duration) ? video.duration : 0
    const numeric = Number(seconds)
    const target = Number.isFinite(numeric) ? Math.max(numeric, 0) : 0
    const nextTime = max > 0 ? Math.min(target, max) : target
    video.currentTime = nextTime
    setCurrentTime(nextTime)
    progressCallbackRef.current?.({
      currentTime: nextTime,
      duration: max,
    })
  }, [])

  return {
    videoRef,
    currentTime,
    duration,
    isPlaying,
    isPreparing,
    codecPath,
    error,
    togglePlayback,
    pausePlayback,
    seekTo,
  }
}
