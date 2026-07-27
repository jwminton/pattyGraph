export type SourceLookupUnavailableReason =
  | 'empty-key'
  | 'synthetic-empty'
  | 'unsafe-key'
  | 'unsafe-path'

export type SourceLookup =
  | {
    available: true
    command: string
    path: string
    pathIsTemplate: boolean
    pathIsRelative: boolean
  }
  | {
    available: false
    reason: SourceLookupUnavailableReason
  }

const sourcePathPlaceholder = '/path/to/access.log'
const unsafeShellText = /[\u0000-\u001f\u007f]/

export function buildSourceLookup(key: string, recordedPath: string): SourceLookup {
  if (key === '') {
    return { available: false, reason: 'empty-key' }
  }
  if (key === '--empty--') {
    return { available: false, reason: 'synthetic-empty' }
  }
  if (unsafeShellText.test(key)) {
    return { available: false, reason: 'unsafe-key' }
  }
  if (unsafeShellText.test(recordedPath)) {
    return { available: false, reason: 'unsafe-path' }
  }

  const path = recordedPath || sourcePathPlaceholder
  return {
    available: true,
    command: `LC_ALL=C grep -nF -- ${shellQuote(key)} ${shellQuotePath(path)}`,
    path,
    pathIsTemplate: recordedPath === '',
    pathIsRelative: recordedPath !== '' && !isAbsoluteOrHomePath(recordedPath),
  }
}

function shellQuote(value: string): string {
  return `'${value.replaceAll("'", `'"'"'`)}'`
}

function shellQuotePath(value: string): string {
  if (value === '~') {
    return '"$HOME"'
  }
  if (value.startsWith('~/')) {
    return `"$HOME"/${shellQuote(value.slice(2))}`
  }
  return shellQuote(value)
}

function isAbsoluteOrHomePath(value: string): boolean {
  return value.startsWith('/') || value === '~' || value.startsWith('~/')
}
