import { useCallback, useDeferredValue, useEffect, useMemo, useState } from 'react'
import { keepPreviousData, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowLeft, FileText, FolderTree, Plus, Search, X } from 'lucide-react'
import { TaskPanel } from './strategy/TaskPanel'
import { nodeTypeRegistry } from './strategy/registry'
import { PanelNavigationContext } from './strategy/PanelNavigationContext'
import { useEntityLookup } from './strategy/useEntityLookup'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Dialog, DialogContent, DialogDescription, DialogTitle } from '@/components/ui/dialog'
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '@/components/ui/breadcrumb'
import { useNavigation } from '../lib/routing'
import { useArtifactFiles, useProjectArtifactFiles } from '../hooks/useArtifactFiles'
import { dedupeArtifactNodes, artifactIdentity } from '../lib/artifactNodes'
import { onNotification, rpcCall } from '../lib/rpc'
import type { ArtifactGetResult, ArtifactNode, FilesListDirResult, Task } from '../lib/types'
import { changedFilesFromNotification, fileChangeAffectsPath } from '../lib/fileChangeNotifications'
import ArtifactViewer from './files/ArtifactViewer'
import ArtifactTree, { type ArtifactTreeDirectory } from './files/ArtifactTree'
import { basename, displayVirtualPathPart, isMarkdownFile } from './files/filePreviewUtils'
import { cn } from '@/lib/utils'

const ARTIFACT_PREVIEW_MAX_BYTES = 8 * 1024 * 1024
const EMPTY_LOADED_PROJECT_DIRS: Record<string, FilesListDirResult> = Object.freeze({})
const EMPTY_LOADING_PROJECT_DIRS = new Set<string>()
type ActiveTab = 'task' | `file:${string}`

interface ContextPanelProps {
  spaceId: string
  projectId?: string | null
  spaceState?: {
    managedBy?: string
    desiredEnabled?: boolean
    status?: string
    lifecyclePhase?: string
  }
  fileOpenRequest?: {
    id: number
    path: string
  } | null
  taskOpenRequest?: {
    id: number
    task: Task
    status: string
  } | null
  onFileOpenRequestHandled?: (id: number) => void
  onTaskOpenRequestHandled?: (id: number) => void
}

interface FileTab {
  tabId: string
  file: ArtifactNode
}

interface SearchCandidate {
  key: string
  artifact: ArtifactNode
  name: string
  path: string
  pathLower: string
}

/**
 * Renders a panel from the navigation stack by looking up entity data
 * and dispatching to the correct panel component via the registry.
 */
function StackedPanel({ nodeId, projectId, projectRoot, onBack, onClose }: {
  nodeId: string
  projectId: string
  projectRoot: string | null
  onBack: () => void
  onClose: () => void
}) {
  const entity = useEntityLookup(nodeId, projectId, projectRoot)

  if (!entity) {
    return (
      <div className="flex items-center justify-center h-full text-muted-foreground text-sm">
        Loading...
      </div>
    )
  }

  const descriptor = nodeTypeRegistry[entity.type]
  if (!descriptor) return null

  return (
    <div className="flex flex-col h-full">
      {/* Back button header */}
      <button
        type="button"
        onClick={onBack}
        className="flex items-center gap-1.5 shrink-0 text-left hover:bg-[var(--bg-surface)] transition-colors"
        style={{
          padding: '8px 16px',
          fontSize: '11px',
          fontWeight: 500,
          color: 'var(--accent)',
          background: 'var(--bg-app)',
          border: 'none',
          borderBottom: '1px solid var(--border)',
          cursor: 'pointer',
        }}
      >
        <ArrowLeft size={12} />
        Back
      </button>
      <div className="flex-1 min-h-0 overflow-hidden flex flex-col">
        <descriptor.Panel
          data={entity.data}
          projectId={projectId}
          projectRoot={projectRoot}
          onClose={onClose}
        />
      </div>
    </div>
  )
}

function taskTitle(task: Task): string {
  const title = task.title?.trim()
  if (title) return title
  const goal = task.description?.trim()
  if (goal) return goal.split('\n')[0] ?? goal
  return 'Untitled task'
}

