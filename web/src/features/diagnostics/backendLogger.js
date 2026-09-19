// 前端调试用：把关键节点数据 POST 到后端，落到 ./data/debug/frontend.log。
// 只在开发时用；生产环境应关闭。

export function debugLog(tag, data = {}) {
  try {
    fetch('/api/v1/debug/log', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      keepalive: true,
      body: JSON.stringify({ tag, ...data }),
    }).catch(() => {})
  } catch {
    // ignore
  }
}

export function snapshotVideo(video, extra = {}) {
  if (!video) return { ...extra, noVideo: true }
  return {
    ...extra,
    src: video.currentSrc || video.src || '',
    paused: video.paused,
    ended: video.ended,
    muted: video.muted,
    volume: video.volume,
    readyState: video.readyState,
    networkState: video.networkState,
    currentTime: video.currentTime,
    duration: video.duration,
    audioDecoded: video.webkitAudioDecodedByteCount ?? null,
    videoDecoded: video.webkitVideoDecodedByteCount ?? null,
    audioTracks: video.audioTracks?.length ?? null,
    audioTrackEnabled: video.audioTracks?.[0]?.enabled ?? null,
    buffered:
      video.buffered && video.buffered.length
        ? [video.buffered.start(0), video.buffered.end(0)]
        : null,
    error: video.error
      ? { code: video.error.code, message: video.error.message }
      : null,
  }
}
