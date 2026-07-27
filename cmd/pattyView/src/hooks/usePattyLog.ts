import { useCallback, useEffect, useRef, useState } from 'preact/hooks'
import { classifyFileChange, snapshotFile, type FileSnapshot } from '../domain/fileChange'
import {
  ParsePublicationBuffer,
  parsePublicationMode,
  shouldPublishProgress,
} from '../domain/parsePublication'
import type { ParseBatch, ParseIssue, PattyLogRecord } from '../domain/types'
import { ParserWorkerClient } from '../workers/parserClient'

export interface PattyFileHandle {
  name: string
  getFile(): Promise<File>
}

interface FilePickerWindow extends Window {
  showOpenFilePicker?: (options?: {
    multiple?: boolean
    types?: Array<{
      description: string
      accept: Record<string, string[]>
    }>
  }) => Promise<PattyFileHandle[]>
}

export type SourceMode = 'idle' | 'snapshot' | 'live'
export type LoadStatus = 'idle' | 'loading' | 'ready' | 'error'

export interface PattyLogSourceState {
  records: PattyLogRecord[]
  issues: ParseIssue[]
  fileName: string
  mode: SourceMode
  status: LoadStatus
  error: string
  bytesRead: number
  totalBytes: number
  lastUpdated: number | null
}

const initialState: PattyLogSourceState = {
  records: [],
  issues: [],
  fileName: '',
  mode: 'idle',
  status: 'idle',
  error: '',
  bytesRead: 0,
  totalBytes: 0,
  lastUpdated: null,
}

