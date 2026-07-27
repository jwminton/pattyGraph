/// <reference lib="webworker" />

import { IncrementalJsonlParser, mergeParseBatches } from '../domain/jsonl'
import type { ParserWorkerRequest, ParserWorkerResponse } from './protocol'

const workerScope = self as DedicatedWorkerGlobalScope
const parser = new IncrementalJsonlParser()
let decoder = new TextDecoder()

workerScope.onmessage = (event: MessageEvent<ParserWorkerRequest>) => {
  const request = event.data
  try {
    if (request.type === 'reset') {
      parser.reset()
      decoder = new TextDecoder()
      post({ type: 'reset', requestId: request.requestId })
      return
    }

    const text = decoder.decode(request.buffer, { stream: !request.finalize })
    let batch = parser.feed(text)
    if (request.finalize) {
      batch = mergeParseBatches(batch, parser.finish())
    }
    post({
      type: 'parsed',
      requestId: request.requestId,
      records: batch.records,
      issues: batch.issues,
    })
  } catch (error) {
    post({
      type: 'error',
      requestId: request.requestId,
      message: error instanceof Error ? error.message : 'JSONL worker failed',
    })
  }
}

function post(response: ParserWorkerResponse): void {
  workerScope.postMessage(response)
}
