import { useMemo, useState } from 'preact/hooks'
import {
  alertStatusSummary,
  type AlertTimeline,
  type RecordedAlert,
} from '../domain/alertTimeline'
import {
  buildIntervalSeries,
  emittedInterestingScore,
  emittedMatcherColor,
  emittedMatcherCount,
  laneHeight,
  type IntervalPoint,
} from '../domain/intervalSeries'
import {
  intervalMetricLanes,
  maximumTrackedLanes,
  optionalCoreIntervalLanes,
  type CoreIntervalLaneKey,
  type IntervalMetricLaneKey,
  type TrackedLane,
} from '../domain/trackedLane'
import type { ChangePoint } from '../domain/changeAnalysis'
import { qualifiesChangeThreshold } from '../domain/changeThreshold'
import type { SearchResultsByInterval } from '../domain/searchSession'
import type { PattyLogRecord } from '../domain/types'

const laneHeightPixels = 16
const laneMaximumHeight = 12

interface RenderedTrackedLane {
  id: string
  label: string
  color: string
  values: Array<number | null>
  maximum: number
}

export function IntervalMap({
  intervals,
  alertsByInterval,
  selectedRecord,
  trackedLanes,
  enabledCoreLanes,
  enabledMetricLanes,
  searchActive,
  searchResultsByInterval,
  changeSeries,
  changeThreshold,
  comparisonActive,
  comparisonTargeting,
  comparisonActiveRecord,
  comparisonOriginRecord,
  onSelect,
}: {
  intervals: PattyLogRecord[]
  alertsByInterval: AlertTimeline
  selectedRecord: PattyLogRecord | null
  trackedLanes: TrackedLane[]
  enabledCoreLanes: CoreIntervalLaneKey[]
  enabledMetricLanes: IntervalMetricLaneKey[]
  searchActive: boolean
  searchResultsByInterval: SearchResultsByInterval
  changeSeries: ChangePoint[]
  changeThreshold: number
  comparisonActive: boolean
  comparisonTargeting: boolean
  comparisonActiveRecord: PattyLogRecord | null
  comparisonOriginRecord: PattyLogRecord | null
  onSelect: (record: PattyLogRecord) => void
}) {
  const points = useMemo(() => buildIntervalSeries(intervals).reverse(), [intervals])
  const renderedCoreLanes: Array<{ key: 'lines' | CoreIntervalLaneKey; label: string }> = [
    { key: 'lines', label: 'Lines' },
    ...optionalCoreIntervalLanes.filter((lane) => enabledCoreLanes.includes(lane.key)),
  ]
  const renderedMetricLanes = intervalMetricLanes.filter((lane) => enabledMetricLanes.includes(lane.key))
  const renderedLanes = useMemo<RenderedTrackedLane[]>(() => trackedLanes
    .slice(0, maximumTrackedLanes)
    .map((lane) => {
      const values = points.map((point) => lane.kind === 'matcher'
        ? emittedMatcherCount(point.record, lane.name)
        : emittedInterestingScore(point.record, lane.stream, lane.key))
      const availableValues = values.flatMap((value) => value === null ? [] : [value])
      const color = lane.color || (lane.kind === 'matcher'
        ? points
            .map((point) => emittedMatcherColor(point.record, lane.name))
            .find((candidate) => candidate !== '') ?? ''
        : '')
      return {
        id: lane.id,
        label: lane.label,
        color,
        values,
        maximum: availableValues.length > 0 ? Math.max(...availableValues) : 0,
      }
    }), [trackedLanes, points])
  const showAlertLane = points.some((point) => (
    point.record.interval !== null && (alertsByInterval.get(point.record.interval)?.length ?? 0) > 0
  ))
  const changesByRecord = useMemo(
    () => new Map(changeSeries.map((point) => [point.recordId, point])),
    [changeSeries],
  )
  const pointChanges = points.map((point) => changesByRecord.get(point.record.id) ?? null)
  const changeContextValues = pointChanges.map((change) => (
    change?.resetPhase === null && change.score !== null && !qualifiesChangeThreshold(change.score, changeThreshold)
      ? change.score
      : null
  ))
  const changeQualifiedValues = pointChanges.map((change) => (
    change?.resetPhase === null && change.score !== null && qualifiesChangeThreshold(change.score, changeThreshold)
      ? change.score
      : null
  ))
  const changeLaneOffset = laneHeightPixels
  const alertLaneOffset = showAlertLane ? laneHeightPixels : 0
  const searchLaneOffset = searchActive ? laneHeightPixels : 0
  const addedLaneCount = 1 + (showAlertLane ? 1 : 0) + (searchActive ? 1 : 0) + renderedMetricLanes.length + renderedLanes.length
  const totalLaneCount = renderedCoreLanes.length + addedLaneCount
  const coreGraphHeight = renderedCoreLanes.length * laneHeightPixels
  const changeLaneTop = coreGraphHeight
  const alertLaneTop = changeLaneTop + changeLaneOffset
  const searchLaneTop = alertLaneTop + alertLaneOffset
  const metricLaneTop = searchLaneTop + searchLaneOffset
  const graphHeight = totalLaneCount * laneHeightPixels
  const [hoveredIndex, setHoveredIndex] = useState<number | null>(null)
  const activeSelectionRecord = comparisonActive ? comparisonActiveRecord : selectedRecord
  const selectedIndex = points.findIndex((point) => (
    point.record.id === activeSelectionRecord?.id || (
      activeSelectionRecord?.schemaVersion === 4 &&
      activeSelectionRecord.eventType === 'alert' &&
      activeSelectionRecord.interval !== null &&
      point.record.interval === activeSelectionRecord.interval
    ) || (searchResultsByInterval.get(point.record.id)?.some((result) => result.record.id === activeSelectionRecord?.id) ?? false)
  ))
  const comparisonOriginIndex = comparisonActive
    ? points.findIndex((point) => point.record.id === comparisonOriginRecord?.id)
    : -1
  const hoverVisible = hoveredIndex !== null &&
    hoveredIndex !== selectedIndex &&
    (!comparisonTargeting || hoveredIndex !== comparisonOriginIndex)
  const activeIndex = hoveredIndex ?? (selectedIndex >= 0 ? selectedIndex : 0)
  const activePoint = points[activeIndex]
  const activeAlerts = activePoint?.record.interval === null || activePoint === undefined
    ? []
    : alertsByInterval.get(activePoint.record.interval) ?? []
  const activeSearchResults = activePoint
    ? searchResultsByInterval.get(activePoint.record.id) ?? []
    : []
  const activeChange = activePoint ? changesByRecord.get(activePoint.record.id) ?? null : null
  const intervalNumbers = points.flatMap((point) => point.record.interval === null ? [] : [point.record.interval])

  const lineValues = points.flatMap((point) => point.lines === null ? [] : [point.lines])
  const byteValues = points.flatMap((point) => point.bytes === null ? [] : [point.bytes])
  const errorValues = points.flatMap((point) => point.errors === null ? [] : [point.errors])
  const lineMinimum = lineValues.length > 0 ? Math.min(...lineValues) : 0
  const lineMaximum = lineValues.length > 0 ? Math.max(...lineValues) : 0
  const byteMinimum = byteValues.length > 0 ? Math.min(...byteValues) : 0
  const byteMaximum = byteValues.length > 0 ? Math.max(...byteValues) : 0
  const errorMaximum = errorValues.length > 0 ? Math.max(...errorValues) : 0
  const coreRanges = {
    lines: { minimum: lineMinimum, maximum: lineMaximum },
    bytes: { minimum: byteMinimum, maximum: byteMaximum },
    errors: { minimum: 0, maximum: errorMaximum },
  }

  if (points.length === 0) {
    return null
  }

  const pointFromPointer = (event: { clientX: number; currentTarget: HTMLElement }) => {
    const bounds = event.currentTarget.getBoundingClientRect()
    const ratio = bounds.width > 0 ? (event.clientX - bounds.left) / bounds.width : 0
    return Math.max(0, Math.min(points.length - 1, Math.floor(ratio * points.length)))
  }

  const pointerIsInSearchLane = (event: { clientY: number; currentTarget: HTMLElement }) => {
    if (!searchActive) {
      return false
    }
    const bounds = event.currentTarget.getBoundingClientRect()
    const y = event.clientY - bounds.top
    return y >= searchLaneTop && y < searchLaneTop + laneHeightPixels
  }

  const selectIndex = (index: number) => {
    const point = points[Math.max(0, Math.min(points.length - 1, index))]
    if (point) {
      onSelect(point.record)
    }
  }

  const handleKey = (event: KeyboardEvent) => {
    const current = selectedIndex >= 0 ? selectedIndex : 0
    let next: number | null = null
    switch (event.key) {
      case 'ArrowLeft': next = current - 1; break
      case 'ArrowRight': next = current + 1; break
      case 'Home': next = 0; break
      case 'End': next = points.length - 1; break
      default: return
    }
    event.preventDefault()
    setHoveredIndex(null)
    selectIndex(next)
  }

  return (
    <section
      class={comparisonTargeting ? 'interval-map comparison-targeting' : 'interval-map'}
      aria-label="Interval navigation map"
    >
      <div class="interval-map-heading">
        <div>
          <strong>Interval map</strong>
          <span aria-live="polite">
            {comparisonTargeting
              ? 'Select comparison interval'
              : `${points.length} selectable intervals · latest to oldest`}
          </span>
        </div>
        {activePoint ? (
          <IntervalReadout
            point={activePoint}
            enabledCoreLanes={enabledCoreLanes}
            metricLanes={renderedMetricLanes}
            lanes={renderedLanes}
            alerts={activeAlerts}
            searchResultCount={activeSearchResults.length}
            change={activeChange}
            activeIndex={activeIndex}
          />
        ) : null}
      </div>
      <div class="interval-map-graph" style={{ height: `${graphHeight}px` }}>
        <div
          class="interval-map-labels"
          style={{ gridTemplateRows: `repeat(${totalLaneCount}, ${laneHeightPixels}px)` }}
          aria-hidden="true"
        >
          {renderedCoreLanes.map((lane) => (
            <span class={`core-lane-label core-lane-${lane.key}`} key={lane.key}>{lane.label.toUpperCase()}</span>
          ))}
          <span class="change-lane-label">CHANGE</span>
          {showAlertLane ? <span class="alert-lane-label">ALERTS</span> : null}
          {searchActive ? <span class="search-lane-label">SEARCH</span> : null}
          {renderedMetricLanes.map((lane) => (
            <span style={{ color: lane.color }} key={lane.key}>{lane.label.toUpperCase()}</span>
          ))}
          {renderedLanes.map((lane) => (
            <span
              class="tracked-label"
              style={lane.color ? { color: lane.color } : undefined}
              title={lane.label}
              key={lane.id}
            >
              {lane.label}
            </span>
          ))}
        </div>
        <div
          class={comparisonTargeting ? 'interval-map-track comparison-targeting' : 'interval-map-track'}
          style={{ height: `${graphHeight}px` }}
          role="slider"
          tabIndex={0}
          aria-label="Select a PattyLog interval"
          aria-valuemin={intervalNumbers.length > 0 ? Math.min(...intervalNumbers) : 0}
          aria-valuemax={intervalNumbers.length > 0 ? Math.max(...intervalNumbers) : points.length - 1}
          aria-valuenow={activePoint?.record.interval ?? activeIndex}
          aria-valuetext={activePoint
            ? intervalReadout(activePoint, enabledCoreLanes, renderedMetricLanes, renderedLanes, activeIndex, activeAlerts, activeSearchResults.length, activeChange)
            : undefined}
          onPointerMove={(event) => setHoveredIndex(pointFromPointer(event))}
          onPointerLeave={() => setHoveredIndex(null)}
          onClick={(event) => {
            const index = pointFromPointer(event)
            if (pointerIsInSearchLane(event)) {
              const hits = searchResultsByInterval.get(points[index]?.record.id ?? '') ?? []
              if (hits.length > 0) {
                const command = hits.find((hit) => hit.record.eventType === 'control_command')
                onSelect(command?.record ?? hits[0].record)
              }
              return
            }
            selectIndex(index)
          }}
          onKeyDown={handleKey}
        >
          <svg
            viewBox={`0 0 ${points.length} ${graphHeight}`}
            preserveAspectRatio="none"
            aria-hidden="true"
          >
            <rect class="interval-map-background" x="0" y="0" width={points.length} height={graphHeight} />
            {Array.from({ length: totalLaneCount - 1 }, (_, index) => (
              <line
                class="interval-map-separator"
                x1="0"
                y1={(index + 1) * laneHeightPixels}
                x2={points.length}
                y2={(index + 1) * laneHeightPixels}
                key={`separator-${index}`}
              />
            ))}
            {comparisonOriginIndex >= 0 ? (
              <line
                class="interval-map-comparison-origin-rail"
                x1={comparisonOriginIndex + 0.5}
                y1="1"
                x2={comparisonOriginIndex + 0.5}
                y2={graphHeight - 1}
                vector-effect="non-scaling-stroke"
              />
            ) : null}
            {selectedIndex >= 0 ? (
              <line
                class={comparisonActive ? 'interval-map-selected-rail comparison' : 'interval-map-selected-rail'}
                x1={selectedIndex + 0.5}
                y1="1"
                x2={selectedIndex + 0.5}
                y2={graphHeight - 1}
                vector-effect="non-scaling-stroke"
              />
            ) : null}
            {renderedCoreLanes.map((lane, index) => (
              <path
                class={`interval-map-${lane.key}`}
                d={verticalPath(
                  points,
                  lane.key,
                  (index + 1) * laneHeightPixels - 2,
                  laneMaximumHeight,
                  coreRanges[lane.key].minimum,
                  coreRanges[lane.key].maximum,
                )}
                vector-effect="non-scaling-stroke"
                key={`path-${lane.key}`}
              />
            ))}
            <path
              class="interval-map-change-context"
              d={verticalValuesPath(
                changeContextValues,
                changeLaneTop + laneHeightPixels - 2,
                laneMaximumHeight,
                100,
              )}
              vector-effect="non-scaling-stroke"
            />
            <path
              class="interval-map-change-qualified"
              d={verticalValuesPath(
                changeQualifiedValues,
                changeLaneTop + laneHeightPixels - 2,
                laneMaximumHeight,
                100,
              )}
              vector-effect="non-scaling-stroke"
            />
            {pointChanges.map((change, index) => change?.resetPhase ? (
              <line
                class={`interval-map-change-reset ${change.resetPhase}`}
                x1={index + 0.5}
                y1={changeLaneTop + 2}
                x2={index + 0.5}
                y2={changeLaneTop + laneHeightPixels - 2}
                vector-effect="non-scaling-stroke"
                key={`change-reset-${change.recordId}`}
              >
                <title>{changeResetLabel(change.resetPhase)}; raw Change {Math.round(change.score ?? 0)}</title>
              </line>
            ) : null)}
            {showAlertLane ? points.map((point, index) => (
              <AlertMarker
                alerts={point.record.interval === null ? [] : alertsByInterval.get(point.record.interval) ?? []}
                index={index}
                laneTop={alertLaneTop}
                key={`alerts-${point.record.id}`}
              />
            )) : null}
            {searchActive ? (
              <path
                class="interval-map-search"
                d={verticalValuesPath(
                  points.map((point) => searchResultsByInterval.has(point.record.id) ? 1 : null),
                  searchLaneTop + laneHeightPixels - 2,
                  laneMaximumHeight,
                  1,
                )}
                vector-effect="non-scaling-stroke"
              />
            ) : null}
            {renderedMetricLanes.map((lane, index) => (
              <path
                class={lane.key === 'markedPercent' ? 'interval-map-marked' : 'interval-map-b16'}
                d={verticalPath(
                  points,
                  lane.key,
                  metricLaneTop + (index + 1) * laneHeightPixels - 2,
                  laneMaximumHeight,
                  0,
                  100,
                )}
                vector-effect="non-scaling-stroke"
                key={`path-${lane.key}`}
              />
            ))}
            {renderedLanes.map((lane, index) => (
              <path
                class="interval-map-tracked"
                d={verticalValuesPath(
                  lane.values,
                  metricLaneTop + (renderedMetricLanes.length + index + 1) * laneHeightPixels - 2,
                  laneMaximumHeight,
                  lane.maximum,
                )}
                style={{ stroke: lane.color || '#aab2ac' }}
                vector-effect="non-scaling-stroke"
                key={`path-${lane.id}`}
              />
            ))}
            {selectedIndex >= 0 ? (
              <>
                {comparisonOriginIndex >= 0 ? (
                  <>
                    <line
                      class="interval-map-comparison-origin-cap"
                      x1={comparisonOriginIndex + 0.5}
                      y1="1"
                      x2={comparisonOriginIndex + 0.5}
                      y2="5"
                      vector-effect="non-scaling-stroke"
                    />
                    <line
                      class="interval-map-comparison-origin-cap"
                      x1={comparisonOriginIndex + 0.5}
                      y1={graphHeight - 5}
                      x2={comparisonOriginIndex + 0.5}
                      y2={graphHeight - 1}
                      vector-effect="non-scaling-stroke"
                    />
                  </>
                ) : null}
                <line
                  class={comparisonActive ? 'interval-map-selected-cap comparison' : 'interval-map-selected-cap'}
                  x1={selectedIndex + 0.5}
                  y1="1"
                  x2={selectedIndex + 0.5}
                  y2="5"
                  vector-effect="non-scaling-stroke"
                />
                <line
                  class={comparisonActive ? 'interval-map-selected-cap comparison' : 'interval-map-selected-cap'}
                  x1={selectedIndex + 0.5}
                  y1={graphHeight - 5}
                  x2={selectedIndex + 0.5}
                  y2={graphHeight - 1}
                  vector-effect="non-scaling-stroke"
                />
              </>
            ) : null}
            {hoverVisible ? (
              <line
                class={comparisonTargeting ? 'interval-map-hover comparison-target' : 'interval-map-hover'}
                x1={hoveredIndex + 0.5}
                y1="1"
                x2={hoveredIndex + 0.5}
                y2={graphHeight - 1}
                vector-effect="non-scaling-stroke"
              />
            ) : null}
          </svg>
        </div>
      </div>
    </section>
  )
}

