import { Activity, Bell, FileJson, Terminal } from 'lucide-preact'
import { formatBytes, formatCount } from '../displayFormat'
import {
  intervalMetricLanes,
  type IntervalMetricLaneKey,
} from '../domain/trackedLane'
import {
  isKnownEventType,
  readArray,
  readBoolean,
  readNumber,
  readObject,
  readString,
  type JsonObject,
  type JsonValue,
  type PattyLogRecord,
} from '../domain/types'

function RecordListItem({ record, selected, onSelect }: {
  record: PattyLogRecord
  selected: boolean
  onSelect: () => void
}) {
  const Icon = record.eventType === 'alert'
    ? Bell
    : record.eventType === 'control_command'
      ? Terminal
      : record.eventType === 'interval'
        ? Activity
        : FileJson
  const title = record.eventType === 'interval'
    ? formatTimestamp(record.logTime, false)
    : eventLabel(record.eventType)
  const subtitle = record.eventType === 'interval'
    ? `Interval ${record.interval ?? '?'}`
    : record.eventType === 'session_start' ? 'Session envelope' : formatTimestamp(record.logTime)
  const detail = record.eventType === 'interval'
    ? `${formatCount(readNumber(record.data, 'interval_lines'))} lines`
    : eventDetail(record)

  return (
    <button
      class={`record-list-item ${selected ? 'selected' : ''} event-${record.eventType}`}
      type="button"
      onClick={onSelect}
      aria-current={selected ? 'true' : undefined}
    >
      <Icon size={16} aria-hidden="true" />
      <span class="record-list-copy">
        <strong>{title}</strong>
        <small>{subtitle}</small>
      </span>
      <span class="record-list-value">{detail}</span>
    </button>
  )
}

export function renderRecordListItem(
  record: PattyLogRecord,
  selected: boolean,
  onSelect: () => void,
) {
  return (
    <RecordListItem
      key={record.id}
      record={record}
      selected={selected}
      onSelect={onSelect}
    />
  )
}

export function RecordTitle({ record }: { record: PattyLogRecord }) {
  if (record.eventType === 'interval') {
    return (
      <div>
        <h1>{formatTimestamp(record.logTime, false)}</h1>
        <p>line {record.lineNumber} · interval {record.interval ?? '?'}</p>
      </div>
    )
  }
  return (
    <div>
      <span class="eyebrow">{eventLabel(record.eventType)}</span>
      <h1>{eventDetail(record)}</h1>
      <p>{record.eventType === 'session_start'
        ? `Session envelope · line ${record.lineNumber}`
        : `${formatTimestamp(record.logTime)} · line ${record.lineNumber}`}</p>
    </div>
  )
}

export function RecordOverview({
  record,
  enabledMetricLanes,
  onToggleMetricLane,
}: {
  record: PattyLogRecord
  enabledMetricLanes: IntervalMetricLaneKey[]
  onToggleMetricLane: (key: IntervalMetricLaneKey) => void
}) {
  const unsupported = record.schemaVersion !== 4
  return (
    <div class="overview-pane">
      {unsupported ? (
        <div class="schema-warning">
          Structured presentation supports schema 4. This record remains available in the Record tab.
        </div>
      ) : null}
      {!unsupported && record.eventType === 'interval' ? (
        <IntervalOverview
          data={record.data}
          enabledMetricLanes={enabledMetricLanes}
          onToggleMetricLane={onToggleMetricLane}
        />
      ) : null}
      {!unsupported && record.eventType === 'session_start' ? <SessionOverview data={record.data} /> : null}
      {!unsupported && record.eventType === 'control_command' ? <CommandOverview data={record.data} /> : null}
      {!unsupported && record.eventType === 'alert' ? <AlertOverview data={record.data} /> : null}
      {!unsupported && !isKnownEventType(record.eventType) ? (
        <div class="schema-warning">Unknown event type. Inspect the complete emitted record in the Record tab.</div>
      ) : null}
    </div>
  )
}

