import { describe, expect, it } from 'vitest'
import { parseJsonlText } from './jsonl'
import {
  groupSearchResultsByInterval,
  searchSessionRecords,
  searchTextMatches,
  searchTextSegments,
} from './searchSession'

function records(values: unknown[]) {
  return parseJsonlText(values.map((value) => JSON.stringify(value)).join('\n')).records
}

function interval(number: number, interesting: unknown[] = [], factoids: unknown[] = []) {
  return {
    schema_version: 4,
    event_type: 'interval',
    session_id: 'search',
    timestamp: `2019-01-22T11:0${number}:00Z`,
    log_time: `2019-01-22T11:0${number}:00Z`,
    interval: number,
    interesting,
    factoids,
  }
}

describe('searchSessionRecords', () => {
  it('shares trimmed, case-insensitive substring matching with visible search emphasis', () => {
    expect(searchTextMatches('Deployment', ' deploy ')).toBe(true)
    expect(searchTextMatches('undeployed', 'DEPLOY')).toBe(true)
    expect(searchTextMatches('catalog', 'deploy')).toBe(false)
    expect(searchTextMatches('deployment', '   ')).toBe(false)
    expect(searchTextSegments('GET /page=4 then PAGE=4', ' page=4 ')).toEqual([
      { text: 'GET /', matched: false },
      { text: 'page=4', matched: true },
      { text: ' then ', matched: false },
      { text: 'PAGE=4', matched: true },
    ])
  })

  it('matches emitted words, refs, IPs, and IP prefixes without duplicating records', () => {
    const values = records([
      interval(0, [
        { name: 'words', top: [{ key: 'ProductModel' }], peaks: [{ key: 'ProductModel' }] },
        { name: 'refs', top: [{ key: 'www.example.test/catalog' }] },
        { name: 'ips', top: [{ key: '203.0.113.17' }], ip_groups: [{ prefix: '203.0.' }] },
      ]),
    ])

    expect(searchSessionRecords(values, 'product').map((result) => result.record.interval)).toEqual([0])
    expect(searchSessionRecords(values, 'EXAMPLE.TEST')).toHaveLength(1)
    expect(searchSessionRecords(values, '113.17')).toHaveLength(1)
    expect(searchSessionRecords(values, '203.0.')).toHaveLength(1)
  })

  it('matches interval factoid names and text', () => {
    const values = records([
      interval(0, [], [{ name: 'traffic.peakErrs', text: 'Peak errs:193/min' }]),
    ])

    expect(searchSessionRecords(values, 'peakErrs')).toHaveLength(1)
    expect(searchSessionRecords(values, '193/MIN')).toHaveLength(1)
  })

  it('associates a standalone fact with the next interval in file order', () => {
    const values = records([
      interval(0),
      {
        schema_version: 4,
        event_type: 'control_command',
        session_id: 'search',
        timestamp: '2019-01-22T11:00:30Z',
        log_time: '2019-01-22T11:00:30Z',
        command: '!!! fact print deployment marker',
        result: { fact: 'print', text: 'Note: deployment rolled back' },
      },
      interval(1),
    ])

    const results = searchSessionRecords(values, 'rolled BACK')
    expect(results).toHaveLength(1)
    expect(results[0].record.eventType).toBe('control_command')
    expect(results[0].intervalRecord?.interval).toBe(1)
    expect(groupSearchResultsByInterval(results).get(results[0].intervalRecord?.id ?? '')).toHaveLength(1)
  })

  it('does not search complete command text', () => {
    const values = records([
      {
        schema_version: 4,
        event_type: 'control_command',
        session_id: 'search',
        timestamp: '2019-01-22T11:00:30Z',
        command: '!!! add secret-command-term value',
        result: { action: 'add_matcher' },
      },
      interval(1),
    ])

    expect(searchSessionRecords(values, 'secret-command-term')).toHaveLength(0)
  })

  it('keeps retained source lines outside the interval search index', () => {
    const values = records([
      {
        ...interval(0, [{ name: 'words', top: [{ key: 'catalog', source_line_ref: 1 }] }]),
        source_examples_enabled: true,
        source_lines: ['GET /catalog?page=4 HTTP/1.1'],
      },
    ])

    expect(searchSessionRecords(values, 'page=4')).toHaveLength(0)
  })

  it('leaves a trailing fact unanchored until a later interval exists', () => {
    const fact = {
      schema_version: 4,
      event_type: 'control_command',
      session_id: 'search',
      timestamp: '2019-01-22T11:00:30Z',
      result: { fact: 'print', text: 'Note: pending marker' },
    }
    const before = searchSessionRecords(records([interval(0), fact]), 'pending marker')
    const after = searchSessionRecords(records([interval(0), fact, interval(1)]), 'pending marker')

    expect(before[0].intervalRecord).toBeNull()
    expect(after[0].intervalRecord?.interval).toBe(1)
  })

  it('does not cross session boundaries or inspect unsupported schemas', () => {
    const values = records([
      interval(0),
      {
        schema_version: 4,
        event_type: 'control_command',
        session_id: 'search',
        result: { fact: 'print', text: 'Note: boundary marker' },
      },
      { ...interval(1), session_id: 'other' },
      { ...interval(2, [{ name: 'words', top: [{ key: 'future-term' }] }]), schema_version: 7 },
    ])

    expect(searchSessionRecords(values, 'boundary marker')[0].intervalRecord).toBeNull()
    expect(searchSessionRecords(values, 'future-term')).toHaveLength(0)
  })
})
