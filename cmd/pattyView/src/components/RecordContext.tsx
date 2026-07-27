import { ChevronDown, ChevronUp } from 'lucide-preact'
import { useMemo, useState } from 'preact/hooks'
import { formatCount } from '../displayFormat'
import type { RecordedAlert } from '../domain/alertTimeline'
import { projectFactoids, type TrafficFactoid } from '../domain/trafficDetail'
import type { PattyLogRecord } from '../domain/types'

export function AlertStrip({
  alerts,
  selectedRecordId,
  onSelect,
}: {
  alerts: RecordedAlert[]
  selectedRecordId: string
  onSelect: (record: PattyLogRecord) => void
}) {
  if (alerts.length === 0) {
    return null
  }
  return (
    <section class="alert-strip" aria-label={`Alerts for interval ${alerts[0].interval}`}>
      <span class="alert-strip-label">ALERTS</span>
      <div class="alert-strip-list">
        {alerts.map((alert) => {
          const selected = alert.record.id === selectedRecordId
          return (
            <button
              class={`alert-strip-item status-${alert.status || 'unknown'} ${selected ? 'selected' : ''}`}
              type="button"
              aria-current={selected ? 'true' : undefined}
              aria-label={`Open ${alert.matcher || 'matcher'} ${alert.status || 'alert'} transition`}
              onClick={() => onSelect(alert.record)}
              key={alert.record.id}
            >
              <span class="alert-strip-status">{(alert.status || 'alert').toUpperCase()}</span>
              <strong>{alert.matcher || 'matcher'}</strong>
              <span>{alert.direction || 'bound'} {formatCount(alert.threshold)}</span>
              <span>value {formatCount(alert.value)}</span>
              <time dateTime={alert.logTime}>{formatTimeWithSeconds(alert.logTime)}</time>
            </button>
          )
        })}
      </div>
    </section>
  )
}

const factoidDividerColors = ['#4ecdc4', '#f0b35a', '#d69bdc', '#79a9e8', '#a8c76f']

export function FactoidRibbon({ record }: { record: PattyLogRecord }) {
  const [expanded, setExpanded] = useState(false)
  const factoids = useMemo(() => projectFactoids(record.data), [record.id, record.data])

  if (factoids.length === 0) {
    return null
  }

  const countLabel = `${factoids.length} ${factoids.length === 1 ? 'factoid' : 'factoids'}`
  return (
    <section class={`factoid-ribbon ${expanded ? 'expanded' : ''}`} aria-label="Recorded factoids">
      <span class="factoid-ribbon-label">FACTOIDS</span>
      <div class="factoid-crawl" aria-hidden={expanded ? 'true' : undefined}>
        {factoids.map((factoid, index) => (
          <FactoidText factoid={factoid} index={index} key={`${factoid.name}-${index}`} />
        ))}
      </div>
      <button
        class="factoid-toggle"
        type="button"
        title={`${expanded ? 'Collapse' : 'Expand'} ${countLabel}`}
        aria-label={`${expanded ? 'Collapse' : 'Expand'} ${countLabel}`}
        aria-expanded={expanded}
        onClick={() => setExpanded((current) => !current)}
      >
        <span>{factoids.length}</span>
        {expanded ? <ChevronUp size={14} aria-hidden="true" /> : <ChevronDown size={14} aria-hidden="true" />}
      </button>
      {expanded ? (
        <div class="factoid-expanded-list">
          {factoids.map((factoid, index) => (
            <div key={`${factoid.name}-${index}`} style={{ borderLeftColor: factoidDividerColors[index % factoidDividerColors.length] }}>
              <span>{factoid.name || 'fact'}</span>
              <strong>{factoid.text}</strong>
            </div>
          ))}
        </div>
      ) : null}
    </section>
  )
}

function FactoidText({ factoid, index }: { factoid: TrafficFactoid; index: number }) {
  return (
    <span
      class="factoid-crawl-item"
      title={factoid.text}
      style={{ borderLeftColor: factoidDividerColors[index % factoidDividerColors.length] }}
    >
      {factoid.text}
    </span>
  )
}

function formatTimeWithSeconds(value: string): string {
  if (!value) {
    return 'No time'
  }
  const parsed = new Date(value)
  if (Number.isNaN(parsed.getTime())) {
    return value
  }
  return new Intl.DateTimeFormat(undefined, {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  }).format(parsed)
}

