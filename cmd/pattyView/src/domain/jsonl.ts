import {
  type JsonObject,
  type ParseBatch,
  type ParseIssue,
  type PattyLogRecord,
  isJsonObject,
  readNumber,
  readString,
} from './types'

const rawPreviewLimit = 240

export class IncrementalJsonlParser {
  private pending = ''
  private lineNumber = 0
  private fileOrder = 0

  reset(): void {
    this.pending = ''
    this.lineNumber = 0
    this.fileOrder = 0
  }

  feed(text: string): ParseBatch {
    this.pending += text
    const records: PattyLogRecord[] = []
    const issues: ParseIssue[] = []
    let newline = this.pending.indexOf('\n')

    while (newline >= 0) {
      let line = this.pending.slice(0, newline)
      this.pending = this.pending.slice(newline + 1)
      if (line.endsWith('\r')) {
        line = line.slice(0, -1)
      }
      this.parseLine(line, records, issues)
      newline = this.pending.indexOf('\n')
    }

    return { records, issues }
  }

  finish(): ParseBatch {
    if (this.pending === '') {
      return { records: [], issues: [] }
    }

    let line = this.pending
    this.pending = ''
    if (line.endsWith('\r')) {
      line = line.slice(0, -1)
    }
    const records: PattyLogRecord[] = []
    const issues: ParseIssue[] = []
    this.parseLine(line, records, issues)
    return { records, issues }
  }

  private parseLine(
    line: string,
    records: PattyLogRecord[],
    issues: ParseIssue[],
  ): void {
    this.lineNumber += 1
    if (line.trim() === '') {
      return
    }

    this.fileOrder += 1
    try {
      const parsed: unknown = JSON.parse(line)
      if (!isJsonObject(parsed)) {
        throw new Error('record is not a JSON object')
      }
      records.push(this.toRecord(parsed))
    } catch (error) {
      const message = error instanceof Error ? error.message : 'invalid JSON record'
      const preview = line.length > rawPreviewLimit
        ? `${line.slice(0, rawPreviewLimit)}...`
        : line
      issues.push({
        id: `issue-${this.lineNumber}`,
        lineNumber: this.lineNumber,
        message,
        rawPreview: preview,
      })
    }
  }

  private toRecord(data: JsonObject): PattyLogRecord {
    const schemaVersion = readNumber(data, 'schema_version')
    const eventType = readString(data, 'event_type') || 'unknown'
    const sessionId = readString(data, 'session_id') || 'unscoped'
    const timestamp = readString(data, 'timestamp')
    const logTime = readString(data, 'log_time') || timestamp
    const interval = readNumber(data, 'interval')

    return {
      id: `record-${this.fileOrder}`,
      lineNumber: this.lineNumber,
      fileOrder: this.fileOrder,
      schemaVersion,
      eventType,
      sessionId,
      timestamp,
      logTime,
      interval,
      data,
    }
  }
}

export function mergeParseBatches(first: ParseBatch, second: ParseBatch): ParseBatch {
  return {
    records: [...first.records, ...second.records],
    issues: [...first.issues, ...second.issues],
  }
}

export function parseJsonlText(text: string): ParseBatch {
  const parser = new IncrementalJsonlParser()
  return mergeParseBatches(parser.feed(text), parser.finish())
}

export function asJsonObject(value: unknown): JsonObject | null {
  return isJsonObject(value) ? value : null
}
