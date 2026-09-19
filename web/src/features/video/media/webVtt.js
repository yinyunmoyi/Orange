export function formatCuesToWebVTT(cues) {
  const blocks = (Array.isArray(cues) ? cues : []).map((cue, index) => {
    const start = formatTimestamp(cue.timestamp)
    const end = formatTimestamp(cue.timestamp + cue.duration)
    const settings = String(cue.settings || '').trim()
    const timeline = `${start} --> ${end}${settings ? ` ${settings}` : ''}`
    return `${index + 1}\n${timeline}\n${String(cue.text || '')}`
  })
  return `WEBVTT\n\n${blocks.join('\n\n')}${blocks.length ? '\n' : ''}`
}

function formatTimestamp(seconds) {
  const milliseconds = Math.max(0, Math.round(Number(seconds) * 1000) || 0)
  const hours = Math.floor(milliseconds / 3_600_000)
  const minutes = Math.floor((milliseconds % 3_600_000) / 60_000)
  const wholeSeconds = Math.floor((milliseconds % 60_000) / 1000)
  const remainder = milliseconds % 1000
  return [hours, minutes, wholeSeconds]
    .map((value) => String(value).padStart(2, '0'))
    .join(':')
    .concat('.', String(remainder).padStart(3, '0'))
}
