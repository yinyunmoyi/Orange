export function formatMediaTime(seconds) {
  const total = Math.max(0, Math.floor(Number(seconds) || 0))
  const hours = Math.floor(total / 3600)
  const minutes = Math.floor((total % 3600) / 60)
  const remainder = total % 60

  if (hours > 0) {
    return `${pad(hours)}:${pad(minutes)}:${pad(remainder)}`
  }
  return `${pad(minutes)}:${pad(remainder)}`
}

export function formatLastPlayedTime(timestamp) {
  const value = Number(timestamp)
  if (!Number.isFinite(value) || value <= 0) return '时间未知'

  const date = new Date(value)
  if (!Number.isFinite(date.getTime())) return '时间未知'

  return [
    `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`,
    `${pad(date.getHours())}:${pad(date.getMinutes())}`,
  ].join(' ')
}

function pad(value) {
  return String(value).padStart(2, '0')
}
