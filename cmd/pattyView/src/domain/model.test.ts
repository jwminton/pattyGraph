import { describe, expect, it } from 'vitest'
import { parseJsonlText } from './jsonl'
import { buildSessionIndexes, newestRecord, recordIndex } from './model'

describe('PattyLog session indexes', () => {
  it('groups multiple sessions while retaining file order', () => {
    const text = [
      { schema_version: 4, event_type: 'session_start', session_id: 'a', timestamp: '2026-07-18T01:00:00Z' },
      { schema_version: 4, event_type: 'interval', session_id: 'a', timestamp: '2026-07-18T01:01:00Z', interval: 0 },
      { schema_version: 4, event_type: 'alert', session_id: 'a', timestamp: '2026-07-18T01:01:40Z', interval: 0 },
      { schema_version: 7, event_type: 'session_start', session_id: 'b', timestamp: '2026-07-18T02:00:00Z' },
      { schema_version: 7, event_type: 'future_event', session_id: 'b', timestamp: '2026-07-18T02:01:00Z' },
    ].map((value) => JSON.stringify(value)).join('\n')
    const sessions = buildSessionIndexes(parseJsonlText(text).records)

    expect(sessions.map((session) => session.id)).toEqual(['a', 'b'])
    expect(sessions[0].intervals).toHaveLength(1)
    expect(sessions[0].alerts).toHaveLength(1)
    expect(sessions[1].schemaVersions).toEqual([7])
    expect(newestRecord(sessions[0])?.eventType).toBe('alert')
    expect(recordIndex(sessions[0], sessions[0].records[0].id)).toBe(0)
  })

  it('keeps records without session ids inspectable', () => {
    const records = parseJsonlText('{"schema_version":4,"event_type":"future"}').records
    const sessions = buildSessionIndexes(records)

    expect(sessions[0].id).toBe('unscoped')
  })

  it('excludes the session envelope from the traffic time range', () => {
    const text = [
      { schema_version: 4, event_type: 'session_start', session_id: 'a', timestamp: '1970-01-01T00:00:00Z', log_time: '1970-01-01T00:00:00Z' },
      { schema_version: 4, event_type: 'interval', session_id: 'a', timestamp: '2019-01-22T11:07:00Z', log_time: '2019-01-22T11:07:00Z', interval: 0 },
      { schema_version: 4, event_type: 'control_command', session_id: 'a', timestamp: '2019-01-22T11:07:30Z', log_time: '2019-01-22T11:07:30Z' },
    ].map((value) => JSON.stringify(value)).join('\n')

    const session = buildSessionIndexes(parseJsonlText(text).records)[0]
    expect(session.firstTimestamp).toBe('2019-01-22T11:07:00Z')
    expect(session.lastTimestamp).toBe('2019-01-22T11:07:30Z')
  })
})
