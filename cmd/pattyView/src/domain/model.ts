import type { PattyLogRecord, SessionIndex } from './types'

export function buildSessionIndexes(records: PattyLogRecord[]): SessionIndex[] {
  const grouped = new Map<string, PattyLogRecord[]>()

  for (const record of records) {
    const sessionRecords = grouped.get(record.sessionId)
    if (sessionRecords) {
      sessionRecords.push(record)
    } else {
      grouped.set(record.sessionId, [record])
    }
  }

  return [...grouped.entries()].map(([id, sessionRecords]) => {
    const schemaVersions = new Set<number>()
    let sessionStart: PattyLogRecord | null = null
    const intervals: PattyLogRecord[] = []
    const alerts: PattyLogRecord[] = []

    for (const record of sessionRecords) {
      if (record.schemaVersion !== null) {
        schemaVersions.add(record.schemaVersion)
      }
      if (record.eventType === 'session_start' && sessionStart === null) {
        sessionStart = record
      }
      if (record.eventType === 'interval') {
        intervals.push(record)
      }
      if (record.eventType === 'alert') {
        alerts.push(record)
      }
    }

    const timelineRecords = sessionRecords.filter((record) => record.eventType !== 'session_start')
    return {
      id,
      records: sessionRecords,
      intervals,
      alerts,
      sessionStart,
      schemaVersions: [...schemaVersions].sort((a, b) => a - b),
      firstTimestamp: timelineRecords.find((record) => record.logTime)?.logTime ?? '',
      lastTimestamp: [...timelineRecords].reverse().find((record) => record.logTime)?.logTime ?? '',
    }
  })
}

export function newestRecord(session: SessionIndex | undefined): PattyLogRecord | null {
  if (!session || session.records.length === 0) {
    return null
  }
  return session.records[session.records.length - 1]
}

export function recordIndex(session: SessionIndex | undefined, id: string): number {
  if (!session) {
    return -1
  }
  return session.records.findIndex((record) => record.id === id)
}
