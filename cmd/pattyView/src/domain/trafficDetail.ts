import {
  isJsonObject,
  readArray,
  readBoolean,
  readNumber,
  readObject,
  readString,
  type JsonObject,
} from './types'

export interface SourceExample {
  label: string
  line: string
}

export type SourceExampleAvailability = 'enabled' | 'disabled' | 'unknown'

export interface CountEntry {
  key: string
  count: number | null
  rank: number | null
}

export interface GroupEntry {
  prefix: string
  count: number | null
  members: number | null
  rank: number | null
}

export interface TrafficMatcher {
  name: string
  colorHex: string
  current: number | null
  previous: number | null
  historyTotal: number | null
  historyPeak: number | null
  topKeys: CountEntry[]
  topGroups: GroupEntry[]
  sources: SourceExample[]
}

export interface InterestingEntry {
  key: string
  rank: number | null
  score: number | null
  count: number | null
  bytes: number | null
  primeFlux: number | null
  burstiness: number | null
  agentDeltaMetric: number | null
  historyTotal: number | null
  historyPeak: number | null
  historyDepth: number | null
  lastSeenTic: number | null
  lastStatus: string
  color: string
  markedState: string
  markedByMatcher: string
  isPeak: boolean
  sources: SourceExample[]
}

export interface IPGroupEntry {
  prefix: string
  rank: number | null
  score: number | null
  count: number | null
  countPlusFirst: number | null
  members: number | null
  bytes: number | null
  burstiness: number | null
  agentDeltaMetric: number | null
  historyDepth: number | null
  color: string
  markedState: string
  markedByMatcher: string
  sources: SourceExample[]
}

export interface InterestingStream {
  name: string
  totalKeys: number | null
  top: InterestingEntry[]
  peaks: InterestingEntry[]
  ipGroups: IPGroupEntry[]
}

export interface TrafficFactoid {
  name: string
  text: string
}

export interface SelectedContext {
  fields: Array<{ label: string; value: string }>
  sources: SourceExample[]
}

export interface TrafficDetail {
  matchers: TrafficMatcher[]
  interesting: InterestingStream[]
  factoids: TrafficFactoid[]
  selected: SelectedContext | null
  sourceExampleAvailability: SourceExampleAvailability
}

// System matchers remain available in the raw record and to domain code that
// consumes PattyLog directly. Ordinary Traffic and Compare views present their
// dedicated navigation surfaces instead of duplicating these implementation
// details as user-managed matchers.
const hiddenSystemMatchers = new Set(['change'])

export function projectTrafficDetail(data: JsonObject): TrafficDetail {
  const sourceLines = readArray(data, 'source_lines')
    .filter((value): value is string => typeof value === 'string')
  return {
    matchers: objects(readArray(data, 'matchers'))
      .map(projectMatcher)
      .filter((matcher) => !hiddenSystemMatchers.has(matcher.name)),
    interesting: objects(readArray(data, 'interesting'))
      .map((stream) => projectInteresting(stream, sourceLines)),
    factoids: projectFactoids(data),
    selected: projectSelected(readObject(data, 'selected')),
    sourceExampleAvailability: projectSourceExampleAvailability(data, sourceLines),
  }
}

function projectSourceExampleAvailability(
  data: JsonObject,
  sourceLines: string[],
): SourceExampleAvailability {
  const enabled = readBoolean(data, 'source_examples_enabled')
  if (enabled !== null) {
    return enabled ? 'enabled' : 'disabled'
  }
  return sourceLines.length > 0 ? 'enabled' : 'unknown'
}

export function projectFactoids(data: JsonObject): TrafficFactoid[] {
  return objects(readArray(data, 'factoids')).map((factoid) => ({
    name: readString(factoid, 'name'),
    text: readString(factoid, 'text'),
  })).filter((factoid) => factoid.text !== '')
}

function projectMatcher(matcher: JsonObject): TrafficMatcher {
  return {
    name: readString(matcher, 'name'),
    colorHex: readString(matcher, 'color_hex'),
    current: readNumber(matcher, 'interval_count'),
    previous: readNumber(matcher, 'last_interval_count'),
    historyTotal: readNumber(matcher, 'history_total'),
    historyPeak: readNumber(matcher, 'history_peak'),
    topKeys: objects(readArray(matcher, 'top_keys')).map((entry) => ({
      key: readString(entry, 'key'),
      count: readNumber(entry, 'count'),
      rank: readNumber(entry, 'rank'),
    })),
    topGroups: objects(readArray(matcher, 'top_groups')).map((entry) => ({
      prefix: readString(entry, 'prefix'),
      count: readNumber(entry, 'count'),
      members: readNumber(entry, 'members'),
      rank: readNumber(entry, 'rank'),
    })),
    sources: stringSources(matcher, [
      ['First seen', 'first_line'],
      ['First this interval', 'interval_line'],
      ['Latest seen', 'last_line'],
    ]),
  }
}

