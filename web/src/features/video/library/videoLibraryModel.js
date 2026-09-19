const COMPLETION_TOLERANCE_SECONDS = 1

export function normalizeProgress(currentTime, duration) {
  const safeDuration = positiveNumber(duration)
  const safeCurrentTime = finiteNumber(currentTime)
  return Math.min(Math.max(safeCurrentTime, 0), safeDuration)
}

export function getProgressPercent(entry) {
  const duration = positiveNumber(entry?.duration)
  if (duration === 0) return 0
  return (normalizeProgress(entry?.currentTime, duration) / duration) * 100
}

export function getResumeTime(entry) {
  const duration = positiveNumber(entry?.duration)
  const currentTime = normalizeProgress(entry?.currentTime, duration)
  if (
    entry?.completed ||
    (duration > 0 &&
      currentTime >= Math.max(0, duration - COMPLETION_TOLERANCE_SECONDS))
  ) {
    return 0
  }
  return currentTime
}

export function createVideoEntry({
  existingEntry,
  asset,
  subtitleFileName,
  subtitleHandle,
  thumbnailDataUrl = '',
  now = Date.now(),
}) {
  const duration = positiveNumber(asset.duration)
  const currentTime = existingEntry
    ? normalizeProgress(existingEntry.currentTime, duration)
    : 0

  return {
    id: asset.id,
    title: titleFromFileName(asset.name),
    fileName: asset.name,
    subtitleFileName,
    size: asset.size,
    lastModified: asset.lastModified,
    mimeType: asset.mimeType,
    duration,
    currentTime,
    completed: Boolean(existingEntry?.completed),
    thumbnailDataUrl:
      thumbnailDataUrl || existingEntry?.thumbnailDataUrl || '',
    videoHandle: asset.source.handle,
    subtitleHandle,
    createdAt: existingEntry?.createdAt || now,
    lastPlayedAt: now,
    updatedAt: now,
  }
}

export function sortVideoEntries(entries) {
  return [...entries].sort((left, right) => {
    const recentDifference =
      finiteNumber(right.lastPlayedAt) - finiteNumber(left.lastPlayedAt)
    if (recentDifference !== 0) return recentDifference
    return finiteNumber(right.createdAt) - finiteNumber(left.createdAt)
  })
}

export function titleFromFileName(fileName) {
  return String(fileName || '').replace(/\.[^.]+$/i, '')
}

function finiteNumber(value) {
  const number = Number(value)
  return Number.isFinite(number) ? number : 0
}

function positiveNumber(value) {
  return Math.max(finiteNumber(value), 0)
}
