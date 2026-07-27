import { ArrowDown, ArrowUp, Crosshair, Minus } from 'lucide-preact'
import { useMemo } from 'preact/hooks'
import { formatBytes, formatCount as formatNumber } from '../displayFormat'
import {
  projectIntervalComparison,
  type ComparisonSignal,
  type ComparisonValueKind,
  type IntervalContext,
  type MatcherComparison,
} from '../domain/intervalComparison'
import {
  type IPGroupEntry,
  type InterestingEntry,
  type InterestingStream,
} from '../domain/trafficDetail'
import type { PattyLogRecord } from '../domain/types'

const streamNames = ['words', 'refs', 'ips'] as const

export function IntervalComparisonDetail({
  selected,
  reference,
  intervals,
  sessionRecords,
  targeting,
  onReference,
  onToggleTargeting,
}: {
  selected: PattyLogRecord
  reference: PattyLogRecord
  intervals: PattyLogRecord[]
  sessionRecords: PattyLogRecord[]
  targeting: boolean
  onReference: (recordId: string) => void
  onToggleTargeting: () => void
}) {
  const comparison = useMemo(
    () => projectIntervalComparison(reference, selected, sessionRecords),
    [reference.id, selected.id, sessionRecords],
  )
  const referenceOptions = [...intervals]
    .filter((record) => record.schemaVersion === 4 && record.id !== selected.id)
    .reverse()

  return (
    <div class="comparison-pane">
      <section class="comparison-controls" aria-label="Interval comparison selection">
        <div class="comparison-side-label selected">
          <span>Selected interval</span>
          <strong>{formatIntervalTitle(selected)}</strong>
        </div>
        <div class="comparison-reference-control">
          <label>
            <span>Compare with</span>
            <select
              aria-label="Compare selected interval with"
              value={reference.id}
              onChange={(event) => onReference(event.currentTarget.value)}
            >
              {referenceOptions.map((record) => (
                <option value={record.id} key={record.id}>{formatIntervalOption(record)}</option>
              ))}
            </select>
          </label>
          <button
            class={targeting ? 'comparison-target-button active' : 'comparison-target-button'}
            type="button"
            aria-pressed={targeting}
            onClick={onToggleTargeting}
          >
            <Crosshair size={15} aria-hidden="true" />
            {targeting ? 'Choosing interval...' : 'Pick from map'}
          </button>
        </div>
        <div class="comparison-side-label comparison">
          <span>Comparison interval</span>
          <strong>{formatIntervalTitle(reference)}</strong>
        </div>
      </section>

      {comparison.peakResetPhases.length > 0 ? (
        <div class="comparison-reset-notice" role="note">
          Peak memory was reset within this comparison range. Peak-derived balance includes a rebuilt baseline.
        </div>
      ) : null}

      <section class="comparison-section">
        <div class="section-heading-row">
          <h2>Interval signals</h2>
          <span>Change from comparison interval</span>
        </div>
        <div class="data-table-wrap comparison-signal-table">
          <table aria-label="Interval signal comparison">
            <thead><tr><th>Signal</th><th>Selected</th><th>Comparison</th><th>Change</th></tr></thead>
            <tbody>
              {comparison.signals.map((signal) => (
                <SignalRow signal={signal} key={signal.key} />
              ))}
            </tbody>
          </table>
        </div>
      </section>

      <section class="comparison-section">
        <div class="section-heading-row">
          <h2>Matchers</h2>
          <span>Emitted order and current interval count</span>
        </div>
        <div class="data-table-wrap comparison-matcher-table">
          <table aria-label="Matcher comparison">
            <thead>
              <tr>
                <th>Matcher</th><th>Selected pos</th><th>Selected</th><th>Comparison pos</th><th>Comparison</th><th>Change</th>
              </tr>
            </thead>
            <tbody>
              {comparison.matchers.map((matcher) => (
                <MatcherRow matcher={matcher} key={matcher.name} />
              ))}
            </tbody>
          </table>
        </div>
      </section>

      {streamNames.map((name) => (
        <StreamComparison
          name={name}
          reference={comparison.referenceStreams.find((stream) => stream.name === name)}
          selected={comparison.selectedStreams.find((stream) => stream.name === name)}
          key={name}
        />
      ))}

      <section class="comparison-section">
        <div class="section-heading-row">
          <h2>Recorded context</h2>
          <span>Factoids and alert transitions</span>
        </div>
        <div class="comparison-columns context-columns">
          <ContextColumn label="Selected" context={comparison.selectedContext} />
          <ContextColumn label="Comparison" context={comparison.referenceContext} />
        </div>
      </section>
    </div>
  )
}

