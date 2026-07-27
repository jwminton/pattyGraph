export type JsonPrimitive = string | number | boolean | null
export type JsonValue = JsonPrimitive | JsonValue[] | { [key: string]: JsonValue }
export type JsonObject = { [key: string]: JsonValue }

export type KnownEventType =
  | 'session_start'
  | 'interval'
  | 'control_command'
  | 'alert'

export interface PattyLogRecord {
  id: string
  lineNumber: number
  fileOrder: number
  schemaVersion: number | null
  eventType: string
  sessionId: string
  timestamp: string
  logTime: string
  interval: number | null
  data: JsonObject
}

export interface ParseIssue {
  id: string
  lineNumber: number
  message: string
  rawPreview: string
}

export interface ParseBatch {
  records: PattyLogRecord[]
  issues: ParseIssue[]
}

export interface SessionIndex {
  id: string
  records: PattyLogRecord[]
  intervals: PattyLogRecord[]
  alerts: PattyLogRecord[]
  sessionStart: PattyLogRecord | null
  schemaVersions: number[]
  firstTimestamp: string
  lastTimestamp: string
}

export function isJsonObject(value: unknown): value is JsonObject {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

export function readString(object: JsonObject, key: string): string {
  const value = object[key]
  return typeof value === 'string' ? value : ''
}

export function readNumber(object: JsonObject, key: string): number | null {
  const value = object[key]
  return typeof value === 'number' && Number.isFinite(value) ? value : null
}

export function readBoolean(object: JsonObject, key: string): boolean | null {
  const value = object[key]
  return typeof value === 'boolean' ? value : null
}

export function readObject(object: JsonObject, key: string): JsonObject | null {
  const value = object[key]
  return isJsonObject(value) ? value : null
}

export function readArray(object: JsonObject, key: string): JsonValue[] {
  const value = object[key]
  return Array.isArray(value) ? value : []
}

export function isKnownEventType(value: string): value is KnownEventType {
  return (
    value === 'session_start' ||
    value === 'interval' ||
    value === 'control_command' ||
    value === 'alert'
  )
}
