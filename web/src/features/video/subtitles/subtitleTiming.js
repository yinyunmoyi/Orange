export const CUE_START_EPSILON_SECONDS = 0.02

export function isCueActive(
  cue,
  currentTime,
  startEpsilon = CUE_START_EPSILON_SECONDS,
) {
  return (
    cue.startSeconds - startEpsilon <= currentTime &&
    currentTime < cue.endSeconds
  )
}

export function findActiveCues(
  cues,
  currentTime,
  startEpsilon = CUE_START_EPSILON_SECONDS,
) {
  return cues.filter((cue) => isCueActive(cue, currentTime, startEpsilon))
}

export function findHighlightedCue(
  cues,
  currentTime,
  startEpsilon = CUE_START_EPSILON_SECONDS,
) {
  if (!cues.length) return null

  const targetTime = currentTime + startEpsilon
  let low = 0
  let high = cues.length - 1
  let highlightedIndex = -1

  while (low <= high) {
    const middle = Math.floor((low + high) / 2)
    if (cues[middle].startSeconds <= targetTime) {
      highlightedIndex = middle
      low = middle + 1
    } else {
      high = middle - 1
    }
  }

  return cues[Math.max(0, highlightedIndex)]
}
