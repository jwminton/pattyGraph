import { describe, expect, it } from 'vitest'
import { classifyFileChange, type FileSnapshot } from './fileChange'

const base: FileSnapshot = {
  size: 100,
  lastModified: 10,
  fingerprint: 'abc123',
}

describe('classifyFileChange', () => {
  it('recognizes append-only growth', () => {
    expect(classifyFileChange(base, { ...base, size: 140, lastModified: 11 }, 100)).toBe('append')
  })

  it('recognizes truncation and replacement', () => {
    expect(classifyFileChange(base, { ...base, size: 50 }, 100)).toBe('reset')
    expect(classifyFileChange(base, { ...base, fingerprint: 'different' }, 100)).toBe('reset')
  })

  it('does not treat metadata-only changes as data', () => {
    expect(classifyFileChange(base, { ...base, lastModified: 20 }, 100)).toBe('none')
  })

  it('waits for a complete first-line fingerprint', () => {
    const pending = { ...base, fingerprint: '' }
    expect(classifyFileChange(pending, { ...base, size: 140 }, 100)).toBe('append')
  })
})