function SignalRow({ signal }: { signal: ComparisonSignal }) {
  return (
    <tr>
      <td class="key-cell">{signal.label}</td>
      <td>{formatValue(signal.selected, signal.kind)}</td>
      <td>{formatValue(signal.reference, signal.kind)}</td>
      <td class={deltaClass(signal.delta)}><DeltaValue value={signal.delta} kind={signal.kind} /></td>
    </tr>
  )
}

function MatcherRow({ matcher }: { matcher: MatcherComparison }) {
  const kind: ComparisonValueKind = matcher.name === 'bytes' ? 'bytes' : 'count'
  return (
    <tr>
      <td class="key-cell"><ColorSwatch color={matcher.colorHex} />{matcher.name || 'unnamed'}</td>
      <td>{formatPosition(matcher.selectedPosition)}</td>
      <td>{formatValue(matcher.selected, kind)}</td>
      <td>{formatPosition(matcher.referencePosition)}</td>
      <td>{formatValue(matcher.reference, kind)}</td>
      <td class={deltaClass(matcher.delta)}><DeltaValue value={matcher.delta} kind={kind} /></td>
    </tr>
  )
}

function DeltaValue({ value, kind }: { value: number | null; kind: ComparisonValueKind }) {
  if (value === null) {
    return <span class="comparison-delta unavailable">—</span>
  }
  const DirectionIcon = value > 0 ? ArrowUp : value < 0 ? ArrowDown : Minus
  const label = value > 0 ? 'Increased' : value < 0 ? 'Decreased' : 'Unchanged'
  return (
    <span class="comparison-delta" title={label} aria-label={`${label}: ${formatDelta(value, kind)}`}>
      <span aria-hidden="true">{formatDelta(value, kind)}</span>
      <span class="delta-direction"><DirectionIcon size={12} strokeWidth={2.5} aria-hidden="true" /></span>
    </span>
  )
}

function StreamComparison({
  name,
  reference,
  selected,
}: {
  name: typeof streamNames[number]
  reference: InterestingStream | undefined
  selected: InterestingStream | undefined
}) {
  const label = name === 'ips' ? 'IPs' : capitalize(name)
  if (name !== 'ips') {
    return (
      <PeakFingerprint
        label={label}
        reference={reference}
        selected={selected}
      />
    )
  }
  return (
    <section class="comparison-section comparison-stream">
      <div class="section-heading-row">
        <h2>{label}</h2>
        <span>Retained working-set snapshots</span>
      </div>
      <div class="comparison-columns">
        <StreamColumn label="Selected" stream={selected} />
        <StreamColumn label="Comparison" stream={reference} />
      </div>
    </section>
  )
}

function PeakFingerprint({
  label,
  reference,
  selected,
}: {
  label: string
  reference: InterestingStream | undefined
  selected: InterestingStream | undefined
}) {
  const peaks = pairedPeaks(reference?.peaks ?? [], selected?.peaks ?? [])
  const maximum = Math.max(0, ...peaks.flatMap((peak) => [
    peak.reference?.score ?? 0,
    peak.selected?.score ?? 0,
  ]))

  return (
    <section class="comparison-section comparison-stream peak-fingerprint">
      <div class="section-heading-row">
        <h2>{label} peaks</h2>
        <span>Peak score fingerprints · retained state only</span>
      </div>
      <div class="peak-summary">
        <span>Selected: {selected ? `${formatNumber(selected.totalKeys)} retained keys` : 'stream not emitted'}</span>
        <span>Comparison: {reference ? `${formatNumber(reference.totalKeys)} retained keys` : 'stream not emitted'}</span>
      </div>
      {peaks.length > 0 ? (
        <div class="peak-comparison" role="table" aria-label={`${label} peak comparison`}>
          <div class="peak-comparison-heading" role="row">
            <span role="columnheader">Selected</span>
            <span role="columnheader">Peak key</span>
            <span role="columnheader">Comparison</span>
          </div>
          {peaks.map((peak) => (
            <div class="peak-comparison-row" role="row" key={peak.key}>
              <PeakValue entry={peak.selected} maximum={maximum} side="left" />
              <div class="peak-key" role="cell">
                <span><ColorSwatch color={peak.selected?.color || peak.reference?.color || ''} />{peak.key || '—'}</span>
                <small>{rankPair(peak.reference, peak.selected)}</small>
              </div>
              <PeakValue entry={peak.reference} maximum={maximum} side="right" />
            </div>
          ))}
        </div>
      ) : <ComparisonEmpty text="No peak entries emitted for either interval." />}
    </section>
  )
}

