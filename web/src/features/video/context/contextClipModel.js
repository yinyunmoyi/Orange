const MAX_CLIP_SECONDS = 60
const MAX_FOCUSED_CLIP_SECONDS = 12

export function buildContextClip({
  sentenceStart,
  sentenceEnd,
  focusStart,
  focusEnd,
  cueRanges,
}) {
  if (
    !Number.isInteger(sentenceStart) ||
    !Number.isInteger(sentenceEnd) ||
    sentenceStart < 0 ||
    sentenceEnd <= sentenceStart ||
    !Array.isArray(cueRanges)
  ) {
    throw new Error('后端返回的句子范围无效')
  }

  let matched = cueRanges.filter(
    (cue) => cue.textStart < sentenceEnd && cue.textEnd > sentenceStart,
  )
  if (matched.length === 0) {
    throw new Error('无法定位句子对应的字幕块')
  }
  matched = focusedCueWindow(matched, focusStart, focusEnd)

  const clipStartSeconds = matched[0].startSeconds
  const clipEndSeconds = matched[matched.length - 1].endSeconds
  const durationSeconds = clipEndSeconds - clipStartSeconds
  if (
    !Number.isFinite(clipStartSeconds) ||
    !Number.isFinite(clipEndSeconds) ||
    durationSeconds <= 0 ||
    durationSeconds > MAX_CLIP_SECONDS
  ) {
    throw new Error('视频语境时长无效')
  }

  const durationMs = Math.round(durationSeconds * 1000)
  const subtitles = matched.map((cue) => ({
    startMs: clamp(
      Math.round((cue.startSeconds - clipStartSeconds) * 1000),
      0,
      durationMs,
    ),
    endMs: clamp(
      Math.round((cue.endSeconds - clipStartSeconds) * 1000),
      0,
      durationMs,
    ),
    text: cue.text,
  }))

  return {
    clipStartSeconds,
    clipEndSeconds,
    durationSeconds,
    durationMs,
    subtitles,
  }
}

function focusedCueWindow(cues, focusStart, focusEnd) {
  const fullDuration =
    cues[cues.length - 1].endSeconds - cues[0].startSeconds
  if (
    fullDuration <= MAX_FOCUSED_CLIP_SECONDS ||
    !Number.isInteger(focusStart) ||
    !Number.isInteger(focusEnd) ||
    focusEnd <= focusStart
  ) {
    return cues
  }

  let first = cues.findIndex(
    (cue) => cue.textStart < focusEnd && cue.textEnd > focusStart,
  )
  if (first < 0) return cues

  let last = first
  while (
    last + 1 < cues.length &&
    cues[last + 1].textStart < focusEnd &&
    cues[last + 1].textEnd > focusStart
  ) {
    last += 1
  }

  const focusClipStart = cues[first].startSeconds
  const focusClipEnd = cues[last].endSeconds
  while (first > 0 || last < cues.length - 1) {
    const leftDuration =
      first > 0 ? cues[last].endSeconds - cues[first - 1].startSeconds : Infinity
    const rightDuration =
      last < cues.length - 1
        ? cues[last + 1].endSeconds - cues[first].startSeconds
        : Infinity
    const canAddLeft = leftDuration <= MAX_FOCUSED_CLIP_SECONDS
    const canAddRight = rightDuration <= MAX_FOCUSED_CLIP_SECONDS
    if (!canAddLeft && !canAddRight) break

    if (!canAddRight) {
      first -= 1
      continue
    }
    if (!canAddLeft) {
      last += 1
      continue
    }

    const leftBalance = Math.abs(
      focusClipStart - cues[first - 1].startSeconds -
        (cues[last].endSeconds - focusClipEnd),
    )
    const rightBalance = Math.abs(
      focusClipStart - cues[first].startSeconds -
        (cues[last + 1].endSeconds - focusClipEnd),
    )
    if (leftBalance <= rightBalance) {
      first -= 1
    } else {
      last += 1
    }
  }

  return cues.slice(first, last + 1)
}

function clamp(value, min, max) {
  return Math.min(Math.max(value, min), max)
}