function AlertMarker({
  alerts,
  index,
  laneTop,
}: {
  alerts: RecordedAlert[]
  index: number
  laneTop: number
}) {
  if (alerts.length === 0) {
    return null
  }
  const x = index + 0.5
  const triggered = alerts.some((alert) => alert.status === 'triggered')
  const recovered = alerts.some((alert) => alert.status === 'recovered')
  const unknown = alerts.some((alert) => alert.status !== 'triggered' && alert.status !== 'recovered')

  if (triggered && recovered) {
    return (
      <>
        <line class="interval-map-alert-triggered" x1={x} y1={laneTop + 2} x2={x} y2={laneTop + 7} vector-effect="non-scaling-stroke" />
        <line class="interval-map-alert-recovered" x1={x} y1={laneTop + 9} x2={x} y2={laneTop + 14} vector-effect="non-scaling-stroke" />
      </>
    )
  }
  const className = triggered
    ? 'interval-map-alert-triggered'
    : recovered ? 'interval-map-alert-recovered' : unknown ? 'interval-map-alert-unknown' : ''
  return className
    ? <line class={className} x1={x} y1={laneTop + 2} x2={x} y2={laneTop + 14} vector-effect="non-scaling-stroke" />
    : null
}

function verticalPath(
  points: IntervalPoint[],
  field: 'lines' | 'bytes' | 'errors' | 'markedPercent' | 'b16Percent',
  bottom: number,
  availableHeight: number,
  minimum: number,
  maximum: number,
): string {
  return points.map((point, index) => {
    const height = laneHeight(point[field], minimum, maximum, availableHeight)
    return height > 0 ? `M${index + 0.5} ${bottom}V${bottom - height}` : ''
  }).filter(Boolean).join('')
}

