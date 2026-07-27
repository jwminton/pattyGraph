export const immediateChangeThresholdIntervals = 512

export function qualifiesChangeThreshold(score: number, threshold: number): boolean {
  return Math.round(score) >= threshold
}

export function changeThresholdCommitDelay(intervalCount: number): number {
  if (intervalCount <= immediateChangeThresholdIntervals) {
    return 0
  }
  return Math.min(240, Math.max(90, Math.round(intervalCount / 8)))
}
