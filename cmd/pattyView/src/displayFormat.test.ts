import { describe, expect, it } from 'vitest'
import { formatBytes, formatCount } from './displayFormat'

describe('display formatting', () => {
  it('formats counts and unavailable values consistently', () => {
    expect(formatCount(null)).toBe('—')
    expect(formatCount(1234)).toBe(new Intl.NumberFormat().format(1234))
  })

  it('formats positive and signed byte values', () => {
    expect(formatBytes(null)).toBe('—')
    expect(formatBytes(0)).toBe('0 B')
    expect(formatBytes(1023)).toBe('1023 B')
    expect(formatBytes(1024)).toBe('1.0 KiB')
    expect(formatBytes(10 * 1024)).toBe('10 KiB')
    expect(formatBytes(-1536)).toBe('-1.5 KiB')
  })
})