export default function ContextPanel({
  spaceId,
  projectId,
  fileOpenRequest,
  taskOpenRequest,
  onFileOpenRequestHandled,
  onTaskOpenRequestHandled,
}: ContextPanelProps) {
  const { focusedProjectRoot } = useNavigation()
  const queryClient = useQueryClient()
  const [activeTab, setActiveTab] = useState<ActiveTab | null>(null)
  const [fileTabs, setFileTabs] = useState<FileTab[]>([])
  const [fileRailHidden, setFileRailHidden] = useState(true)
  const [quickOpenOpen, setQuickOpenOpen] = useState(false)
  const [quickOpenQuery, setQuickOpenQuery] = useState('')
  const deferredQuickOpenQuery = useDeferredValue(quickOpenQuery)
  const [pendingOpenPath, setPendingOpenPath] = useState<string | null>(null)
  const projectRailKey = `${projectId ?? ''}:${focusedProjectRoot ?? ''}`
  const [loadedProjectDirsState, setLoadedProjectDirsState] = useState<{ key: string; dirs: Record<string, FilesListDirResult> }>({ key: '', dirs: {} })
  const [loadingProjectDirsState, setLoadingProjectDirsState] = useState<{ key: string; dirs: Set<string> }>({ key: '', dirs: new Set() })
  const loadedProjectDirs = useMemo(
    () => loadedProjectDirsState.key === projectRailKey ? loadedProjectDirsState.dirs : EMPTY_LOADED_PROJECT_DIRS,
    [loadedProjectDirsState, projectRailKey],
  )
  const loadingProjectDirs = useMemo(
    () => loadingProjectDirsState.key === projectRailKey ? loadingProjectDirsState.dirs : EMPTY_LOADING_PROJECT_DIRS,
    [loadingProjectDirsState, projectRailKey],
  )
  const [selectedTask, setSelectedTask] = useState<{ task: Task; status: string } | null>(null)
  const [panelStack, setPanelStack] = useState<string[]>([]) // stack of nodeIds for drill-down
  const stackTopEntity = useEntityLookup(
    panelStack.length > 0 ? panelStack[panelStack.length - 1] : null,
    projectId ?? null,
    focusedProjectRoot,
  )

  const projectQuery = useProjectArtifactFiles(projectId ?? null, focusedProjectRoot ?? '', spaceId)
  const spaceQuery = useArtifactFiles(spaceId)
  const artifactQuery = focusedProjectRoot ? projectQuery : spaceQuery
  const artifacts = useMemo(() => dedupeArtifactNodes(artifactQuery.data ?? []), [artifactQuery.data])

  const artifactByTabId = useMemo(() => {
    const map = new Map<string, ArtifactNode>()
    for (const artifact of artifacts) {
      map.set(artifactIdentity(artifact), artifact)
    }
    return map
  }, [artifacts])

  const resolvedFileTabs = useMemo(() => {
    return fileTabs.map((tab) => ({ ...tab, file: artifactByTabId.get(tab.tabId) ?? tab.file }))
  }, [artifactByTabId, fileTabs])

  const activeFileTab = useMemo(() => {
    if (!activeTab?.startsWith('file:')) return null
    const tabId = activeTab.slice('file:'.length)
    return resolvedFileTabs.find((tab) => tab.tabId === tabId) ?? null
  }, [activeTab, resolvedFileTabs])

  const previewVPath = activeFileTab?.file.vpath ?? ''
  const previewArtifactId = previewVPath ? undefined : activeFileTab?.file.artifactId
  const activeFileQueryKey = activeFileTab?.tabId ?? ''

  const previewQuery = useQuery<ArtifactGetResult>({
    queryKey: ['context-panel-artifact.get', spaceId, activeFileQueryKey, previewVPath, previewArtifactId ?? ''],
    queryFn: async () => {
      if (!activeFileTab) {
        throw new Error('active file tab is required')
      }
      if (!previewArtifactId && focusedProjectRoot && previewVPath) {
        return rpcCall<ArtifactGetResult>('files.get', {
          projectId: projectId ?? undefined,
          projectRoot: focusedProjectRoot,
          path: previewVPath,
          maxBytes: ARTIFACT_PREVIEW_MAX_BYTES,
        })
      }
      return rpcCall<ArtifactGetResult>('artifact.get', {
        projectRoot: focusedProjectRoot ?? undefined,
        spaceId,
        artifactId: previewArtifactId,
        vpath: previewVPath || undefined,
        maxBytes: ARTIFACT_PREVIEW_MAX_BYTES,
      })
    },
    enabled: !!spaceId && !!activeFileTab,
    retry: false,
    staleTime: 30_000,
    placeholderData: keepPreviousData,
  })

  useEffect(() => {
    if (!focusedProjectRoot || !previewVPath) return
    const handleFileChange = (notification: { method: string; params?: unknown }) => {
      const change = changedFilesFromNotification(notification)
      if (!change) return
      if (change.projectRoot && change.projectRoot !== focusedProjectRoot) return
      if (!fileChangeAffectsPath(change, previewVPath, focusedProjectRoot)) return
      void queryClient.invalidateQueries({
        queryKey: ['context-panel-artifact.get', spaceId, activeFileQueryKey, previewVPath, previewArtifactId ?? ''],
      })
    }
    const unsubFiles = onNotification('files.changed', handleFileChange)
    const unsubEvents = onNotification('event.append', handleFileChange)
    return () => {
      unsubFiles()
      unsubEvents()
    }
  }, [activeFileQueryKey, focusedProjectRoot, previewArtifactId, previewVPath, queryClient, spaceId])

  const normalizedArtifactIndex = useMemo(() => {
    const map = new Map<string, ArtifactNode>()
    for (const artifact of artifacts) {
      const candidates = [artifact.vpath, artifact.displayName, artifact.diskPath]
      for (const candidate of candidates) {
        const key = normalizePath(candidate)
        if (key) map.set(key, artifact)
      }
    }
    return map
  }, [artifacts])

  const closeFileTab = useCallback((tabId: string) => {
    setFileTabs((prev) => {
      const index = prev.findIndex((tab) => tab.tabId === tabId)
      if (index < 0) return prev
      const next = prev.filter((tab) => tab.tabId !== tabId)
      setActiveTab((current) => {
        if (current !== `file:${tabId}`) return current
        if (next.length === 0) {
          return selectedTask ? 'task' : null
        }
        const replacement = next[Math.min(index, next.length - 1)]
        return `file:${replacement.tabId}`
      })
      return next
    })
  }, [selectedTask])

  const openFileTab = useCallback((file: ArtifactNode) => {
    const tabId = artifactIdentity(file)
    setFileTabs((prev) => {
      const existing = prev.find((tab) => tab.tabId === tabId)
      if (existing) return prev
      return [...prev, { tabId, file }]
    })
    setActiveTab(`file:${tabId}`)
    setQuickOpenOpen(false)
    setQuickOpenQuery('')
  }, [])

  const findArtifactByPath = useCallback((path: string): ArtifactNode | null => {
    const resolvedPath = resolveLinkedPath(path, focusedProjectRoot)
    if (!resolvedPath) return null

    for (const lookupKey of resolvedPath.lookupKeys) {
      const exact = normalizedArtifactIndex.get(lookupKey)
      if (exact) return exact
    }

    for (const lookupKey of resolvedPath.lookupKeys) {
      for (const [candidate, artifact] of normalizedArtifactIndex.entries()) {
        if (candidate.endsWith(lookupKey) || lookupKey.endsWith(candidate)) {
          return artifact
        }
      }
    }

    const normalizedFileName = basename(resolvedPath.vpath || resolvedPath.normalizedInput)
    for (const artifact of artifacts) {
      const artifactName = basename(artifact.vpath ?? artifact.displayName ?? artifact.diskPath ?? '')
      if (artifactName && artifactName === normalizedFileName) return artifact
    }

    return null
  }, [artifacts, focusedProjectRoot, normalizedArtifactIndex])

  useEffect(() => {
    if (!fileOpenRequest) return
    const requestId = fileOpenRequest.id
    const path = fileOpenRequest.path
    queueMicrotask(() => {
      setPendingOpenPath(path)
      onFileOpenRequestHandled?.(requestId)
    })
  }, [fileOpenRequest, onFileOpenRequestHandled])

  useEffect(() => {
    if (!taskOpenRequest) return
    const requestId = taskOpenRequest.id
    queueMicrotask(() => {
      setSelectedTask({ task: taskOpenRequest.task, status: taskOpenRequest.status })
      setPanelStack([])
      setActiveTab('task')
      onTaskOpenRequestHandled?.(requestId)
    })
  }, [onTaskOpenRequestHandled, taskOpenRequest])

  useEffect(() => {
    if (!pendingOpenPath) return
    const artifact = findArtifactByPath(pendingOpenPath)
    if (artifact) {
      queueMicrotask(() => {
        openFileTab(artifact)
        setPendingOpenPath(null)
      })
      return
    }
    const resolvedPath = resolveLinkedPath(pendingOpenPath, focusedProjectRoot)
    if (resolvedPath?.vpath) {
      queueMicrotask(() => {
        openFileTab(syntheticFileArtifact(resolvedPath.vpath))
        setPendingOpenPath(null)
      })
      return
    }
    if (artifactQuery.isFetched && !artifactQuery.isFetching) {
      const missingPath = pendingOpenPath
      queueMicrotask(() => {
        toast.error(`Could not find file in this workspace: ${missingPath}`)
        setPendingOpenPath(null)
      })
    }
  }, [artifactQuery.isFetched, artifactQuery.isFetching, findArtifactByPath, focusedProjectRoot, openFileTab, pendingOpenPath])

  const projectFileIndexQuery = useQuery<FilesListDirResult>({
    queryKey: ['context-panel.project-file-index', projectId ?? '', focusedProjectRoot ?? '', spaceId],
    queryFn: async () => {
      if (!projectId || !focusedProjectRoot) return { path: '/project', entries: [] }
      return listProjectDir(projectId, focusedProjectRoot, '/project')
    },
    enabled: (quickOpenOpen || !!activeTab?.startsWith('file:')) && !!projectId && !!focusedProjectRoot,
    staleTime: 5 * 60_000,
    gcTime: 15 * 60_000,
    retry: false,
  })
  const mergedArtifacts = useMemo<ArtifactNode[]>(() => {
    const merged = new Map<string, ArtifactNode>()
    const addArtifact = (artifact: ArtifactNode) => {
      const key = normalizePath(artifact.vpath ?? artifact.displayName ?? artifact.diskPath)
      if (!key || merged.has(key)) return
      merged.set(key, artifact)
    }

    for (const artifact of artifacts) addArtifact(artifact)
    if (focusedProjectRoot && !projectFileIndexQuery.isError) {
      for (const artifact of fileArtifactsFromResults([projectFileIndexQuery.data])) addArtifact(artifact)
    }
    return Array.from(merged.values())
  }, [artifacts, focusedProjectRoot, projectFileIndexQuery.data, projectFileIndexQuery.isError])

  const searchableCandidates = useMemo<SearchCandidate[]>(() => {
    return mergedArtifacts.map((artifact) => {
      const path = artifact.vpath ?? artifact.displayName ?? artifact.diskPath ?? artifact.label
      return {
        key: normalizePath(path),
        artifact,
        name: basename(path),
        path,
        pathLower: path.toLowerCase(),
      }
    })
  }, [mergedArtifacts])

  const filteredSearchCandidates = useMemo(() => {
    const rawQuery = deferredQuickOpenQuery.trim()
    const q = rawQuery.toLowerCase()
    if (!q) return searchableCandidates.slice(0, 60)

    const matches = searchableCandidates.filter((candidate) => candidate.pathLower.includes(q)).slice(0, 119)
    const directPath = resolveQuickOpenPath(rawQuery, focusedProjectRoot)
    if (!directPath) return matches

    const directKey = normalizePath(directPath)
    if (matches.some((candidate) => candidate.key === directKey)) return matches
    return [
      {
        key: directKey,
        artifact: syntheticFileArtifact(directPath),
        name: basename(directPath),
        path: directPath,
        pathLower: directPath.toLowerCase(),
      },
      ...matches,
    ]
  }, [deferredQuickOpenQuery, focusedProjectRoot, searchableCandidates])

  const railRootLabels = useMemo<Record<string, string> | undefined>(() => {
    if (!focusedProjectRoot) return undefined
    const repoName = basename(focusedProjectRoot.replace(/\/+$/, ''))
    return repoName ? { project: repoName } : undefined
  }, [focusedProjectRoot])

  // The rail mirrors only the real project filesystem — agents write into the repo,
  // so there is no separate "workspace" overlay. Agent artifacts feed quick-open, not the tree.
  const railArtifacts = useMemo<ArtifactNode[]>(() => {
    if (!focusedProjectRoot || projectFileIndexQuery.isError) return []
    const merged = new Map<string, ArtifactNode>()
    const addArtifact = (artifact: ArtifactNode | null | undefined) => {
      if (!artifact) return
      const key = artifactIdentity(artifact)
      if (!key || merged.has(key)) return
      merged.set(key, artifact)
    }
    for (const artifact of fileArtifactsFromResults([projectFileIndexQuery.data, ...Object.values(loadedProjectDirs)])) addArtifact(artifact)
    if (activeFileTab?.file.vpath) addArtifact(activeFileTab.file)
    return Array.from(merged.values())
  }, [activeFileTab, focusedProjectRoot, loadedProjectDirs, projectFileIndexQuery.data, projectFileIndexQuery.isError])

  const railDirectories = useMemo<ArtifactTreeDirectory[]>(() => {
    if (!focusedProjectRoot || projectFileIndexQuery.isError) return []
    const paths = new Set<string>(['/project'])
    for (const result of [projectFileIndexQuery.data, ...Object.values(loadedProjectDirs)]) {
      for (const entry of result?.entries ?? []) {
        if (entry.isDir) paths.add(entry.path)
      }
    }
    return Array.from(paths).map((path) => ({ path }))
  }, [focusedProjectRoot, loadedProjectDirs, projectFileIndexQuery.data, projectFileIndexQuery.isError])

  const loadRailDirectory = useCallback((path: string) => {
    if (!projectId || !focusedProjectRoot) return
    if (path === '/project' || loadedProjectDirs[path] || loadingProjectDirs.has(path)) return
    setLoadingProjectDirsState((prev) => {
      const base = prev.key === projectRailKey ? prev.dirs : new Set<string>()
      return { key: projectRailKey, dirs: new Set(base).add(path) }
    })
    void listProjectDir(projectId, focusedProjectRoot, path)
      .then((result) => {
        setLoadedProjectDirsState((prev) => {
          const base = prev.key === projectRailKey ? prev.dirs : {}
          return { key: projectRailKey, dirs: { ...base, [path]: result } }
        })
      })
      .catch((error) => {
        toast.error(`Could not load ${path}: ${error instanceof Error ? error.message : String(error)}`)
      })
      .finally(() => {
        setLoadingProjectDirsState((prev) => {
          const next = new Set(prev.key === projectRailKey ? prev.dirs : [])
          next.delete(path)
          return { key: projectRailKey, dirs: next }
        })
      })
  }, [focusedProjectRoot, loadedProjectDirs, loadingProjectDirs, projectId, projectRailKey])

  const activeFilePath = activeFileTab?.file.vpath
    ?? activeFileTab?.file.displayName
    ?? activeFileTab?.file.diskPath
    ?? ''

  const breadcrumbParts = useMemo(() => {
    const rawPath = activeFilePath.trim()
    if (!rawPath) return []
    return rawPath.split('/').filter(Boolean)
  }, [activeFilePath])
  const showContextBreadcrumbs = !!activeFileTab && breadcrumbParts.length > 0 && !isMarkdownFile(activeFilePath)

  const fileTabBaseClass = 'inline-flex items-center gap-1.5 h-full px-2.5 text-[12px] font-medium whitespace-nowrap border-b-2 border-b-transparent transition-colors -mb-px'

  return (
    <div className="flex flex-col h-full bg-[var(--bg-panel)] min-w-0 min-h-0 overflow-hidden">
      <div className="h-12 flex items-center gap-1 px-3 shrink-0">
        <div className="flex-1 min-w-0 overflow-x-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
          <div className="flex items-center gap-1 min-w-max">
            {selectedTask ? (
              <button
                type="button"
                className={cn(
                  fileTabBaseClass,
                  activeTab === 'task'
                    ? 'text-[var(--text-1)] border-b-[var(--accent)]'
                    : 'text-[var(--text-3)] hover:text-[var(--text-1)]',
                )}
                onClick={() => { setActiveTab('task'); setPanelStack([]) }}
                title={stackTopEntity?.title ?? taskTitle(selectedTask.task)}
              >
                <span className="truncate max-w-[120px]">
                  {panelStack.length > 0 && stackTopEntity
                    ? stackTopEntity.title
                    : taskTitle(selectedTask.task)}
                </span>
              </button>
            ) : null}
            {resolvedFileTabs.map((tab) => {
              const isActive = activeTab === `file:${tab.tabId}`
              const label = basename(tab.file.vpath ?? tab.file.displayName ?? tab.file.diskPath ?? tab.file.label)
              return (
                <button
                  key={tab.tabId}
                  type="button"
                  className={cn(
                    fileTabBaseClass,
                    'group',
                    isActive
                      ? 'text-[var(--text-1)] border-b-[var(--accent)]'
                      : 'text-[var(--text-3)] hover:text-[var(--text-1)]',
                  )}
                  onClick={() => setActiveTab(`file:${tab.tabId}`)}
                  title={tab.file.vpath ?? tab.file.displayName ?? tab.file.diskPath ?? label}
                >
                  <FileText size={13} className="shrink-0" />
                  <span className="max-w-[130px] truncate">{label}</span>
                  <span
                    role="button"
                    tabIndex={0}
                    className="inline-flex h-4 w-4 items-center justify-center rounded-[4px] text-[var(--text-3)] hover:text-[var(--text-1)] hover:bg-[var(--bg-hover)] opacity-0 pointer-events-none transition-opacity duration-150 group-hover:opacity-100 group-hover:pointer-events-auto group-focus-within:opacity-100 group-focus-within:pointer-events-auto"
                    onClick={(event) => {
                      event.preventDefault()
                      event.stopPropagation()
                      closeFileTab(tab.tabId)
                    }}
                    onKeyDown={(event) => {
                      if (event.key !== 'Enter' && event.key !== ' ') return
                      event.preventDefault()
                      event.stopPropagation()
                      closeFileTab(tab.tabId)
                    }}
                    aria-label={`Close ${label}`}
                  >
                    <X size={12} />
                  </span>
                </button>
              )
            })}
          </div>
        </div>

        <Button type="button" variant="ghost" size="icon" className="h-7 w-7 shrink-0" aria-label="Open file" onClick={() => setQuickOpenOpen(true)}>
          <Plus size={14} />
        </Button>
        {activeFileTab && activeTab?.startsWith('file:') && (
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="h-7 w-7 shrink-0"
            aria-label={fileRailHidden ? 'Show file rail' : 'Hide file rail'}
            title={fileRailHidden ? 'Show file rail' : 'Hide file rail'}
            onClick={() => setFileRailHidden((hidden) => !hidden)}
          >
            <FolderTree size={14} />
          </Button>
        )}

      </div>

      <Dialog open={quickOpenOpen} onOpenChange={(open) => {
        setQuickOpenOpen(open)
        if (!open) setQuickOpenQuery('')
      }}>
        <DialogContent
          className="w-[min(92vw,720px)] p-0 gap-0 border border-[var(--border)] bg-[var(--bg-panel)] shadow-[var(--shadow-overlay)] rounded-[14px] overflow-hidden [&>button]:hidden"
        >
          <DialogTitle className="sr-only">Search files</DialogTitle>
          <DialogDescription className="sr-only">Search and open files from the current project.</DialogDescription>
          <div className="border-b border-[var(--border)] bg-[var(--bg-panel)]">
            <div className="flex items-center justify-between gap-3 px-4 pt-3 pb-2">
              <div className="min-w-0">
                <div className="text-[11px] uppercase tracking-[0.14em] text-[var(--text-3)]">Quick open</div>
                <div className="mt-0.5 text-[13px] text-[var(--text-2)]">Search project files by name or path.</div>
              </div>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                className="h-7 w-7 shrink-0"
                aria-label="Close file search"
                onClick={() => setQuickOpenOpen(false)}
              >
                <X size={14} />
              </Button>
            </div>
            <div className="px-3 pb-3">
              <div className="relative rounded-[9px] border border-[var(--border)] bg-[var(--bg-app)] shadow-[inset_0_1px_0_rgba(255,255,255,0.03)]">
                <Search size={16} className="absolute left-3 top-1/2 -translate-y-1/2 text-[var(--text-3)] pointer-events-none" />
              <Input
                value={quickOpenQuery}
                onChange={(event) => setQuickOpenQuery(event.target.value)}
                placeholder="Search files"
                aria-label="Search files"
                className="h-11 pl-10 pr-20 text-[14px] border-0 bg-transparent focus-visible:ring-0 focus-visible:outline-none focus-visible:border-0"
                autoFocus
                onKeyDown={(event) => {
                  if (event.key !== 'Enter') return
                  const first = filteredSearchCandidates[0]
                  if (!first) return
                  openFileTab(first.artifact)
                }}
              />
                <div className="pointer-events-none absolute right-2 top-1/2 -translate-y-1/2 rounded-[6px] border border-[var(--border)] bg-[var(--bg-surface)] px-1.5 py-0.5 font-mono text-[10px] text-[var(--text-3)]">
                  Enter
                </div>
              </div>
            </div>
          </div>
          <div className="max-h-[54vh] overflow-y-auto p-2">
            {projectFileIndexQuery.isLoading && filteredSearchCandidates.length === 0 ? (
              <div className="px-3 py-8 text-center text-[12px] text-[var(--text-3)]">Loading project files...</div>
            ) : projectFileIndexQuery.error ? (
              <div className="rounded-[10px] border border-[var(--red)]/30 bg-[var(--red)]/10 px-3 py-3 text-[12px] text-[var(--red)]">
                Failed to index project files: {projectFileIndexQuery.error instanceof Error ? projectFileIndexQuery.error.message : String(projectFileIndexQuery.error)}
              </div>
            ) : filteredSearchCandidates.length === 0 ? (
              <div className="px-3 py-8 text-center text-[12px] text-[var(--text-3)]">No files found</div>
            ) : (
              filteredSearchCandidates.map((candidate) => {
                return (
                  <button
                    key={candidate.key}
                    type="button"
                    className="group flex w-full items-center gap-3 rounded-[10px] px-2.5 py-2 text-left transition-colors hover:bg-[var(--bg-hover)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]"
                    onClick={() => openFileTab(candidate.artifact)}
                  >
                    <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-[8px] border border-[var(--border)] bg-[var(--bg-surface)] text-[var(--text-3)] group-hover:text-[var(--accent)]">
                      <FileText size={15} />
                    </span>
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-[13px] font-medium text-[var(--text-1)]">{candidate.name}</span>
                      <span className="mt-0.5 block truncate font-mono text-[11px] text-[var(--text-3)]">{candidate.path}</span>
                    </span>
                    <span className="hidden shrink-0 rounded-[6px] border border-[var(--border)] px-1.5 py-0.5 font-mono text-[10px] text-[var(--text-3)] group-hover:inline-flex">
                      Open
                    </span>
                  </button>
                )
              })
            )}
          </div>
        </DialogContent>
      </Dialog>

      {showContextBreadcrumbs && (
        <div className="px-3 pt-2 pb-1.5 shrink-0">
          <Breadcrumb>
            <BreadcrumbList>
              {breadcrumbParts.map((part, index) => {
                const fullPath = `/${breadcrumbParts.slice(0, index + 1).join('/')}`
                const isLast = index === breadcrumbParts.length - 1
                return (
                  <div key={`${fullPath}-${index}`} className="flex items-center">
                    <BreadcrumbItem>
                      {isLast ? (
                        <BreadcrumbPage className="max-w-[160px] truncate text-xs">
                          {displayVirtualPathPart(part, index, focusedProjectRoot)}
                        </BreadcrumbPage>
                      ) : (
                        <BreadcrumbLink
                          className="max-w-[120px] truncate text-xs cursor-pointer"
                          onClick={(event) => {
                            event.preventDefault()
                            setQuickOpenQuery(fullPath)
                            setQuickOpenOpen(true)
                          }}
                        >
                          {displayVirtualPathPart(part, index, focusedProjectRoot)}
                        </BreadcrumbLink>
                      )}
                    </BreadcrumbItem>
                    {!isLast && <BreadcrumbSeparator />}
                  </div>
                )
              })}
            </BreadcrumbList>
          </Breadcrumb>
        </div>
      )}

      <div className="flex-1 min-h-0 overflow-hidden">
        {activeTab === 'task' && selectedTask && (
          <PanelNavigationContext.Provider value={(nodeId) => setPanelStack(prev => [...prev, nodeId])}>
            {panelStack.length > 0 ? (
              <StackedPanel
                nodeId={panelStack[panelStack.length - 1]}
                projectId={projectId ?? ''}
                projectRoot={focusedProjectRoot}
                onBack={() => setPanelStack(prev => prev.slice(0, -1))}
                onClose={() => {
                  setPanelStack([])
                  setSelectedTask(null)
                  setActiveTab(null)
                }}
              />
            ) : (
              <TaskPanel
                data={{ task: selectedTask.task }}
                projectId={projectId ?? ''}
                projectRoot={focusedProjectRoot}
                onClose={() => {
                  setSelectedTask(null)
                  setActiveTab(null)
                }}
              />
            )}
          </PanelNavigationContext.Provider>
        )}

        {activeFileTab && activeTab?.startsWith('file:') && (
          <div className="flex h-full min-h-0">
            <div className="flex-1 min-w-0 min-h-0">
              <ArtifactViewer
                file={activeFileTab.file}
                preview={previewQuery.data}
                isLoading={previewQuery.isLoading && !previewQuery.isPlaceholderData}
                error={!!previewQuery.error}
                variant="slideover"
              />
            </div>
            {!fileRailHidden && (
              <ArtifactTree
                artifacts={railArtifacts}
                directories={railDirectories}
                selectedIdentity={activeFileTab.tabId}
                onSelectFile={openFileTab}
                onExpandDirectory={loadRailDirectory}
                isLoading={railArtifacts.length === 0 && projectFileIndexQuery.isLoading}
                loadingDirectories={loadingProjectDirs}
                rootLabels={railRootLabels}
                className="w-[264px] shrink-0 border-l border-[var(--border)]"
              />
            )}
          </div>
        )}
      </div>
    </div>
  )
}