function verticalValuesPath(
  values: Array<number | null>,
  bottom: number,
  availableHeight: number,
  maximum: number,
): string {
  return values.map((value, index) => {
    const height = laneHeight(value, 0, maximum, availableHeight)
    return height > 0 ? `M${index + 0.5} ${bottom}V${bottom - height}` : ''
  }).filter(Boolean).join('')
}

function IntervalReadout({
  point,
  enabledCoreLanes,
  metricLanes,
  lanes,
  alerts,
  searchResultCount,
  change,
  activeIndex,
}: {
  point: IntervalPoint
  enabledCoreLanes: CoreIntervalLaneKey[]
  metricLanes: typeof intervalMetricLanes[number][]
  lanes: RenderedTrackedLane[]
  alerts: RecordedAlert[]
  searchResultCount: number
  change: ChangePoint | null
  activeIndex: number
}) {
  return (
    <div class="interval-map-readout">
      <strong>Interval {point.record.interval ?? '?'}</strong>
      <span>{formatCount(point.lines)} lines</span>
      {enabledCoreLanes.includes('bytes') ? <span class="byte-value">{formatBytes(point.bytes)}</span> : null}
      {enabledCoreLanes.includes('errors') ? <span class="error-value">{formatCount(point.errors)} errs</span> : null}
      {alerts.length > 0 ? <span class="alert-value">{alertStatusSummary(alerts)}</span> : null}
      {searchResultCount > 0 ? <span class="search-value">{searchResultCount} search</span> : null}
      {change?.score !== null && change?.score !== undefined ? (
        <span class={change.resetPhase ? 'change-value reset' : 'change-value'} title={changeTooltip(change)}>
          {change.resetPhase ? `${changeResetLabel(change.resetPhase)} · raw ` : ''}
          Change {Math.round(change.score)} · {change.components[0]?.label ?? 'change'}
        </span>
      ) : null}
      {metricLanes.map((lane) => (
        <span
          class={lane.key === 'markedPercent' ? 'marked-value' : 'b16-value'}
          key={lane.key}
        >
          {formatPercent(point[lane.key])} {metricReadoutLabel(lane.key)}
        </span>
      ))}
      {lanes.map((lane) => (
        <span
          class="tracked-value"
          style={lane.color ? { color: lane.color } : undefined}
          key={lane.id}
        >
          {lane.label} {formatTrackedValue(lane.values[activeIndex] ?? null)}
        </span>
      ))}
    </div>
  )
}

