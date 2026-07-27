import { describe, expect, it } from 'vitest'
import { parseJsonlText } from './jsonl'
import {
  buildIntervalSeries,
  emittedInterestingScore,
  emittedMatcherColor,
  emittedMatcherCount,
  laneHeight,
} from './intervalSeries'

describe('interval series projection', () => {
  it('reads emitted line, byte, and errs matcher counts without deriving traffic state', () => {
    const records = parseJsonlText([
      JSON.stringify({
        schema_version: 4,
        event_type: 'interval',
        session_id: 'a',
        interval: 0,
        interval_lines: 1200,
        matchers: [
          { name: 'Bots', color_hex: '#a0ffff', interval_count: 50 },
          {
            name: 'lines',
            interval_count: 1200,
            top_keys: [
              { key: 'marked', count: 300 },
              { key: ' b16', count: 660 },
            ],
          },
          { name: 'bytes', interval_count: 3_400_000 },
          { name: 'errs', interval_count: 14 },
        ],
      }),
      JSON.stringify({
        schema_version: 4,
        event_type: 'interval',
        session_id: 'a',
        interval: 1,
        interval_lines: 900,
        matchers: [],
      }),
    ].join('\n')).records

    const series = buildIntervalSeries(records)
    expect(series.map(({ lines, bytes, errors, markedPercent, b16Percent }) => ({
      lines,
      bytes,
      errors,
      markedPercent,
      b16Percent,
    }))).toEqual([
      { lines: 1200, bytes: 3_400_000, errors: 14, markedPercent: 25, b16Percent: 55 },
      { lines: 900, bytes: null, errors: null, markedPercent: null, b16Percent: null },
    ])
    expect(emittedMatcherCount(records[0], 'Bots')).toBe(50)
    expect(emittedMatcherColor(records[0], 'Bots')).toBe('#a0ffff')
    expect(emittedMatcherCount(records[1], 'Bots')).toBeNull()
    expect(emittedMatcherColor(records[1], 'Bots')).toBe('')
  })

  it('scales lane values into a fixed compressed height', () => {
    expect(laneHeight(null, 0, 100, 20)).toBe(0)
    expect(laneHeight(0, 0, 100, 20)).toBe(0)
    expect(laneHeight(50, 0, 100, 20)).toBeCloseTo(10.5)
    expect(laneHeight(100, 0, 100, 20)).toBe(20)
  })

  it('reads emitted interesting scores from top or peaks without deriving a value', () => {
    const [record] = parseJsonlText(JSON.stringify({
      schema_version: 4,
      event_type: 'interval',
      session_id: 'scores',
      interval: 0,
      interesting: [
        {
          name: 'words',
          top: [
            { key: 'catalog', score: 80, count: 200, bytes: 4096 },
            { key: 'missing-score', count: 900, prime_flux: 120 },
          ],
          peaks: [{ key: 'historic', score: 44 }],
        },
      ],
    })).records

    expect(emittedInterestingScore(record, 'words', 'catalog')).toBe(80)
    expect(emittedInterestingScore(record, 'words', 'historic')).toBe(44)
    expect(emittedInterestingScore(record, 'words', 'missing-score')).toBeNull()
    expect(emittedInterestingScore(record, 'words', 'absent')).toBeNull()
    expect(emittedInterestingScore(record, 'refs', 'catalog')).toBeNull()
  })
})
