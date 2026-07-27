const byteUnits = ['B', 'KiB', 'MiB', 'GiB', 'TiB']

export function formatCount(value: number | null): string {
  return value === null ? '—' : new Intl.NumberFormat().format(value)
}

export function formatBytes(value: number | null): string {
  if (value === null) {
    return '—'
  }

  const sign = value < 0 ? '-' : ''
  let amount = Math.abs(value)
  let unit = 0
  while (amount >= 1024 && unit < byteUnits.length - 1) {
    amount /= 1024
    unit += 1
  }
  const precision = unit === 0 || amount >= 10 ? 0 : 1
  return `${sign}${amount.toFixed(precision)} ${byteUnits[unit]}`
}
