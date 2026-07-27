import { describe, expect, it } from 'vitest'
import { parseJsonlText } from './jsonl'
import { buildIntervalContexts, projectIntervalComparison } from './intervalComparison'

function records(values: unknown[]) {
  return parseJsonlText(values.map((value) => JSON.stringify(value)).join('\n')).records
}

function interval(number: number, values: Record<string, unknown> = {}) {
  return {
    schema_version: 4,
    event_type: 'interval',
    session_id: 'comparison',
    timestamp: `2019-01-22T11:0${number}:00Z`,
    log_time: `2019-01-22T11:0${number}:00Z`,
    interval: number,
    interval_lines: 100 + number,
    matchers: [],
    interesting: [],
    ...values,
  }
}

describe('interval comparison', () => {
  it('compares interval signals and preserves selected matcher order', () => {
    const values = records([
      interval(0, {
        interval_lines: 100,
        matchers: [
          { name: 'Bots', color_hex: '#aaaaaa', interval_count: 20 },
          {
            name: 'lines',
            interval_count: 100,
            top_keys: [{ key: 'marked', count: 25 }, { key: ' b16', count: 55 }],
          },
          { name: 'bytes', interval_count: 1024 },
          { name: 'legacy', interval_count: 8 },
          { name: 'errs', interval_count: 4 },
        ],
      }),
      interval(1, {
        interval_lines: 150,
        matchers: [
          { name: 'Googlebot', color_hex: '#44aa44', interval_count: 30 },
          { name: 'Bots', color_hex: '#bbbbbb', interval_count: 15 },
          {
            name: 'lines',
            interval_count: 150,
            top_keys: [{ key: 'marked', count: 75 }, { key: ' b16', count: 60 }],
          },
          { name: 'bytes', interval_count: 3072 },
          { name: 'errs', interval_count: 10 },
        ],
      }),
    ])

    const comparison = projectIntervalComparison(values[0], values[1], values)
    expect(comparison.signals).toEqual([
      { key: 'lines', label: 'Lines', kind: 'count', reference: 100, selected: 150, delta: 50 },
      { key: 'bytes', label: 'Bytes', kind: 'bytes', reference: 1024, selected: 3072, delta: 2048 },
      { key: 'errors', label: 'Errors', kind: 'count', reference: 4, selected: 10, delta: 6 },
      { key: 'marked', label: 'Marked', kind: 'percent', reference: 25, selected: 50, delta: 25 },
      { key: 'b16', label: 'B16', kind: 'percent', reference: 55, selected: 40, delta: -15 },
    ])
    expect(comparison.matchers.map((matcher) => matcher.name)).toEqual([
      'Googlebot', 'Bots', 'lines', 'bytes', 'errs', 'legacy',
    ])
    expect(comparison.matchers[1]).toMatchObject({
      colorHex: '#bbbbbb',
      referencePosition: 1,
      selectedPosition: 2,
      reference: 20,
      selected: 15,
      delta: -5,
    })
    expect(comparison.matchers[5]).toMatchObject({
      referencePosition: 4,
      selectedPosition: null,
      reference: 8,
      selected: null,
      delta: null,
    })
  })

  it('keeps missing signal values unavailable', () => {
    const values = records([interval(0), interval(1)])
    const comparison = projectIntervalComparison(values[0], values[1], values)

    expect(comparison.signals.find((signal) => signal.key === 'bytes')).toMatchObject({
      reference: null,
      selected: null,
      delta: null,
    })
  })

  it('associates fact commands and alerts with their recorded intervals', () => {
    const values = records([
      interval(0, { factoids: [{ name: 'traffic.lines', text: 'Lines:100' }] }),
      {
        schema_version: 4,
        event_type: 'control_command',
        session_id: 'comparison',
        log_time: '2019-01-22T11:00:30Z',
        result: { fact: 'print', text: 'Note: deployed release' },
      },
      interval(1),
      {
        schema_version: 4,
        event_type: 'alert',
        session_id: 'comparison',
        interval: 1,
        log_time: '2019-01-22T11:01:10Z',
        matcher: 'errs',
        status: 'triggered',
        value: 20,
      },
    ])

    const contexts = buildIntervalContexts(values)
    expect(contexts.get(values[0].id)?.factoids).toEqual([
      { name: 'traffic.lines', text: 'Lines:100' },
    ])
    expect(contexts.get(values[2].id)?.factoids).toEqual([
      { name: 'print', text: 'Note: deployed release' },
    ])
    expect(contexts.get(values[2].id)?.alerts[0]).toMatchObject({
      matcher: 'errs',
      status: 'triggered',
      interval: 1,
    })
  })

  it('does not carry standalone facts across session boundaries', () => {
    const values = records([
      interval(0),
      {
        schema_version: 4,
        event_type: 'control_command',
        session_id: 'comparison',
        result: { fact: 'print', text: 'Note: old session' },
      },
      {
        schema_version: 4,
        event_type: 'session_start',
        session_id: 'next',
        log_time: '1970-01-01T00:00:00Z',
      },
      { ...interval(1), session_id: 'next' },
    ])

    const contexts = buildIntervalContexts(values)
    expect(contexts.get(values[3].id)?.factoids).toEqual([])
  })

  it('annotates comparisons that cross or touch a Peak reset window', () => {
    const values = records([
      interval(0),
      {
        schema_version: 4,
        event_type: 'control_command',
        session_id: 'comparison',
        command_name: 'purge',
        status: 'applied',
      },
      interval(1),
      interval(2),
      interval(3),
    ])
    const intervals = values.filter((record) => record.eventType === 'interval')

    expect(projectIntervalComparison(intervals[0], intervals[1], values).peakResetPhases).toEqual(['reset'])
    expect(projectIntervalComparison(intervals[0], intervals[2], values).peakResetPhases).toEqual(['reset', 'rebaseline'])
    expect(projectIntervalComparison(intervals[2], intervals[3], values).peakResetPhases).toEqual(['rebaseline'])
    expect(projectIntervalComparison(intervals[0], intervals[3], [intervals[0], intervals[3]]).peakResetPhases).toEqual([])
  })
})
