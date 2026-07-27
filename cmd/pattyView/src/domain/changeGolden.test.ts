import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'
import { buildChangeSeries, type ChangeComponentKey } from './changeAnalysis'
import { parseJsonlText } from './jsonl'

interface GoldenEntry {
  key: string
  count: number
}

interface GoldenInput {
  lines: number
  bytes: number
  errors: number
  marked: number
  b16: number
  word_peaks: GoldenEntry[]
  ref_peaks: GoldenEntry[]
  ip_peaks: GoldenEntry[]
  word_wave: GoldenEntry[]
}

interface GoldenComponent {
  key: string
  score: number
}

interface GoldenCase {
  name: string
  reference: GoldenInput
  selected: GoldenInput
  expected: {
    score: number
    primary: string
    components: GoldenComponent[]
  }
}

const corpus = JSON.parse(readFileSync(
  resolve(process.cwd(), '../../testdata/change_golden.json'),
  'utf8',
)) as { cases: GoldenCase[] }

const canonicalComponent: Record<ChangeComponentKey, string> = {
  lines: 'lines',
  bytes: 'bytes',
  averageBytes: 'avg bytes/line',
  errors: 'errs',
  marked: 'marked',
  b16: 'b16',
  peakBalance: 'peak balance',
  wordWave: 'word wave',
}

describe('shared Change golden corpus', () => {
  for (const golden of corpus.cases) {
    it(golden.name, () => {
      const records = parseJsonlText([
        goldenInterval(0, golden.reference),
        goldenInterval(1, golden.selected),
      ].map((record) => JSON.stringify(record)).join('\n')).records
      const point = buildChangeSeries(records)[1]

      expect(Math.abs((point.score ?? -1) - golden.expected.score)).toBeLessThanOrEqual(1e-9)
      expect(point.primary === null ? null : canonicalComponent[point.primary]).toBe(golden.expected.primary)
      expect(point.components.map((component) => canonicalComponent[component.key])).toEqual(
        golden.expected.components.map((component) => component.key),
      )
      point.components.forEach((component, index) => {
        expect(Math.abs(component.score - golden.expected.components[index].score)).toBeLessThanOrEqual(1e-9)
      })
    })
  }
})

function goldenInterval(number: number, input: GoldenInput) {
  const collection = (entries: GoldenEntry[], isPeak: boolean) => entries.map((entry) => ({
    key: entry.key,
    count: entry.count,
    is_peak: isPeak,
  }))
  const stream = (name: string, top: GoldenEntry[], peaks: GoldenEntry[]) => ({
    name,
    top: collection(top, false),
    peaks: collection(peaks, true),
  })

  return {
    schema_version: 4,
    event_type: 'interval',
    session_id: 'change-golden',
    interval: number,
    interval_lines: input.lines,
    matchers: [
      {
        name: 'lines',
        interval_count: input.lines,
        top_keys: [
          { key: 'marked', count: input.marked },
          { key: ' b16', count: input.b16 },
        ],
      },
      { name: 'bytes', interval_count: input.bytes },
      { name: 'errs', interval_count: input.errors },
    ],
    interesting: [
      stream('words', input.word_wave, input.word_peaks),
      stream('refs', [], input.ref_peaks),
      stream('ips', [], input.ip_peaks),
    ],
  }
}
