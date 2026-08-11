import type { PattyLogRecord } from './types'

export interface IncidentRangeSelection {
  from: PattyLogRecord
  through: PattyLogRecord
  intervalCount: number
}

export interface IncidentRangeCommand {
  command: string
  error: string
}

export function selectIncidentRange(
  intervals: PattyLogRecord[],
  first: PattyLogRecord,
  second: PattyLogRecord,
): IncidentRangeSelection {
  const firstIndex = intervals.findIndex((record) => record.id === first.id)
  const secondIndex = intervals.findIndex((record) => record.id === second.id)
  if (firstIndex < 0 || secondIndex < 0) {
    throw new Error('Incident range endpoints must belong to the current session')
  }
  if (firstIndex === secondIndex) {
    throw new Error('Incident range must contain at least two intervals')
  }
  const fromIndex = Math.min(firstIndex, secondIndex)
  const throughIndex = Math.max(firstIndex, secondIndex)
  return {
    from: intervals[fromIndex],
    through: intervals[throughIndex],
    intervalCount: throughIndex - fromIndex + 1,
  }
}

export function buildIncidentRangeCommand(
  selection: IncidentRangeSelection,
  intervals: PattyLogRecord[],
  sourceName: string,
  sessionId: string,
): IncidentRangeCommand {
  if (sourceName.trim() === '') {
    return invalidCommand('The PattyLog filename is unavailable')
  }
  if (sessionId.trim() === '') {
    return invalidCommand('The PattyLog session is unavailable')
  }
  const fromTime = selection.from.logTime
  const throughTime = selection.through.logTime
  if (!validLogTime(fromTime) || !validLogTime(throughTime)) {
    return invalidCommand('The selected range has an invalid endpoint log-time')
  }
  if (intervals.filter((record) => record.logTime === fromTime).length !== 1 ||
      intervals.filter((record) => record.logTime === throughTime).length !== 1) {
    return invalidCommand('The selected range has a repeated endpoint log-time')
  }
  if (Date.parse(fromTime) > Date.parse(throughTime)) {
    return invalidCommand('The selected endpoint clocks move backward and cannot be expressed by the bundle command')
  }

  return {
    command: [
      'pattyView --bundle',
      shellQuote(sourceName),
      '--from',
      shellQuote(fromTime),
      '--through',
      shellQuote(throughTime),
      '--session',
      shellQuote(sessionId),
    ].join(' '),
    error: '',
  }
}

function validLogTime(value: string): boolean {
  return value.trim() !== '' && Number.isFinite(Date.parse(value))
}

function shellQuote(value: string): string {
  return `'${value.replaceAll("'", `'"'"'`)}'`
}

function invalidCommand(error: string): IncidentRangeCommand {
  return { command: '', error }
}
