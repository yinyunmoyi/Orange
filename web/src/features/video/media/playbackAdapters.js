import { recordRuntimeDiagnostic } from '../../../diagnostics/runtimeDiagnostics.js'
import { debugLog } from '../../diagnostics/backendLogger.js'

const adapters = new Map()

export function registerPlaybackAdapter(adapter) {
  const id = String(adapter?.id || '').trim()
  if (!id || typeof adapter.attach !== 'function') {
    throw new Error('无效的视频播放适配器')
  }
  adapters.set(id, adapter)
}

export function resolvePlaybackAdapter(id) {
  const adapter = adapters.get(String(id || 'native'))
  if (!adapter) throw new Error(`未注册的视频播放适配器：${id}`)
  return adapter
}

registerPlaybackAdapter({
  id: 'native',
  requiresFile: false,
  async attach(video, source) {
    recordRuntimeDiagnostic('media:attached', { adapter: 'native' })
    video.src = source.url
    video.load()
    return {
      mode: 'native',
      codecPath: null,
      destroy() {
        video.pause()
        video.removeAttribute('src')
        video.srcObject = null
        video.load()
      },
    }
  },
})

registerPlaybackAdapter({
  id: 'hls',
  requiresFile: true,
  async attach(video, source, options = {}) {
    if (!source.file) throw new Error('HLS 播放需要原始视频文件')
    if (!source.fingerprint) throw new Error('HLS 播放需要视频指纹')
    const { signal, onPrepareProgress } = options

    const playlistUrl = await ensureHLSPlaylist({
      file: source.file,
      fingerprint: source.fingerprint,
      duration: source.duration,
      signal,
      onProgress: onPrepareProgress,
    })
    if (signal?.aborted) throw abortError()

    const { default: Hls } = await import('hls.js')
    onPrepareProgress?.({ phase: 'loading' })

    // Chrome/Edge 的 canPlayType('application/vnd.apple.mpegurl') 偶尔返回非空字符串，
    // 但它们的 native HLS 走 MSE 直挂 mpegts + AAC 解不了（CHUNK_DEMUXER_ERROR_APPEND_FAILED）。
    // 只有真正无法运行 hls.js 的环境（Safari / iOS WebKit）才回退到 native。
    if (!Hls.isSupported() && video.canPlayType('application/vnd.apple.mpegurl')) {
      recordRuntimeDiagnostic('media:attached', { adapter: 'hls-native' })
      video.src = playlistUrl
      video.load()
      return {
        mode: 'hls-native',
        codecPath: { mode: 'hls-native' },
        playlistUrl,
        destroy() {
          video.pause()
          video.removeAttribute('src')
          video.load()
        },
      }
    }

    if (!Hls.isSupported()) {
      throw new Error('当前浏览器不支持 HLS 播放')
    }

    const hls = new Hls({
      // 播放期间才拉分片，避免空跑消耗
      startFragPrefetch: true,
      lowLatencyMode: false,
      maxBufferLength: 30,
      maxMaxBufferLength: 60,
    })
    let destroyed = false

    return new Promise((resolve, reject) => {
      const handleAbort = () => {
        cleanup()
        try {
          hls.destroy()
        } catch {
          // ignore
        }
        reject(abortError())
      }
      const handleManifestParsed = () => {
        cleanup()
        recordRuntimeDiagnostic('media:attached', { adapter: 'hls-js' })
        resolve({
          mode: 'hls-js',
          codecPath: { mode: 'hls-js' },
          playlistUrl,
          destroy() {
            if (destroyed) return
            destroyed = true
            try {
              hls.destroy()
            } catch {
              // ignore
            }
            video.pause()
            video.removeAttribute('src')
            video.load()
          },
        })
      }
      const handleError = (_event, data) => {
        if (!data?.fatal) return
        cleanup()
        try {
          hls.destroy()
        } catch {
          // ignore
        }
        recordRuntimeDiagnostic('media:error', {
          adapter: 'hls-js',
          message: data?.details || 'hls-js fatal',
        })
        reject(new Error('HLS 播放引擎加载失败'))
      }
      const cleanup = () => {
        hls.off(Hls.Events.MANIFEST_PARSED, handleManifestParsed)
        hls.off(Hls.Events.ERROR, handleError)
        signal?.removeEventListener('abort', handleAbort)
      }

      hls.on(Hls.Events.MANIFEST_PARSED, handleManifestParsed)
      hls.on(Hls.Events.ERROR, handleError)
      hls.on(Hls.Events.LEVEL_LOADED, (_e, data) => {
        debugLog('hls:level-loaded', {
          fragments: data?.details?.fragments?.length,
          totalduration: data?.details?.totalduration,
          startSN: data?.details?.startSN,
          endSN: data?.details?.endSN,
        })
      })
      hls.on(Hls.Events.FRAG_LOADING, (_e, data) => {
        debugLog('hls:frag-loading', { sn: data?.frag?.sn, start: data?.frag?.start, end: data?.frag?.end })
      })
      hls.on(Hls.Events.FRAG_LOADED, (_e, data) => {
        debugLog('hls:frag-loaded', { sn: data?.frag?.sn, start: data?.frag?.start, end: data?.frag?.end })
      })
      hls.on(Hls.Events.FRAG_PARSED, (_e, data) => {
        debugLog('hls:frag-parsed', { sn: data?.frag?.sn })
      })
      hls.on(Hls.Events.BUFFER_APPENDED, (_e, data) => {
        debugLog('hls:buffer-appended', { type: data?.type, timeRanges: Object.keys(data?.timeRanges || {}) })
      })
      hls.on(Hls.Events.ERROR, (_e, data) => {
        if (data?.fatal) return
        debugLog('hls:error-non-fatal', { type: data?.type, details: data?.details, reason: data?.reason })
      })
      signal?.addEventListener('abort', handleAbort, { once: true })
      hls.loadSource(playlistUrl)
      hls.attachMedia(video)
    })
  },
})

