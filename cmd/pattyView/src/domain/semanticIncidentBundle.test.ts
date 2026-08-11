import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'
import { openIncidentBundle, validateIncidentBundleRecords } from './incidentBundle'
import { selectIncidentRange } from './incidentRange'
import { parseJsonlText } from './jsonl'
import { buildSessionIndexes } from './model'
import {
  planSemanticIncident,
  semanticBundleName,
  semanticIncidentBlob,
} from './semanticIncidentBundle'

describe('semantic incident selection', () => {
  it('suggests compact filenames while preserving useful source context', () => {
    expect(semanticBundleName(
      '/tmp/pattyLog.jsonl',
      '2026-08-09T14:20:00-07:00',
      '2026-08-09T15:10:59-07:00',
    )).toBe('pattyLog_20260809_1420-1510.incident.zip')
    expect(semanticBundleName(
      'traffic.incident.zip',
      '2026-08-09T23:50:00-07:00',
      '2026-08-10T00:10:00-07:00',
    )).toBe('traffic_20260809_2350-20260810_0010.incident.zip')
    expect(semanticBundleName(
      `${'é'.repeat(40)}.jsonl`,
      '2026-08-09T14:20:00Z',
      '2026-08-09T14:21:00Z',
    )).toBe(`${'é'.repeat(32)}_20260809_1420-1421.incident.zip`)
  })

  it('retains interval-owned records and explicit selected alerts', () => {
    const parsed = parseJsonlText([
      frame('session_start'),
      frame('control_command', { command: 'before zero' }),
      interval(0, '2026-08-09T08:01:00-07:00'),
      frame('control_command', { command: 'before one' }),
      interval(1, '2026-08-09T08:02:00-07:00'),
      frame('future_event', { value: 'before two' }),
      frame('alert', { interval: 1, status: 'triggered' }),
      interval(2, '2026-08-09T08:03:00-07:00'),
      frame('alert', { interval: 2, status: 'recovered' }),
      frame('control_command', { command: 'after final' }),
      interval(3, '2026-08-09T08:04:00-07:00'),
    ].join('\n'))
    const session = buildSessionIndexes(parsed.records)[0]
    const selection = selectIncidentRange(session.intervals, session.intervals[1], session.intervals[2])
    const plan = planSemanticIncident(session, selection, '/tmp/source.jsonl', '0.1.8')

    expect(plan.records.map((record) => record.eventType)).toEqual([
      'session_start',
      'control_command',
      'interval',
      'future_event',
      'alert',
      'interval',
      'alert',
    ])
    expect(plan.records.map((record) => record.data.command).filter(Boolean)).toEqual(['before one'])
    expect(plan.manifest.pattylog).toMatchObject({
      representation: 'semantic',
      source_name: 'source.jsonl',
      record_count: 7,
    })
    expect(plan.manifest.range).toMatchObject({
      from_interval: 1,
      through_interval: 2,
      interval_count: 2,
    })
  })

  it('matches the valid record membership of the Go-created bundle', async () => {
    const source = parseJsonlText(readFileSync(
      resolve(process.cwd(), 'tests/fixtures/schema4.jsonl'),
      'utf8',
    ))
    const session = buildSessionIndexes(source.records)[0]
    const selection = selectIncidentRange(session.intervals, session.intervals[0], session.intervals[1])
    const semantic = planSemanticIncident(session, selection, 'schema4.jsonl', '0.1.8')

    const goBundle = await openIncidentBundle(new Blob([new Uint8Array(readFileSync(
      resolve(process.cwd(), 'tests/fixtures/schema4.incident.zip'),
    ))]))
    const stream = new TransformStream<Uint8Array, Uint8Array>()
    const text = new Response(stream.readable).text()
    await goBundle.streamPattyLog(stream.writable, new AbortController().signal)
    const sourceBundle = parseJsonlText(await text)
    await goBundle.close()

    expect(semantic.records.map((record) => record.data)).toEqual(
      sourceBundle.records.map((record) => record.data),
    )
  })

  it('streams a bundle that reopens as the equivalent recorded model', async () => {
    const source = parseJsonlText([
      frame('session_start'),
      interval(0, '2026-08-09T08:01:00-07:00', ['GET /catalog HTTP/1.1']),
      frame('control_command', { command: '!!! fact print deployment' }),
      interval(1, '2026-08-09T08:02:00-07:00'),
    ].join('\n'))
    const session = buildSessionIndexes(source.records)[0]
    const selection = selectIncidentRange(session.intervals, session.intervals[0], session.intervals[1])
    const plan = planSemanticIncident(session, selection, 'source.jsonl', '0.1.8')
    const opened = await openIncidentBundle(await semanticIncidentBlob(plan))
    const stream = new TransformStream<Uint8Array, Uint8Array>()
    const text = new Response(stream.readable).text()
    await opened.streamPattyLog(stream.writable, new AbortController().signal)
    const parsed = parseJsonlText(await text)
    validateIncidentBundleRecords(opened.manifest, parsed)
    await opened.close()

    expect(opened.manifest.pattylog.representation).toBe('semantic')
    expect(opened.manifest.pattylog.retained_source_intervals).toBe(1)
    expect(parsed.records.map((record) => record.data)).toEqual(plan.records.map((record) => record.data))
  })
})

function frame(eventType: string, fields: Record<string, unknown> = {}): string {
  return JSON.stringify({
    schema_version: 4,
    event_type: eventType,
    session_id: 'semantic-test',
    timestamp: '2026-08-09T08:00:00-07:00',
    log_time: '2026-08-09T08:00:00-07:00',
    ...fields,
  })
}

function interval(number: number, logTime: string, sourceLines: string[] = []): string {
  return JSON.stringify({
    schema_version: 4,
    event_type: 'interval',
    session_id: 'semantic-test',
    timestamp: logTime,
    log_time: logTime,
    interval: number,
    interval_lines: 100 + number,
    source_lines: sourceLines,
    matchers: [],
    interesting: [],
  })
}