function intervalReadout(
  point: IntervalPoint,
  enabledCoreLanes: CoreIntervalLaneKey[],
  metricLanes: typeof intervalMetricLanes[number][],
  lanes: RenderedTrackedLane[],
  activeIndex: number,
  alerts: RecordedAlert[],
  searchResultCount: number,
  change: ChangePoint | null,
): string {
  const metrics = metricLanes.map((lane) => (
    `${formatPercent(point[lane.key])} ${metricReadoutLabel(lane.key)}`
  ))
  const tracked = lanes.map((lane) => (
    `${lane.label} ${formatTrackedValue(lane.values[activeIndex] ?? null)}`
  ))
  const alertText = alerts.length > 0
    ? [`${alerts.length} alert ${alerts.length === 1 ? 'transition' : 'transitions'}: ${alerts.map((alert) => alertStatusSummary([alert])).join(', ')}`]
    : []
  const searchText = searchResultCount > 0 ? [`${searchResultCount} search ${searchResultCount === 1 ? 'record' : 'records'}`] : []
  const changeText = change?.score !== null && change?.score !== undefined
    ? [`${change.resetPhase ? `${changeResetLabel(change.resetPhase)}, raw ` : ''}change ${Math.round(change.score)}, primarily ${change.components[0]?.label ?? 'change'}`]
    : []
  return [
    `Interval ${point.record.interval ?? '?'}`,
    `${formatCount(point.lines)} lines`,
    ...(enabledCoreLanes.includes('bytes') ? [`${formatBytes(point.bytes)} served`] : []),
    ...(enabledCoreLanes.includes('errors') ? [`${formatCount(point.errors)} errors`] : []),
    ...alertText,
    ...searchText,
    ...changeText,
    ...metrics,
    ...tracked,
  ].join(', ')
}

