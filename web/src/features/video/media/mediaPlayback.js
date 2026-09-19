import { runPlaybackPipeline } from './playbackPipeline.js'

export function createPlaybackSource({
  file = null,
  url = '',
  fingerprint = '',
  duration = 0,
} = {}) {
  if (!url) throw new Error('缺少视频播放地址')
  return {
    file,
    url,
    fingerprint,
    ...(Number.isFinite(duration) && duration > 0 ? { duration } : {}),
  }
}

export async function attachPlaybackSource(video, source, options = {}) {
  if (!video || !source) throw new Error('缺少视频播放源')
  if (options.signal?.aborted) throw abortError()
  return runPlaybackPipeline(video, source, options)
}

function abortError() {
  return new DOMException('视频加载已取消', 'AbortError')
}
