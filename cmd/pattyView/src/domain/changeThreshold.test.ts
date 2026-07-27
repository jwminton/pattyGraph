import { describe, expect, it } from 'vitest'
import {
  changeThresholdCommitDelay,
  immediateChangeThresholdIntervals,
  qualifiesChangeThreshold,
} from './changeThreshold'

describe('change threshold update policy', () => {
  it('keeps smaller sessions immediate and spaces larger-session updates', () => {
    expect(changeThresholdCommitDelay(immediateChangeThresholdIntervals)).toBe(0)
    expect(changeThresholdCommitDelay(555)).toBe(90)
    expect(changeThresholdCommitDelay(1710)).toBe(214)
    expect(changeThresholdCommitDelay(10000)).toBe(240)
  })

  it('qualifies the same rounded score shown in the Change lane', () => {
    expect(qualifiesChangeThreshold(39.6, 40)).toBe(true)
    expect(qualifiesChangeThreshold(39.4, 40)).toBe(false)
    expect(qualifiesChangeThreshold(40, 40)).toBe(true)
  })
})
