import { Check, Copy, Download, LoaderCircle, X } from 'lucide-preact'
import { useEffect, useState } from 'preact/hooks'
import type { IncidentRangeCommand, IncidentRangeSelection } from '../domain/incidentRange'
import { saveSemanticIncident } from '../domain/semanticIncidentDownload'
import type { SemanticIncidentPlan } from '../domain/semanticIncidentBundle'

type CopyState = 'idle' | 'copied' | 'failed'
type DownloadState = 'idle' | 'saving' | 'saved' | 'failed'

export function IncidentRangeBar({
  selection,
  command,
  semanticPlan,
  onClear,
}: {
  selection: IncidentRangeSelection
  command: IncidentRangeCommand | null
  semanticPlan: SemanticIncidentPlan
  onClear: () => void
}) {
  const [copyState, setCopyState] = useState<CopyState>('idle')
  const [downloadState, setDownloadState] = useState<DownloadState>('idle')
  const [downloadError, setDownloadError] = useState('')

  useEffect(() => {
    setCopyState('idle')
    setDownloadState('idle')
    setDownloadError('')
  }, [command?.command, command?.error, semanticPlan])

  useEffect(() => {
    if (copyState !== 'copied') {
      return
    }
    const timer = window.setTimeout(() => setCopyState('idle'), 1800)
    return () => window.clearTimeout(timer)
  }, [copyState])

  const copyCommand = async () => {
    try {
      if (!navigator.clipboard || !command || command.command === '') {
        throw new Error('clipboard unavailable')
      }
      await navigator.clipboard.writeText(command.command)
      setCopyState('copied')
    } catch {
      setCopyState('failed')
    }
  }
  const copyLabel = copyState === 'copied'
    ? 'Bundle command copied'
    : copyState === 'failed' ? 'Copy failed; retry' : 'Copy bundle command'
  const downloadIncident = async () => {
    setDownloadState('saving')
    setDownloadError('')
    try {
      const result = await saveSemanticIncident(semanticPlan)
      setDownloadState(result === 'cancelled' ? 'idle' : 'saved')
    } catch (error) {
      setDownloadState('failed')
      setDownloadError(error instanceof Error ? error.message : 'Unable to download incident')
    }
  }

  return (
    <section
      class={`incident-range-bar${command ? '' : ' download-only'}`}
      aria-label="Investigation range"
    >
      <div class="incident-range-summary">
        <strong>Investigation range</strong>
        <span>
          <time dateTime={selection.from.logTime}>{formatEndpoint(selection.from.logTime)}</time>
          {' through '}
          <time dateTime={selection.through.logTime}>{formatEndpoint(selection.through.logTime)}</time>
        </span>
        <span>{selection.intervalCount} intervals</span>
      </div>
      {command ? command.error ? (
        <span class="incident-range-error" role="alert">{command.error}</span>
      ) : (
        <code title={command.command}>{command.command}</code>
      ) : null}
      <div class="incident-range-actions">
        <button
          class="primary-button incident-download-button"
          type="button"
          title={downloadError || 'Download semantic incident bundle'}
          disabled={downloadState === 'saving'}
          onClick={() => void downloadIncident()}
        >
          {downloadState === 'saving'
            ? <LoaderCircle class="incident-download-spinner" size={16} aria-hidden="true" />
            : downloadState === 'saved'
              ? <Check size={16} aria-hidden="true" />
              : <Download size={16} aria-hidden="true" />}
          {downloadState === 'saving'
            ? 'Creating...'
            : downloadState === 'saved' ? 'Downloaded' : downloadState === 'failed' ? 'Retry download' : 'Download incident'}
        </button>
        {downloadError ? <span class="incident-download-error" role="alert">Download failed</span> : null}
        {command ? (
          <button
            class="icon-button"
            type="button"
            title={copyLabel}
            aria-label={copyLabel}
            disabled={command.command === ''}
            onClick={() => void copyCommand()}
          >
            {copyState === 'copied' ? <Check size={16} aria-hidden="true" /> : <Copy size={16} aria-hidden="true" />}
          </button>
        ) : null}
        <button
          class="icon-button"
          type="button"
          title="Clear investigation range"
          aria-label="Clear investigation range"
          onClick={onClear}
        >
          <X size={16} aria-hidden="true" />
        </button>
      </div>
      <span class="visually-hidden" aria-live="polite">
        {downloadError || (copyState === 'copied'
          ? 'Bundle command copied'
          : copyState === 'failed' ? 'Copy failed' : '')}
      </span>
    </section>
  )
}

function formatEndpoint(value: string): string {
  const parsed = new Date(value)
  if (!Number.isFinite(parsed.getTime())) {
    return value || 'unknown log-time'
  }
  return new Intl.DateTimeFormat(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(parsed)
}
