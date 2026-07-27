import { Check, Copy } from 'lucide-preact'
import type { ComponentChildren } from 'preact'
import { useEffect, useMemo, useState } from 'preact/hooks'
import { formatBytes, formatCount as formatNumber } from '../displayFormat'
import { alwaysVisibleIntervalMatcherNames } from '../domain/intervalSeries'
import { searchTextMatches, searchTextSegments } from '../domain/searchSession'
import { buildSourceLookup, type SourceLookupUnavailableReason } from '../domain/sourceLookup'
import {
  interestingTrackedLane,
  matcherTrackedLane,
  maximumTrackedLanes,
  optionalCoreIntervalLanes,
  type CoreIntervalLaneKey,
  type TrackedLane,
} from '../domain/trackedLane'
import {
  projectTrafficDetail,
  type InterestingEntry,
  type InterestingStream,
  type SourceExampleAvailability,
  type SourceExample,
  type TrafficMatcher,
} from '../domain/trafficDetail'
import { readString, type PattyLogRecord } from '../domain/types'

type StreamMode = 'top' | 'peaks'
type CopyState = 'idle' | 'copied' | 'failed'

interface SourceLookupContext {
  recordId: string
  filePath: string
  machine: string
  logTime: string
}

const coreSignals = [
  { name: 'lines', label: 'Lines', color: '#4ecdc4' },
  { name: 'bytes', label: 'Bytes', color: '#e3ba72' },
  { name: 'errs', label: 'Errors', color: '#ff6b6b' },
]

const streamNames = ['words', 'refs', 'ips']

export function TrafficDetail({
  record,
  searchQuery,
  trackedLanes,
  enabledCoreLanes,
  onToggleCoreLane,
  onToggleTrackedLane,
}: {
  record: PattyLogRecord
  searchQuery: string
  trackedLanes: TrackedLane[]
  enabledCoreLanes: CoreIntervalLaneKey[]
  onToggleCoreLane: (key: CoreIntervalLaneKey) => void
  onToggleTrackedLane: (lane: TrackedLane) => void
}) {
  const detail = useMemo(() => projectTrafficDetail(record.data), [record.id, record.data])
  const sourceLookupContext = useMemo(() => ({
    recordId: record.id,
    filePath: readString(record.data, 'file_path'),
    machine: readString(record.data, 'machine'),
    logTime: readString(record.data, 'log_time') || record.logTime,
  }), [record.id, record.data, record.logTime])
  const [matcherName, setMatcherName] = useState('errs')
  const [streamName, setStreamName] = useState('words')
  const [streamMode, setStreamMode] = useState<StreamMode>('top')
  const [entryKey, setEntryKey] = useState('')

  const matcher = detail.matchers.find((candidate) => candidate.name === matcherName)
    ?? detail.matchers.find((candidate) => candidate.name === 'errs')
    ?? detail.matchers[0]
  const stream = detail.interesting.find((candidate) => candidate.name === streamName)
    ?? detail.interesting.find((candidate) => candidate.name === 'words')
    ?? detail.interesting[0]
  const entries = stream ? stream[streamMode] : []
  const entry = entries.find((candidate) => candidate.key === entryKey) ?? entries[0]

  useEffect(() => {
    setEntryKey((current) => entries.some((candidate) => candidate.key === current)
      ? current
      : entries[0]?.key ?? '')
  }, [record.id, stream?.name, streamMode, entries])

  return (
    <div class="traffic-pane">
      <CoreSignalSection matchers={detail.matchers} />
      <TrackedLaneSection
        lanes={trackedLanes}
        onToggle={onToggleTrackedLane}
      />
      <MatcherSection
        matchers={detail.matchers}
        selected={matcher}
        searchQuery={searchQuery}
        trackedLanes={trackedLanes}
        enabledCoreLanes={enabledCoreLanes}
        onSelect={setMatcherName}
        onToggleCore={onToggleCoreLane}
        onToggle={onToggleTrackedLane}
      />
      <InterestingSection
        streams={detail.interesting}
        stream={stream}
        mode={streamMode}
        entry={entry}
        trackedLanes={trackedLanes}
        sourceExampleAvailability={detail.sourceExampleAvailability}
        sourceLookupContext={sourceLookupContext}
        searchQuery={searchQuery}
        onStream={(name) => {
          setStreamName(name)
          setEntryKey('')
        }}
        onMode={(mode) => {
          setStreamMode(mode)
          setEntryKey('')
        }}
        onEntry={setEntryKey}
        onToggle={onToggleTrackedLane}
      />
      {detail.selected ? (
        <section class="traffic-section">
          <h2>Recorded selection</h2>
          {detail.selected.fields.length > 0 ? (
            <dl class="traffic-metrics compact">
              {detail.selected.fields.map((field) => (
                <div key={field.label}>
                  <dt>{field.label}</dt>
                  <dd>{field.value}</dd>
                </div>
              ))}
            </dl>
          ) : null}
          <SourceExamples sources={detail.selected.sources} searchQuery={searchQuery} />
        </section>
      ) : null}
    </div>
  )
}

