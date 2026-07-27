import { buildAlertTimeline, type RecordedAlert } from './alertTimeline'
import { buildIntervalSeries } from './intervalSeries'
import {
  projectFactoids,
  projectTrafficDetail,
  type InterestingStream,
  type TrafficFactoid,
  type TrafficMatcher,
} from './trafficDetail'
import {
  readObject,
  readString,
  type PattyLogRecord,
} from './types'
import { peakResetPhasesInRange, type PeakResetPhase } from './peakReset'

export type ComparisonValueKind = 'count' | 'bytes' | 'percent'

export interface ComparisonSignal {
  key: string
  label: string
  kind: ComparisonValueKind
  reference: number | null
  selected: number | null
  delta: number | null
}

export interface MatcherComparison {
  name: string
  colorHex: string
  referencePosition: number | null
  selectedPosition: number | null
  reference: number | null
  selected: number | null
  delta: number | null
}

export interface IntervalContext {
  factoids: TrafficFactoid[]
  alerts: RecordedAlert[]
}

export interface IntervalComparison {
  reference: PattyLogRecord
  selected: PattyLogRecord
  signals: ComparisonSignal[]
  matchers: MatcherComparison[]
  referenceStreams: InterestingStream[]
  selectedStreams: InterestingStream[]
  referenceContext: IntervalContext
  selectedContext: IntervalContext
  peakResetPhases: PeakResetPhase[]
}

const emptyContext = (): IntervalContext => ({ factoids: [], alerts: [] })

export function projectIntervalComparison(
  reference: PattyLogRecord,
  selected: PattyLogRecord,
  sessionRecords: PattyLogRecord[],
): IntervalComparison {
  const referencePoint = buildIntervalSeries([reference])[0]
  const selectedPoint = buildIntervalSeries([selected])[0]
  const referenceDetail = projectTrafficDetail(reference.data)
  const selectedDetail = projectTrafficDetail(selected.data)
  const contexts = buildIntervalContexts(sessionRecords)

  return {
    reference,
    selected,
    signals: [
      signal('lines', 'Lines', 'count', referencePoint?.lines ?? null, selectedPoint?.lines ?? null),
      signal('bytes', 'Bytes', 'bytes', referencePoint?.bytes ?? null, selectedPoint?.bytes ?? null),
      signal('errors', 'Errors', 'count', referencePoint?.errors ?? null, selectedPoint?.errors ?? null),
      signal('marked', 'Marked', 'percent', referencePoint?.markedPercent ?? null, selectedPoint?.markedPercent ?? null),
      signal('b16', 'B16', 'percent', referencePoint?.b16Percent ?? null, selectedPoint?.b16Percent ?? null),
    ],
    matchers: compareMatchers(referenceDetail.matchers, selectedDetail.matchers),
    referenceStreams: referenceDetail.interesting,
    selectedStreams: selectedDetail.interesting,
    referenceContext: contexts.get(reference.id) ?? emptyContext(),
    selectedContext: contexts.get(selected.id) ?? emptyContext(),
    peakResetPhases: peakResetPhasesInRange(sessionRecords, reference, selected),
  }
}

// Context records are projected onto the interval they describe. Standalone fact
// commands precede that interval in file order, while alerts carry an interval ID.
export function buildIntervalContexts(records: PattyLogRecord[]): ReadonlyMap<string, IntervalContext> {
  const contexts = new Map<string, IntervalContext>()
  const intervalsByNumber = new Map<number, PattyLogRecord>()

  for (const record of records) {
    if (record.eventType !== 'interval' || record.schemaVersion !== 4) {
      continue
    }
    contexts.set(record.id, {
      factoids: projectFactoids(record.data),
      alerts: [],
    })
    if (record.interval !== null) {
      intervalsByNumber.set(record.interval, record)
    }
  }

  const alertTimeline = buildAlertTimeline(records)
  for (const [interval, alerts] of alertTimeline) {
    const record = intervalsByNumber.get(interval)
    const context = record ? contexts.get(record.id) : null
    if (context) {
      context.alerts.push(...alerts)
    }
  }

  let nextInterval: PattyLogRecord | null = null
  for (let index = records.length - 1; index >= 0; index -= 1) {
    const record = records[index]
    if (record.eventType === 'session_start') {
      nextInterval = null
      continue
    }
    if (record.eventType === 'interval' && record.schemaVersion === 4) {
      nextInterval = record
      continue
    }
    if (record.eventType !== 'control_command' || !nextInterval) {
      continue
    }
    const result = readObject(record.data, 'result')
    const name = result ? readString(result, 'fact') : ''
    const text = result ? readString(result, 'text') : ''
    const context = contexts.get(nextInterval.id)
    if (context && text) {
      context.factoids.unshift({ name, text })
    }
  }

  return contexts
}

function signal(
  key: string,
  label: string,
  kind: ComparisonValueKind,
  reference: number | null,
  selected: number | null,
): ComparisonSignal {
  return {
    key,
    label,
    kind,
    reference,
    selected,
    delta: difference(reference, selected),
  }
}

function compareMatchers(
  referenceMatchers: TrafficMatcher[],
  selectedMatchers: TrafficMatcher[],
): MatcherComparison[] {
  const names = [
    ...selectedMatchers.map((matcher) => matcher.name),
    ...referenceMatchers
      .filter((matcher) => !selectedMatchers.some((selected) => selected.name === matcher.name))
      .map((matcher) => matcher.name),
  ]

  return names.map((name) => {
    const referencePosition = referenceMatchers.findIndex((matcher) => matcher.name === name)
    const selectedPosition = selectedMatchers.findIndex((matcher) => matcher.name === name)
    const reference = referencePosition >= 0 ? referenceMatchers[referencePosition] : null
    const selected = selectedPosition >= 0 ? selectedMatchers[selectedPosition] : null
    return {
      name,
      colorHex: selected?.colorHex || reference?.colorHex || '',
      referencePosition: referencePosition >= 0 ? referencePosition + 1 : null,
      selectedPosition: selectedPosition >= 0 ? selectedPosition + 1 : null,
      reference: reference?.current ?? null,
      selected: selected?.current ?? null,
      delta: difference(reference?.current ?? null, selected?.current ?? null),
    }
  })
}

function difference(reference: number | null, selected: number | null): number | null {
  return reference === null || selected === null ? null : selected - reference
}
