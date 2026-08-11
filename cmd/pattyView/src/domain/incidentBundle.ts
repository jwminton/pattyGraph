import {
  BlobReader,
  TextWriter,
  ZipReader,
  type Entry,
  type FileEntry,
} from '@zip.js/zip.js'
import type { ParseBatch, PattyLogRecord } from './types'
import { isJsonObject } from './types'

export const incidentBundleType = 'pattygraph_incident'
export const incidentBundleSchema = 1
export const incidentManifestEntry = 'manifest.json'
export const incidentPattyLogEntry = 'pattyLog.jsonl'

const maximumManifestSize = 1024 * 1024

export interface IncidentBundleManifest {
  bundle_schema: number
  bundle_type: string
  creator: {
    name: string
    version: string
  }
  pattylog: {
    entry: string
    representation: 'source' | 'semantic'
    source_name: string
    session_id: string
    schema_versions: number[]
    record_count: number
    retained_source_intervals: number
  }
  range: {
    from_log_time: string
    through_log_time: string
    from_interval: number
    through_interval: number
    interval_count: number
  }
}

export interface OpenIncidentBundle {
  manifest: IncidentBundleManifest
  pattyLogSize: number
  streamPattyLog: (
    writable: WritableStream<Uint8Array>,
    signal: AbortSignal,
  ) => Promise<void>
  close: () => Promise<void>
}

export async function isIncidentBundleFile(file: File): Promise<boolean> {
  if (file.name.toLowerCase().endsWith('.zip')) {
    return true
  }
  const signature = new Uint8Array(await file.slice(0, 4).arrayBuffer())
  return signature.length === 4 &&
    signature[0] === 0x50 &&
    signature[1] === 0x4b &&
    (
      (signature[2] === 0x03 && signature[3] === 0x04) ||
      (signature[2] === 0x05 && signature[3] === 0x06) ||
      (signature[2] === 0x07 && signature[3] === 0x08)
    )
}

export async function openIncidentBundle(file: Blob, signal?: AbortSignal): Promise<OpenIncidentBundle> {
  const reader = new ZipReader(new BlobReader(file), {
    strictness: 'strict',
    useCompressionStream: true,
    // Keep Vite's committed asset shape stable. Modern browsers still use the
    // native streaming decompressor without a second library-owned worker.
    useWebWorkers: false,
  })
  try {
    const entries = await reader.getEntries()
    const manifestEntry = selectIncidentBundleFile(entries, incidentManifestEntry)
    if (manifestEntry.uncompressedSize > maximumManifestSize) {
      throw new Error(`Incident bundle manifest exceeds ${maximumManifestSize} bytes`)
    }
    const manifestText = await manifestEntry.getData(new TextWriter(), {
      checkOverlappingEntry: true,
      checkSignature: true,
      signal,
    })
    const manifest = parseIncidentBundleManifest(manifestText)
    const pattyLogEntry = selectIncidentBundleFile(entries, manifest.pattylog.entry)
    let closed = false
    let streamed = false

    return {
      manifest,
      pattyLogSize: pattyLogEntry.uncompressedSize,
      streamPattyLog: async (writable, signal) => {
        if (closed) {
          throw new Error('Incident bundle is already closed')
        }
        if (streamed) {
          throw new Error('Incident bundle PattyLog has already been read')
        }
        streamed = true
        await pattyLogEntry.getData(writable, {
          checkOverlappingEntry: true,
          checkSignature: true,
          signal,
        })
      },
      close: async () => {
        if (!closed) {
          closed = true
          await reader.close()
        }
      },
    }
  } catch (error) {
    await reader.close().catch(() => undefined)
    throw bundleError(error)
  }
}

export function parseIncidentBundleManifest(text: string): IncidentBundleManifest {
  let value: unknown
  try {
    value = JSON.parse(text)
  } catch (error) {
    throw new Error(`Invalid incident bundle manifest JSON: ${errorMessage(error)}`)
  }
  if (!isJsonObject(value)) {
    throw new Error('Invalid incident bundle manifest: root must be an object')
  }

  const bundleSchema = requiredInteger(value.bundle_schema, 'bundle_schema', 0)
  if (bundleSchema !== incidentBundleSchema) {
    throw new Error(`Unsupported incident bundle schema ${bundleSchema}; expected ${incidentBundleSchema}`)
  }
  const bundleType = requiredString(value.bundle_type, 'bundle_type')
  if (bundleType !== incidentBundleType) {
    throw new Error(`Unsupported incident bundle type ${JSON.stringify(bundleType)}`)
  }
  const creator = requiredObject(value.creator, 'creator')
  const pattylog = requiredObject(value.pattylog, 'pattylog')
  const range = requiredObject(value.range, 'range')
  const entry = requiredString(pattylog.entry, 'pattylog.entry')
  if (entry !== incidentPattyLogEntry) {
    throw new Error(`Unsupported PattyLog bundle entry ${JSON.stringify(entry)}; expected ${incidentPattyLogEntry}`)
  }
  const schemaVersions = requiredIntegerArray(pattylog.schema_versions, 'pattylog.schema_versions')
  const representation = optionalRepresentation(pattylog.representation)
  const fromLogTime = requiredLogTime(range.from_log_time, 'range.from_log_time')
  const throughLogTime = requiredLogTime(range.through_log_time, 'range.through_log_time')

  return {
    bundle_schema: bundleSchema,
    bundle_type: bundleType,
    creator: {
      name: requiredString(creator.name, 'creator.name'),
      version: requiredString(creator.version, 'creator.version'),
    },
    pattylog: {
      entry,
      representation,
      source_name: requiredString(pattylog.source_name, 'pattylog.source_name'),
      session_id: requiredString(pattylog.session_id, 'pattylog.session_id'),
      schema_versions: schemaVersions,
      record_count: requiredInteger(pattylog.record_count, 'pattylog.record_count', 1),
      retained_source_intervals: requiredInteger(
        pattylog.retained_source_intervals,
        'pattylog.retained_source_intervals',
        0,
      ),
    },
    range: {
      from_log_time: fromLogTime,
      through_log_time: throughLogTime,
      from_interval: requiredInteger(range.from_interval, 'range.from_interval'),
      through_interval: requiredInteger(range.through_interval, 'range.through_interval'),
      interval_count: requiredInteger(range.interval_count, 'range.interval_count', 1),
    },
  }
}

