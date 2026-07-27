import { describe, expect, it } from 'vitest'
import { projectTrafficDetail } from './trafficDetail'

describe('traffic detail projection', () => {
  it('preserves emitted matcher and interesting ordering', () => {
    const detail = projectTrafficDetail({
      source_lines: ['latest catalog line', 'latest grouped IP line'],
      matchers: [
        {
          name: 'errs',
          color_hex: '#e8cfe3',
          interval_count: 18,
          last_interval_count: 14,
          history_total: 32,
          history_peak: 18,
          top_keys: [
            { key: '500', count: 10, rank: 1 },
            { key: '404', count: 8, rank: 2 },
          ],
          top_groups: [{ prefix: '5xx', count: 10, members: 2, rank: 1 }],
          interval_line: 'interval error line',
        },
        { name: 'change', interval_count: 41, top_keys: [{ key: 'errs', count: 60 }] },
        { name: 'bytes', interval_count: 4_400_000 },
      ],
      interesting: [
        {
          name: 'words',
          total_keys: 2,
          top: [
            {
              key: 'catalog',
              rank: 1,
              score: 80,
              count: 20,
              bytes: 4000,
              prime_flux: 60,
              burstiness: 0.5,
              agent_delta_metric: 0.25,
              history_total: 100,
              history_peak: 30,
              history_depth: 4,
              last_seen_tic: 120,
              last_status: '200',
              marked_state: 'marked',
              marked_by_matcher: 'catalog matcher',
              is_peak: true,
              source: { log_line: 'first catalog line' },
              first_interval_line: 'interval catalog line',
              last_line: 'latest catalog line',
              source_line_ref: 1,
            },
          ],
          peaks: [{ key: 'catalog', rank: 1, score: 80, is_peak: true }],
        },
        {
          name: 'ips',
          total_keys: 1,
          top: [],
          peaks: [],
          ip_groups: [{
            prefix: '192.0.',
            rank: 1,
            score: 12,
            count: 8,
            count_plus_first: 20,
            members: 3,
            bytes: 2048,
            history_depth: 2,
            source_line_ref: 2,
          }],
        },
      ],
      factoids: [{ name: 'alerts.active', text: 'Active Alerts:errs', probability: 100 }],
      selected: {
        graph_value: 18,
        matcher: 'errs',
        first_source: { log_line: 'selected first line' },
      },
    })

    expect(detail.matchers.map((matcher) => matcher.name)).toEqual(['errs', 'bytes'])
    expect(detail.matchers[0]).toMatchObject({
      current: 18,
      previous: 14,
      historyTotal: 32,
      historyPeak: 18,
      topKeys: [
        { key: '500', count: 10, rank: 1 },
        { key: '404', count: 8, rank: 2 },
      ],
      sources: [{ label: 'First this interval', line: 'interval error line' }],
    })
    expect(detail.interesting.map((stream) => stream.name)).toEqual(['words', 'ips'])
    expect(detail.interesting[0].top[0]).toMatchObject({
      key: 'catalog',
      score: 80,
      markedByMatcher: 'catalog matcher',
      sources: [
        { label: 'First seen', line: 'first catalog line' },
        { label: 'First this interval', line: 'interval catalog line' },
        { label: 'Latest seen', line: 'latest catalog line' },
      ],
    })
    expect(detail.interesting[1].ipGroups[0]).toMatchObject({
      prefix: '192.0.',
      countPlusFirst: 20,
      bytes: 2048,
      sources: [{ label: 'Latest retained', line: 'latest grouped IP line' }],
    })
    expect(detail.factoids).toEqual([{ name: 'alerts.active', text: 'Active Alerts:errs' }])
    expect(detail.selected).toEqual({
      fields: [
        { label: 'Graph value', value: '18' },
        { label: 'Matcher', value: 'errs' },
      ],
      sources: [{ label: 'First seen', line: 'selected first line' }],
    })
    expect(detail.sourceExampleAvailability).toBe('enabled')
  })

  it('keeps the native Change matcher out of ordinary traffic projections', () => {
    const detail = projectTrafficDetail({
      matchers: [
        { name: 'lines', interval_count: 100 },
        { name: 'change', interval_count: 41, top_keys: [{ key: 'errs', count: 60 }] },
      ],
    })

    expect(detail.matchers.map((matcher) => matcher.name)).toEqual(['lines'])
  })

  it('resolves one-based source references and ignores malformed references', () => {
    const detail = projectTrafficDetail({
      source_lines: ['first example', 'second example'],
      interesting: [{
        name: 'words',
        top: [
          { key: 'valid', source_line_ref: 2 },
          { key: 'zero', source_line_ref: 0 },
          { key: 'fractional', source_line_ref: 1.5 },
          { key: 'outside', source_line_ref: 3 },
        ],
      }],
    })

    expect(detail.interesting[0].top.map((entry) => entry.sources)).toEqual([
      [{ label: 'Latest retained', line: 'second example' }],
      [],
      [],
      [],
    ])
  })

  it('keeps omitted values unavailable instead of deriving replacements', () => {
    const detail = projectTrafficDetail({
      matchers: [{ name: 'bytes', total_bytes: 99_000 }],
      interesting: [{ name: 'refs', top: [{ key: 'direct' }] }],
      selected: {},
    })

    expect(detail.matchers[0]).toMatchObject({
      current: null,
      previous: null,
      historyTotal: null,
      historyPeak: null,
    })
    expect(detail.interesting[0].top[0]).toMatchObject({
      key: 'direct',
      count: null,
      score: null,
      bytes: null,
    })
    expect(detail.selected).toBeNull()
    expect(detail.sourceExampleAvailability).toBe('unknown')
  })

  it('tracks source-example availability per interval', () => {
    const compact = projectTrafficDetail({
      source_examples_enabled: false,
      interesting: [{ name: 'words', top: [{ key: 'compact' }] }],
    })
    const enrichedWithoutLine = projectTrafficDetail({
      source_examples_enabled: true,
      interesting: [{ name: 'words', top: [{ key: 'enriched' }] }],
    })

    expect(compact.sourceExampleAvailability).toBe('disabled')
    expect(compact.interesting[0].top[0].sources).toEqual([])
    expect(enrichedWithoutLine.sourceExampleAvailability).toBe('enabled')
    expect(enrichedWithoutLine.interesting[0].top[0].sources).toEqual([])
  })
})
