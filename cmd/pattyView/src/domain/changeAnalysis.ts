import {
  isJsonObject,
  readArray,
  readNumber,
  readString,
  type JsonObject,
  type PattyLogRecord,
} from './types'
import { buildPeakResetPhases, type PeakResetPhase } from './peakReset'

export type ChangeComponentKey =
  | 'lines'
  | 'bytes'
  | 'averageBytes'
  | 'errors'
  | 'marked'
  | 'b16'
  | 'peakBalance'
  | 'wordWave'

export interface ChangeComponent {
  key: ChangeComponentKey
  label: string
  score: number
}

export interface ChangePoint {
  recordId: string
  referenceRecordId: string | null
  score: number | null
  primary: ChangeComponentKey | null
  components: ChangeComponent[]
  resetPhase: PeakResetPhase | null
}

const componentLabels: Record<ChangeComponentKey, string> = {
  lines: 'Lines',
  bytes: 'Bytes',
  averageBytes: 'Avg bytes/line',
  errors: 'Errs',
  marked: 'Marked',
  b16: 'B16',
  peakBalance: 'Peak balance',
  wordWave: 'Word wave',
}

// Candidate constants intentionally live together. Change analysis is an
// experimental navigation measure, and these values are expected to evolve as
// representative PattyLogs expose which components carry useful texture.
const candidate = {
  lines: { floor: 100, halfScale: 0.25 },
  bytes: { floor: 1024 * 1024, halfScale: 0.25 },
  averageBytes: { floor: 128, halfScale: 0.25 },
  errors: { floor: 10, halfScale: 0.5 },
  marked: { halfScale: 4 },
  b16: { halfScale: 12 },
  wordPeaks: { halfScale: 0.10, weight: 1 },
  refPeaks: { halfScale: 0.12, weight: 0.8 },
  ipPeaks: { halfScale: 0.25, weight: 0.4 },
  wordWave: { halfScale: 0.30, weight: 0.75 },
} as const

export function buildChangeSeries(
  intervals: PattyLogRecord[],
  contextRecords: PattyLogRecord[] = intervals,
): ChangePoint[] {
  const resetPhases = buildPeakResetPhases(contextRecords)
  return intervals.map((record, index) => {
    const reference = intervals[index - 1]
    if (
      !reference ||
      reference.sessionId !== record.sessionId ||
      reference.schemaVersion !== 4 ||
      record.schemaVersion !== 4
    ) {
      return emptyChangePoint(record, null, resetPhases.get(record.id) ?? null)
    }
    return compareIntervals(reference, record, resetPhases.get(record.id) ?? null)
  })
}

function compareIntervals(
  reference: PattyLogRecord,
  selected: PattyLogRecord,
  resetPhase: PeakResetPhase | null,
): ChangePoint {
  const referenceLines = readNumber(reference.data, 'interval_lines')
  const selectedLines = readNumber(selected.data, 'interval_lines')
  const referenceBytes = matcherCount(reference, 'bytes')
  const selectedBytes = matcherCount(selected, 'bytes')
  const referenceErrors = matcherCount(reference, 'errs')
  const selectedErrors = matcherCount(selected, 'errs')

  const components = [
    component('lines', relativeChange(referenceLines, selectedLines, candidate.lines)),
    component('bytes', relativeChange(referenceBytes, selectedBytes, candidate.bytes)),
    component('averageBytes', relativeChange(
      averageBytesPerLine(referenceBytes, referenceLines),
      averageBytesPerLine(selectedBytes, selectedLines),
      candidate.averageBytes,
    )),
    component('errors', relativeChange(referenceErrors, selectedErrors, candidate.errors)),
    component('marked', pointChange(
      matcherKeyPercentage(reference, 'lines', 'marked'),
      matcherKeyPercentage(selected, 'lines', 'marked'),
      candidate.marked.halfScale,
    )),
    component('b16', pointChange(
      matcherKeyPercentage(reference, 'lines', ' b16'),
      matcherKeyPercentage(selected, 'lines', ' b16'),
      candidate.b16.halfScale,
    )),
    component('peakBalance', peakBalanceChange(reference, selected)),
    component('wordWave', wordWaveChange(reference, selected)),
  ].filter((value): value is ChangeComponent => value !== null)
    .sort((left, right) => right.score - left.score)

  if (components.length === 0) {
    return emptyChangePoint(selected, reference.id, resetPhase)
  }

  const primary = components[0].score
  const secondary = (components[1]?.score ?? 0) / 100
  const tertiary = (components[2]?.score ?? 0) / 100
  const composite = Math.min(
    100,
    primary + (100 - primary) * (0.15 * secondary + 0.05 * tertiary),
  )
  // Preserve attribution and interval ordering while giving the threshold
  // useful resolution below the middle of the range.
  const score = composite * composite / 100

  return {
    recordId: selected.id,
    referenceRecordId: reference.id,
    score,
    primary: components[0].key,
    components,
    resetPhase,
  }
}

function emptyChangePoint(
  record: PattyLogRecord,
  referenceRecordId: string | null = null,
  resetPhase: PeakResetPhase | null = null,
): ChangePoint {
  return {
    recordId: record.id,
    referenceRecordId,
    score: null,
    primary: null,
    components: [],
    resetPhase,
  }
}

function component(key: ChangeComponentKey, score: number | null): ChangeComponent | null {
  return score === null ? null : { key, label: componentLabels[key], score }
}

function relativeChange(
  reference: number | null,
  selected: number | null,
  settings: { floor: number; halfScale: number },
): number | null {
  if (reference === null || selected === null) {
    return null
  }
  const denominator = Math.max(Math.abs(reference), Math.abs(selected), settings.floor)
  return softChangeScore(Math.abs(selected - reference) / denominator, settings.halfScale)
}

