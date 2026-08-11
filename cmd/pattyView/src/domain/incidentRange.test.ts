import { describe, expect, it } from 'vitest'
import type { PattyLogRecord } from './types'
import { buildIncidentRangeCommand, selectIncidentRange } from './incidentRange'

describe('incident range selection', () => {
  it('normalizes either drag direction to file order', () => {
    const intervals = [record('old', 2, '2026-08-09T08:01:00-07:00'),
      record('middle', 3, '2026-08-09T08:02:00-07:00'),
      record('new', 4, '2026-08-09T08:03:00-07:00')]

    expect(selectIncidentRange(intervals, intervals[2], intervals[0])).toEqual({
      from: intervals[0],
      through: intervals[2],
      intervalCount: 3,
    })
    expect(selectIncidentRange(intervals, intervals[0], intervals[1])).toEqual({
      from: intervals[0],
      through: intervals[1],
      intervalCount: 2,
    })
  })

  it('builds an explicit, shell-quoted bundle command', () => {
    const intervals = [record('old', 2, '2026-08-09T08:01:00-07:00'),
      record('new', 3, '2026-08-09T08:02:00-07:00')]
    const result = buildIncidentRangeCommand(
      selectIncidentRange(intervals, intervals[0], intervals[1]),
      intervals,
      "Bob's PattyLog.jsonl",
      "operator's-session",
    )

    expect(result.error).toBe('')
    expect(result.command).toBe(
      `pattyView --bundle 'Bob'"'"'s PattyLog.jsonl' --from '2026-08-09T08:01:00-07:00' ` +
      `--through '2026-08-09T08:02:00-07:00' --session 'operator'"'"'s-session'`,
    )
  })

  it('rejects ranges the current CLI cannot identify safely', () => {
    const repeated = [record('a', 1, '2026-08-09T08:01:00-07:00'),
      record('b', 2, '2026-08-09T08:01:00-07:00')]
    expect(buildIncidentRangeCommand(
      selectIncidentRange(repeated, repeated[0], repeated[1]), repeated, 'input.jsonl', 'session',
    ).error).toContain('repeated endpoint')

    const reversed = [record('a', 1, '2026-08-09T08:02:00-07:00'),
      record('b', 2, '2026-08-09T08:01:00-07:00')]
    expect(buildIncidentRangeCommand(
      selectIncidentRange(reversed, reversed[0], reversed[1]), reversed, 'input.jsonl', 'session',
    ).error).toContain('clocks move backward')
  })

  it('requires distinct endpoints from the current session', () => {
    const intervals = [record('a', 1, '2026-08-09T08:01:00-07:00')]
    expect(() => selectIncidentRange(intervals, intervals[0], intervals[0])).toThrow('at least two')
    expect(() => selectIncidentRange(intervals, intervals[0], record('other', 2, '2026-08-09T08:02:00-07:00')))
      .toThrow('current session')
  })
})

function record(id: string, interval: number, logTime: string): PattyLogRecord {
  return {
    id,
    lineNumber: interval + 1,
    fileOrder: interval,
    schemaVersion: 4,
    eventType: 'interval',
    sessionId: 'session',
    timestamp: logTime,
    logTime,
    interval,
    data: {},
  }
}
