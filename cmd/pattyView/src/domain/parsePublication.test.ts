import { describe, expect, it } from 'vitest'
import {
  ParsePublicationBuffer,
  bulkFileThreshold,
  coarseFileThreshold,
  coarseProgressStep,
  coarseRecordBatchSize,
  parsePublicationMode,
  shouldPublishProgress,
} from './parsePublication'
import type { ParseIssue, PattyLogRecord } from './types'

describe('parse publication policy', () => {
  it('preserves immediate publication through the 16 MiB boundary', () => {
    expect(parsePublicationMode(coarseFileThreshold)).toBe('immediate')
    expect(parsePublicationMode(coarseFileThreshold + 1)).toBe('coarse')
    expect(parsePublicationMode(bulkFileThreshold)).toBe('coarse')
    expect(parsePublicationMode(bulkFileThreshold + 1)).toBe('bulk')

    const buffer = new ParsePublicationBuffer('immediate')
    const batch = { records: [record(1)], issues: [issue(1)] }
    expect(buffer.append(batch)).toEqual([batch])
    expect(buffer.finish()).toEqual([])
  })

  it('publishes large loads in ordered 256-record batches and flushes the remainder', () => {
    const buffer = new ParsePublicationBuffer('coarse')
    const values = Array.from({ length: 600 }, (_, index) => record(index + 1))
    const publications = [
      ...buffer.append({ records: values.slice(0, 180), issues: [issue(4)] }),
      ...buffer.append({ records: values.slice(180, 410), issues: [] }),
      ...buffer.append({ records: values.slice(410), issues: [issue(519)] }),
      ...buffer.finish(),
    ]

    expect(publications.map((batch) => batch.records.length)).toEqual([
      coarseRecordBatchSize,
      coarseRecordBatchSize,
      600 - 2 * coarseRecordBatchSize,
    ])
    expect(publications.flatMap((batch) => batch.records).map((value) => value.fileOrder))
      .toEqual(values.map((value) => value.fileOrder))
    expect(publications.flatMap((batch) => batch.issues).map((value) => value.lineNumber))
      .toEqual([4, 519])
  })

  it('flushes issue-only input at completion', () => {
    const buffer = new ParsePublicationBuffer('coarse')
    expect(buffer.append({ records: [], issues: [issue(7)] })).toEqual([])
    expect(buffer.finish()).toEqual([{ records: [], issues: [issue(7)] }])
  })

  it('throttles coarse progress while preserving completion', () => {
    expect(shouldPublishProgress(false, 1, 100, 0)).toBe(true)
    expect(shouldPublishProgress(true, coarseProgressStep - 1, 100_000_000, 0)).toBe(false)
    expect(shouldPublishProgress(true, coarseProgressStep, 100_000_000, 0)).toBe(true)
    expect(shouldPublishProgress(true, 100_000_000, 100_000_000, coarseProgressStep)).toBe(true)
  })

  it('holds bulk loads until one ordered final publication', () => {
    const buffer = new ParsePublicationBuffer('bulk')
    const values = Array.from({ length: 600 }, (_, index) => record(index + 1))

    expect(buffer.append({ records: values.slice(0, 300), issues: [issue(4)] })).toEqual([])
    expect(buffer.append({ records: values.slice(300), issues: [issue(519)] })).toEqual([])

    const publications = buffer.finish()
    expect(publications).toHaveLength(1)
    expect(publications[0].records.map((value) => value.fileOrder))
      .toEqual(values.map((value) => value.fileOrder))
    expect(publications[0].issues.map((value) => value.lineNumber)).toEqual([4, 519])
    expect(buffer.finish()).toEqual([])
  })
})

function record(order: number): PattyLogRecord {
  return {
    id: `record-${order}`,
    lineNumber: order,
    fileOrder: order,
    schemaVersion: 4,
    eventType: 'interval',
    sessionId: 'test',
    timestamp: '',
    logTime: '',
    interval: order - 1,
    data: {},
  }
}

function issue(lineNumber: number): ParseIssue {
  return {
    id: `issue-${lineNumber}`,
    lineNumber,
    message: 'test issue',
    rawPreview: '',
  }
}
