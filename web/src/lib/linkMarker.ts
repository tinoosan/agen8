// Composes and writes the `.agen8/` link marker that binds a folder on disk to
// a project (and optional workspace) on this server.
//
// The marker is three files inside `<project-root>/.agen8/`:
//   - workspace.json  committed pointer: { server_url, project_id, workspace_id? }
//   - token           gitignored secret: the one-line wlt_… link token
//   - .gitignore      generated: ignores `token` so the secret never gets committed
//
// `buildMarkerFiles` is pure and validated; `writeMarkerToDirectory` uses the
// File System Access API (Chromium, secure-context only) and fails loudly when
// the browser cannot write folders so the caller can surface the copy/paste path.

export const MARKER_DIR = '.agen8'

export interface MarkerInput {
  /** Origin of this server, e.g. window.location.origin. */
  serverUrl: string
  projectId: string
  /** Optional workspace binding; omitted from the pointer when blank. */
  workspaceId?: string
  /** The minted wlt_ link token (shown once). */
  token: string
}

export interface MarkerFile {
  /** File name relative to the `.agen8/` directory. */
  name: string
  contents: string
}

/**
 * Builds the three marker files. Throws when any required input is blank — an
 * incomplete marker is worse than none, so we never emit a half-written binding.
 */
export function buildMarkerFiles(input: MarkerInput): MarkerFile[] {
  const serverUrl = input.serverUrl.trim()
  const projectId = input.projectId.trim()
  const token = input.token.trim()
  if (!serverUrl) throw new Error('link marker requires a server URL')
  if (!projectId) throw new Error('link marker requires a project id')
  if (!token) throw new Error('link marker requires a link token')

  const pointer: { server_url: string; project_id: string; workspace_id?: string } = {
    server_url: serverUrl,
    project_id: projectId,
  }
  const workspaceId = input.workspaceId?.trim()
  if (workspaceId) pointer.workspace_id = workspaceId

  return [
    { name: 'workspace.json', contents: JSON.stringify(pointer, null, 2) + '\n' },
    { name: 'token', contents: token + '\n' },
    { name: '.gitignore', contents: 'token\n' },
  ]
}

/* ── File System Access API (not yet in the TS DOM lib) ──────────────── */

interface WritableFileStream {
  write(data: string): Promise<void>
  close(): Promise<void>
}

interface FileHandleLike {
  createWritable(): Promise<WritableFileStream>
}

interface DirectoryHandleLike {
  getDirectoryHandle(name: string, options?: { create?: boolean }): Promise<DirectoryHandleLike>
  getFileHandle(name: string, options?: { create?: boolean }): Promise<FileHandleLike>
}

type ShowDirectoryPicker = (options?: { mode?: 'read' | 'readwrite' }) => Promise<DirectoryHandleLike>

function getDirectoryPicker(): ShowDirectoryPicker | null {
  if (typeof window === 'undefined') return null
  const picker = (window as unknown as { showDirectoryPicker?: ShowDirectoryPicker }).showDirectoryPicker
  return typeof picker === 'function' ? picker : null
}

/** True when this browser can write a user-picked folder directly. */
export function supportsDirectoryPicker(): boolean {
  return getDirectoryPicker() !== null
}

/**
 * Prompts the user to pick their project folder, then writes the marker files
 * into `<picked>/.agen8/`. Throws when the browser lacks the API so the caller
 * falls back to the visible copy/paste panel rather than silently doing nothing.
 */
export async function writeMarkerToDirectory(files: MarkerFile[], dirName: string = MARKER_DIR): Promise<void> {
  const picker = getDirectoryPicker()
  if (!picker) {
    throw new Error('This browser cannot write folders directly — copy the files into the folder manually.')
  }
  const root = await picker({ mode: 'readwrite' })
  const dir = await root.getDirectoryHandle(dirName, { create: true })
  for (const file of files) {
    const handle = await dir.getFileHandle(file.name, { create: true })
    const writable = await handle.createWritable()
    await writable.write(file.contents)
    await writable.close()
  }
}
