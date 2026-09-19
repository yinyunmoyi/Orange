import { resolvePlaybackAdapter } from './playbackAdapters.js'
import { debugLog, snapshotVideo } from '../../diagnostics/backendLogger.js'

const STRATEGY_STORAGE_KEY = 'orange:playback-strategy:v1'
const DEFAULT_STRATEGY_CHAIN = ['native', 'hls']
const PROBE_WINDOW_MS = 1200
const PROBE_MIN_DECODED_BYTES = 1

export async function runPlaybackPipeline(video, source, options = {}) {
  if (!video || !source) throw new Error('缺少视频播放源')
  const { signal, onPrepareProgress } = options
  if (signal?.aborted) throw abortError()

  const cachedAdapterId = readCachedStrategy(source.fingerprint)
  const chain = buildAdapterChain(source, cachedAdapterId)

  let lastError = null
  for (let index = 0; index < chain.length; index += 1) {
    const adapterId = chain[index]
    const adapter = resolvePlaybackAdapter(adapterId)
    if (adapter.requiresFile && !source.file) {
      lastError = new Error(`${adapterId} 播放需要原始视频文件`)
      continue
    }

    if (signal?.aborted) throw abortError()
    if (adapterId === 'native') onPrepareProgress?.({ phase: 'probing' })
    debugLog('pipeline:try', { adapter: adapterId, index, fingerprint: source.fingerprint })

    let attachment = null
    try {
      attachment = await adapter.attach(video, source, {
        signal,
        onPrepareProgress,
      })
      if (signal?.aborted) throw abortError()
    } catch (reason) {
      if (reason?.name === 'AbortError') throw reason
      lastError = reason
      debugLog('pipeline:attach-failed', { adapter: adapterId, message: reason?.message })
      continue
    }

    const probe = await probeAttachment(video, {
      signal,
      isFinalCandidate: index === chain.length - 1,
    })
    debugLog('pipeline:probe-result', { adapter: adapterId, ...probe })
    if (signal?.aborted || probe.reason === 'aborted') throw abortError()

    if (probe.ok) {
      persistCachedStrategy(source.fingerprint, adapterId)
      return attachment
    }

    try { attachment.destroy?.() } catch { /* ignore */ }
    lastError = new Error(probe.reason || '播放探针失败')
  }

  throw lastError || new Error('无法播放该视频')
}

function buildAdapterChain(_source, cachedAdapterId) {
  const chain = [...DEFAULT_STRATEGY_CHAIN]
  if (cachedAdapterId && chain.includes(cachedAdapterId)) {
    // 命中缓存：把它提到最前面，跳过之前证明不行的策略。
    return [cachedAdapterId, ...chain.filter((id) => id !== cachedAdapterId)]
  }
  return chain
}

function probeAttachment(video, { signal, isFinalCandidate }) {
  return new Promise((resolve) => {
    if (signal?.aborted) {
      resolve({ ok: false, reason: 'aborted', snap: snapshotVideo(video) })
      return
    }
    let settled = false
    const finish = (payload) => {
      if (settled) return
      settled = true
      video.removeEventListener('loadedmetadata', onMetadata)
      video.removeEventListener('error', onError)
      signal?.removeEventListener('abort', onAbort)
      window.clearTimeout(timeoutId)
      resolve(payload)
    }
    const onMetadata = () => {
      window.setTimeout(() => {
        if (settled) return
        const snap = snapshotVideo(video)
        const audioOk = (snap.audioDecoded ?? 0) >= PROBE_MIN_DECODED_BYTES
        const videoOk = (snap.videoDecoded ?? 0) >= PROBE_MIN_DECODED_BYTES
        if (isFinalCandidate) {
          // 兜底策略：只要 attach 成功且无 error，就接受它。
          finish({ ok: !video.error, reason: video.error ? 'element-error' : '', snap })
          return
        }
        if (video.error) {
          finish({ ok: false, reason: 'element-error', snap })
          return
        }
        if (!audioOk || !videoOk) {
          finish({
            ok: false,
            reason: !audioOk && !videoOk ? 'no-decode' : !audioOk ? 'no-audio' : 'no-video',
            snap,
          })
          return
        }
        finish({ ok: true, snap })
      }, PROBE_WINDOW_MS)
    }
    const onError = () => {
      // 元素直接报错，立刻判失败
      finish({ ok: false, reason: 'element-error', snap: snapshotVideo(video) })
    }
    const onAbort = () => finish({ ok: false, reason: 'aborted', snap: snapshotVideo(video) })

    video.addEventListener('loadedmetadata', onMetadata)
    video.addEventListener('error', onError)
    signal?.addEventListener('abort', onAbort, { once: true })

    // 超时兜底：某些格式 loadedmetadata 都不触发
    const timeoutId = window.setTimeout(() => {
      finish({
        ok: false,
        reason: video.readyState >= 1 ? 'no-decode-timeout' : 'no-metadata',
        snap: snapshotVideo(video),
      })
    }, PROBE_WINDOW_MS * 6)

    // 如果 loadedmetadata 已经触发过（缓存命中场景），直接进入探测窗口
    if (video.readyState >= 1) onMetadata()
  })
}

function readCachedStrategy(fingerprint) {
  if (!fingerprint) return ''
  try {
    const raw = window.localStorage.getItem(STRATEGY_STORAGE_KEY)
    if (!raw) return ''
    const map = JSON.parse(raw)
    return typeof map?.[fingerprint] === 'string' ? map[fingerprint] : ''
  } catch {
    return ''
  }
}

function persistCachedStrategy(fingerprint, adapterId) {
  if (!fingerprint || !adapterId) return
  try {
    const raw = window.localStorage.getItem(STRATEGY_STORAGE_KEY)
    const map = raw ? JSON.parse(raw) : {}
    if (map[fingerprint] === adapterId) return
    map[fingerprint] = adapterId
    window.localStorage.setItem(STRATEGY_STORAGE_KEY, JSON.stringify(map))
  } catch {
    // ignore
  }
}

function abortError() {
  return new DOMException('视频加载已取消', 'AbortError')
}
