export const maximumTrackedLanes = 8

export const optionalCoreIntervalLanes = [
  { key: 'bytes', matcherName: 'bytes', label: 'Bytes', color: '#e3ba72' },
  { key: 'errors', matcherName: 'errs', label: 'Errs', color: '#ff6b6b' },
] as const

export type CoreIntervalLaneKey = typeof optionalCoreIntervalLanes[number]['key']

export const intervalMetricLanes = [
  { key: 'markedPercent', label: 'Marked %', color: '#d69bdc' },
  { key: 'b16Percent', label: 'B16 %', color: '#83cf79' },
] as const

export type IntervalMetricLaneKey = typeof intervalMetricLanes[number]['key']

export type TrackedLane = MatcherTrackedLane | InterestingTrackedLane

export interface MatcherTrackedLane {
  kind: 'matcher'
  id: string
  name: string
  label: string
  color: string
}

export interface InterestingTrackedLane {
  kind: 'interesting'
  id: string
  stream: string
  key: string
  label: string
  color: string
}

export function matcherTrackedLane(name: string, color: string): MatcherTrackedLane {
  return {
    kind: 'matcher',
    id: `matcher:${name}`,
    name,
    label: name,
    color,
  }
}

export function interestingTrackedLane(
  stream: string,
  key: string,
  color: string,
): InterestingTrackedLane {
  return {
    kind: 'interesting',
    id: `interesting:${stream}:${key}`,
    stream,
    key,
    label: `${streamLabel(stream)} ${key}`,
    color,
  }
}

function streamLabel(stream: string): string {
  switch (stream) {
    case 'words': return 'W'
    case 'refs': return 'R'
    case 'ips': return 'IP'
    default: return stream.toUpperCase()
  }
}
