import { describe, expect, it } from 'vitest'
import { buildChangeSeries, type ChangeComponentKey } from './changeAnalysis'
import { parseJsonlText } from './jsonl'

describe('change analysis', () => {
  it('compares each schema-4 interval with its immediately older session interval', () => {
    const records = parseRecords([
      interval('a', 0, { lines: 1000 }),
      interval('a', 1, { lines: 1250 }),
      interval('b', 0, { lines: 900 }),
    ])

    const series = buildChangeSeries(records)
    expect(series).toHaveLength(3)
    expect(series[0]).toMatchObject({ referenceRecordId: null, score: null })
    expect(series[1].referenceRecordId).toBe(records[0].id)
    expect(series[1].score).not.toBeNull()
    expect(series[2]).toMatchObject({ referenceRecordId: null, score: null })
  })

  it('measures changes in Peak balance independently from proportional volume', () => {
    const records = parseRecords([
      interval('peaks', 0, {
        lines: 1000,
        wordsPeaks: [['catalog', 80], ['search', 20]],
      }),
      interval('peaks', 1, {
        lines: 2000,
        wordsPeaks: [['catalog', 160], ['search', 40]],
      }),
      interval('peaks', 2, {
        lines: 2000,
        wordsPeaks: [['catalog', 40], ['search', 160]],
      }),
    ])

    const series = buildChangeSeries(records)
    expect(componentScore(series[1], 'peakBalance')).toBe(0)
    expect(componentScore(series[2], 'peakBalance')).toBeGreaterThan(90)
    expect(series[2].primary).toBe('peakBalance')
  })

  it('weights Ref and IP Peak movement below Word Peak movement', () => {
    const wordSeries = buildChangeSeries(parseRecords([
      interval('word', 0, { wordsPeaks: [['a', 100]] }),
      interval('word', 1, { wordsPeaks: [['b', 100]] }),
    ]))
    const refSeries = buildChangeSeries(parseRecords([
      interval('ref', 0, { refsPeaks: [['a', 100]] }),
      interval('ref', 1, { refsPeaks: [['b', 100]] }),
    ]))
    const ipSeries = buildChangeSeries(parseRecords([
      interval('ip', 0, { ipsPeaks: [['a', 100]] }),
      interval('ip', 1, { ipsPeaks: [['b', 100]] }),
    ]))

    const wordScore = componentScore(wordSeries[1], 'peakBalance')
    const refScore = componentScore(refSeries[1], 'peakBalance')
    const ipScore = componentScore(ipSeries[1], 'peakBalance')
    expect(wordScore).toBeGreaterThan(refScore)
    expect(refScore).toBeGreaterThan(ipScore)
  })

  it('tracks non-Peak Word waves while excluding Peak entries from that component', () => {
    const records = parseRecords([
      interval('waves', 0, {
        wordsTop: [['stable', 100, true], ['checkout', 90, false], ['search', 10, false]],
        wordsPeaks: [['stable', 100]],
      }),
      interval('waves', 1, {
        wordsTop: [['stable', 10, true], ['checkout', 10, false], ['search', 90, false]],
        wordsPeaks: [['stable', 10]],
      }),
    ])

    const point = buildChangeSeries(records)[1]
    expect(componentScore(point, 'peakBalance')).toBe(0)
    expect(componentScore(point, 'wordWave')).toBeGreaterThan(60)
  })

  it('leaves missing components unavailable and keeps every composite finite and bounded', () => {
    const records = parseRecords([
      interval('missing', 0, { lines: 0, includeBytes: false, includeInteresting: false }),
      interval('missing', 1, { lines: 100, includeBytes: false, includeInteresting: false }),
    ])
    const point = buildChangeSeries(records)[1]

    expect(point.components.some((component) => component.key === 'averageBytes')).toBe(false)
    expect(point.components.some((component) => component.key === 'peakBalance')).toBe(false)
    expect(point.score).not.toBeNull()
    expect(point.score).toBeGreaterThanOrEqual(0)
    expect(point.score).toBeLessThanOrEqual(100)
  })

  it('produces distinct low, medium, and high candidate scores', () => {
    const records = parseRecords([
      interval('texture', 0, { lines: 1000 }),
      interval('texture', 1, { lines: 1020 }),
      interval('texture', 2, { lines: 1275 }),
      interval('texture', 3, { lines: 2500 }),
    ])
    const scores = buildChangeSeries(records).slice(1).map((point) => point.score ?? 0)

    expect(scores[0]).toBeLessThan(scores[1])
    expect(scores[1]).toBeLessThan(scores[2])
    expect(scores[0]).toBeLessThan(10)
    expect(scores[2]).toBeGreaterThan(50)
  })

  it('marks two raw comparisons as Peak reset context without changing their calculations', () => {
    const contextRecords = parseRecords([
      interval('reset', 0, { lines: 1000 }),
      {
        schema_version: 4,
        event_type: 'control_command',
        session_id: 'reset',
        command_name: 'purge',
        status: 'applied',
      },
      interval('reset', 1, { lines: 2000 }),
      interval('reset', 2, { lines: 1000 }),
      interval('reset', 3, { lines: 2000 }),
    ])
    const intervals = contextRecords.filter((record) => record.eventType === 'interval')
    const withContext = buildChangeSeries(intervals, contextRecords)
    const raw = buildChangeSeries(intervals)

    expect(withContext.map((point) => point.resetPhase)).toEqual([
      null, 'reset', 'rebaseline', null,
    ])
    expect(withContext.map((point) => point.score)).toEqual(raw.map((point) => point.score))
  })
})

function componentScore(
  point: ReturnType<typeof buildChangeSeries>[number],
  key: ChangeComponentKey,
): number {
  return point.components.find((component) => component.key === key)?.score ?? -1
}

interface IntervalOptions {
  lines?: number
  includeBytes?: boolean
  includeInteresting?: boolean
  wordsTop?: Array<[string, number, boolean]>
  wordsPeaks?: Array<[string, number]>
  refsPeaks?: Array<[string, number]>
  ipsPeaks?: Array<[string, number]>
}

function interval(session: string, number: number, options: IntervalOptions = {}) {
  const lines = options.lines ?? 1000
  const matchers: unknown[] = [
    {
      name: 'lines',
      interval_count: lines,
      top_keys: [{ key: 'marked', count: 100 }, { key: ' b16', count: 300 }],
    },
    { name: 'errs', interval_count: 10 },
  ]
  if (options.includeBytes !== false) {
    matchers.push({ name: 'bytes', interval_count: lines * 1024 })
  }

  const stream = (
    name: string,
    top: Array<[string, number, boolean]> = [],
    peaks: Array<[string, number]> = [],
  ) => ({
    name,
    top: top.map(([key, count, isPeak]) => ({ key, count, is_peak: isPeak })),
    peaks: peaks.map(([key, count]) => ({ key, count, is_peak: true })),
  })
  const interesting = options.includeInteresting === false ? undefined : [
    stream('words', options.wordsTop, options.wordsPeaks),
    stream('refs', [], options.refsPeaks),
    stream('ips', [], options.ipsPeaks),
  ]

  return {
    schema_version: 4,
    event_type: 'interval',
    session_id: session,
    interval: number,
    interval_lines: lines,
    matchers,
    ...(interesting ? { interesting } : {}),
  }
}

function parseRecords(values: unknown[]) {
  return parseJsonlText(values.map((value) => JSON.stringify(value)).join('\n')).records
}
