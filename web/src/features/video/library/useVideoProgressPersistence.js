import { useCallback, useEffect, useRef } from 'react'
import { updateVideoProgress } from './videoLibraryRepository.js'
import { normalizeProgress } from './videoLibraryModel.js'

const WRITE_INTERVAL_MS = 2_000

export function useVideoProgressPersistence({ entryId, onError }) {
  const latestRef = useRef(null)
  const dirtyRef = useRef(false)
  const writingRef = useRef(false)
  const flushAfterWriteRef = useRef(false)
  const timerRef = useRef(null)
  const onErrorRef = useRef(onError)
  const activeRef = useRef(true)

  useEffect(() => {
    onErrorRef.current = onError
  }, [onError])

  const clearTimer = useCallback(() => {
    if (timerRef.current !== null) {
      window.clearTimeout(timerRef.current)
      timerRef.current = null
    }
  }, [])

  const writeLatest = useCallback(
    (immediate = false) => {
      if (!entryId || !latestRef.current) return
      if (immediate) clearTimer()
      if (writingRef.current) {
        flushAfterWriteRef.current =
          flushAfterWriteRef.current || immediate
        return
      }
      if (!dirtyRef.current) return

      const snapshot = latestRef.current
      dirtyRef.current = false
      writingRef.current = true
      updateVideoProgress(entryId, snapshot)
        .catch((error) => {
          if (activeRef.current) {
            onErrorRef.current?.(error.message || '保存播放进度失败')
          }
        })
        .finally(() => {
          writingRef.current = false
          if (!dirtyRef.current) return
          if (flushAfterWriteRef.current) {
            flushAfterWriteRef.current = false
            writeLatest(true)
            return
          }
          if (timerRef.current === null) {
            timerRef.current = window.setTimeout(() => {
              timerRef.current = null
              writeLatest()
            }, WRITE_INTERVAL_MS)
          }
        })
    },
    [clearTimer, entryId],
  )

  const scheduleWrite = useCallback(() => {
    if (!entryId || timerRef.current !== null) return
    timerRef.current = window.setTimeout(() => {
      timerRef.current = null
      writeLatest()
    }, WRITE_INTERVAL_MS)
  }, [entryId, writeLatest])

  const onProgress = useCallback(
    ({ currentTime, duration }) => {
      if (!entryId) return
      const safeDuration = Math.max(Number(duration) || 0, 0)
      latestRef.current = {
        currentTime: normalizeProgress(currentTime, safeDuration),
        duration: safeDuration,
        completed: false,
        updatedAt: Date.now(),
      }
      dirtyRef.current = true
      scheduleWrite()
    },
    [entryId, scheduleWrite],
  )

  const onPlaybackEvent = useCallback(
    ({ type, currentTime }) => {
      if (!entryId || !latestRef.current) return
      const duration = latestRef.current.duration
      latestRef.current = {
        currentTime:
          type === 'ended'
            ? duration
            : normalizeProgress(currentTime, duration),
        duration,
        completed: type === 'ended',
        updatedAt: Date.now(),
      }
      dirtyRef.current = true
      if (type === 'pause' || type === 'ended' || type === 'error') {
        writeLatest(true)
      }
    },
    [entryId, writeLatest],
  )

  useEffect(() => {
    activeRef.current = true
    const handleVisibilityChange = () => {
      if (document.visibilityState === 'hidden') writeLatest(true)
    }
    document.addEventListener('visibilitychange', handleVisibilityChange)

    return () => {
      document.removeEventListener('visibilitychange', handleVisibilityChange)
      writeLatest(true)
      clearTimer()
      activeRef.current = false
    }
  }, [clearTimer, entryId, writeLatest])

  return { onProgress, onPlaybackEvent }
}