function normalizePath(value: string | undefined): string {
  const raw = (value ?? '').trim()
  if (!raw) return ''
  return raw.replace(/\\/g, '/').replace(/\/+$/g, '').toLowerCase()
}

interface ResolvedLinkedPath {
  normalizedInput: string
  vpath: string
  lookupKeys: string[]
}

function resolveLinkedPath(path: string, projectRoot: string | null | undefined): ResolvedLinkedPath | null {
  const normalizedInput = normalizeFsPath(path)
  if (!normalizedInput) return null

  const vpath = inferManagedVPath(normalizedInput, projectRoot)
  const lookupKeys = dedupeLookupKeys([normalizedInput, vpath])
  return { normalizedInput, vpath, lookupKeys }
}

function normalizeFsPath(value: string | undefined): string {
  const raw = (value ?? '').trim()
  if (!raw) return ''
  const normalized = stripPathLocationSuffix(raw).replace(/\\/g, '/')
  if (normalized === '/') return normalized
  const trimmed = normalized.replace(/\/+$/g, '')
  return trimmed || normalized
}

function stripPathLocationSuffix(path: string): string {
  return path.replace(/#L\d+(?:C\d+)?$/i, '').replace(/:\d+(?::\d+)?$/, '')
}

function inferManagedVPath(path: string, projectRoot: string | null | undefined): string {
  if (path === '/workspace' || path.startsWith('/workspace/')) return path
  if (path === '/project' || path.startsWith('/project/')) return path

  const normalizedProjectRoot = normalizeFsPath(projectRoot ?? '')
  if (normalizedProjectRoot) {
    const workspaceRoots = [
      `${normalizedProjectRoot}/workspace`,
      `${normalizedProjectRoot}/.agen8/workspace`,
    ]
    for (const workspaceRoot of workspaceRoots) {
      if (path === workspaceRoot || path.startsWith(`${workspaceRoot}/`)) {
        const relative = path.slice(workspaceRoot.length).replace(/^\/+/, '')
        return relative ? `/workspace/${relative}` : '/workspace'
      }
    }
    if (path === normalizedProjectRoot || path.startsWith(`${normalizedProjectRoot}/`)) {
      const relative = path.slice(normalizedProjectRoot.length).replace(/^\/+/, '')
      return relative ? `/project/${relative}` : '/project'
    }
  }

  const relative = path.replace(/^\.\/+/, '')
  if (relative === '.agen8/workspace' || relative.startsWith('.agen8/workspace/')) {
    const suffix = relative.slice('.agen8/workspace'.length).replace(/^\/+/, '')
    return suffix ? `/workspace/${suffix}` : '/workspace'
  }
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
  if (normalizedProjectRoot) {
    const projectName = basename(normalizedProjectRoot)
    if (projectName && (relative === projectName || relative.startsWith(`${projectName}/`))) {
      const suffix = relative.slice(projectName.length).replace(/^\/+/, '')
      return suffix ? `/project/${suffix}` : '/project'
    }
  }
  if (!path.startsWith('/')) {
    return `/project/${relative.replace(/^\/+/, '')}`
  }

  return ''
}

function dedupeLookupKeys(values: string[]): string[] {
  const unique = new Set<string>()
  for (const value of values) {
    const key = normalizePath(value)
    if (!key) continue
    unique.add(key)
  }
  return Array.from(unique)
}

function syntheticFileArtifact(vpath: string): ArtifactNode {
  const label = basename(vpath)
  return {
    nodeKey: `file:${vpath}`,
    kind: 'file',
    label,
    displayName: label,
    vpath,
  }
}

function resolveQuickOpenPath(query: string, projectRoot: string | null | undefined): string {
  const normalized = normalizeFsPath(query)
  if (!normalized) return ''
  const looksLikePath = normalized.includes('/') || normalized.startsWith('/project') || normalized.startsWith('/workspace') || /\.[A-Za-z0-9]+$/.test(normalized)
  if (!looksLikePath) return ''

  const resolved = resolveLinkedPath(normalized, projectRoot)
  if (resolved?.vpath) return resolved.vpath
  if (!normalized.startsWith('/')) return `/project/${normalized.replace(/^\/+/, '')}`
  return normalized
}

function fileArtifactsFromResults(results: Array<FilesListDirResult | undefined>): ArtifactNode[] {
  const artifacts: ArtifactNode[] = []
  for (const result of results) {
    for (const entry of result?.entries ?? []) {
      if (!entry.isDir) artifacts.push(syntheticFileArtifact(entry.path))
    }
  }
  return artifacts
}

async function listProjectDir(projectId: string, projectRoot: string, path: string): Promise<FilesListDirResult> {
  return rpcCall<FilesListDirResult>('files.listDir', {
    projectId,
    projectRoot,
    path,
    includeHidden: true,
  })
}
