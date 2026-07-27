import { describe, expect, it } from 'vitest'
import { interestingTrackedLane, matcherTrackedLane } from './trackedLane'

describe('tracked interval lanes', () => {
  it('uses stable matcher identity and captures its selected color', () => {
    expect(matcherTrackedLane('Googlebot', '#ff00ff')).toEqual({
      kind: 'matcher',
      id: 'matcher:Googlebot',
      name: 'Googlebot',
      label: 'Googlebot',
      color: '#ff00ff',
    })
  })

  it('uses one identity for an interesting key across top and peaks', () => {
    const top = interestingTrackedLane('words', 'catalog', '#4ecdc4')
    const peak = interestingTrackedLane('words', 'catalog', '#f18a8a')

    expect(top.id).toBe(peak.id)
    expect(top.label).toBe('W catalog')
    expect(interestingTrackedLane('refs', 'direct', '').label).toBe('R direct')
    expect(interestingTrackedLane('ips', '192.0.2.10', '').label).toBe('IP 192.0.2.10')
  })
})
