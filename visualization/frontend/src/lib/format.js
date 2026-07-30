export function formatTime(value) { return value ? new Date(value).toLocaleString() : '-' }

export function formatNanos(value) {
  if (!value) return '0 ms'
  if (value < 1000) return `${value} ns`
  if (value < 1000000) return `${(value / 1000).toFixed(2)} μs`
  return `${(value / 1000000).toFixed(2)} ms`
}