function PeakValue({
  entry,
  maximum,
  side,
}: {
  entry: InterestingEntry | undefined
  maximum: number
  side: 'left' | 'right'
}) {
  if (!entry) {
    return <div class={`peak-value ${side} missing`} role="cell">not emitted</div>
  }
  const width = maximum > 0 && entry.score !== null
    ? Math.max(2, entry.score * 100 / maximum)
    : 0
  return (
    <div class={`peak-value ${side}`} role="cell">
      <div class="peak-value-text">
        <strong>{formatNumber(entry.score)}</strong>
        <span>{formatNumber(entry.count)} hits · {formatBytes(entry.bytes)}</span>
      </div>
      <div class="peak-bar" aria-label={`Peak score ${formatNumber(entry.score)}`}>
        <span style={{ width: `${width}%`, backgroundColor: entry.color || undefined }} />
      </div>
    </div>
  )
}

function pairedPeaks(reference: InterestingEntry[], selected: InterestingEntry[]) {
  const keys = [
    ...selected.map((entry) => entry.key),
    ...reference.filter((entry) => !selected.some((candidate) => candidate.key === entry.key)).map((entry) => entry.key),
  ]
  return keys.map((key) => ({
    key,
    reference: reference.find((entry) => entry.key === key),
    selected: selected.find((entry) => entry.key === key),
  }))
}

function rankPair(reference: InterestingEntry | undefined, selected: InterestingEntry | undefined): string {
  const referenceRank = reference?.rank === null || reference?.rank === undefined ? '—' : `#${reference.rank}`
  const selectedRank = selected?.rank === null || selected?.rank === undefined ? '—' : `#${selected.rank}`
  return `${selectedRank} → ${referenceRank}`
}

function StreamColumn({ label, stream }: { label: string; stream: InterestingStream | undefined }) {
  return (
    <div class="comparison-column">
      <div class="comparison-column-heading">
        <h3>{label}</h3>
        <span>{stream ? `${formatNumber(stream.totalKeys)} retained keys` : 'Stream not emitted'}</span>
      </div>
      {stream ? (
        <>
          <EntryTable label={`${label} top entries`} title="Top" entries={stream.top} />
          <EntryTable label={`${label} peak entries`} title="Peaks" entries={stream.peaks} />
          {stream.name === 'ips' ? <GroupTable label={label} groups={stream.ipGroups} /> : null}
        </>
      ) : <ComparisonEmpty text="Stream not emitted for this interval." />}
    </div>
  )
}

