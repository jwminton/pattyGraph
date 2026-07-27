import { describe, expect, it } from 'vitest'
import {
  recordRowHeight,
  recordRowOverscan,
  recordScrollOffset,
  virtualRecordWindow,
} from './virtualRecordList'

describe('virtual record list', () => {
  it('renders a bounded window at the beginning of a large list', () => {
    expect(virtualRecordWindow(6000, 0, 520)).toEqual({
      start: 0,
      end: 10 + recordRowOverscan,
      paddingTop: 0,
      paddingBottom: (6000 - 10 - recordRowOverscan) * recordRowHeight,
    })
  })

  it('adds overscan around middle and ending windows', () => {
    const middle = virtualRecordWindow(6000, 3000 * recordRowHeight, 520)
    expect(middle).toEqual({
      start: 3000 - recordRowOverscan,
      end: 3010 + recordRowOverscan,
      paddingTop: (3000 - recordRowOverscan) * recordRowHeight,
      paddingBottom: (6000 - 3010 - recordRowOverscan) * recordRowHeight,
    })

    const ending = virtualRecordWindow(6000, Number.MAX_SAFE_INTEGER, 520)
    expect(ending.end).toBe(6000)
    expect(ending.paddingBottom).toBe(0)
    expect(ending.end - ending.start).toBe(10 + recordRowOverscan)
  })

  it('returns the nearest scroll offset only for an off-screen selection', () => {
    expect(recordScrollOffset(10, 0, 520)).toBe(recordRowHeight)
    expect(recordScrollOffset(5, 0, 520)).toBeNull()
    expect(recordScrollOffset(2, 10 * recordRowHeight, 520)).toBe(2 * recordRowHeight)
  })
})
