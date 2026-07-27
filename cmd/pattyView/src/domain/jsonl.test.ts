import { describe, expect, it } from 'vitest'
import { IncrementalJsonlParser, parseJsonlText } from './jsonl'

const interval = JSON.stringify({
  schema_version: 4,
  event_type: 'interval',
  session_id: 'session-a',
  timestamp: '2026-07-18T08:00:00-07:00',
  interval: 3,
  interval_lines: 120,
})

describe('IncrementalJsonlParser', () => {
  it('holds a partial record until the newline arrives', () => {
    const parser = new IncrementalJsonlParser()
    const split = Math.floor(interval.length / 2)

    expect(parser.feed(interval.slice(0, split)).records).toHaveLength(0)
    const result = parser.feed(`${interval.slice(split)}\n`)

    expect(result.records).toHaveLength(1)
    expect(result.records[0].interval).toBe(3)
    expect(result.records[0].lineNumber).toBe(1)
  })

  it('finalizes a valid last line without a trailing newline', () => {
    const parser = new IncrementalJsonlParser()
    parser.feed(interval)

    expect(parser.finish().records).toHaveLength(1)
  })

  it('accepts blank lines and CRLF records', () => {
    const result = parseJsonlText(`\r\n${interval}\r\n`)

    expect(result.records).toHaveLength(1)
    expect(result.records[0].lineNumber).toBe(2)
    expect(result.issues).toHaveLength(0)
  })

  it('preserves malformed and unknown records without stopping', () => {
    const unknown = JSON.stringify({
      schema_version: 4,
      event_type: 'future_event',
      session_id: 'session-a',
    })
    const result = parseJsonlText(`{bad json}\n${unknown}\n${interval}\n`)

    expect(result.issues).toHaveLength(1)
    expect(result.issues[0].lineNumber).toBe(1)
    expect(result.records.map((record) => record.eventType)).toEqual([
      'future_event',
      'interval',
    ])
  })

  it('rejects JSON values that are not record objects', () => {
    const result = parseJsonlText('[1,2,3]\n')

    expect(result.records).toHaveLength(0)
    expect(result.issues[0].message).toContain('not a JSON object')
  })

  it('prefers log_time and falls back to timestamp for older records', () => {
    const current = JSON.stringify({
      schema_version: 4,
      event_type: 'interval',
      timestamp: '2026-07-18T08:00:00Z',
      log_time: '2019-01-22T11:07:00-08:00',
    })
    const records = parseJsonlText(`${current}\n${interval}`).records

    expect(records[0].logTime).toBe('2019-01-22T11:07:00-08:00')
    expect(records[1].logTime).toBe('2026-07-18T08:00:00-07:00')
  })
})
