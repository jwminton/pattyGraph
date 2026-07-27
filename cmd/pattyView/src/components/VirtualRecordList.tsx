import type { ComponentChild } from 'preact'
import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'preact/hooks'
import type { PattyLogRecord } from '../domain/types'
import {
  recordScrollOffset,
  virtualRecordWindow,
} from '../domain/virtualRecordList'

const initialViewportHeight = 520

export function VirtualRecordList({
  records,
  selectedRecordId,
  onSelect,
  renderRecord,
}: {
  records: PattyLogRecord[]
  selectedRecordId: string
  onSelect: (record: PattyLogRecord) => void
  renderRecord: (
    record: PattyLogRecord,
    selected: boolean,
    onSelect: () => void,
  ) => ComponentChild
}) {
  const containerRef = useRef<HTMLDivElement>(null)
  const frameRef = useRef<number | null>(null)
  const pendingScrollTopRef = useRef(0)
  const [scrollTop, setScrollTop] = useState(0)
  const [viewportHeight, setViewportHeight] = useState(initialViewportHeight)
  const selectedSourceIndex = useMemo(
    () => records.findIndex((record) => record.id === selectedRecordId),
    [records, selectedRecordId],
  )
  const selectedDisplayIndex = selectedSourceIndex < 0
    ? -1
    : records.length - 1 - selectedSourceIndex
  const recordWindow = virtualRecordWindow(records.length, scrollTop, viewportHeight)

  useLayoutEffect(() => {
    const element = containerRef.current
    if (!element) {
      return
    }
    const nextScrollTop = recordScrollOffset(
      selectedDisplayIndex,
      element.scrollTop,
      element.clientHeight,
    )
    if (nextScrollTop !== null) {
      element.scrollTop = nextScrollTop
      pendingScrollTopRef.current = nextScrollTop
      setScrollTop(nextScrollTop)
    }
  }, [records.length, selectedDisplayIndex, selectedRecordId, viewportHeight])

  useLayoutEffect(() => {
    const element = containerRef.current
    if (!element) {
      return
    }
    const measure = () => setViewportHeight(element.clientHeight || initialViewportHeight)
    measure()
    if (typeof ResizeObserver === 'undefined') {
      window.addEventListener('resize', measure)
      return () => window.removeEventListener('resize', measure)
    }
    const observer = new ResizeObserver(measure)
    observer.observe(element)
    return () => observer.disconnect()
  }, [])

  useEffect(() => () => {
    if (frameRef.current !== null) {
      window.cancelAnimationFrame(frameRef.current)
    }
  }, [])

  const handleScroll = (event: Event) => {
    pendingScrollTopRef.current = event.currentTarget instanceof HTMLElement
      ? event.currentTarget.scrollTop
      : 0
    if (frameRef.current !== null) {
      return
    }
    frameRef.current = window.requestAnimationFrame(() => {
      frameRef.current = null
      setScrollTop(pendingScrollTopRef.current)
    })
  }

  const visibleRecords: ComponentChild[] = []
  for (let displayIndex = recordWindow.start; displayIndex < recordWindow.end; displayIndex += 1) {
    const record = records[records.length - 1 - displayIndex]
    if (record) {
      visibleRecords.push(renderRecord(
        record,
        record.id === selectedRecordId,
        () => onSelect(record),
      ))
    }
  }

  return (
    <div class="record-list" ref={containerRef} onScroll={handleScroll}>
      <div class="record-list-spacer" style={{ height: `${recordWindow.paddingTop}px` }} aria-hidden="true" />
      {visibleRecords}
      <div class="record-list-spacer" style={{ height: `${recordWindow.paddingBottom}px` }} aria-hidden="true" />
    </div>
  )
}
