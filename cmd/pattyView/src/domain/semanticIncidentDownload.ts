import {
  semanticIncidentBlob,
  writeSemanticIncident,
  type SemanticIncidentPlan,
} from './semanticIncidentBundle'

export type SemanticIncidentSaveResult = 'saved' | 'downloaded' | 'cancelled'

interface SaveFileHandle {
  createWritable(): Promise<WritableStream<Uint8Array>>
}

interface SavePickerWindow extends Window {
  showSaveFilePicker?: (options: {
    suggestedName: string
    types: Array<{
      description: string
      accept: Record<string, string[]>
    }>
  }) => Promise<SaveFileHandle>
}

export async function saveSemanticIncident(
  plan: SemanticIncidentPlan,
  scope: Window = window,
  documentScope: Document = document,
): Promise<SemanticIncidentSaveResult> {
  const picker = (scope as SavePickerWindow).showSaveFilePicker
  if (picker) {
    let handle: SaveFileHandle
    try {
      handle = await picker.call(scope, {
        suggestedName: plan.suggestedName,
        types: [{
          description: 'pattyView incident bundle',
          accept: { 'application/zip': ['.zip'] },
        }],
      })
    } catch (error) {
      if (isAbort(error)) {
        return 'cancelled'
      }
      throw error
    }
    const writable = await handle.createWritable()
    try {
      await writeSemanticIncident(plan, writable)
    } catch (error) {
      await writable.abort(error).catch(() => undefined)
      throw error
    }
    return 'saved'
  }

  const blob = await semanticIncidentBlob(plan)
  const urlAPI = (scope as Window & { URL: typeof URL }).URL
  const url = urlAPI.createObjectURL(blob)
  const anchor = documentScope.createElement('a')
  anchor.href = url
  anchor.download = plan.suggestedName
  anchor.style.display = 'none'
  documentScope.body.append(anchor)
  anchor.click()
  anchor.remove()
  scope.setTimeout(() => urlAPI.revokeObjectURL(url), 0)
  return 'downloaded'
}

function isAbort(error: unknown): boolean {
  return (error instanceof DOMException && error.name === 'AbortError') ||
    (error instanceof Error && error.name === 'AbortError')
}