export function validateIncidentBundleRecords(
  manifest: IncidentBundleManifest,
  batch: ParseBatch,
): void {
  if (manifest.pattylog.representation === 'semantic' && batch.issues.length > 0) {
    throw new Error('Semantic incident bundle contains malformed PattyLog records')
  }
  const observedRecordCount = batch.records.length + batch.issues.length
  if (observedRecordCount !== manifest.pattylog.record_count) {
    throw new Error(
      `Incident bundle record count is ${observedRecordCount}; manifest declares ${manifest.pattylog.record_count}`,
    )
  }
  const sessions = new Set(batch.records.map((record) => record.sessionId))
  if (sessions.size !== 1 || !sessions.has(manifest.pattylog.session_id)) {
    throw new Error(
      `Incident bundle records do not match session ${JSON.stringify(manifest.pattylog.session_id)}`,
    )
  }
  const sessionStarts = batch.records.filter((record) => record.eventType === 'session_start')
  if (sessionStarts.length !== 1) {
    throw new Error(`Incident bundle contains ${sessionStarts.length} session_start records; expected 1`)
  }
  const intervals = batch.records.filter((record) => record.eventType === 'interval')
  if (intervals.length !== manifest.range.interval_count) {
    throw new Error(
      `Incident bundle contains ${intervals.length} intervals; manifest declares ${manifest.range.interval_count}`,
    )
  }
  validateEndpoint(intervals[0], manifest.range.from_interval, manifest.range.from_log_time, 'first')
  validateEndpoint(
    intervals[intervals.length - 1],
    manifest.range.through_interval,
    manifest.range.through_log_time,
    'final',
  )

  const schemaVersions = [...new Set(batch.records.flatMap((record) => (
    record.schemaVersion === null ? [] : [record.schemaVersion]
  )))].sort((left, right) => left - right)
  if (!equalNumbers(schemaVersions, manifest.pattylog.schema_versions)) {
    throw new Error(
      `Incident bundle schema versions ${schemaVersions.join(', ')} do not match manifest ` +
      manifest.pattylog.schema_versions.join(', '),
    )
  }
}

export function selectIncidentBundleFile(entries: Entry[], name: string): FileEntry {
  const matches = entries.filter((entry) => entry.filename === name)
  if (matches.length !== 1) {
    throw new Error(`Incident bundle requires exactly one ${name} entry; found ${matches.length}`)
  }
  const entry = matches[0]
  if (entry.directory) {
    throw new Error(`Incident bundle entry ${name} is a directory`)
  }
  if (entry.encrypted) {
    throw new Error(`Incident bundle entry ${name} is encrypted and cannot be read`)
  }
  return entry
}

function requiredObject(value: unknown, name: string) {
  if (!isJsonObject(value)) {
    throw new Error(`Invalid incident bundle manifest: ${name} must be an object`)
  }
  return value
}

function requiredString(value: unknown, name: string): string {
  if (typeof value !== 'string' || value.trim() === '') {
    throw new Error(`Invalid incident bundle manifest: ${name} must be a non-empty string`)
  }
  return value
}

function requiredInteger(value: unknown, name: string, minimum?: number): number {
  if (!Number.isSafeInteger(value) || (minimum !== undefined && (value as number) < minimum)) {
    const suffix = minimum === undefined ? '' : ` at least ${minimum}`
    throw new Error(`Invalid incident bundle manifest: ${name} must be an integer${suffix}`)
  }
  return value as number
}

function requiredIntegerArray(value: unknown, name: string): number[] {
  if (!Array.isArray(value) || value.some((entry) => !Number.isSafeInteger(entry))) {
    throw new Error(`Invalid incident bundle manifest: ${name} must contain only integers`)
  }
  return [...value].sort((left, right) => left - right)
}

function optionalRepresentation(value: unknown): 'source' | 'semantic' {
  if (value === undefined) {
    return 'source'
  }
  if (value !== 'source' && value !== 'semantic') {
    throw new Error('Invalid incident bundle manifest: pattylog.representation must be source or semantic')
  }
  return value
}

function requiredLogTime(value: unknown, name: string): string {
  const result = requiredString(value, name)
  if (!Number.isFinite(Date.parse(result))) {
    throw new Error(`Invalid incident bundle manifest: ${name} must be an RFC3339 log-time`)
  }
  return result
}

function validateEndpoint(
  record: PattyLogRecord | undefined,
  interval: number,
  logTime: string,
  label: string,
) {
  if (!record || record.interval !== interval || record.logTime !== logTime) {
    throw new Error(
      `Incident bundle ${label} interval does not match manifest interval ${interval} at ${logTime}`,
    )
  }
}

function equalNumbers(left: number[], right: number[]): boolean {
  return left.length === right.length && left.every((value, index) => value === right[index])
}

function bundleError(error: unknown): Error {
  const message = errorMessage(error)
  return message.startsWith('Incident bundle') || message.startsWith('Invalid incident') ||
    message.startsWith('Unsupported incident') || message.startsWith('Unsupported PattyLog')
    ? new Error(message)
    : new Error(`Unable to open incident bundle: ${message}`)
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}
