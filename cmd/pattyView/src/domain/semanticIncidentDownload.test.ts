import { describe, expect, it, vi } from 'vitest'
import { openIncidentBundle } from './incidentBundle'
import { saveSemanticIncident } from './semanticIncidentDownload'
import type { SemanticIncidentPlan } from './semanticIncidentBundle'

describe('semantic incident download', () => {
  it('streams directly into a save-file writable', async () => {
    const chunks: ArrayBuffer[] = []
    const writable = new WritableStream<Uint8Array>({
      write(chunk) {
        chunks.push(chunk.slice().buffer as ArrayBuffer)
      },
    })
    const picker = vi.fn(async () => ({ createWritable: async () => writable }))
    const result = await saveSemanticIncident(plan(), { showSaveFilePicker: picker } as unknown as Window)

    expect(result).toBe('saved')
    expect(picker).toHaveBeenCalledWith(expect.objectContaining({ suggestedName: 'incident.incident.zip' }))
    const opened = await openIncidentBundle(new Blob(chunks))
    expect(opened.manifest.pattylog.representation).toBe('semantic')
    await opened.close()
  })

  it('treats save-picker cancellation as a non-error', async () => {
    const picker = vi.fn(async () => {
      throw new DOMException('cancelled', 'AbortError')
    })
    await expect(saveSemanticIncident(
      plan(),
      { showSaveFilePicker: picker } as unknown as Window,
    )).resolves.toBe('cancelled')
  })

  it('falls back to a browser Blob download', async () => {
    const anchor = {
      href: '',
      download: '',
      style: { display: '' },
      click: vi.fn(),
      remove: vi.fn(),
    }
    const append = vi.fn()
    const createObjectURL = vi.fn((_blob: Blob) => 'blob:incident')
    const revokeObjectURL = vi.fn()
    const scope = {
      URL: { createObjectURL, revokeObjectURL },
      setTimeout: (callback: () => void) => {
        callback()
        return 1
      },
    } as unknown as Window
    const documentScope = {
      createElement: vi.fn(() => anchor),
      body: { append },
    } as unknown as Document

    await expect(saveSemanticIncident(plan(), scope, documentScope)).resolves.toBe('downloaded')
    expect(anchor.download).toBe('incident.incident.zip')
    expect(anchor.click).toHaveBeenCalledOnce()
    expect(anchor.remove).toHaveBeenCalledOnce()
    expect(append).toHaveBeenCalledWith(anchor)
    const downloadedBlob = createObjectURL.mock.calls[0]?.[0] as Blob
    expect(downloadedBlob.type).toBe('application/zip')
    expect(downloadedBlob.size).toBeGreaterThan(0)
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:incident')
  })

  it('surfaces writable failures', async () => {
    const writable = new WritableStream<Uint8Array>({
      write() {
        throw new Error('disk full')
      },
    })
    const scope = {
      showSaveFilePicker: async () => ({ createWritable: async () => writable }),
    } as unknown as Window
    await expect(saveSemanticIncident(plan(), scope)).rejects.toThrow('disk full')
  })
})

function plan(): SemanticIncidentPlan {
  return {
    suggestedName: 'incident.incident.zip',
    records: [{
      id: 'record-1',
      lineNumber: 1,
      fileOrder: 1,
      schemaVersion: 4,
      eventType: 'session_start',
      sessionId: 'download-test',
      timestamp: '2026-08-09T08:00:00Z',
      logTime: '2026-08-09T08:00:00Z',
      interval: null,
      data: {
        schema_version: 4,
        event_type: 'session_start',
        session_id: 'download-test',
        log_time: '2026-08-09T08:00:00Z',
      },
    }, {
      id: 'record-2',
      lineNumber: 2,
      fileOrder: 2,
      schemaVersion: 4,
      eventType: 'interval',
      sessionId: 'download-test',
      timestamp: '2026-08-09T08:01:00Z',
      logTime: '2026-08-09T08:01:00Z',
      interval: 0,
      data: {
        schema_version: 4,
        event_type: 'interval',
        session_id: 'download-test',
        log_time: '2026-08-09T08:01:00Z',
        interval: 0,
      },
    }],
    manifest: {
      bundle_schema: 1,
      bundle_type: 'pattygraph_incident',
      creator: { name: 'pattyView', version: '0.1.8' },
      pattylog: {
        entry: 'pattyLog.jsonl',
        representation: 'semantic',
        source_name: 'source.jsonl',
        session_id: 'download-test',
        schema_versions: [4],
        record_count: 2,
        retained_source_intervals: 0,
      },
      range: {
        from_log_time: '2026-08-09T08:01:00Z',
        through_log_time: '2026-08-09T08:01:00Z',
        from_interval: 0,
        through_interval: 0,
        interval_count: 1,
      },
    },
  }
}