function pointChange(
  reference: number | null,
  selected: number | null,
  halfScale: number,
): number | null {
  return reference === null || selected === null
    ? null
    : softChangeScore(Math.abs(selected - reference), halfScale)
}

function softChangeScore(value: number, halfScale: number): number {
  if (!Number.isFinite(value) || value <= 0) {
    return 0
  }
  const squared = value * value
  return 100 * squared / (squared + halfScale * halfScale)
}

function averageBytesPerLine(bytes: number | null, lines: number | null): number | null {
  return bytes === null || lines === null || lines <= 0 ? null : bytes / lines
}

function matcherCount(record: PattyLogRecord, name: string): number | null {
  const matcher = emittedMatcher(record, name)
  return matcher ? readNumber(matcher, 'interval_count') : null
}

function matcherKeyPercentage(record: PattyLogRecord, name: string, key: string): number | null {
  const matcher = emittedMatcher(record, name)
  const total = matcher ? readNumber(matcher, 'interval_count') : null
  if (!matcher || total === null || total <= 0) {
    return null
  }
  for (const entry of readArray(matcher, 'top_keys')) {
    if (!isJsonObject(entry) || readString(entry, 'key') !== key) {
      continue
    }
    const count = readNumber(entry, 'count')
    return count === null ? null : count * 100 / total
  }
  return null
}

function emittedMatcher(record: PattyLogRecord, name: string): JsonObject | null {
  for (const matcher of readArray(record.data, 'matchers')) {
    if (isJsonObject(matcher) && readString(matcher, 'name') === name) {
      return matcher
    }
  }
  return null
}

function peakBalanceChange(reference: PattyLogRecord, selected: PattyLogRecord): number | null {
  const candidates = [
    weightedDistributionChange(reference, selected, 'words', 'peaks', candidate.wordPeaks),
    weightedDistributionChange(reference, selected, 'refs', 'peaks', candidate.refPeaks),
    weightedDistributionChange(reference, selected, 'ips', 'peaks', candidate.ipPeaks),
  ].filter((value): value is number => value !== null)
  return candidates.length > 0 ? Math.max(...candidates) : null
}

function wordWaveChange(reference: PattyLogRecord, selected: PattyLogRecord): number | null {
  const referenceEntries = nonPeakWordEntries(reference)
  const selectedEntries = nonPeakWordEntries(selected)
  if (referenceEntries === null || selectedEntries === null) {
    return null
  }
  const distance = distributionDistance(referenceEntries, selectedEntries)
  return distance === null
    ? null
    : candidate.wordWave.weight * softChangeScore(distance, candidate.wordWave.halfScale)
}

function weightedDistributionChange(
  reference: PattyLogRecord,
  selected: PattyLogRecord,
  streamName: string,
  collection: string,
  settings: { halfScale: number; weight: number },
): number | null {
  const referenceEntries = streamCollection(reference, streamName, collection)
  const selectedEntries = streamCollection(selected, streamName, collection)
  if (referenceEntries === null || selectedEntries === null) {
    return null
  }
  const distance = distributionDistance(referenceEntries, selectedEntries)
  return distance === null ? null : settings.weight * softChangeScore(distance, settings.halfScale)
}

function nonPeakWordEntries(record: PattyLogRecord): JsonObject[] | null {
  const top = streamCollection(record, 'words', 'top')
  const peaks = streamCollection(record, 'words', 'peaks')
  if (top === null || peaks === null) {
    return null
  }
  const peakKeys = new Set(peaks.map((entry) => readString(entry, 'key')).filter(Boolean))
  return top.filter((entry) => entry.is_peak !== true && !peakKeys.has(readString(entry, 'key')))
}

function streamCollection(
  record: PattyLogRecord,
  streamName: string,
  collection: string,
): JsonObject[] | null {
  for (const stream of readArray(record.data, 'interesting')) {
    if (!isJsonObject(stream) || readString(stream, 'name') !== streamName) {
      continue
    }
    return readArray(stream, collection).filter(isJsonObject)
  }
  return null
}

function distributionDistance(reference: JsonObject[], selected: JsonObject[]): number | null {
  const referenceDistribution = countDistribution(reference)
  const selectedDistribution = countDistribution(selected)
  if (!referenceDistribution.available || !selectedDistribution.available) {
    return null
  }
  if (referenceDistribution.total === 0 && selectedDistribution.total === 0) {
    return 0
  }
  if (referenceDistribution.total === 0 || selectedDistribution.total === 0) {
    return 1
  }

  const keys = new Set([
    ...referenceDistribution.counts.keys(),
    ...selectedDistribution.counts.keys(),
  ])
  let difference = 0
  for (const key of keys) {
    const referenceShare = (referenceDistribution.counts.get(key) ?? 0) / referenceDistribution.total
    const selectedShare = (selectedDistribution.counts.get(key) ?? 0) / selectedDistribution.total
    difference += Math.abs(referenceShare - selectedShare)
  }
  return difference / 2
}

function countDistribution(entries: JsonObject[]) {
  const counts = new Map<string, number>()
  let validCount = entries.length === 0
  for (const entry of entries) {
    const key = readString(entry, 'key')
    const count = readNumber(entry, 'count')
    if (key === '' || count === null || count < 0) {
      continue
    }
    validCount = true
    counts.set(key, (counts.get(key) ?? 0) + count)
  }
  return {
    counts,
    total: [...counts.values()].reduce((sum, value) => sum + value, 0),
    available: validCount,
  }
}
