import { useEffect, useRef, useState } from 'preact/hooks'
import { changeThresholdCommitDelay } from '../domain/changeThreshold'

export function ChangeThresholdControl({
  value,
  available,
  intervalCount,
  onCommit,
}: {
  value: number
  available: boolean
  intervalCount: number
  onCommit: (value: number) => void
}) {
  const [displayValue, setDisplayValue] = useState(value)
  const pendingValueRef = useRef(value)
  const publishedValueRef = useRef(value)
  const timerRef = useRef<number | null>(null)
  const delay = changeThresholdCommitDelay(intervalCount)

  const publish = (next: number) => {
    if (next === publishedValueRef.current) {
      return
    }
    publishedValueRef.current = next
    onCommit(next)
  }

  const flush = () => {
    if (timerRef.current !== null) {
      window.clearTimeout(timerRef.current)
      timerRef.current = null
    }
    publish(pendingValueRef.current)
  }

  const update = (next: number) => {
    pendingValueRef.current = next
    setDisplayValue(next)
    if (delay === 0) {
      publish(next)
      return
    }
    if (timerRef.current === null) {
      timerRef.current = window.setTimeout(() => {
        timerRef.current = null
        publish(pendingValueRef.current)
      }, delay)
    }
  }

  useEffect(() => {
    if (timerRef.current === null) {
      pendingValueRef.current = value
      publishedValueRef.current = value
      setDisplayValue(value)
    }
  }, [value])

  useEffect(() => () => {
    if (timerRef.current !== null) {
      window.clearTimeout(timerRef.current)
    }
  }, [])

  return (
    <label class={`header-change ${available ? '' : 'disabled'}`}>
      <span>Change</span>
      <input
        type="range"
        min="0"
        max="100"
        step="1"
        value={displayValue}
        aria-label="Change threshold"
        disabled={!available}
        onInput={(event) => update(Number(event.currentTarget.value))}
        onPointerUp={flush}
        onPointerCancel={flush}
        onKeyUp={flush}
        onBlur={flush}
      />
      <output>&ge; {displayValue}</output>
    </label>
  )
}