const hlsImportInFlight = new Map()

async function ensureHLSPlaylist({
  file,
  fingerprint,
  duration,
  signal,
  onProgress,
}) {
  const statusUrl = `/api/v1/media/hls/${fingerprint}/status`
  const playlistUrl = `/api/v1/media/hls/${fingerprint}/master.m3u8`

  onProgress?.({ phase: 'cache-check' })
  try {
    const response = await fetch(statusUrl, { signal })
    if (response.ok) {
      const payload = await response.json()
      if (payload?.data?.ready) {
        recordRuntimeDiagnostic('hls:cache-hit', { fingerprint })
        onProgress?.({ phase: 'loading' })
        return playlistUrl
      }
    }
  } catch (error) {
    if (signal?.aborted) throw abortError()
    // status 查询失败时继续尝试上传
    recordRuntimeDiagnostic('hls:status-failed', {
      fingerprint,
      message: error?.message || '',
    })
  }

  if (!hlsImportInFlight.has(fingerprint)) {
    const task = {
      listeners: new Set(),
      promise: null,
    }
    task.promise = uploadHLSSource({
      file,
      fingerprint,
      duration,
      onProgress(info) {
        task.listeners.forEach((listener) => listener(info))
      },
    }).finally(() => {
        hlsImportInFlight.delete(fingerprint)
      })
    hlsImportInFlight.set(fingerprint, task)
  }
  const task = hlsImportInFlight.get(fingerprint)
  if (typeof onProgress === 'function') task.listeners.add(onProgress)
  const raceAbort = new Promise((_, reject) => {
    if (!signal) return
    if (signal.aborted) reject(abortError())
    signal.addEventListener('abort', () => reject(abortError()), { once: true })
  })
  try {
    await Promise.race([task.promise, raceAbort])
  } finally {
    task.listeners.delete(onProgress)
  }
  onProgress?.({ phase: 'loading' })
  return playlistUrl
}

function uploadHLSSource({ file, fingerprint, duration, onProgress }) {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest()
    const statusUrl = `/api/v1/media/hls/${fingerprint}/status`
    const durationParam =
      Number.isFinite(duration) && duration > 0
        ? `&duration=${encodeURIComponent(duration)}`
        : ''
    let settled = false
    let progressTimer = null
    const stopProgressPolling = () => {
      settled = true
      if (progressTimer) window.clearTimeout(progressTimer)
    }
    const pollProgress = async () => {
      if (settled) return
      try {
        const response = await fetch(statusUrl)
        const payload = response.ok ? await response.json() : null
        const status = payload?.data
        if (status?.phase === 'transcoding') {
          onProgress?.({
            phase: 'transcode',
            progress: Number(status.progress) || 0,
          })
        }
      } catch {
        // 导入请求本身会负责报告最终错误，轮询失败不打断转码。
      }
      if (!settled) progressTimer = window.setTimeout(pollProgress, 500)
    }
    xhr.open(
      'POST',
      `/api/v1/media/hls/import?fingerprint=${encodeURIComponent(fingerprint)}${durationParam}`,
    )
    xhr.setRequestHeader(
      'Content-Type',
      file.type || 'application/octet-stream',
    )
    xhr.upload.onprogress = (event) => {
      if (!event.lengthComputable || typeof onProgress !== 'function') return
      onProgress({
        phase: 'upload',
        loaded: event.loaded,
        total: event.total,
      })
    }
    xhr.upload.onload = () => {
      onProgress?.({ phase: 'transcode', progress: 0 })
      void pollProgress()
    }
    xhr.onload = () => {
      stopProgressPolling()
      if (xhr.status >= 200 && xhr.status < 300) {
        try {
          const payload = JSON.parse(xhr.responseText || '{}')
          if (payload.code === 0) {
            onProgress?.({
              phase: 'ready',
              loaded: file.size,
              total: file.size,
            })
            recordRuntimeDiagnostic('hls:import-ok', {
              fingerprint,
              cached: Boolean(payload?.data?.cachedHit),
              size: file.size,
            })
            resolve()
            return
          }
          reject(new Error(payload.msg || '视频切片失败'))
          return
        } catch {
          reject(new Error('视频切片响应解析失败'))
          return
        }
      }
      recordRuntimeDiagnostic('hls:import-failed', {
        fingerprint,
        status: xhr.status,
      })
      if (xhr.status === 0) {
        reject(new Error('后端服务不可用，请稍后再试'))
      } else {
        reject(new Error(`视频切片失败（${xhr.status}）`))
      }
    }
    xhr.onerror = () => {
      stopProgressPolling()
      reject(new Error('后端服务不可用，请稍后再试'))
    }
    xhr.onabort = () => {
      stopProgressPolling()
      reject(abortError())
    }
    onProgress?.({ phase: 'upload', loaded: 0, total: file.size })
    xhr.send(file)
  })
}

function abortError() {
  return new DOMException('视频加载已取消', 'AbortError')
}
