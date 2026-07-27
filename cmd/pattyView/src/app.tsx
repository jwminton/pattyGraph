import {
  ChevronLeft,
  ChevronRight,
  ChevronsDown,
  FileJson,
  FolderOpen,
  Search,
  X,
} from 'lucide-preact'
import { useEffect, useMemo, useRef, useState } from 'preact/hooks'
import {
  Brand,
  EmptyState,
  IssueBanner,
  LoadingState,
  SourceStatus,
} from './components/AppChrome'
import { ChangeThresholdControl } from './components/ChangeThresholdControl'
import { IntervalComparisonDetail } from './components/IntervalComparison'
import { IntervalMap } from './components/IntervalMap'
import { AlertStrip, FactoidRibbon } from './components/RecordContext'
import {
  RawRecord,
  RecordOverview,
  RecordTitle,
  renderRecordListItem,
} from './components/RecordPresentation'
import { TrafficDetail } from './components/TrafficDetail'
import { VirtualRecordList } from './components/VirtualRecordList'
import { buildAlertTimeline } from './domain/alertTimeline'
import { buildChangeSeries } from './domain/changeAnalysis'
import { buildSessionIndexes, newestRecord, recordIndex } from './domain/model'
import { groupSearchResultsByInterval, searchSessionRecords } from './domain/searchSession'
import {
  maximumTrackedLanes,
  optionalCoreIntervalLanes,
  type CoreIntervalLaneKey,
  type IntervalMetricLaneKey,
  type TrackedLane,
} from './domain/trackedLane'
import type { PattyLogRecord } from './domain/types'
import { usePattyLog } from './hooks/usePattyLog'

type DetailTab = 'overview' | 'traffic' | 'compare' | 'raw'

