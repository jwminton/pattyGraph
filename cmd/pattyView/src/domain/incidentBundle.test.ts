import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { Uint8ArrayReader, Uint8ArrayWriter, ZipWriter, type Entry } from '@zip.js/zip.js'
import { describe, expect, it } from 'vitest'
import { parseJsonlText } from './jsonl'
import {
  incidentBundleSchema,
  incidentBundleType,
  incidentManifestEntry,
  incidentPattyLogEntry,
  openIncidentBundle,
  parseIncidentBundleManifest,
  selectIncidentBundleFile,
  validateIncidentBundleRecords,
  type IncidentBundleManifest,
} from './incidentBundle'

const bundleFixture = new Blob([new Uint8Array(readFileSync(
  resolve(process.cwd(), 'tests/fixtures/schema4.incident.zip'),
))])

describe('incident bundle reader', () => {
  it('streams and validates a bundle created by the Go command', async () => {
    const bundle = await openIncidentBundle(bundleFixture)
    const stream = new TransformStream<Uint8Array, Uint8Array>()
    const text = new Response(stream.readable).text()

    await bundle.streamPattyLog(stream.writable, new AbortController().signal)
    const parsed = parseJsonlText(await text)
    validateIncidentBundleRecords(bundle.manifest, parsed)
    await bundle.close()

    expect(bundle.manifest.bundle_type).toBe(incidentBundleType)
    expect(bundle.manifest.bundle_schema).toBe(incidentBundleSchema)
    expect(bundle.manifest.pattylog.session_id).toBe('test-session')
    expect(bundle.manifest.range.interval_count).toBe(2)
    expect(parsed.records).toHaveLength(6)
    expect(parsed.issues).toHaveLength(1)
  })

  it('accepts future archive entries without extracting them', async () => {
    const bundle = await createBundle([
      { name: incidentManifestEntry, text: JSON.stringify(manifest()) },
      { name: incidentPattyLogEntry, text: pattyLog() },
      { name: 'access.log', text: 'future source evidence' },
    ])
    const opened = await openIncidentBundle(bundle)

    expect(opened.manifest.pattylog.entry).toBe(incidentPattyLogEntry)
    await opened.close()
  })

  it('cancels bundle extraction before publishing its PattyLog', async () => {
    const opened = await openIncidentBundle(bundleFixture)
    const controller = new AbortController()
    controller.abort()

    await expect(opened.streamPattyLog(
      new TransformStream<Uint8Array, Uint8Array>().writable,
      controller.signal,
    )).rejects.toThrow()
    await opened.close()
  })

  it('rejects missing, duplicate, and encrypted required entries', async () => {
    const missing = await createBundle([
      { name: incidentManifestEntry, text: JSON.stringify(manifest()) },
    ])
    await expect(openIncidentBundle(missing)).rejects.toThrow('exactly one pattyLog.jsonl')

    const duplicate = fakeEntry(incidentPattyLogEntry)
    expect(() => selectIncidentBundleFile(
      [duplicate, duplicate],
      incidentPattyLogEntry,
    )).toThrow('found 2')

    const encrypted = await createBundle([
      { name: incidentManifestEntry, text: JSON.stringify(manifest()) },
      { name: incidentPattyLogEntry, text: pattyLog(), password: 'secret' },
    ])
    await expect(openIncidentBundle(encrypted)).rejects.toThrow('encrypted')
  })
})

describe('incident bundle manifest', () => {
  it('defaults legacy manifests to source representation', () => {
    const legacy = manifest()
    delete (legacy.pattylog as Partial<IncidentBundleManifest['pattylog']>).representation
    expect(parseIncidentBundleManifest(JSON.stringify(legacy)).pattylog.representation).toBe('source')
    expect(() => parseIncidentBundleManifest(JSON.stringify({
      ...manifest(),
      pattylog: { ...manifest().pattylog, representation: 'other' },
    }))).toThrow('representation must be source or semantic')
  })

  it('rejects unsupported schemas, types, and PattyLog entry names', () => {
    expect(() => parseIncidentBundleManifest(JSON.stringify({
      ...manifest(),
      bundle_schema: 2,
    }))).toThrow('Unsupported incident bundle schema 2')
    expect(() => parseIncidentBundleManifest(JSON.stringify({
      ...manifest(),
      bundle_type: 'other',
    }))).toThrow('Unsupported incident bundle type')
    expect(() => parseIncidentBundleManifest(JSON.stringify({
      ...manifest(),
      pattylog: { ...manifest().pattylog, entry: '../pattyLog.jsonl' },
    }))).toThrow('Unsupported PattyLog bundle entry')
  })

  it('rejects a parsed corpus that disagrees with the manifest', () => {
    const parsed = parseJsonlText(pattyLog())
    expect(() => validateIncidentBundleRecords({
      ...manifest(),
      pattylog: { ...manifest().pattylog, session_id: 'other' },
    }, parsed)).toThrow('do not match session')
    expect(() => validateIncidentBundleRecords({
      ...manifest(),
      range: { ...manifest().range, interval_count: 2 },
    }, parsed)).toThrow('manifest declares 2')
    expect(() => validateIncidentBundleRecords({
      ...manifest(),
      pattylog: { ...manifest().pattylog, record_count: 3 },
    }, parsed)).toThrow('manifest declares 3')
  })
})

function manifest(): IncidentBundleManifest {
  return {
    bundle_schema: 1,
    bundle_type: incidentBundleType,
    creator: { name: 'pattyView', version: '0.1.8' },
    pattylog: {
      entry: incidentPattyLogEntry,
      representation: 'source',
      source_name: 'source.jsonl',
      session_id: 'bundle-test',
      schema_versions: [4],
      record_count: 2,
      retained_source_intervals: 0,
    },
    range: {
      from_log_time: '2026-08-09T08:01:00-07:00',
      through_log_time: '2026-08-09T08:01:00-07:00',
      from_interval: 4,
      through_interval: 4,
      interval_count: 1,
    },
  }
}

function pattyLog(): string {
  return [
    JSON.stringify({
      schema_version: 4,
      event_type: 'session_start',
      session_id: 'bundle-test',
      log_time: '1970-01-01T00:00:00Z',
    }),
    JSON.stringify({
      schema_version: 4,
      event_type: 'interval',
      session_id: 'bundle-test',
      log_time: '2026-08-09T08:01:00-07:00',
      interval: 4,
    }),
  ].join('\n') + '\n'
}

function fakeEntry(filename: string): Entry {
  return { filename, directory: false, encrypted: false } as unknown as Entry
}

async function createBundle(entries: Array<{ name: string; text: string; password?: string }>): Promise<Blob> {
  const output = new Uint8ArrayWriter()
  const archive = new ZipWriter(output, { useWebWorkers: false })
  for (const entry of entries) {
    await archive.add(
      entry.name,
      new Uint8ArrayReader(new TextEncoder().encode(entry.text)),
      entry.password ? { password: entry.password } : undefined,
    )
  }
  return new Blob([await archive.close()])
}