function TrackedLaneSection({
  lanes,
  onToggle,
}: {
  lanes: TrackedLane[]
  onToggle: (lane: TrackedLane) => void
}) {
  return (
    <section class="traffic-section tracked-lane-section" aria-label="Tracked interval lanes">
      <div class="section-heading-row">
        <h2>Tracked lanes</h2>
        <span>{lanes.length} / {maximumTrackedLanes}</span>
      </div>
      {lanes.length > 0 ? (
        <div class="tracked-lane-list">
          {lanes.map((lane) => (
            <label title={`Remove ${lane.label} from the interval map`} key={lane.id}>
              <input
                type="checkbox"
                aria-label={`Remove ${lane.label} from interval map`}
                checked
                onChange={() => onToggle(lane)}
              />
              <ColorSwatch color={lane.color} />
              <span>{lane.label}</span>
            </label>
          ))}
        </div>
      ) : <span class="tracked-lane-empty">No matcher or traffic-item lanes selected.</span>}
    </section>
  )
}

function CoreSignalSection({ matchers }: { matchers: TrafficMatcher[] }) {
  return (
    <section class="traffic-section traffic-signals" aria-label="Core interval signals">
      <div class="section-heading-row">
        <h2>Interval signals</h2>
        <span>Now · prior · peak · history</span>
      </div>
      <div class="signal-grid">
        {coreSignals.map((signal) => {
          const matcher = matchers.find((candidate) => candidate.name === signal.name)
          return (
            <div class={`signal signal-${signal.name}`} key={signal.name}>
              <div class="signal-name">
                <ColorSwatch color={signal.color} />
                <strong>{signal.label}</strong>
              </div>
              <SignalValue label="Now" value={formatMatcherValue(signal.name, matcher?.current ?? null)} />
              <SignalValue label="Prior" value={formatMatcherValue(signal.name, matcher?.previous ?? null)} />
              <SignalValue label="Peak" value={formatMatcherValue(signal.name, matcher?.historyPeak ?? null)} />
              <SignalValue label="History" value={formatMatcherValue(signal.name, matcher?.historyTotal ?? null)} />
            </div>
          )
        })}
      </div>
    </section>
  )
}

function SignalValue({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <span>{label}</span>
      <strong title={value}>{value}</strong>
    </div>
  )
}