function changeTooltip(change: ChangePoint): string {
  const components = change.components
    .map((component) => `${component.label} ${Math.round(component.score)}`)
    .join(' · ')
  return change.resetPhase ? `${changeResetLabel(change.resetPhase)} · raw calculation · ${components}` : components
}

function changeResetLabel(phase: NonNullable<ChangePoint['resetPhase']>): string {
  return phase === 'reset' ? 'Peak reset' : 'Peak rebaseline'
}

function metricReadoutLabel(key: IntervalMetricLaneKey): string {
  return key === 'markedPercent' ? 'marked' : 'b16'
}

function formatTrackedValue(value: number | null): string {
  return formatCount(value)
}

function formatCount(value: number | null): string {
  return value === null ? 'unavailable' : new Intl.NumberFormat().format(value)
}

function formatBytes(value: number | null): string {
  if (value === null) {
    return 'bytes unavailable'
  }
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB']
  let amount = value
  let unit = 0
  while (amount >= 1024 && unit < units.length - 1) {
    amount /= 1024
    unit += 1
  }
  const precision = unit === 0 || amount >= 10 ? 0 : 1
  return `${amount.toFixed(precision)} ${units[unit]}`
}

function formatPercent(value: number | null): string {
  return value === null ? 'unavailable' : `${value.toFixed(1)}%`
}
