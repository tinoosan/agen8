export interface ContextPanelOpenFileRequest {
  path: string
}

const OPEN_FILE_EVENT_NAME = 'agen8:context-panel-open-file'

export function emitContextPanelOpenFile(request: ContextPanelOpenFileRequest): void {
  const path = request.path.trim()
  if (!path) return
  window.dispatchEvent(new CustomEvent<ContextPanelOpenFileRequest>(OPEN_FILE_EVENT_NAME, { detail: { path } }))
}

export function onContextPanelOpenFile(
  handler: (request: ContextPanelOpenFileRequest) => void,
): () => void {
  const listener = (event: Event) => {
    const customEvent = event as CustomEvent<ContextPanelOpenFileRequest>
    const detail = customEvent.detail
    if (!detail || typeof detail.path !== 'string') return
    const path = detail.path.trim()
    if (!path) return
    handler({ path })
  }

  window.addEventListener(OPEN_FILE_EVENT_NAME, listener as EventListener)
  return () => window.removeEventListener(OPEN_FILE_EVENT_NAME, listener as EventListener)
}

export function pathFromChatHref(href: string | undefined): string | null {
  const raw = (href ?? '').trim()
  if (!raw) return null

  const decoded = safeDecode(raw)
  const normalized = decoded.trim()
  if (!normalized) return null

  if (
    normalized.startsWith('#')
    || normalized.toLowerCase().startsWith('http://')
    || normalized.toLowerCase().startsWith('https://')
    || normalized.toLowerCase().startsWith('mailto:')
    || normalized.toLowerCase().startsWith('tel:')
    || normalized.toLowerCase().startsWith('data:')
    || normalized.toLowerCase().startsWith('www.')
  ) {
    return null
  }

  if (normalized.toLowerCase().startsWith('file://')) {
    const filePath = normalized.slice('file://'.length).trim()
    return sanitizePathForLookup(filePath)
  }

  const sanitized = sanitizePathForLookup(normalized)
  if (!sanitized) return null

  const looksLikePath = sanitized.startsWith('/')
    || sanitized.startsWith('./')
    || sanitized.startsWith('../')
    || (sanitized.includes('/') && /\.[a-zA-Z][a-zA-Z0-9]*$/.test(sanitized))
  const looksLikeFilename = /^[^\\/\s]+\.[A-Za-z0-9_-]+$/.test(sanitized)
  if (!looksLikePath && !looksLikeFilename) return null

  return sanitized
}

function sanitizePathForLookup(rawPath: string): string | null {
  const withoutQueryOrFragment = rawPath.split(/[?#]/, 1)[0]?.trim() ?? ''
  if (!withoutQueryOrFragment) return null

  const withoutLineSuffix = stripPathLocationSuffix(withoutQueryOrFragment)
  const normalized = normalizeWorkspaceRelativePath(withoutLineSuffix.trim())
  if (!normalized || /^[./\\]+$/.test(normalized)) return null

  const looksLikePath = normalized.startsWith('/')
    || normalized.startsWith('./')
    || normalized.startsWith('../')
    || (normalized.includes('/') && /\.[a-zA-Z][a-zA-Z0-9]*$/.test(normalized))
  const looksLikeFilename = /^[^\\/\s]+\.[A-Za-z0-9_-]+$/.test(normalized)
  if (!looksLikePath && !looksLikeFilename) return null
  return normalized
}

function stripPathLocationSuffix(path: string): string {
  return path.replace(/#L\d+(?:C\d+)?$/i, '').replace(/:\d+(?::\d+)?$/, '')
}

function normalizeWorkspaceRelativePath(path: string): string {
  const relative = path.replace(/^\.\/+/, '')
  if (relative === 'playground/workspace' || relative.startsWith('playground/workspace/')) {
    const suffix = relative.slice('playground/workspace'.length).replace(/^\/+/, '')
    return suffix ? `/workspace/${suffix}` : '/workspace'
  }
  if (relative === 'workspace' || relative.startsWith('workspace/')) {
    const suffix = relative.slice('workspace'.length).replace(/^\/+/, '')
    return suffix ? `/workspace/${suffix}` : '/workspace'
  }
  if (relative === 'project' || relative.startsWith('project/')) {
    const suffix = relative.slice('project'.length).replace(/^\/+/, '')
    return suffix ? `/project/${suffix}` : '/project'
  }
  return path
}

function safeDecode(value: string): string {
  try {
    return decodeURIComponent(value)
  } catch {
    return value
  }
}
