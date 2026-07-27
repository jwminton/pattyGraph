import type { ParseBatch, ParseIssue, PattyLogRecord } from './types'

export const coarseFileThreshold = 16 * 1024 * 1024
export const bulkFileThreshold = 128 * 1024 * 1024
export const coarseRecordBatchSize = 256
export const coarseProgressStep = 8 * 1024 * 1024

export type ParsePublicationMode = 'immediate' | 'coarse' | 'bulk'

export function parsePublicationMode(fileSize: number): ParsePublicationMode {
  if (fileSize > bulkFileThreshold) {
    return 'bulk'
  }
  return fileSize > coarseFileThreshold ? 'coarse' : 'immediate'
}

export function shouldPublishProgress(
  coarse: boolean,
  bytesRead: number,
  totalBytes: number,
  lastPublishedBytes: number,
): boolean {
  return !coarse || bytesRead >= totalBytes || bytesRead - lastPublishedBytes >= coarseProgressStep
}

// Worker batches stay small for responsive parsing. This buffer controls how
// often those batches become application state and trigger whole-model work.
export class ParsePublicationBuffer {
  private readonly records: PattyLogRecord[] = []
  private readonly issues: ParseIssue[] = []

  constructor(private readonly mode: ParsePublicationMode) {}

  append(batch: ParseBatch): ParseBatch[] {
    if (this.mode === 'immediate') {
      return batch.records.length === 0 && batch.issues.length === 0 ? [] : [batch]
    }

    this.records.push(...batch.records)
    this.issues.push(...batch.issues)
    if (this.mode === 'bulk') {
      return []
    }
    const publications: ParseBatch[] = []
    while (this.records.length >= coarseRecordBatchSize) {
      publications.push({
        records: this.records.splice(0, coarseRecordBatchSize),
        issues: this.drainIssues(),
      })
    }
    return publications
  }

  finish(): ParseBatch[] {
    if (this.records.length === 0 && this.issues.length === 0) {
      return []
    }
    return [{
      records: this.records.splice(0),
      issues: this.drainIssues(),
    }]
  }

  private drainIssues(): ParseIssue[] {
    return this.issues.splice(0)
  }
}