export function usePattyLog() {
  const [state, setState] = useState<PattyLogSourceState>(initialState)
  const parserRef = useRef<ParserWorkerClient | null>(null)
  const handleRef = useRef<PattyFileHandle | null>(null)
  const offsetRef = useRef(0)
  const snapshotRef = useRef<FileSnapshot | null>(null)
  const timerRef = useRef<number | null>(null)
  const pollingRef = useRef(false)
  const generationRef = useRef(0)

  const supportsLiveFile = typeof (window as FilePickerWindow).showOpenFilePicker === 'function'

  const stopPolling = useCallback(() => {
    if (timerRef.current !== null) {
      window.clearInterval(timerRef.current)
      timerRef.current = null
    }
    handleRef.current = null
  }, [])

  const replaceParser = useCallback(() => {
    parserRef.current?.terminate()
    parserRef.current = new ParserWorkerClient()
    return parserRef.current
  }, [])

  const appendBatch = useCallback((records: PattyLogRecord[], issues: ParseIssue[]) => {
    if (records.length === 0 && issues.length === 0) {
      return
    }
    setState((current) => ({
      ...current,
      records: records.length > 0 ? [...current.records, ...records] : current.records,
      issues: issues.length > 0 ? [...current.issues, ...issues] : current.issues,
      lastUpdated: Date.now(),
    }))
  }, [])

  const parseFile = useCallback(async (
    parser: ParserWorkerClient,
    file: File,
    finalize: boolean,
    generation: number,
  ): Promise<ParseBatch | null> => {
    const mode = parsePublicationMode(file.size)
    const publications = new ParsePublicationBuffer(mode)
    const coarseProgress = mode !== 'immediate'
    let lastProgressBytes = 0

    await parser.parseBlob(
      file,
      finalize,
      (batch) => {
        if (generationRef.current !== generation) {
          return
        }
        for (const publication of publications.append(batch)) {
          appendBatch(publication.records, publication.issues)
        }
      },
      (bytesRead, totalBytes) => {
        if (
          generationRef.current === generation &&
          shouldPublishProgress(coarseProgress, bytesRead, totalBytes, lastProgressBytes)
        ) {
          lastProgressBytes = bytesRead
          setState((current) => ({ ...current, bytesRead, totalBytes }))
        }
      },
    )
    if (generationRef.current !== generation) {
      return null
    }
    const finalPublications = publications.finish()
    if (mode === 'bulk') {
      return finalPublications[0] ?? null
    }
    for (const publication of finalPublications) {
      appendBatch(publication.records, publication.issues)
    }
    return null
  }, [appendBatch])

  const resetForFile = useCallback(async (
    file: File,
    mode: SourceMode,
    finalize: boolean,
  ) => {
    const generation = ++generationRef.current
    const parser = replaceParser()
    await parser.reset()
    setState({
      ...initialState,
      fileName: file.name,
      mode,
      status: 'loading',
      totalBytes: file.size,
    })
    const bulkPublication = await parseFile(parser, file, finalize, generation)
    if (generationRef.current !== generation) {
      return
    }
    offsetRef.current = file.size
    const snapshot = await snapshotFile(file)
    if (generationRef.current !== generation) {
      return
    }
    snapshotRef.current = snapshot
    setState((current) => ({
      ...current,
      records: bulkPublication && bulkPublication.records.length > 0
        ? [...current.records, ...bulkPublication.records]
        : current.records,
      issues: bulkPublication && bulkPublication.issues.length > 0
        ? [...current.issues, ...bulkPublication.issues]
        : current.issues,
      status: 'ready',
      bytesRead: file.size,
      totalBytes: file.size,
      lastUpdated: Date.now(),
    }))
  }, [parseFile, replaceParser])

  const reportError = useCallback((error: unknown) => {
    if (error instanceof DOMException && error.name === 'AbortError') {
      return
    }
    const message = error instanceof Error ? error.message : 'Unable to read PattyLog'
    setState((current) => ({ ...current, status: 'error', error: message }))
  }, [])

  const poll = useCallback(async () => {
    const handle = handleRef.current
    const previous = snapshotRef.current
    const parser = parserRef.current
    if (!handle || !previous || !parser || pollingRef.current) {
      return
    }

    pollingRef.current = true
    try {
      const file = await handle.getFile()
      const next = await snapshotFile(file)
      const change = classifyFileChange(previous, next, offsetRef.current)
      if (change === 'reset') {
        await resetForFile(file, 'live', false)
        handleRef.current = handle
      } else if (change === 'append') {
        const generation = generationRef.current
        const start = offsetRef.current
        const appendedRecords: PattyLogRecord[] = []
        const appendedIssues: ParseIssue[] = []
        await parser.parseBlob(file.slice(start), false, (batch) => {
          if (generationRef.current === generation) {
            appendedRecords.push(...batch.records)
            appendedIssues.push(...batch.issues)
          }
        })
        if (generationRef.current === generation) {
          offsetRef.current = file.size
          snapshotRef.current = next
          setState((current) => ({
            ...current,
            records: appendedRecords.length > 0
              ? [...current.records, ...appendedRecords]
              : current.records,
            issues: appendedIssues.length > 0
              ? [...current.issues, ...appendedIssues]
              : current.issues,
            bytesRead: file.size,
            totalBytes: file.size,
            lastUpdated: Date.now(),
          }))
        }
      } else {
        snapshotRef.current = next
      }
    } catch (error) {
      reportError(error)
    } finally {
      pollingRef.current = false
    }
  }, [reportError, resetForFile])

  const openSnapshot = useCallback(async (file: File) => {
    stopPolling()
    try {
      await resetForFile(file, 'snapshot', true)
    } catch (error) {
      reportError(error)
    }
  }, [reportError, resetForFile, stopPolling])

  const openLive = useCallback(async () => {
    const picker = (window as FilePickerWindow).showOpenFilePicker
    if (!picker) {
      throw new Error('Live file following is unavailable in this browser')
    }
    try {
      const [handle] = await picker({
        multiple: false,
        types: [{
          description: 'PattyLog JSONL',
          accept: { 'application/x-ndjson': ['.jsonl'] },
        }],
      })
      if (!handle) {
        return
      }
      stopPolling()
      const file = await handle.getFile()
      handleRef.current = handle
      await resetForFile(file, 'live', false)
      handleRef.current = handle
      timerRef.current = window.setInterval(() => void poll(), 1000)
    } catch (error) {
      reportError(error)
    }
  }, [poll, reportError, resetForFile, stopPolling])

  useEffect(() => () => {
    stopPolling()
    parserRef.current?.terminate()
  }, [stopPolling])

  return {
    state,
    supportsLiveFile,
    openLive,
    openSnapshot,
  }
}
