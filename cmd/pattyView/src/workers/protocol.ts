import type { ParseIssue, PattyLogRecord } from '../domain/types'

export type ParserWorkerRequest =
  | { type: 'reset'; requestId: number }
  | {
      type: 'parse'
      requestId: number
      buffer: ArrayBuffer
      finalize: boolean
    }

export type ParserWorkerRequestBody =
  | { type: 'reset' }
  | { type: 'parse'; buffer: ArrayBuffer; finalize: boolean }

export type ParserWorkerResponse =
  | { type: 'reset'; requestId: number }
  | {
      type: 'parsed'
      requestId: number
      records: PattyLogRecord[]
      issues: ParseIssue[]
    }
  | { type: 'error'; requestId: number; message: string }
