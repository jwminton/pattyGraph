import { describe, expect, it } from 'vitest'
import { buildSourceLookup } from './sourceLookup'

describe('source lookup command construction', () => {
  it('builds fixed-string searches for emitted keys and paths', () => {
    expect(buildSourceLookup('catalog', '/var/log/nginx/access.log')).toEqual({
      available: true,
      command: "LC_ALL=C grep -nF -- 'catalog' '/var/log/nginx/access.log'",
      path: '/var/log/nginx/access.log',
      pathIsTemplate: false,
      pathIsRelative: false,
    })
    expect(buildSourceLookup('-probe', './access log')).toMatchObject({
      available: true,
      command: "LC_ALL=C grep -nF -- '-probe' './access log'",
      pathIsRelative: true,
    })
  })

  it('quotes apostrophes and preserves Unicode as fixed text', () => {
    expect(buildSourceLookup("Bob's-تست", "/srv/Bob's/access.log")).toMatchObject({
      available: true,
      command: `LC_ALL=C grep -nF -- 'Bob'"'"'s-تست' '/srv/Bob'"'"'s/access.log'`,
    })
  })

  it('keeps home-relative paths expandable', () => {
    expect(buildSourceLookup('catalog', '~/logs/access.log')).toMatchObject({
      available: true,
      command: `LC_ALL=C grep -nF -- 'catalog' "$HOME"/'logs/access.log'`,
      pathIsRelative: false,
    })
  })

  it('provides an editable template when file_path is unavailable', () => {
    expect(buildSourceLookup('192.0.2.10', '')).toEqual({
      available: true,
      command: "LC_ALL=C grep -nF -- '192.0.2.10' '/path/to/access.log'",
      path: '/path/to/access.log',
      pathIsTemplate: true,
      pathIsRelative: false,
    })
  })

  it('does not invent literal searches for special or unsafe values', () => {
    expect(buildSourceLookup('--empty--', './access.log')).toEqual({
      available: false,
      reason: 'synthetic-empty',
    })
    expect(buildSourceLookup('', './access.log')).toEqual({
      available: false,
      reason: 'empty-key',
    })
    expect(buildSourceLookup('bad\nkey', './access.log')).toEqual({
      available: false,
      reason: 'unsafe-key',
    })
    expect(buildSourceLookup('catalog', 'bad\npath')).toEqual({
      available: false,
      reason: 'unsafe-path',
    })
  })
})