function IntervalLaneOptions({
  enabledMetricLanes,
  onToggleMetric,
}: {
  enabledMetricLanes: IntervalMetricLaneKey[]
  onToggleMetric: (key: IntervalMetricLaneKey) => void
}) {
  return (
    <section class="traffic-section tracked-lane-section metric-only" aria-label="Tracked interval lanes">
      <div class="section-heading-row">
        <h2>Tracked lanes</h2>
      </div>
      <div class="metric-lane-options" aria-label="Optional interval metrics">
        {intervalMetricLanes.map((lane) => {
          return (
            <IntervalLaneOption
              label={lane.label}
              color={lane.color}
              enabled={enabledMetricLanes.includes(lane.key)}
              onToggle={() => onToggleMetric(lane.key)}
              key={lane.key}
            />
          )
        })}
      </div>
    </section>
  )
}

function IntervalLaneOption({
  label,
  color,
  enabled,
  onToggle,
}: {
  label: string
  color: string
  enabled: boolean
  onToggle: () => void
}) {
  return (
    <label title={`${enabled ? 'Remove' : 'Add'} ${label} ${enabled ? 'from' : 'to'} the interval map`}>
      <input
        type="checkbox"
        aria-label={`${enabled ? 'Hide' : 'Show'} ${label} in interval map`}
        checked={enabled}
        onChange={onToggle}
      />
      <span class="color-swatch" style={{ backgroundColor: color }} aria-hidden="true" />
      <span>{label}</span>
    </label>
  )
}

function IntervalOverview({
  data,
  enabledMetricLanes,
  onToggleMetricLane,
}: {
  data: JsonObject
  enabledMetricLanes: IntervalMetricLaneKey[]
  onToggleMetricLane: (key: IntervalMetricLaneKey) => void
}) {
  const summary = readObject(data, 'summary')
  const runtime = {
    ...(readObject(data, 'runtime') ?? {}),
    schema_version: readNumber(data, 'schema_version'),
  }
  const matchers = readArray(data, 'matchers')
  const factoids = readArray(data, 'factoids')
  const selected = readObject(data, 'selected')

  return (
    <>
      <section class="metric-band" aria-label="Interval traffic totals">
        <Metric label="Interval lines" value={formatCount(readNumber(data, 'interval_lines'))} />
        <Metric label="Total lines" value={formatCount(readNumber(data, 'total_lines'))} />
        <Metric label="Total bytes" value={formatBytes(readNumber(data, 'total_bytes'))} />
        <Metric label="Unmarked" value={formatCount(readNumber(data, 'unmarked'))} />
        <Metric label="Logical cycle" value={formatCount(readNumber(data, 'logical_cycles'))} />
        <Metric label="Phase" value={readString(data, 'phase') || '—'} />
      </section>
      <IntervalLaneOptions
        enabledMetricLanes={enabledMetricLanes}
        onToggleMetric={onToggleMetricLane}
      />
      <OverviewSection title="Summary" object={summary} />
      <OverviewSection title="Runtime" object={runtime} />
      <section class="overview-section">
        <h2>Recorded detail</h2>
        <div class="inventory-row">
          <Inventory label="Matchers" value={matchers.length} />
          <Inventory label="Factoids" value={factoids.length} />
          <Inventory label="Selected context" value={selected && Object.keys(selected).length > 0 ? 1 : 0} />
        </div>
      </section>
    </>
  )
}

function SessionOverview({ data }: { data: JsonObject }) {
  return <OverviewSection title="Session" object={data} omit={['args']} />
}

function CommandOverview({ data }: { data: JsonObject }) {
  return (
    <>
      <section class="metric-band">
        <Metric label="Command" value={readString(data, 'command_name') || '—'} />
        <Metric label="Status" value={readString(data, 'status') || '—'} />
        <Metric label="Source" value={readString(data, 'source') || '—'} />
        <Metric label="Control file" value={readBoolean(data, 'control_file_enabled') === true ? 'on' : 'off'} />
      </section>
      <OverviewSection title="Result" object={readObject(data, 'result')} />
      <section class="overview-section">
        <h2>Command text</h2>
        <pre class="command-text">{readString(data, 'command') || '—'}</pre>
      </section>
    </>
  )
}

