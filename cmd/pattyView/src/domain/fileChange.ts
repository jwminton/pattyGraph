export interface FileSnapshot {
  size: number
  lastModified: number
  fingerprint: string
}

export type FileChange = 'none' | 'append' | 'reset'

export function classifyFileChange(
  previous: FileSnapshot,
  next: FileSnapshot,
  readOffset: number,
): FileChange {
  if (
    previous.fingerprint !== '' &&
    next.fingerprint !== '' &&
    previous.fingerprint !== next.fingerprint
  ) {
    return 'reset'
  }
  if (next.size < readOffset) {
    return 'reset'
  }
  if (next.size > readOffset) {
    return 'append'
  }
  return 'none'
}

export async function snapshotFile(file: File): Promise<FileSnapshot> {
  const prefix = await file.slice(0, Math.min(file.size, 4096)).arrayBuffer()
  const bytes = new Uint8Array(prefix)
  const newline = bytes.indexOf(10)
  return {
    size: file.size,
    lastModified: file.lastModified,
    fingerprint: newline >= 0 ? hashBytes(bytes.slice(0, newline + 1)) : '',
  }
}

function hashBytes(bytes: Uint8Array): string {
  let hash = 0x811c9dc5
  for (const byte of bytes) {
    hash ^= byte
    hash = Math.imul(hash, 0x01000193)
  }
  return (hash >>> 0).toString(16).padStart(8, '0')
}
