import { readString, type PattyLogRecord } from './types'

export type PeakResetPhase = 'reset' | 'rebaseline'

// Peak purge is a model discontinuity. Associate it with the next two interval
// records without changing any traffic values: emptying Peak memory and then
// establishing the rebuilt Peak baseline.
export function buildPeakResetPhases(
  records: PattyLogRecord[],
): ReadonlyMap<string, PeakResetPhase> {
  const phases = new Map<string, PeakResetPhase>()
  visitPeakResetPhases(records, (record, phase) => phases.set(record.id, phase))

  return phases
}

export function peakResetPhasesInRange(
  records: PattyLogRecord[],
  left: PattyLogRecord,
  right: PattyLogRecord,
): PeakResetPhase[] {
  const first = Math.min(left.fileOrder, right.fileOrder)
  const last = Math.max(left.fileOrder, right.fileOrder)
  const found = new Set<PeakResetPhase>()

  visitPeakResetPhases(records, (record, phase) => {
    if (record.fileOrder >= first && record.fileOrder <= last) {
      found.add(phase)
    }
  })

  return [...found]
}

function visitPeakResetPhases(
  records: PattyLogRecord[],
  visit: (record: PattyLogRecord, phase: PeakResetPhase) => void,
): void {
  const pendingBySession = new Map<string, number>()

  for (const record of records) {
    if (record.eventType === 'session_start') {
      pendingBySession.delete(record.sessionId)
      continue
    }
    if (isAppliedPeakPurge(record)) {
      pendingBySession.set(record.sessionId, 2)
      continue
    }
    if (record.eventType !== 'interval' || record.schemaVersion !== 4) {
      continue
    }

    const pending = pendingBySession.get(record.sessionId) ?? 0
    if (pending === 2) {
      visit(record, 'reset')
      pendingBySession.set(record.sessionId, 1)
    } else if (pending === 1) {
      visit(record, 'rebaseline')
      pendingBySession.delete(record.sessionId)
    }
  }
}

function isAppliedPeakPurge(record: PattyLogRecord): boolean {
  return record.eventType === 'control_command' &&
    readString(record.data, 'command_name').toLowerCase() === 'purge' &&
    readString(record.data, 'status').toLowerCase() === 'applied'
}