function projectInteresting(stream: JsonObject, sourceLines: string[]): InterestingStream {
  return {
    name: readString(stream, 'name'),
    totalKeys: readNumber(stream, 'total_keys'),
    top: objects(readArray(stream, 'top'))
      .map((entry) => projectInterestingEntry(entry, sourceLines)),
    peaks: objects(readArray(stream, 'peaks'))
      .map((entry) => projectInterestingEntry(entry, sourceLines)),
    ipGroups: objects(readArray(stream, 'ip_groups'))
      .map((entry) => projectIPGroup(entry, sourceLines)),
  }
}

function projectInterestingEntry(entry: JsonObject, sourceLines: string[]): InterestingEntry {
  return {
    key: readString(entry, 'key'),
    rank: readNumber(entry, 'rank'),
    score: readNumber(entry, 'score'),
    count: readNumber(entry, 'count'),
    bytes: readNumber(entry, 'bytes'),
    primeFlux: readNumber(entry, 'prime_flux'),
    burstiness: readNumber(entry, 'burstiness'),
    agentDeltaMetric: readNumber(entry, 'agent_delta_metric'),
    historyTotal: readNumber(entry, 'history_total'),
    historyPeak: readNumber(entry, 'history_peak'),
    historyDepth: readNumber(entry, 'history_depth'),
    lastSeenTic: readNumber(entry, 'last_seen_tic'),
    lastStatus: readString(entry, 'last_status'),
    color: readString(entry, 'color'),
    markedState: readString(entry, 'marked_state'),
    markedByMatcher: readString(entry, 'marked_by_matcher'),
    isPeak: entry.is_peak === true,
    sources: uniqueSources([
      ...objectSources(entry, [['First seen', 'source']]),
      ...stringSources(entry, [
        ['First this interval', 'first_interval_line'],
        ['Latest seen', 'last_line'],
      ]),
      ...referencedSource(entry, sourceLines),
    ]),
  }
}

function projectIPGroup(group: JsonObject, sourceLines: string[]): IPGroupEntry {
  return {
    prefix: readString(group, 'prefix'),
    rank: readNumber(group, 'rank'),
    score: readNumber(group, 'score'),
    count: readNumber(group, 'count'),
    countPlusFirst: readNumber(group, 'count_plus_first'),
    members: readNumber(group, 'members'),
    bytes: readNumber(group, 'bytes'),
    burstiness: readNumber(group, 'burstiness'),
    agentDeltaMetric: readNumber(group, 'agent_delta_metric'),
    historyDepth: readNumber(group, 'history_depth'),
    color: readString(group, 'color'),
    markedState: readString(group, 'marked_state'),
    markedByMatcher: readString(group, 'marked_by_matcher'),
    sources: uniqueSources([
      ...stringSources(group, [
        ['First seen', 'first_line'],
        ['First this interval', 'first_interval_line'],
        ['Latest seen', 'last_line'],
      ]),
      ...referencedSource(group, sourceLines),
    ]),
  }
}

function projectSelected(selected: JsonObject | null): SelectedContext | null {
  if (!selected || Object.keys(selected).length === 0) {
    return null
  }
  const fields = [
    ['Graph value', 'graph_value'],
    ['Selection', 'selection_value'],
    ['Interesting stream', 'interesting_matcher'],
    ['Interesting key', 'interesting_key'],
    ['Matcher', 'matcher'],
  ].flatMap(([label, key]) => {
    const number = readNumber(selected, key)
    const value = number === null ? readString(selected, key) : String(number)
    return value === '' ? [] : [{ label, value }]
  })
  const sources = objectSources(selected, [
    ['First seen', 'first_source'],
    ['First this interval', 'first_interval_source'],
    ['Latest seen', 'last_source'],
  ])
  return fields.length > 0 || sources.length > 0 ? { fields, sources } : null
}

function objects(values: unknown[]): JsonObject[] {
  return values.filter(isJsonObject)
}

function stringSources(object: JsonObject, fields: Array<[string, string]>): SourceExample[] {
  return fields.flatMap(([label, key]) => {
    const line = readString(object, key)
    return line === '' ? [] : [{ label, line }]
  })
}

function objectSources(object: JsonObject, fields: Array<[string, string]>): SourceExample[] {
  return fields.flatMap(([label, key]) => {
    const source = readObject(object, key)
    const line = source ? readString(source, 'log_line') : ''
    return line === '' ? [] : [{ label, line }]
  })
}

function referencedSource(object: JsonObject, sourceLines: string[]): SourceExample[] {
  const ref = readNumber(object, 'source_line_ref')
  if (ref === null || !Number.isInteger(ref) || ref <= 0 || ref > sourceLines.length) {
    return []
  }
  const line = sourceLines[ref - 1]
  return line === '' ? [] : [{ label: 'Latest retained', line }]
}

function uniqueSources(sources: SourceExample[]): SourceExample[] {
  const lines = new Set<string>()
  return sources.filter((source) => {
    if (lines.has(source.line)) {
      return false
    }
    lines.add(source.line)
    return true
  })
}
