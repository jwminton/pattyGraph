import {
  BlobWriter,
  ZipWriter,
} from '@zip.js/zip.js'
import { readArray, type PattyLogRecord, type SessionIndex } from './types'
import {
  incidentBundleSchema,
  incidentBundleType,
  incidentManifestEntry,
  incidentPattyLogEntry,
  type IncidentBundleManifest,
} from './incidentBundle'
import type { IncidentRangeSelection } from './incidentRange'

export interface SemanticIncidentPlan {
  manifest: IncidentBundleManifest
  records: PattyLogRecord[]
  suggestedName: string
}

export function planSemanticIncident(
  session: SessionIndex,
  selection: IncidentRangeSelection,
  sourceName: string,
  creatorVersion: string,
): SemanticIncidentPlan {
  if (!session.sessionStart) {
    throw new Error(`Session ${JSON.stringify(session.id)} has no session_start record`)
  }
  const fromIndex = session.intervals.findIndex((record) => record.id === selection.from.id)
  const throughIndex = session.intervals.findIndex((record) => record.id === selection.through.id)
  if (fromIndex < 0 || throughIndex < fromIndex) {
    throw new Error('Semantic incident range does not belong to the selected session')
  }
  const selectedIntervals = session.intervals.slice(fromIndex, throughIndex + 1)
  const selectedRecordIds = new Set(selectedIntervals.map((record) => record.id))
  const selectedIntervalNumbers = new Set(selectedIntervals.flatMap((record) => (
    record.interval === null ? [] : [record.interval]
  )))
  const records = selectOwnedRecords(
    session.records,
    session.sessionStart,
    selectedRecordIds,
    selectedIntervalNumbers,
  )
  const schemaVersions = [...new Set(records.flatMap((record) => (
    record.schemaVersion === null ? [] : [record.schemaVersion]
  )))].sort((left, right) => left - right)
  const retainedSourceIntervals = selectedIntervals.filter((record) => (
    readArray(record.data, 'source_lines').length > 0
  )).length
  const normalizedSourceName = basename(sourceName) || 'pattyLog.jsonl'

  return {
    manifest: {
      bundle_schema: incidentBundleSchema,
      bundle_type: incidentBundleType,
      creator: { name: 'pattyView', version: creatorVersion },
      pattylog: {
        entry: incidentPattyLogEntry,
        representation: 'semantic',
        source_name: normalizedSourceName,
        session_id: session.id,
        schema_versions: schemaVersions,
        record_count: records.length,
        retained_source_intervals: retainedSourceIntervals,
      },
      range: {
        from_log_time: selection.from.logTime,
        through_log_time: selection.through.logTime,
        from_interval: selection.from.interval ?? fromIndex,
        through_interval: selection.through.interval ?? throughIndex,
        interval_count: selectedIntervals.length,
      },
    },
    records,
    suggestedName: semanticBundleName(normalizedSourceName, selection.from.logTime, selection.through.logTime),
  }
}

export async function writeSemanticIncident(
  plan: SemanticIncidentPlan,
  destination: WritableStream<Uint8Array>,
): Promise<void> {
  const modified = new Date(plan.manifest.range.through_log_time)
  if (!Number.isFinite(modified.getTime())) {
    throw new Error('Semantic incident has an invalid final log-time')
  }
  const archive = new ZipWriter(destination, {
    useCompressionStream: true,
    useWebWorkers: false,
  })
  try {
    const entryOptions = { lastModDate: modified }
    await archive.add(
      incidentManifestEntry,
      singleTextStream(`${JSON.stringify(plan.manifest, null, 2)}\n`),
      entryOptions,
    )
    await archive.add(incidentPattyLogEntry, recordStream(plan.records), entryOptions)
    await archive.close()
  } catch (error) {
    await archive.close().catch(() => undefined)
    throw error
  }
}

export async function semanticIncidentBlob(plan: SemanticIncidentPlan): Promise<Blob> {
  const writer = new BlobWriter('application/zip')
  await writeSemanticIncident(plan, writer.writable as WritableStream<Uint8Array>)
  return writer.getData()
}

function selectOwnedRecords(
  records: PattyLogRecord[],
  sessionStart: PattyLogRecord,
  selectedRecordIds: ReadonlySet<string>,
  selectedIntervalNumbers: ReadonlySet<number>,
): PattyLogRecord[] {
  const selected: PattyLogRecord[] = [sessionStart]
  let pending: PattyLogRecord[] = []

  const flushPending = (nextIntervalSelected: boolean) => {
    for (const record of pending) {
      const include = record.eventType === 'alert'
        ? record.interval !== null && selectedIntervalNumbers.has(record.interval)
        : nextIntervalSelected
      if (include) {
        selected.push(record)
      }
    }
    pending = []
  }

  for (const record of records) {
    if (record.id === sessionStart.id || record.eventType === 'session_start') {
      continue
    }
    if (record.eventType !== 'interval') {
      pending.push(record)
      continue
    }
    const intervalSelected = selectedRecordIds.has(record.id)
    flushPending(intervalSelected)
    if (intervalSelected) {
      selected.push(record)
    }
  }
  flushPending(false)
  return selected
}

function recordStream(records: PattyLogRecord[]): ReadableStream<Uint8Array> {
  const encoder = new TextEncoder()
  let index = 0
  return new ReadableStream({
    pull(controller) {
      if (index >= records.length) {
        controller.close()
        return
      }
      controller.enqueue(encoder.encode(`${JSON.stringify(records[index].data)}\n`))
      index += 1
    },
  })
}

function singleTextStream(value: string): ReadableStream<Uint8Array> {
  const encoded = new TextEncoder().encode(value)
  return new ReadableStream({
    start(controller) {
      controller.enqueue(encoded)
      controller.close()
    },
  })
}

export function semanticBundleName(sourceName: string, fromLogTime: string, throughLogTime: string): string {
  return `${compactSourceStem(sourceName)}_${compactLogTimeRange(fromLogTime, throughLogTime)}.incident.zip`
}

function compactLogTimeRange(fromValue: string, throughValue: string): string {
  const from = logTimeParts(fromValue)
  const through = logTimeParts(throughValue)
  if (!from || !through) {
    return 'unknown-range'
  }
  if (from.date === through.date) {
    return `${from.date}_${from.minute}-${through.minute}`
  }
  return `${from.date}_${from.minute}-${through.date}_${through.minute}`
}

function logTimeParts(value: string): { date: string; minute: string } | null {
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.\d+)?(Z|[+-]\d{2}:\d{2})$/.exec(value)
  if (!match) {
    return null
  }
  const [, year, month, day, hour, minute] = match
  return { date: `${year}${month}${day}`, minute: `${hour}${minute}` }
}

function compactSourceStem(sourceName: string): string {
  let stem = basename(sourceName).trim()
  while (/\.(?:jsonl|zip|incident)$/i.test(stem)) {
    stem = stem.replace(/\.(?:jsonl|zip|incident)$/i, '')
  }
  stem = Array.from(stem).slice(0, 32).join('')
  return stem || 'pattyLog'
}

function basename(value: string): string {
  return value.split(/[\\/]/).pop() ?? ''
}