function MatcherSection({
  matchers,
  selected,
  searchQuery,
  trackedLanes,
  enabledCoreLanes,
  onSelect,
  onToggleCore,
  onToggle,
}: {
  matchers: TrafficMatcher[]
  selected: TrafficMatcher | undefined
  searchQuery: string
  trackedLanes: TrackedLane[]
  enabledCoreLanes: CoreIntervalLaneKey[]
  onSelect: (name: string) => void
  onToggleCore: (key: CoreIntervalLaneKey) => void
  onToggle: (lane: TrackedLane) => void
}) {
  return (
    <section class="traffic-section matcher-section">
      <h2>Matchers</h2>
      {matchers.length === 0 ? <EmptyTrafficState text="No matcher state emitted." /> : (
        <>
          <div class="matcher-table" aria-label="Matcher detail">
            <div class="matcher-table-heading" aria-hidden="true">
              <span>Map</span><span>Matcher</span><span>Now</span><span>Prior</span><span>Peak</span>
            </div>
            {matchers.map((matcher) => {
              const fixed = alwaysVisibleIntervalMatcherNames.some((name) => name === matcher.name)
              const coreLane = optionalCoreIntervalLanes.find((lane) => lane.matcherName === matcher.name)
              const lane = matcherTrackedLane(matcher.name, matcher.colorHex)
              const tracked = coreLane
                ? enabledCoreLanes.includes(coreLane.key)
                : trackedLanes.some((candidate) => candidate.id === lane.id)
              const limitReached = trackedLanes.length >= maximumTrackedLanes
              const trackDisabled = fixed || (!coreLane && !tracked && limitReached)
              const trackTitle = fixed
                ? 'Already shown in the interval map'
                : !coreLane && !tracked && limitReached
                  ? `Up to ${maximumTrackedLanes} optional lanes can be tracked`
                  : tracked ? 'Remove matcher lane' : 'Add matcher lane'
              return (
                <div class={`matcher-table-row ${matcher.name === selected?.name ? 'selected' : ''}`} key={matcher.name}>
                  <label class="matcher-pin" title={trackTitle}>
                    <input
                      type="checkbox"
                      aria-label={fixed
                        ? `${matcher.name} is always shown in the interval map`
                        : `${tracked ? 'Remove' : 'Add'} ${matcher.name} ${tracked ? 'from' : 'to'} interval map`}
                      checked={fixed || tracked}
                      disabled={trackDisabled}
                      onChange={() => coreLane ? onToggleCore(coreLane.key) : onToggle(lane)}
                    />
                  </label>
                  <button
                    type="button"
                    aria-pressed={matcher.name === selected?.name}
                    class={matcher.name === selected?.name ? 'selected' : ''}
                    onClick={() => onSelect(matcher.name)}
                  >
                    <span class="matcher-name"><ColorSwatch color={matcher.colorHex} />{matcher.name || 'unnamed'}</span>
                    <span>{formatMatcherValue(matcher.name, matcher.current)}</span>
                    <span>{formatMatcherValue(matcher.name, matcher.previous)}</span>
                    <span>{formatMatcherValue(matcher.name, matcher.historyPeak)}</span>
                  </button>
                </div>
              )
            })}
          </div>
          {selected ? <MatcherBreakdown matcher={selected} searchQuery={searchQuery} /> : null}
        </>
      )}
    </section>
  )
}

function MatcherBreakdown({ matcher, searchQuery }: { matcher: TrafficMatcher; searchQuery: string }) {
  return (
    <div class="matcher-breakdown" role="tabpanel" aria-label={`${matcher.name} detail`}>
      <div class="section-heading-row">
        <h3>{matcher.name}</h3>
        <span>{formatMatcherValue(matcher.name, matcher.historyTotal)} history total</span>
      </div>
      {matcher.topKeys.length > 0 ? (
        <DataTable label={`${matcher.name} retained keys`}>
          <thead><tr><th>Rank</th><th>Key</th><th>Count</th></tr></thead>
          <tbody>
            {matcher.topKeys.map((entry, index) => (
              <tr key={`${entry.key}-${index}`}>
                <td>{formatNumber(entry.rank)}</td>
                <td class="key-cell">{entry.key || '—'}</td>
                <td>{formatMatcherValue(matcher.name, entry.count)}</td>
              </tr>
            ))}
          </tbody>
        </DataTable>
      ) : <EmptyTrafficState text="No retained keys emitted for this matcher." />}
      {matcher.topGroups.length > 0 ? (
        <DataTable label={`${matcher.name} retained groups`}>
          <thead><tr><th>Rank</th><th>Prefix</th><th>Count</th><th>Members</th></tr></thead>
          <tbody>
            {matcher.topGroups.map((group, index) => (
              <tr key={`${group.prefix}-${index}`}>
                <td>{formatNumber(group.rank)}</td>
                <td class="key-cell">{group.prefix || '—'}</td>
                <td>{formatNumber(group.count)}</td>
                <td>{formatNumber(group.members)}</td>
              </tr>
            ))}
          </tbody>
        </DataTable>
      ) : null}
      <SourceExamples sources={matcher.sources} searchQuery={searchQuery} />
    </div>
  )
}

