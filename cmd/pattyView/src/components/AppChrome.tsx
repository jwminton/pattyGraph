import { Activity, AlertTriangle, FileJson, FolderOpen, Radio } from 'lucide-preact'

export function Brand() {
  const colors = ['#ff6b6b', '#f0b35a', '#e8d34f', '#4ecb71', '#4ecdc4']
  return (
    <div class="brand" aria-label="pattyView">
      {'patty'.split('').map((letter, index) => (
        <span key={`${letter}-${index}`} style={{ color: colors[index] }}>{letter}</span>
      ))}
      <span class="brand-view">View</span>
    </div>
  )
}

export function SourceStatus({ mode, status }: { mode: string; status: string }) {
  if (status === 'idle') {
    return null
  }
  return (
    <div class={`source-status ${status}`}>
      {mode === 'live' ? <Radio size={14} aria-hidden="true" /> : <FileJson size={14} aria-hidden="true" />}
      <span>{status === 'loading' ? 'Reading' : mode === 'live' ? 'Following' : 'Snapshot'}</span>
    </div>
  )
}

export function EmptyState({
  supportsLiveFile,
  onOpen,
  onSnapshot,
}: {
  supportsLiveFile: boolean
  onOpen: () => void
  onSnapshot: () => void
}) {
  return (
    <main class="empty-state">
      <FileJson size={42} strokeWidth={1.4} aria-hidden="true" />
      <h1>Open a PattyLog</h1>
      <p>Explore the traffic model recorded by PattyGraph. The file stays in this browser.</p>
      <div class="empty-actions">
        <button class="primary-button" type="button" onClick={onOpen}>
          <FolderOpen size={18} aria-hidden="true" />
          {supportsLiveFile ? 'Open and follow' : 'Open file'}
        </button>
        {supportsLiveFile ? (
          <button class="secondary-button" type="button" onClick={onSnapshot}>
            Open snapshot
          </button>
        ) : null}
      </div>
      <span class="drop-hint">or drop a .jsonl snapshot here</span>
      <span class="viewer-version">pattyView {__PATTY_VIEW_VERSION__}</span>
    </main>
  )
}

export function LoadingState({ fileName, bytesRead, totalBytes }: {
  fileName: string
  bytesRead: number
  totalBytes: number
}) {
  const percent = totalBytes > 0 ? Math.round((bytesRead / totalBytes) * 100) : 0
  return (
    <main class="loading-state">
      <Activity class="loading-icon" size={34} aria-hidden="true" />
      <strong>Reading {fileName}</strong>
      <span>{percent}%</span>
      <div class="progress-track" aria-label={`${percent}% parsed`}>
        <div style={{ width: `${percent}%` }} />
      </div>
    </main>
  )
}

export function IssueBanner({ issues }: { issues: Array<{ lineNumber: number; message: string; rawPreview: string }> }) {
  return (
    <details class="issue-banner">
      <summary>
        <AlertTriangle size={16} aria-hidden="true" />
        {issues.length} malformed {issues.length === 1 ? 'line' : 'lines'} preserved as diagnostics
      </summary>
      <div class="issue-list">
        {issues.map((issue) => (
          <div key={issue.lineNumber}>
            <strong>Line {issue.lineNumber}: {issue.message}</strong>
            <code>{issue.rawPreview}</code>
          </div>
        ))}
      </div>
    </details>
  )
}