export function App() {
  const { state, supportsLiveFile, openLive, openSnapshot } = usePattyLog()
  const sessions = useMemo(() => buildSessionIndexes(state.records), [state.records])
  const [selectedSessionId, setSelectedSessionId] = useState('')
  const [selectedRecordId, setSelectedRecordId] = useState('')
  const [followLatest, setFollowLatest] = useState(true)
  const [detailTab, setDetailTab] = useState<DetailTab>('overview')
  const [trackedLanes, setTrackedLanes] = useState<TrackedLane[]>([])
  const [enabledCoreLanes, setEnabledCoreLanes] = useState<CoreIntervalLaneKey[]>(
    optionalCoreIntervalLanes.map((lane) => lane.key),
  )
  const [enabledMetricLanes, setEnabledMetricLanes] = useState<IntervalMetricLaneKey[]>([])
  const [searchText, setSearchText] = useState('')
  const [searchQuery, setSearchQuery] = useState('')
  const [changeThreshold, setChangeThreshold] = useState(40)
  const [comparisonReferenceId, setComparisonReferenceId] = useState('')
  const [comparisonOriginId, setComparisonOriginId] = useState('')
  const [comparisonTargeting, setComparisonTargeting] = useState(false)
  const fileInputRef = useRef<HTMLInputElement>(null)

  const selectedSession = sessions.find((session) => session.id === selectedSessionId)
    ?? sessions[sessions.length - 1]
  const selectedRecord = selectedSession?.records.find((record) => record.id === selectedRecordId)
    ?? newestRecord(selectedSession)
  const changeSeries = useMemo(
    () => buildChangeSeries(selectedSession?.intervals ?? [], selectedSession?.records ?? []),
    [selectedSession?.intervals, selectedSession?.records],
  )
  const changeAvailable = changeSeries.some((point) => point.score !== null)
  const searchResults = useMemo(
    () => searchSessionRecords(selectedSession?.records ?? [], searchQuery),
    [searchQuery, selectedSession?.records],
  )
  const searchResultsByInterval = useMemo(
    () => groupSearchResultsByInterval(searchResults),
    [searchResults],
  )
  const searchActive = searchQuery.trim() !== ''
  const alertTimeline = useMemo(
    () => buildAlertTimeline(selectedSession?.alerts ?? []),
    [selectedSession?.alerts],
  )
  const selectedAlerts = !selectedRecord || selectedRecord.interval === null
    ? []
    : alertTimeline.get(selectedRecord.interval) ?? []
  const selectedIndex = recordIndex(selectedSession, selectedRecord?.id ?? '')
  const trafficAvailable = selectedRecord?.eventType === 'interval' && selectedRecord.schemaVersion === 4
  const comparableIntervals = selectedSession?.intervals.filter((record) => record.schemaVersion === 4) ?? []
  const comparisonAvailable = trafficAvailable && comparableIntervals.length > 1
  const comparisonReference = comparisonAvailable && selectedRecord
    ? comparableIntervals.find((record) => record.id === comparisonReferenceId && record.id !== selectedRecord.id)
      ?? adjacentInterval(comparableIntervals, selectedRecord)
    : null
  const comparisonOrigin = detailTab === 'compare'
    ? comparableIntervals.find((record) => record.id === comparisonOriginId) ?? null
    : null

  useEffect(() => {
    const timer = window.setTimeout(() => setSearchQuery(searchText), 125)
    return () => window.clearTimeout(timer)
  }, [searchText])

  useEffect(() => {
    if (sessions.length === 0) {
      setSelectedSessionId('')
      setSelectedRecordId('')
      return
    }
    if (!sessions.some((session) => session.id === selectedSessionId)) {
      const session = sessions[sessions.length - 1]
      setSelectedSessionId(session.id)
      setSelectedRecordId(newestRecord(session)?.id ?? '')
      setFollowLatest(true)
    }
  }, [sessions, selectedSessionId])

  useEffect(() => {
    if (followLatest && selectedSession) {
      setSelectedRecordId(newestRecord(selectedSession)?.id ?? '')
    }
  }, [followLatest, selectedSession, selectedSession?.records.length])

  useEffect(() => {
    if (!searchActive || searchResults.length !== 1) {
      return
    }

    const result = searchResults[0].record
    setSelectedRecordId((current) => current === result.id ? current : result.id)
    setFollowLatest(false)
  }, [searchActive, searchResults])

  useEffect(() => {
    if (detailTab === 'traffic' && !trafficAvailable) {
      setDetailTab('overview')
    }
    if (detailTab === 'compare' && !comparisonAvailable) {
      const origin = selectedSession?.records.find((record) => record.id === comparisonOriginId)
      if (origin) {
        setSelectedRecordId(origin.id)
        setFollowLatest(origin.id === newestRecord(selectedSession)?.id)
      }
      setComparisonOriginId('')
      setComparisonTargeting(false)
      setDetailTab('overview')
    }
  }, [comparisonAvailable, comparisonOriginId, detailTab, selectedSession, trafficAvailable])

  useEffect(() => {
    setComparisonReferenceId('')
    setComparisonOriginId('')
    setComparisonTargeting(false)
  }, [selectedSession?.id])

  useEffect(() => {
    if (!comparisonTargeting) {
      return
    }
    const cancelTargeting = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        setComparisonTargeting(false)
      }
    }
    window.addEventListener('keydown', cancelTargeting)
    return () => window.removeEventListener('keydown', cancelTargeting)
  }, [comparisonTargeting])

  useEffect(() => {
    if (detailTab === 'compare' && comparisonReference && comparisonReference.id !== comparisonReferenceId) {
      setComparisonReferenceId(comparisonReference.id)
    }
  }, [comparisonReference?.id, comparisonReferenceId, detailTab])

  useEffect(() => {
    const handleKey = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement | null
      if (target?.closest('input, select, textarea, pre, button')) {
        return
      }
      if (event.key === 'ArrowUp' || event.key === 'ArrowLeft') {
        event.preventDefault()
        selectRelative(event.key === 'ArrowLeft' ? 1 : -1)
      } else if (event.key === 'ArrowDown' || event.key === 'ArrowRight') {
        event.preventDefault()
        selectRelative(event.key === 'ArrowRight' ? -1 : 1)
      }
    }
    window.addEventListener('keydown', handleKey)
    return () => window.removeEventListener('keydown', handleKey)
  })

  const selectRecord = (record: PattyLogRecord) => {
    setSelectedRecordId(record.id)
    setFollowLatest(record.id === newestRecord(selectedSession)?.id)
    setComparisonTargeting(false)
  }

  const selectIntervalMapRecord = (record: PattyLogRecord) => {
    if (detailTab === 'compare' && comparisonOrigin) {
      if (record.schemaVersion !== 4) {
        return
      }
      if (comparisonTargeting) {
        if (record.id === comparisonOrigin.id) {
          return
        }
        setComparisonReferenceId(record.id)
        setComparisonTargeting(false)
        return
      }

      selectRecord(record)
      setComparisonOriginId(record.id)
      setComparisonReferenceId('')
      return
    }
    selectRecord(record)
  }

  const selectDetailTab = (next: DetailTab) => {
    if (next === 'compare') {
      if (detailTab !== 'compare' && trafficAvailable && selectedRecord) {
        setComparisonOriginId(selectedRecord.id)
        setComparisonReferenceId('')
      }
      setComparisonTargeting(false)
      setDetailTab(next)
      return
    }

    if (detailTab === 'compare') {
      const origin = selectedSession?.records.find((record) => record.id === comparisonOriginId)
      if (origin) {
        selectRecord(origin)
      }
      setComparisonOriginId('')
    }
    setComparisonTargeting(false)
    setDetailTab(next)
  }

  const selectComparisonReference = (recordId: string) => {
    setComparisonReferenceId(recordId)
    setComparisonTargeting(false)
  }

  const toggleTrackedLane = (lane: TrackedLane) => {
    setTrackedLanes((current) => current.some((candidate) => candidate.id === lane.id)
      ? current.filter((candidate) => candidate.id !== lane.id)
      : current.length < maximumTrackedLanes ? [...current, lane] : current)
  }

  const toggleMetricLane = (key: IntervalMetricLaneKey) => {
    setEnabledMetricLanes((current) => current.includes(key)
      ? current.filter((candidate) => candidate !== key)
      : [...current, key])
  }

  const toggleCoreLane = (key: CoreIntervalLaneKey) => {
    setEnabledCoreLanes((current) => current.includes(key)
      ? current.filter((candidate) => candidate !== key)
      : [...current, key])
  }

  const selectRelative = (direction: -1 | 1) => {
    if (!selectedSession || selectedSession.records.length === 0) {
      return
    }
    const nextIndex = Math.min(
      Math.max(selectedIndex + direction, 0),
      selectedSession.records.length - 1,
    )
    selectRecord(selectedSession.records[nextIndex])
  }

  const resumeLive = () => {
    if (!selectedSession) {
      return
    }
    setFollowLatest(true)
    setSelectedRecordId(newestRecord(selectedSession)?.id ?? '')
  }

  const selectSession = (sessionId: string) => {
    const session = sessions.find((candidate) => candidate.id === sessionId)
    setSelectedSessionId(sessionId)
    setSelectedRecordId(newestRecord(session)?.id ?? '')
    setFollowLatest(true)
  }

  const chooseFile = async () => {
    if (supportsLiveFile) {
      await openLive()
    } else {
      fileInputRef.current?.click()
    }
  }

  const handleSnapshotInput = (event: Event) => {
    const input = event.currentTarget as HTMLInputElement
    const file = input.files?.[0]
    if (file) {
      void openSnapshot(file)
    }
    input.value = ''
  }

  const handleDrop = (event: DragEvent) => {
    event.preventDefault()
    const file = event.dataTransfer?.files[0]
    if (file) {
      void openSnapshot(file)
    }
  }

  const loaded = state.status !== 'idle'

  return (
    <div class="app-shell" onDragOver={(event) => event.preventDefault()} onDrop={handleDrop}>
      <input
        ref={fileInputRef}
        class="visually-hidden"
        type="file"
        accept=".jsonl,application/x-ndjson,application/json"
        onChange={handleSnapshotInput}
      />

      <header class="app-header">
        <Brand />
        <div class="header-context">
          <div class="header-file">
            {state.fileName ? <FileJson size={16} aria-hidden="true" /> : null}
            <span title={state.fileName}>{state.fileName || 'No PattyLog open'}</span>
          </div>
          <div class="header-search">
            <Search size={15} aria-hidden="true" />
            <input
              type="search"
              value={searchText}
              placeholder="Search PattyLog"
              aria-label="Search PattyLog"
              disabled={!loaded || (selectedSession?.records.length ?? 0) === 0}
              onInput={(event) => setSearchText(event.currentTarget.value)}
            />
            {searchActive ? <span>{searchResults.length} {searchResults.length === 1 ? 'record' : 'records'}</span> : null}
            {searchText !== '' ? (
              <button
                type="button"
                title="Clear search"
                aria-label="Clear search"
                onClick={() => setSearchText('')}
              >
                <X size={14} aria-hidden="true" />
              </button>
            ) : null}
          </div>
          <ChangeThresholdControl
            value={changeThreshold}
            available={changeAvailable}
            intervalCount={selectedSession?.intervals.length ?? 0}
            onCommit={setChangeThreshold}
          />
        </div>
        <div class="header-actions">
          {sessions.length > 1 ? (
            <label class="session-select">
              <span>Session</span>
              <select
                value={selectedSession?.id ?? ''}
                onChange={(event) => selectSession(event.currentTarget.value)}
              >
                {sessions.map((session) => (
                  <option key={session.id} value={session.id}>{session.id}</option>
                ))}
              </select>
            </label>
          ) : null}
          <SourceStatus mode={state.mode} status={state.status} />
          <button class="command-button" type="button" onClick={() => void chooseFile()}>
            <FolderOpen size={17} aria-hidden="true" />
            {supportsLiveFile ? 'Open live' : 'Open file'}
          </button>
          {supportsLiveFile ? (
            <button
              class="icon-button"
              type="button"
              title="Open a static snapshot"
              aria-label="Open a static snapshot"
              onClick={() => fileInputRef.current?.click()}
            >
              <FileJson size={17} aria-hidden="true" />
            </button>
          ) : null}
        </div>
      </header>

      {state.error ? <div class="error-banner" role="alert">{state.error}</div> : null}
      {state.issues.length > 0 ? <IssueBanner issues={state.issues} /> : null}

      {!loaded ? (
        <EmptyState
          supportsLiveFile={supportsLiveFile}
          onOpen={() => void chooseFile()}
          onSnapshot={() => fileInputRef.current?.click()}
        />
      ) : state.status === 'loading' && state.records.length === 0 ? (
        <LoadingState fileName={state.fileName} bytesRead={state.bytesRead} totalBytes={state.totalBytes} />
      ) : (
        <main class="viewer-main">
          <IntervalMap
            intervals={selectedSession?.intervals ?? []}
            alertsByInterval={alertTimeline}
            selectedRecord={selectedRecord ?? null}
            trackedLanes={trackedLanes}
            enabledCoreLanes={enabledCoreLanes}
            enabledMetricLanes={enabledMetricLanes}
            searchActive={searchActive}
            searchResultsByInterval={searchResultsByInterval}
            changeSeries={changeSeries}
            changeThreshold={changeThreshold}
            comparisonActive={detailTab === 'compare'}
            comparisonTargeting={comparisonTargeting}
            comparisonActiveRecord={comparisonReference}
            comparisonOriginRecord={comparisonOrigin}
            onSelect={selectIntervalMapRecord}
          />
          <div class="workspace">
            <aside class="record-navigator" aria-label="PattyLog records">
            <div class="navigator-heading">
              <div>
                <span class="eyebrow">Recorded model</span>
                <strong>{selectedSession?.records.length ?? 0} records</strong>
              </div>
              {state.mode === 'live' && !followLatest ? (
                <button class="resume-button" type="button" onClick={resumeLive}>
                  <ChevronsDown size={15} aria-hidden="true" />
                  Resume live
                </button>
              ) : null}
            </div>
            <VirtualRecordList
              records={selectedSession?.records ?? []}
              selectedRecordId={selectedRecord?.id ?? ''}
              onSelect={selectRecord}
              renderRecord={renderRecordListItem}
            />
            </aside>

            <section class="record-detail" aria-live="polite">
            {selectedRecord ? (
              <>
                <div class="detail-heading">
                  <RecordTitle record={selectedRecord} />
                  <div class="record-nav-buttons">
                    <button
                      class="icon-button"
                      type="button"
                      title="Newer record"
                      aria-label="Newer record"
                      disabled={selectedIndex >= (selectedSession?.records.length ?? 0) - 1}
                      onClick={() => selectRelative(1)}
                    >
                      <ChevronLeft size={18} aria-hidden="true" />
                    </button>
                    <span>{selectedIndex + 1}</span>
                    <button
                      class="icon-button"
                      type="button"
                      title="Older record"
                      aria-label="Older record"
                      disabled={selectedIndex <= 0}
                      onClick={() => selectRelative(-1)}
                    >
                      <ChevronRight size={18} aria-hidden="true" />
                    </button>
                  </div>
                </div>

                <div class="detail-tabs" role="tablist" aria-label="Record detail mode">
                  <button
                    type="button"
                    role="tab"
                    aria-selected={detailTab === 'overview'}
                    class={detailTab === 'overview' ? 'active' : ''}
                    onClick={() => selectDetailTab('overview')}
                  >
                    Overview
                  </button>
                  <button
                    type="button"
                    role="tab"
                    aria-selected={detailTab === 'traffic'}
                    class={detailTab === 'traffic' ? 'active' : ''}
                    disabled={!trafficAvailable}
                    onClick={() => selectDetailTab('traffic')}
                  >
                    Traffic
                  </button>
                  <button
                    type="button"
                    role="tab"
                    aria-selected={detailTab === 'compare'}
                    class={detailTab === 'compare' ? 'active' : ''}
                    disabled={!comparisonAvailable}
                    onClick={() => selectDetailTab('compare')}
                  >
                    Compare
                  </button>
                  <button
                    type="button"
                    role="tab"
                    aria-selected={detailTab === 'raw'}
                    class={detailTab === 'raw' ? 'active' : ''}
                    onClick={() => selectDetailTab('raw')}
                  >
                    Record
                  </button>
                </div>

                {detailTab !== 'compare' ? (
                  <>
                    <AlertStrip
                      alerts={selectedAlerts}
                      selectedRecordId={selectedRecord.id}
                      onSelect={selectRecord}
                    />
                    <FactoidRibbon key={selectedRecord.id} record={selectedRecord} />
                  </>
                ) : null}

                {detailTab === 'overview' ? (
                  <RecordOverview
                    record={selectedRecord}
                    enabledMetricLanes={enabledMetricLanes}
                    onToggleMetricLane={toggleMetricLane}
                  />
                ) : null}
                {detailTab === 'traffic' ? (
                  <TrafficDetail
                    record={selectedRecord}
                    searchQuery={searchQuery}
                    trackedLanes={trackedLanes}
                    enabledCoreLanes={enabledCoreLanes}
                    onToggleCoreLane={toggleCoreLane}
                    onToggleTrackedLane={toggleTrackedLane}
                  />
                ) : null}
                {detailTab === 'compare' && comparisonReference && selectedSession ? (
                  <IntervalComparisonDetail
                    selected={selectedRecord}
                    reference={comparisonReference}
                    intervals={comparableIntervals}
                    sessionRecords={selectedSession.records}
                    targeting={comparisonTargeting}
                    onReference={selectComparisonReference}
                    onToggleTargeting={() => setComparisonTargeting((current) => !current)}
                  />
                ) : null}
                {detailTab === 'raw' ? <RawRecord record={selectedRecord} /> : null}
              </>
            ) : (
              <div class="empty-detail">No records in this session.</div>
            )}
            </section>
          </div>
        </main>
      )}
    </div>
  )
}

function adjacentInterval(
  intervals: PattyLogRecord[],
  selected: PattyLogRecord,
): PattyLogRecord | null {
  const index = intervals.findIndex((record) => record.id === selected.id)
  if (index < 0) {
    return null
  }
  return intervals[index - 1] ?? intervals[index + 1] ?? null
}