function InterestingSection({
  streams,
  stream,
  mode,
  entry,
  trackedLanes,
  sourceExampleAvailability,
  sourceLookupContext,
  searchQuery,
  onStream,
  onMode,
  onEntry,
  onToggle,
}: {
  streams: InterestingStream[]
  stream: InterestingStream | undefined
  mode: StreamMode
  entry: InterestingEntry | undefined
  trackedLanes: TrackedLane[]
  sourceExampleAvailability: SourceExampleAvailability
  sourceLookupContext: SourceLookupContext
  searchQuery: string
  onStream: (name: string) => void
  onMode: (mode: StreamMode) => void
  onEntry: (key: string) => void
  onToggle: (lane: TrackedLane) => void
}) {
  const entries = stream ? stream[mode] : []
  return (
    <section class="traffic-section interesting-section">
      <div class="section-heading-row">
        <h2>Interesting</h2>
        {stream ? <span>{formatNumber(stream.totalKeys)} retained keys</span> : null}
      </div>
      <div class="traffic-controls">
        <div class="segmented-control" role="tablist" aria-label="Interesting stream">
          {streamNames.map((name) => {
            const available = streams.some((candidate) => candidate.name === name)
            return (
              <button
                type="button"
                role="tab"
                aria-selected={name === stream?.name}
                disabled={!available}
                class={name === stream?.name ? 'active' : ''}
                onClick={() => onStream(name)}
                key={name}
              >
                {name === 'ips' ? 'IPs' : capitalize(name)}
              </button>
            )
          })}
        </div>
        <div class="segmented-control" role="tablist" aria-label="Interesting ranking">
          {(['top', 'peaks'] as StreamMode[]).map((candidate) => (
            <button
              type="button"
              role="tab"
              aria-selected={candidate === mode}
              class={candidate === mode ? 'active' : ''}
              onClick={() => onMode(candidate)}
              key={candidate}
            >
              {capitalize(candidate)}
            </button>
          ))}
        </div>
      </div>
      {entries.length > 0 ? (
        <DataTable label={`${stream?.name ?? ''} ${mode}`} className="interesting-table">
          <thead><tr><th>Map</th><th>Rank</th><th>Key</th><th>Score</th><th>Count</th><th>Bytes</th><th>Status</th><th>Marked by</th></tr></thead>
          <tbody>
            {entries.map((candidate, index) => {
              const lane = interestingTrackedLane(stream?.name ?? '', candidate.key, candidate.color)
              const tracked = trackedLanes.some((trackedLane) => trackedLane.id === lane.id)
              const limitReached = trackedLanes.length >= maximumTrackedLanes
              const trackDisabled = candidate.key === '' || (!tracked && limitReached)
              const trackTitle = candidate.key === ''
                ? 'An empty key cannot be tracked'
                : !tracked && limitReached
                  ? `Up to ${maximumTrackedLanes} optional lanes can be tracked`
                  : tracked ? 'Remove interesting lane' : 'Add interesting lane'
              return (
                <tr
                  class={candidate.key === entry?.key ? 'selected' : ''}
                  onClick={() => onEntry(candidate.key)}
                  key={`${candidate.key}-${index}`}
                >
                  <td class="interesting-pin-cell" onClick={(event) => event.stopPropagation()}>
                    <label title={trackTitle}>
                      <input
                        type="checkbox"
                        aria-label={`${tracked ? 'Remove' : 'Add'} ${lane.label} ${tracked ? 'from' : 'to'} interval map`}
                        checked={tracked}
                        disabled={trackDisabled}
                        onChange={() => onToggle(lane)}
                      />
                    </label>
                  </td>
                  <td>{formatNumber(candidate.rank)}</td>
                  <td class="key-cell">
                    <button
                      type="button"
                      onClick={(event) => {
                        event.stopPropagation()
                        onEntry(candidate.key)
                      }}
                    >
                      <ColorSwatch color={candidate.color} />
                      <span class={searchTextMatches(candidate.key, searchQuery) ? 'search-text-match' : undefined}>
                        {candidate.key || '—'}
                      </span>
                    </button>
                  </td>
                  <td>{formatNumber(candidate.score)}</td>
                  <td>{formatNumber(candidate.count)}</td>
                  <td>{formatBytes(candidate.bytes)}</td>
                  <td>{candidate.lastStatus || '—'}</td>
                  <td>{candidate.markedByMatcher || candidate.markedState || '—'}</td>
                </tr>
              )
            })}
          </tbody>
        </DataTable>
      ) : <EmptyTrafficState text={`No ${mode} entries emitted for ${stream?.name || 'this stream'}.`} />}
      {entry ? (
        <InterestingEntryDetail
          entry={entry}
          searchQuery={searchQuery}
          sourceExampleAvailability={sourceExampleAvailability}
          sourceLookupContext={sourceLookupContext}
        />
      ) : null}
      {stream?.name === 'ips' && stream.ipGroups.length > 0
        ? <IPGroups stream={stream} searchQuery={searchQuery} />
        : null}
    </section>
  )
}