function AlertOverview({ data }: { data: JsonObject }) {
  return (
    <section class="metric-band">
      <Metric label="Matcher" value={readString(data, 'matcher') || '—'} />
      <Metric label="Status" value={readString(data, 'status') || '—'} />
      <Metric label="Direction" value={readString(data, 'direction') || '—'} />
      <Metric label="Value" value={formatCount(readNumber(data, 'value'))} />
      <Metric label="Threshold" value={formatCount(readNumber(data, 'threshold'))} />
      <Metric label="Streak" value={formatCount(readNumber(data, 'streak'))} />
      <Metric label="Flux depth" value={formatCount(readNumber(data, 'flux_depth'))} />
      <Metric label="Interval" value={formatCount(readNumber(data, 'interval'))} />
      <Metric label="Cycle" value={formatCount(readNumber(data, 'current_cycle'))} />
    </section>
  )
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div class="metric">
      <span>{label}</span>
      <strong title={value}>{value}</strong>
    </div>
  )
}

function Inventory({ label, value }: { label: string; value: number }) {
  return (
    <div>
      <strong>{value}</strong>
      <span>{label}</span>
    </div>
  )
}

function OverviewSection({ title, object, omit = [] }: {
  title: string
  object: JsonObject | null
  omit?: string[]
}) {
  if (!object) {
    return null
  }
  const entries = Object.entries(object).filter(([key]) => !omit.includes(key))
  return (
    <section class="overview-section">
      <h2>{title}</h2>
      <dl class="field-table">
        {entries.map(([key, value]) => (
          <div key={key}>
            <dt>{humanize(key)}</dt>
            <dd title={displayValue(value)}>{displayValue(value)}</dd>
          </div>
        ))}
      </dl>
    </section>
  )
}

export function RawRecord({ record }: { record: PattyLogRecord }) {
  return (
    <div class="raw-pane">
      <pre>{JSON.stringify(record.data, null, 2)}</pre>
    </div>
  )
}

function eventLabel(eventType: string): string {
  switch (eventType) {
    case 'session_start': return 'Session start'
    case 'control_command': return 'Control command'
    case 'interval': return 'Interval'
    case 'alert': return 'Alert transition'
    default: return eventType || 'Unknown record'
  }
}

function eventDetail(record: PattyLogRecord): string {
  switch (record.eventType) {
    case 'session_start': return readString(record.data, 'version') || 'PattyGraph session'
    case 'control_command': return readString(record.data, 'command_name') || 'command'
    case 'alert': return `${readString(record.data, 'matcher') || 'matcher'} ${readString(record.data, 'status') || ''}`.trim()
    default: return record.eventType
  }
}

function formatTimestamp(value: string, includeSeconds = true): string {
  if (!value) {
    return 'No timestamp'
  }
  const parsed = new Date(value)
  if (Number.isNaN(parsed.getTime())) {
    return value
  }
  const options: Intl.DateTimeFormatOptions = {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }
  if (includeSeconds) {
    options.second = '2-digit'
  }
  return new Intl.DateTimeFormat(undefined, options).format(parsed)
}

function humanize(value: string): string {
  return value.replaceAll('_', ' ').replace(/^./, (letter) => letter.toUpperCase())
}

function displayValue(value: JsonValue): string {
  if (Array.isArray(value)) {
    return `${value.length} entries`
  }
  if (value !== null && typeof value === 'object') {
    return `${Object.keys(value).length} fields`
  }
  if (typeof value === 'boolean') {
    return value ? 'on' : 'off'
  }
  if (value === null || value === '') {
    return '—'
  }
  return String(value)
}