function EntryTable({
  label,
  title,
  entries,
}: {
  label: string
  title: string
  entries: InterestingEntry[]
}) {
  return (
    <div class="comparison-subsection">
      <h4>{title}</h4>
      {entries.length > 0 ? (
        <div class="data-table-wrap comparison-entry-table">
          <table aria-label={label}>
            <thead><tr><th>Rank</th><th>Key</th><th>Score</th><th>Count</th><th>Bytes</th><th>Status</th><th>Marked by</th></tr></thead>
            <tbody>
              {entries.map((entry, index) => (
                <tr key={`${entry.key}-${index}`}>
                  <td>{formatNumber(entry.rank)}</td>
                  <td class="key-cell"><ColorSwatch color={entry.color} />{entry.key || '—'}</td>
                  <td>{formatNumber(entry.score)}</td>
                  <td>{formatNumber(entry.count)}</td>
                  <td>{formatBytes(entry.bytes)}</td>
                  <td>{entry.lastStatus || '—'}</td>
                  <td>{entry.markedByMatcher || entry.markedState || '—'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : <ComparisonEmpty text={`No ${title.toLowerCase()} entries emitted.`} />}
    </div>
  )
}

function GroupTable({ label, groups }: { label: string; groups: IPGroupEntry[] }) {
  return (
    <div class="comparison-subsection">
      <h4>IP groups</h4>
      {groups.length > 0 ? (
        <div class="data-table-wrap comparison-group-table">
          <table aria-label={`${label} IP groups`}>
            <thead><tr><th>Rank</th><th>Prefix</th><th>Score</th><th>Count</th><th>Members</th><th>Bytes</th><th>Depth</th></tr></thead>
            <tbody>
              {groups.map((group, index) => (
                <tr key={`${group.prefix}-${index}`}>
                  <td>{formatNumber(group.rank)}</td>
                  <td class="key-cell"><ColorSwatch color={group.color} />{group.prefix || '—'}</td>
                  <td>{formatNumber(group.score)}</td>
                  <td>{formatNumber(group.count)}</td>
                  <td>{formatNumber(group.members)}</td>
                  <td>{formatBytes(group.bytes)}</td>
                  <td>{formatNumber(group.historyDepth)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : <ComparisonEmpty text="No IP groups emitted." />}
    </div>
  )
}

function ContextColumn({ label, context }: { label: string; context: IntervalContext }) {
  return (
    <div class="comparison-column comparison-context-column">
      <div class="comparison-column-heading"><h3>{label}</h3></div>
      <div class="comparison-subsection">
        <h4>Factoids</h4>
        {context.factoids.length > 0 ? (
          <dl class="comparison-context-list">
            {context.factoids.map((factoid, index) => (
              <div key={`${factoid.name}-${index}`}><dt>{factoid.name || 'fact'}</dt><dd>{factoid.text}</dd></div>
            ))}
          </dl>
        ) : <ComparisonEmpty text="No factoids emitted." />}
      </div>
      <div class="comparison-subsection">
        <h4>Alerts</h4>
        {context.alerts.length > 0 ? (
          <dl class="comparison-context-list alerts">
            {context.alerts.map((alert) => (
              <div key={alert.record.id}>
                <dt>{alert.matcher || 'matcher'} {alert.status || 'alert'}</dt>
                <dd>{formatAlert(alert)}</dd>
              </div>
            ))}
          </dl>
        ) : <ComparisonEmpty text="No alert transitions recorded." />}
      </div>
    </div>
  )
}

function ComparisonEmpty({ text }: { text: string }) {
  return <div class="comparison-empty">{text}</div>
}

function ColorSwatch({ color }: { color: string }) {
  return <span class="color-swatch" style={color ? { backgroundColor: color } : undefined} aria-hidden="true" />
}

function formatIntervalTitle(record: PattyLogRecord): string {
  return `${formatTimestamp(record.logTime)} · interval ${record.interval ?? '—'}`
}

function formatIntervalOption(record: PattyLogRecord): string {
  const lines = typeof record.data.interval_lines === 'number'
    ? `${formatNumber(record.data.interval_lines)} lines`
    : 'lines unavailable'
  return `${formatTimestamp(record.logTime)} · interval ${record.interval ?? '—'} · ${lines}`
}

function formatTimestamp(value: string): string {
  const parsed = new Date(value)
  if (!value || Number.isNaN(parsed.getTime())) {
    return value || 'No log time'
  }
  return new Intl.DateTimeFormat(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(parsed)
}

function formatPosition(value: number | null): string {
  return value === null ? 'not emitted' : String(value)
}

function formatValue(value: number | null, kind: ComparisonValueKind): string {
  if (kind === 'bytes') {
    return formatBytes(value)
  }
  if (kind === 'percent') {
    return value === null ? '—' : `${formatDecimal(value)}%`
  }
  return formatNumber(value)
}

function formatDelta(value: number | null, kind: ComparisonValueKind): string {
  if (value === null) {
    return '—'
  }
  const sign = value > 0 ? '+' : ''
  if (kind === 'bytes') {
    return `${sign}${formatBytes(value)}`
  }
  if (kind === 'percent') {
    return `${sign}${formatDecimal(value)} pt`
  }
  return `${sign}${new Intl.NumberFormat().format(value)}`
}

function formatDecimal(value: number): string {
  return new Intl.NumberFormat(undefined, { maximumFractionDigits: 1 }).format(value)
}

function deltaClass(value: number | null): string {
  return value === null || value === 0 ? '' : value > 0 ? 'delta-positive' : 'delta-negative'
}

function formatAlert(alert: IntervalContext['alerts'][number]): string {
  const direction = alert.direction ? `${alert.direction} · ` : ''
  const value = alert.value === null ? 'value unavailable' : `value ${formatNumber(alert.value)}`
  const threshold = alert.threshold === null ? '' : ` · threshold ${formatNumber(alert.threshold)}`
  return `${direction}${value}${threshold}`
}

function capitalize(value: string): string {
  return value.charAt(0).toUpperCase() + value.slice(1)
}