function InterestingEntryDetail({
  entry,
  searchQuery,
  sourceExampleAvailability,
  sourceLookupContext,
}: {
  entry: InterestingEntry
  searchQuery: string
  sourceExampleAvailability: SourceExampleAvailability
  sourceLookupContext: SourceLookupContext
}) {
  const metrics = [
    ['Score', formatNumber(entry.score)],
    ['Count', formatNumber(entry.count)],
    ['Bytes', formatBytes(entry.bytes)],
    ['Prime flux', formatNumber(entry.primeFlux)],
    ['Burstiness', formatDecimal(entry.burstiness)],
    ['Agent delta', formatDecimal(entry.agentDeltaMetric)],
    ['History total', formatNumber(entry.historyTotal)],
    ['History peak', formatNumber(entry.historyPeak)],
    ['History depth', formatNumber(entry.historyDepth)],
    ['Last seen tic', formatNumber(entry.lastSeenTic)],
    ['Marked state', entry.markedState || '—'],
    ['Peak', entry.isPeak ? 'yes' : 'no'],
  ]
  return (
    <div class="entry-detail">
      <h3>{entry.key || 'Selected entry'}</h3>
      <dl class="traffic-metrics">
        {metrics.map(([label, value]) => (
          <div key={label}><dt>{label}</dt><dd title={value}>{value}</dd></div>
        ))}
      </dl>
      {entry.sources.length > 0
        ? <SourceExamples sources={entry.sources} searchQuery={searchQuery} />
        : (
          <SourceLookupPanel
            entryKey={entry.key}
            availability={sourceExampleAvailability}
            context={sourceLookupContext}
          />
        )}
    </div>
  )
}

function IPGroups({ stream, searchQuery }: { stream: InterestingStream; searchQuery: string }) {
  return (
    <div class="ip-groups">
      <h3>IP groups</h3>
      <DataTable label="IP groups">
        <thead><tr><th>Rank</th><th>Prefix</th><th>Score</th><th>Count</th><th>+ first</th><th>Members</th><th>Bytes</th><th>Depth</th><th>Marked by</th></tr></thead>
        <tbody>
          {stream.ipGroups.map((group, index) => (
            <tr key={`${group.prefix}-${index}`}>
              <td>{formatNumber(group.rank)}</td>
              <td class="key-cell">
                <ColorSwatch color={group.color} />
                <span class={searchTextMatches(group.prefix, searchQuery) ? 'search-text-match' : undefined}>
                  {group.prefix || '—'}
                </span>
              </td>
              <td>{formatNumber(group.score)}</td>
              <td>{formatNumber(group.count)}</td>
              <td>{formatNumber(group.countPlusFirst)}</td>
              <td>{formatNumber(group.members)}</td>
              <td>{formatBytes(group.bytes)}</td>
              <td>{formatNumber(group.historyDepth)}</td>
              <td>{group.markedByMatcher || group.markedState || '—'}</td>
            </tr>
          ))}
        </tbody>
      </DataTable>
    </div>
  )
}

function SourceExamples({
  sources,
  searchQuery,
}: {
  sources: SourceExample[]
  searchQuery: string
}) {
  if (sources.length === 0) {
    return null
  }
  return (
    <div class="source-examples">
      <h3>Retained source lines</h3>
      {sources.map((source, index) => (
        <div key={`${source.label}-${index}`}>
          <span>{source.label}</span>
          <pre>
            {searchTextSegments(source.line, searchQuery).map((segment, segmentIndex) => (
              segment.matched
                ? <mark key={segmentIndex}>{segment.text}</mark>
                : segment.text
            ))}
          </pre>
        </div>
      ))}
    </div>
  )
}

