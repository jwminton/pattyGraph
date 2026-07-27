// Keep this aligned with .record-list-item's fixed CSS height.
export const recordRowHeight = 52
export const recordRowOverscan = 12

export interface VirtualRecordWindow {
  start: number
  end: number
  paddingTop: number
  paddingBottom: number
}

export function virtualRecordWindow(
  recordCount: number,
  scrollTop: number,
  viewportHeight: number,
): VirtualRecordWindow {
  if (recordCount <= 0) {
    return { start: 0, end: 0, paddingTop: 0, paddingBottom: 0 }
  }

  const visibleRows = Math.max(1, Math.ceil(viewportHeight / recordRowHeight))
  const maximumFirstVisible = Math.max(0, recordCount - visibleRows)
  const firstVisible = Math.min(
    maximumFirstVisible,
    Math.max(0, Math.floor(scrollTop / recordRowHeight)),
  )
  const start = Math.max(0, firstVisible - recordRowOverscan)
  const end = Math.min(recordCount, firstVisible + visibleRows + recordRowOverscan)

  return {
    start,
    end,
    paddingTop: start * recordRowHeight,
    paddingBottom: (recordCount - end) * recordRowHeight,
  }
}

export function recordScrollOffset(
  displayIndex: number,
  scrollTop: number,
  viewportHeight: number,
): number | null {
  if (displayIndex < 0 || viewportHeight <= 0) {
    return null
  }
  const rowTop = displayIndex * recordRowHeight
  const rowBottom = rowTop + recordRowHeight
  if (rowTop < scrollTop) {
    return rowTop
  }
  if (rowBottom > scrollTop + viewportHeight) {
    return Math.max(0, rowBottom - viewportHeight)
  }
  return null
}
