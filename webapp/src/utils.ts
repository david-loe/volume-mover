export function formatBytes(size: number): string {
  if (size < 1024) return `${size} B`
  const units = ['KB', 'MB', 'GB', 'TB', 'PB']
  let value = size
  let unit = 'B'
  for (const next of units) {
    value /= 1024
    unit = next
    if (value < 1024) break
  }
  return value >= 10 ? `${value.toFixed(0)} ${unit}` : `${value.toFixed(1)} ${unit}`
}

export function formatDateTime(value?: string): string {
  if (!value) return '—'
  return new Date(value).toLocaleString()
}

export function classNames(...values: Array<string | false | null | undefined>): string {
  return values.filter(Boolean).join(' ')
}