function SourceLookupPanel({
  entryKey,
  availability,
  context,
}: {
  entryKey: string
  availability: SourceExampleAvailability
  context: SourceLookupContext
}) {
  const lookup = useMemo(
    () => buildSourceLookup(entryKey, context.filePath),
    [entryKey, context.filePath],
  )
  const [copyState, setCopyState] = useState<CopyState>('idle')
  const lookupIdentity = lookup.available ? lookup.command : lookup.reason

  useEffect(() => setCopyState('idle'), [lookupIdentity, context.recordId])
  useEffect(() => {
    if (copyState !== 'copied') {
      return
    }
    const timer = window.setTimeout(() => setCopyState('idle'), 1600)
    return () => window.clearTimeout(timer)
  }, [copyState])

  async function copyCommand() {
    if (!lookup.available) {
      return
    }
    try {
      if (!navigator.clipboard) {
        throw new Error('Clipboard API unavailable')
      }
      await navigator.clipboard.writeText(lookup.command)
      setCopyState('copied')
    } catch {
      setCopyState('failed')
    }
  }

  const copyLabel = copyState === 'copied'
    ? 'Copied command'
    : copyState === 'failed' ? 'Copy failed; retry' : 'Copy command'

  return (
    <div class="source-lookup" role="region" aria-label="Find likely source lines">
      <p class="source-availability">{sourceAvailabilityMessage(availability)}</p>
      <div class="source-lookup-heading">
        <div>
          <h3>Find likely source lines</h3>
          <p>Best-effort fixed-string search. Matches may span other intervals or log fields.</p>
        </div>
      </div>
      {lookup.available ? (
        <>
          <div class="source-lookup-command">
            <code>{lookup.command}</code>
            <button
              class="icon-button source-copy-button"
              type="button"
              title={copyLabel}
              aria-label={copyLabel}
              onClick={() => void copyCommand()}
            >
              {copyState === 'copied'
                ? <Check size={16} aria-hidden="true" />
                : <Copy size={16} aria-hidden="true" />}
            </button>
          </div>
          <div class="source-lookup-context">
            {context.machine ? <span>Recorded on <strong>{context.machine}</strong></span> : null}
            {isInitializedLogTime(context.logTime)
              ? <span>Interval log time <code>{context.logTime}</code></span>
              : null}
          </div>
          {lookup.pathIsRelative ? (
            <p class="source-lookup-note">The recorded path is relative; run from PattyGraph's original working directory.</p>
          ) : null}
          {lookup.pathIsTemplate ? (
            <p class="source-lookup-note">Replace the placeholder with the original access-log path.</p>
          ) : null}
          <span class="source-copy-status" aria-live="polite">
            {copyState === 'copied'
              ? 'Copied'
              : copyState === 'failed' ? 'Copy failed; select the command manually.' : ''}
          </span>
        </>
      ) : <p class="source-lookup-note">{sourceLookupUnavailableMessage(lookup.reason)}</p>}
    </div>
  )
}

function sourceAvailabilityMessage(availability: SourceExampleAvailability): string {
  switch (availability) {
    case 'disabled': return 'Source examples were not recorded for this interval.'
    case 'enabled': return 'No retained source line was available.'
    default: return 'This record does not report source-example availability.'
  }
}

function sourceLookupUnavailableMessage(reason: SourceLookupUnavailableReason): string {
  switch (reason) {
    case 'synthetic-empty':
      return '--empty-- is PattyGraph\'s synthetic empty-token bucket and has no literal grep term.'
    case 'unsafe-key': return 'This key cannot be represented safely in a shell command.'
    case 'unsafe-path': return 'The recorded source path cannot be represented safely in a shell command.'
    default: return 'No literal key is available for an original-log search.'
  }
}

function isInitializedLogTime(value: string): boolean {
  return value !== '' && !value.startsWith('1970-01-01T00:00:00')
}

function DataTable({
  label,
  className = '',
  children,
}: {
  label: string
  className?: string
  children: ComponentChildren
}) {
  return (
    <div class={`data-table-wrap ${className}`}>
      <table aria-label={label}>{children}</table>
    </div>
  )
}

function EmptyTrafficState({ text }: { text: string }) {
  return <div class="traffic-empty">{text}</div>
}

function ColorSwatch({ color }: { color: string }) {
  return <span class="color-swatch" style={color ? { backgroundColor: color } : undefined} aria-hidden="true" />
}

function formatMatcherValue(name: string, value: number | null): string {
  return name === 'bytes' ? formatBytes(value) : formatNumber(value)
}

function formatDecimal(value: number | null): string {
  if (value === null) {
    return '—'
  }
  if (value !== 0 && Math.abs(value) < 0.001) {
    return value.toExponential(2)
  }
  return new Intl.NumberFormat(undefined, { maximumFractionDigits: 3 }).format(value)
}

function capitalize(value: string): string {
  return value.charAt(0).toUpperCase() + value.slice(1)
}
