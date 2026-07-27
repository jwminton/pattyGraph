import {
  readNumber,
  readString,
  type PattyLogRecord,
} from './types'

export interface RecordedAlert {
  record: PattyLogRecord
  interval: number
  status: string
  matcher: string
  direction: string
  value: number | null
  threshold: number | null
  fluxDepth: number | null
  streak: number | null
  currentCycle: number | null
  logTime: string
}

export type AlertTimeline = ReadonlyMap<number, RecordedAlert[]>

export function projectRecordedAlert(record: PattyLogRecord): RecordedAlert | null {
  if (record.schemaVersion !== 4 || record.eventType !== 'alert' || record.interval === null) {
    return null
  }
  return {
    record,
    interval: record.interval,
    status: readString(record.data, 'status'),
    matcher: readString(record.data, 'matcher'),
    direction: readString(record.data, 'direction'),
    value: readNumber(record.data, 'value'),
    threshold: readNumber(record.data, 'threshold'),
    fluxDepth: readNumber(record.data, 'flux_depth'),
    streak: readNumber(record.data, 'streak'),
    currentCycle: readNumber(record.data, 'current_cycle'),
    logTime: record.logTime,
  }
}

export function buildAlertTimeline(records: PattyLogRecord[]): AlertTimeline {
  const timeline = new Map<number, RecordedAlert[]>()
  for (const record of records) {
    const alert = projectRecordedAlert(record)
    if (!alert) {
      continue
    }
    const current = timeline.get(alert.interval)
    if (current) {
      current.push(alert)
    } else {
      timeline.set(alert.interval, [alert])
    }
  }
  return timeline
}

export function alertStatusSummary(alerts: RecordedAlert[]): string {
  if (alerts.length === 1) {
    const alert = alerts[0]
    return `${alert.matcher || 'matcher'} ${alert.status || 'alert'}`
  }
  return `${alerts.length} alerts`
}
