import type { ParseBatch } from '../domain/types'
import type {
  ParserWorkerRequest,
  ParserWorkerRequestBody,
  ParserWorkerResponse,
} from './protocol'

const chunkSize = 512 * 1024

interface PendingRequest {
  resolve: (batch: ParseBatch) => void
  reject: (error: Error) => void
}

export class ParserWorkerClient {
  private readonly worker: Worker
  private readonly pending = new Map<number, PendingRequest>()
  private nextRequestId = 1

  constructor() {
    this.worker = new Worker(new URL('./jsonl.worker.ts', import.meta.url), {
      type: 'module',
    })
    this.worker.onmessage = (event: MessageEvent<ParserWorkerResponse>) => {
      this.handleResponse(event.data)
    }
    this.worker.onerror = (event) => {
      this.rejectAll(new Error(event.message || 'JSONL worker failed'))
    }
  }

  async reset(): Promise<void> {
    await this.request({ type: 'reset' })
  }

  async parseBlob(
    blob: Blob,
    finalize: boolean,
    onBatch: (batch: ParseBatch) => void,
    onProgress?: (bytesRead: number, totalBytes: number) => void,
  ): Promise<void> {
    if (blob.size === 0) {
      if (finalize) {
        onBatch(await this.request({
          type: 'parse',
          buffer: new ArrayBuffer(0),
          finalize: true,
        }))
      }
      onProgress?.(0, 0)
      return
    }

    let offset = 0
    while (offset < blob.size) {
      const end = Math.min(offset + chunkSize, blob.size)
      const buffer = await blob.slice(offset, end).arrayBuffer()
      const batch = await this.request({
        type: 'parse',
        buffer,
        finalize: finalize && end === blob.size,
      }, [buffer])
      onBatch(batch)
      offset = end
      onProgress?.(offset, blob.size)
    }
  }

  terminate(): void {
    this.worker.terminate()
    this.rejectAll(new Error('JSONL worker terminated'))
  }

  private request(
    request: ParserWorkerRequestBody,
    transfer: Transferable[] = [],
  ): Promise<ParseBatch> {
    const requestId = this.nextRequestId++
    return new Promise((resolve, reject) => {
      this.pending.set(requestId, { resolve, reject })
      this.worker.postMessage({ ...request, requestId } as ParserWorkerRequest, transfer)
    })
  }

  private handleResponse(response: ParserWorkerResponse): void {
    const request = this.pending.get(response.requestId)
    if (!request) {
      return
    }
    this.pending.delete(response.requestId)
    if (response.type === 'error') {
      request.reject(new Error(response.message))
      return
    }
    if (response.type === 'reset') {
      request.resolve({ records: [], issues: [] })
      return
    }
    request.resolve({ records: response.records, issues: response.issues })
  }

  private rejectAll(error: Error): void {
    for (const request of this.pending.values()) {
      request.reject(error)
    }
    this.pending.clear()
  }
}
