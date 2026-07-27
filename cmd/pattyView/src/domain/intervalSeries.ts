import {
  isJsonObject,
  readArray,
  readNumber,
  readString,
  type PattyLogRecord,
} from './types'

export const alwaysVisibleIntervalMatcherNames = ['lines'] as const

export interface IntervalPoint {
  record: PattyLogRecord
  lines: number | null
  bytes: number | null
  errors: number | null
  markedPercent: number | null
  b16Percent: number | null
}

export function buildIntervalSeries(records: PattyLogRecord[]): IntervalPoint[] {
  return records
    .filter((record) => record.eventType === 'interval')
    .map((record) => ({
      record,
      lines: readNumber(record.data, 'interval_lines'),
      bytes: emittedMatcherCount(record, 'bytes'),
      errors: emittedMatcherCount(record, 'errs'),
      markedPercent: emittedMatcherKeyPercentage(record, 'lines', 'marked'),
      b16Percent: emittedMatcherKeyPercentage(record, 'lines', ' b16'),
    }))
}

export function emittedMatcherCount(record: PattyLogRecord, matcherName: string): number | null {
  const matcher = emittedMatcher(record, matcherName)
  return matcher ? readNumber(matcher, 'interval_count') : null
}

export function emittedMatcherColor(record: PattyLogRecord, matcherName: string): string {
  const matcher = emittedMatcher(record, matcherName)
  return matcher ? readString(matcher, 'color_hex') : ''
}

export function emittedInterestingScore(
  record: PattyLogRecord,
  streamName: string,
  key: string,
): number | null {
  for (const stream of readArray(record.data, 'interesting')) {
    if (!isJsonObject(stream) || readString(stream, 'name') !== streamName) {
      continue
    }
    for (const collection of ['top', 'peaks']) {
      for (const entry of readArray(stream, collection)) {
        if (isJsonObject(entry) && readString(entry, 'key') === key) {
          return readNumber(entry, 'score')
        }
      }
    }
    return null
  }
  return null
}

export function emittedMatcherKeyPercentage(
  record: PattyLogRecord,
  matcherName: string,
  key: string,
): number | null {
  const matcher = emittedMatcher(record, matcherName)
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

function emittedMatcher(record: PattyLogRecord, matcherName: string) {
  for (const matcher of readArray(record.data, 'matchers')) {
    if (isJsonObject(matcher) && readString(matcher, 'name') === matcherName) {
      return matcher
    }
  }
  return null
}

export function laneHeight(
  value: number | null,
  minimum: number,
  maximum: number,
  height: number,
): number {
  if (value === null || value <= 0) {
    return 0
  }
  if (maximum <= minimum) {
    return height
  }
  const normalized = Math.max(0, Math.min(1, (value - minimum) / (maximum - minimum)))
  return Math.min(height, 1 + normalized * (height - 1))
}
