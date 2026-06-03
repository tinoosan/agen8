interface RpcNotificationLike {
  method: string
  params?: unknown
}

export interface FileChangeNotification {
  projectRoot?: string
  paths: string[]
}

interface EventRecordLike {
  type?: string
  data?: Record<string, string | undefined>
}

const FILE_CHANGE_OPS = new Set(['write_file', 'edit_file', 'file_change'])

export function changedFilesFromNotification(notification: RpcNotificationLike): FileChangeNotification | null {
  if (notification.method === 'files.changed') {
    const params = notification.params as { projectRoot?: string; path?: string; previousPath?: string } | undefined
    const paths = compactPaths([params?.path, params?.previousPath])
    if (paths.length === 0) return null
    return { projectRoot: params?.projectRoot, paths }
  }

  if (notification.method !== 'event.append') return null
  const event = eventFromAppendParams(notification.params)
  const data = event?.data
  const op = data?.op ?? data?.toolName
  if (!op || !FILE_CHANGE_OPS.has(op)) return null

  const paths = compactPaths([
    data?.path,
    ...pathsFromChangesJSON(data?.changes),
  ])
  if (paths.length === 0) return null
  return { projectRoot: data?.projectRoot, paths }
}

export function fileChangeAffectsPath(change: FileChangeNotification, targetPath: string, projectRoot?: string | null): boolean {
  const target = normalizeManagedPath(targetPath, projectRoot)
  if (!target) return false
  return change.paths.some((path) => normalizeManagedPath(path, projectRoot) === target)
}

export function parentDirsForFileChange(change: FileChangeNotification, projectRoot?: string | null): string[] {
  const dirs = new Set<string>()
  for (const changedPath of change.paths) {
    const managedPath = normalizeManagedPath(changedPath, projectRoot)
    if (!managedPath) continue
    const index = managedPath.lastIndexOf('/')
    dirs.add(index <= 0 ? '/' : managedPath.slice(0, index))
  }
  return Array.from(dirs)
}

function eventFromAppendParams(params: unknown): EventRecordLike | null {
  if (!params || typeof params !== 'object') return null
  const payload = params as { event?: EventRecordLike; type?: string; data?: Record<string, string | undefined> }
  return payload.event ?? payload
}

function compactPaths(paths: Array<string | undefined>): string[] {
  const unique = new Set<string>()
  for (const path of paths) {
    const value = path?.trim()
    if (value) unique.add(value)
  }
  return Array.from(unique)
}

function pathsFromChangesJSON(raw: string | undefined): string[] {
  if (!raw) return []
  try {
    const parsed = JSON.parse(raw) as unknown
    if (!Array.isArray(parsed)) return []
    return parsed
      .map((entry) => {
        if (!entry || typeof entry !== 'object') return ''
        return String((entry as { path?: unknown }).path ?? '')
      })
      .filter(Boolean)
  } catch {
    return []
  }
}

function normalizeManagedPath(value: string | undefined, projectRoot?: string | null): string {
  const raw = value?.trim().replace(/\\/g, '/')
  if (!raw) return ''
  const normalizedProjectRoot = projectRoot?.trim().replace(/\\/g, '/').replace(/\/+$/, '')
  if (normalizedProjectRoot && (raw === normalizedProjectRoot || raw.startsWith(`${normalizedProjectRoot}/`))) {
    const workspaceRoot = `${normalizedProjectRoot}/.agen8/workspace`
    if (raw === workspaceRoot || raw.startsWith(`${workspaceRoot}/`)) {
      const relative = raw.slice(workspaceRoot.length).replace(/^\/+/, '')
      return relative ? `/workspace/${relative}` : '/workspace'
    }
    const relative = raw.slice(normalizedProjectRoot.length).replace(/^\/+/, '')
    return relative ? `/project/${relative}` : '/project'
  }
  if (raw === '/project' || raw.startsWith('/project/') || raw === '/workspace' || raw.startsWith('/workspace/')) {
    return raw.replace(/\/+$/, '') || '/'
  }
  const relative = raw.replace(/^\.\/+/, '').replace(/^\/+/, '')
  if (relative === '.agen8/workspace' || relative.startsWith('.agen8/workspace/')) {
    const suffix = relative.slice('.agen8/workspace'.length).replace(/^\/+/, '')
    return suffix ? `/workspace/${suffix}` : '/workspace'
  }
  return relative ? `/project/${relative}` : ''
}
