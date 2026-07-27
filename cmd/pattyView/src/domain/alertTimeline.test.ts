import { describe, expect, it } from 'vitest'
import { parseJsonlText } from './jsonl'
import { alertStatusSummary, buildAlertTimeline, projectRecordedAlert } from './alertTimeline'

function records(values: object[]) {
  return parseJsonlText(values.map((value) => JSON.stringify(value)).join('\n')).records
}

describe('recorded alert timeline', () => {
  it('groups supported alerts by emitted interval in file order', () => {
    const parsed = records([
      { schema_version: 4, event_type: 'alert', session_id: 'a', timestamp: '2026-07-18T08:01:20Z', interval: 3, status: 'triggered', matcher: 'errs', direction: 'above', threshold: 15, value: 18 },
      { schema_version: 4, event_type: 'alert', session_id: 'a', timestamp: '2026-07-18T08:01:40Z', interval: 3, status: 'recovered', matcher: 'errs', direction: 'above', threshold: 15, value: 8 },
      { schema_version: 4, event_type: 'alert', session_id: 'a', timestamp: '2026-07-18T08:02:00Z', interval: 4, status: 'triggered', matcher: 'Bots' },
    ])

    const timeline = buildAlertTimeline(parsed)

    expect(timeline.get(3)?.map((alert) => alert.status)).toEqual(['triggered', 'recovered'])
    expect(timeline.get(4)?.[0].matcher).toBe('Bots')
    expect(alertStatusSummary(timeline.get(3) ?? [])).toBe('2 alerts')
    expect(alertStatusSummary(timeline.get(4) ?? [])).toBe('Bots triggered')
  })

  it('keeps incomplete supported fields without inventing values', () => {
    const alert = projectRecordedAlert(records([
      { schema_version: 4, event_type: 'alert', interval: 2, status: 'future-status' },
    ])[0])

    expect(alert).toMatchObject({ interval: 2, status: 'future-status', matcher: '', value: null })
  })

  it('excludes unsupported and interval-less alerts from structured grouping', () => {
    const parsed = records([
      { schema_version: 7, event_type: 'alert', interval: 1, status: 'triggered' },
      { schema_version: 4, event_type: 'alert', status: 'triggered' },
    ])

    expect(buildAlertTimeline(parsed).size).toBe(0)
  })
})
