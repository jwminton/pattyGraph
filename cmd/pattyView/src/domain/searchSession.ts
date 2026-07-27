import {
  isJsonObject,
  readArray,
  readObject,
  readString,
  type JsonObject,
  type PattyLogRecord,
} from './types'

const searchableInterestingStreams = new Set(['words', 'refs', 'ips'])

export interface SessionSearchResult {
  record: PattyLogRecord
  intervalRecord: PattyLogRecord | null
}

export type SearchResultsByInterval = ReadonlyMap<string, SessionSearchResult[]>

export interface SearchTextSegment {
  text: string
  matched: boolean
}

export function searchTextMatches(value: string, rawQuery: string): boolean {
  const query = normalizeSearchQuery(rawQuery)
  return query !== '' && textMatches(value, query)
}

export function searchTextSegments(value: string, rawQuery: string): SearchTextSegment[] {
  const query = normalizeSearchQuery(rawQuery)
  if (query === '') {
    return [{ text: value, matched: false }]
  }

  const comparableValue = value.toLowerCase()
  const segments: SearchTextSegment[] = []
  let offset = 0
  while (offset < value.length) {
    const matchIndex = comparableValue.indexOf(query, offset)
    if (matchIndex < 0) {
      segments.push({ text: value.slice(offset), matched: false })
      break
    }
    if (matchIndex > offset) {
      segments.push({ text: value.slice(offset, matchIndex), matched: false })
    }
    segments.push({ text: value.slice(matchIndex, matchIndex + query.length), matched: true })
    offset = matchIndex + query.length
  }
  return segments.length > 0 ? segments : [{ text: value, matched: false }]
}

// Search walks only PattyGraph's bounded, emitted text fields. The reverse pass
// also provides the next interval needed to place standalone factoids.
export function searchSessionRecords(records: PattyLogRecord[], rawQuery: string): SessionSearchResult[] {
  const query = normalizeSearchQuery(rawQuery)
  if (query === '') {
    return []
  }

  const results: SessionSearchResult[] = []
  let nextInterval: PattyLogRecord | null = null
  let sessionId = ''
  for (let index = records.length - 1; index >= 0; index -= 1) {
    const record = records[index]
    if (sessionId !== '' && record.sessionId !== sessionId) {
      nextInterval = null
    }
    sessionId = record.sessionId
    if (record.eventType === 'interval' && record.schemaVersion === 4) {
      nextInterval = record
    }
    if (record.schemaVersion !== 4 || !recordMatches(record, query)) {
      continue
    }
    results.push({
      record,
      intervalRecord: record.eventType === 'interval' ? record : nextInterval,
    })
  }
  return results
}

export function groupSearchResultsByInterval(results: SessionSearchResult[]): SearchResultsByInterval {
  const grouped = new Map<string, SessionSearchResult[]>()
  for (const result of results) {
    if (!result.intervalRecord) {
      continue
    }
    const existing = grouped.get(result.intervalRecord.id)
    if (existing) {
      existing.push(result)
    } else {
      grouped.set(result.intervalRecord.id, [result])
    }
  }
  return grouped
}

function recordMatches(record: PattyLogRecord, query: string): boolean {
  if (record.eventType === 'interval') {
    return interestingMatches(record.data, query) || factoidArrayMatches(record.data, query)
  }
  if (record.eventType === 'control_command') {
    return factoidResultMatches(readObject(record.data, 'result'), query)
  }
  return false
}

function interestingMatches(data: JsonObject, query: string): boolean {
  for (const value of readArray(data, 'interesting')) {
    if (!isJsonObject(value) || !searchableInterestingStreams.has(readString(value, 'name'))) {
      continue
    }
    for (const collection of ['top', 'peaks']) {
      for (const entry of readArray(value, collection)) {
        if (isJsonObject(entry) && textMatches(readString(entry, 'key'), query)) {
          return true
        }
      }
    }
    if (readString(value, 'name') === 'ips') {
      for (const group of readArray(value, 'ip_groups')) {
        if (isJsonObject(group) && textMatches(readString(group, 'prefix'), query)) {
          return true
        }
      }
    }
  }
  return false
}

function factoidArrayMatches(data: JsonObject, query: string): boolean {
  for (const value of readArray(data, 'factoids')) {
    if (isJsonObject(value) && factoidFieldsMatch(value, query)) {
      return true
    }
  }
  return false
}

function factoidResultMatches(result: JsonObject | null, query: string): boolean {
  return result !== null && factoidFieldsMatch(result, query)
}

function factoidFieldsMatch(factoid: JsonObject, query: string): boolean {
  return textMatches(readString(factoid, 'name'), query) ||
    textMatches(readString(factoid, 'fact'), query) ||
    textMatches(readString(factoid, 'text'), query)
}

function textMatches(value: string, query: string): boolean {
  return value !== '' && value.toLowerCase().includes(query)
}

function normalizeSearchQuery(value: string): string {
  return value.trim().toLowerCase()
}
