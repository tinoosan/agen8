import { useState, useMemo, type ReactNode } from 'react'
import clsx from 'clsx'
import { artifactIdentity } from '../../lib/artifactNodes'
import { File, ChevronRight, ChevronDown, Folder, FolderOpen, Search } from 'lucide-react'
import { Skeleton } from '@/components/ui/skeleton'
import type { ArtifactNode } from '../../lib/types'
import { basename } from './filePreviewUtils'

/* ── Icon helpers ─────────────────────────────── */

function fileExtension(path: string): string {
  const name = basename(path).toLowerCase()
  if (name === 'dockerfile') return 'docker'
  if (name === 'license') return 'license'
  if (name === 'makefile') return 'make'
  if (name === 'docker-compose.yml' || name === 'docker-compose.yaml') return 'docker'
  const ext = name.split('.').pop() ?? ''
  return ext === name ? '' : ext
}

function fileBadgeLabel(ext: string): string {
  if (ext === 'typescript') return 'TS'
  if (ext === 'javascript') return 'JS'
  if (ext === 'yaml' || ext === 'yml') return 'Y'
  if (ext === 'json') return '{}'
  if (ext === 'toml') return 'T'
  if (ext === 'md' || ext === 'markdown') return 'M'
  if (ext === 'docker') return 'D'
  if (ext === 'make') return 'MK'
  if (ext === 'go') return 'GO'
  if (ext === 'tsx') return 'TS'
  if (ext === 'jsx') return 'JS'
  if (ext === 'ts') return 'TS'
  if (ext === 'js') return 'JS'
  if (ext === 'css') return 'C'
  if (ext === 'html') return 'H'
  if (ext === 'svg') return 'S'
  if (ext === 'png' || ext === 'jpg' || ext === 'jpeg' || ext === 'webp' || ext === 'gif') return 'I'
  return ext.slice(0, 2).toUpperCase() || ''
}

function fileBadgeClass(ext: string): string {
  if (ext === 'md' || ext === 'markdown') return 'bg-[#4f7cff] text-white'
  if (ext === 'go') return 'bg-[#00add8] text-white'
  if (ext === 'json') return 'bg-[#d9a441] text-[#1a1a1c]'
  if (ext === 'yaml' || ext === 'yml' || ext === 'toml') return 'bg-[#8b5cf6] text-white'
  if (ext === 'docker') return 'bg-[#2496ed] text-white'
  if (ext === 'make') return 'bg-[#e8743b] text-white'
  if (ext === 'png' || ext === 'jpg' || ext === 'jpeg' || ext === 'webp' || ext === 'gif' || ext === 'svg') return 'bg-[#22a06b] text-white'
  if (ext === 'ts' || ext === 'tsx') return 'bg-[#3178c6] text-white'
  if (ext === 'js' || ext === 'jsx') return 'bg-[#f7df1e] text-[#1a1a1c]'
  if (ext === 'css') return 'bg-[#2965f1] text-white'
  if (ext === 'html') return 'bg-[#e34f26] text-white'
  if (ext === 'txt' || ext === 'license') return 'bg-[#6b7280] text-white'
  return 'bg-[#475569] text-white'
}

function FileTypeBadge({ path }: { path: string }) {
  const ext = fileExtension(path)
  const label = fileBadgeLabel(ext)
  if (!label) return <File size={14} className="text-[var(--text-3)]" />
  return (
    <span
      aria-hidden="true"
      className={clsx(
        'flex h-[17px] w-[17px] shrink-0 items-center justify-center rounded-[4px] text-[0.5rem] font-extrabold leading-none tracking-normal',
        fileBadgeClass(ext),
      )}
    >
      {label}
    </span>
  )
}

/* ── Artifact tree model ──────────────────────── */

interface ArtifactTreeNode {
  name: string
  fullPath: string
  children: ArtifactTreeNode[]
  files: ArtifactNode[]
  explicitDir?: boolean
}

export interface ArtifactTreeDirectory {
  path: string
}

function buildArtifactTree(artifacts: ArtifactNode[], rootLabels?: Record<string, string>, directories: ArtifactTreeDirectory[] = []): ArtifactTreeNode[] {
  const root: ArtifactTreeNode = { name: '', fullPath: '', children: [], files: [] }

  function ensureDir(parts: string[], explicitDir: boolean) {
    let current = root
    let pathSoFar = ''
    for (const part of parts) {
      pathSoFar += '/' + part
      let child = current.children.find(c => c.fullPath === pathSoFar)
      if (!child) {
        child = { name: part, fullPath: pathSoFar, children: [], files: [] }
        current.children.push(child)
      }
      if (explicitDir && pathSoFar === `/${parts.join('/')}`) child.explicitDir = true
      current = child
    }
  }

  for (const directory of directories) {
    const parts = directory.path.split('/').filter(Boolean)
    if (parts.length > 0) ensureDir(parts, true)
  }

  for (const artifact of artifacts) {
    const vpath = artifact.vpath ?? artifact.displayName ?? artifact.artifactId ?? ''
    if (!vpath) { root.files.push(artifact); continue }

    const parts = vpath.split('/').filter(Boolean)
    const fileName = parts.pop()
    if (!fileName) { root.files.push(artifact); continue }

    ensureDir(parts, false)
    let current = root
    for (const part of parts) {
      const nextPath = `${current.fullPath}/${part}`.replace(/\/+/g, '/')
      const child = current.children.find(c => c.fullPath === nextPath)
      if (!child) break
      current = child
    }
    current.files.push(artifact)
  }

  // Relabel top-level virtual-path segments for display only (e.g. "project" → repo name).
  // fullPath is preserved so artifact identity, RPC paths, and expand state are unaffected.
  if (rootLabels) {
    for (const child of root.children) {
      const alias = rootLabels[child.name]
      if (alias) child.name = alias
    }
  }

  function collapse(node: ArtifactTreeNode): ArtifactTreeNode {
    node.children = node.children.map(collapse)
    if (!node.explicitDir && node.children.length === 1 && node.files.length === 0 && node.name !== '') {
      const child = node.children[0]
      return { name: `${node.name}/${child.name}`, fullPath: child.fullPath, children: child.children, files: child.files, explicitDir: child.explicitDir }
    }
    return node
  }
  root.children = root.children.map(collapse)

  function sortTree(node: ArtifactTreeNode) {
    node.children.sort((a, b) => a.name.localeCompare(b.name))
    node.files.sort((a, b) => {
      const na = basename(a.displayName ?? a.vpath ?? a.artifactId ?? '')
      const nb = basename(b.displayName ?? b.vpath ?? b.artifactId ?? '')
      return na.localeCompare(nb)
    })
    node.children.forEach(sortTree)
  }
  sortTree(root)

  return root.children.length > 0 || root.files.length > 0
    ? root.children.concat(root.files.length > 0 && root.children.length > 0
      ? [{ name: 'Other', fullPath: '/__other__', children: [], files: root.files }]
      : root.files.length > 0 && root.children.length === 0
        ? [{ name: '', fullPath: '/__root_files__', children: [], files: root.files }]
        : [])
    : []
}

function countAllFiles(node: ArtifactTreeNode): number {
  return node.files.length + node.children.reduce((acc, c) => acc + countAllFiles(c), 0)
}

function collectAllDirPaths(nodes: ArtifactTreeNode[]): Set<string> {
  const paths = new Set<string>()
  function walk(node: ArtifactTreeNode) {
    if (node.children.length > 0 || node.files.length > 0) paths.add(node.fullPath)
    node.children.forEach(walk)
  }
  nodes.forEach(walk)
  return paths
}

function findMatchingDirPaths(nodes: ArtifactTreeNode[], filter: string): Set<string> {
  const paths = new Set<string>()
  const lower = filter.toLowerCase()
  function walk(node: ArtifactTreeNode, ancestors: string[]) {
    const hasMatch = node.files.some(f =>
      (f.displayName ?? f.vpath ?? f.artifactId ?? '').toLowerCase().includes(lower)
    )
    const cur = [...ancestors, node.fullPath]
    if (hasMatch) cur.forEach(a => paths.add(a))
    node.children.forEach(c => walk(c, cur))
  }
  nodes.forEach(n => walk(n, []))
  return paths
}

/* ── Component ─────────────────────────────────── */

interface ArtifactTreeProps {
  artifacts: ArtifactNode[]
  directories?: ArtifactTreeDirectory[]
  selectedIdentity: string | null
  onSelectFile: (file: ArtifactNode) => void
  onExpandDirectory?: (path: string) => void
  isLoading?: boolean
  loadingDirectories?: Set<string>
  className?: string
  rootLabels?: Record<string, string>
}

export default function ArtifactTree({
  artifacts,
  directories = [],
  selectedIdentity,
  onSelectFile,
  onExpandDirectory,
  isLoading = false,
  loadingDirectories,
  className,
  rootLabels,
}: ArtifactTreeProps) {
  const [fileSearch, setFileSearch] = useState('')
  const [userCollapsedDirs, setUserCollapsedDirs] = useState<Set<string>>(new Set())

  const tree = useMemo(() => buildArtifactTree(artifacts, rootLabels, directories), [artifacts, directories, rootLabels])

  const expandedDirs = useMemo(() => {
    if (fileSearch) return findMatchingDirPaths(tree, fileSearch)
    const topLevel = new Set(tree.map(n => n.fullPath))
    const allDirs = collectAllDirPaths(tree)
    const result = new Set<string>()
    for (const path of allDirs) {
      const isDefault = topLevel.has(path)
      const isToggled = userCollapsedDirs.has(path)
      if (isDefault ? !isToggled : isToggled) result.add(path)
    }
    return result
  }, [tree, fileSearch, userCollapsedDirs])

  const filteredArtifacts = useMemo(() => {
    if (!fileSearch) return artifacts
    const lower = fileSearch.toLowerCase()
    return artifacts.filter(a =>
      (a.displayName ?? a.vpath ?? a.artifactId ?? '').toLowerCase().includes(lower)
    )
  }, [artifacts, fileSearch])

  function toggleDir(fullPath: string) {
    setUserCollapsedDirs(prev => {
      const next = new Set(prev)
      const isDefaultExpanded = tree.some(node => node.fullPath === fullPath)
      const isExpanded = isDefaultExpanded ? !next.has(fullPath) : next.has(fullPath)
      const willExpand = !isExpanded
      if (next.has(fullPath)) next.delete(fullPath)
      else next.add(fullPath)
      if (willExpand) onExpandDirectory?.(fullPath)
      return next
    })
  }

  function renderFileItem(a: ArtifactNode, i: number, depth: number) {
    const path = a.displayName ?? a.vpath ?? a.artifactId ?? `artifact-${i}`
    const isSelected = selectedIdentity != null && selectedIdentity === artifactIdentity(a)

    if (fileSearch) {
      const filePath = (a.displayName ?? a.vpath ?? a.artifactId ?? '').toLowerCase()
      if (!filePath.includes(fileSearch.toLowerCase())) return null
    }

    return (
      <button
        type="button"
        key={a.nodeKey ?? artifactIdentity(a) ?? a.artifactId ?? i}
        onClick={() => onSelectFile(a)}
        className={clsx(
          'w-full text-left flex items-center gap-2.5 py-2 cursor-pointer rounded-[var(--r-md)] mb-0.5 transition-[background,border-color] duration-[120ms] bg-transparent border-none font-[inherit]',
          isSelected
            ? 'bg-[var(--bg-active)] border-l-2 border-l-[var(--accent)]'
            : 'row-hover border-l-2 border-l-transparent',
        )}
        style={{ paddingLeft: 12 + depth * 14, paddingRight: 12 }}
      >
        <div className="shrink-0 flex items-center">
          <FileTypeBadge path={path} />
        </div>
        <div className="flex-1 min-w-0">
          <div className={clsx('truncate text-xs text-[var(--text-1)] tracking-[-0.01em]', isSelected ? 'font-semibold' : 'font-medium')}>
            {basename(path)}
          </div>
          {a.member && (
            <div className="text-[0.625rem] text-[var(--text-3)] mt-0.5 truncate">{a.member}</div>
          )}
        </div>
      </button>
    )
  }

  function renderTreeNode(node: ArtifactTreeNode, depth: number): ReactNode {
    if (node.fullPath === '/__root_files__') {
      return node.files.map((f, i) => renderFileItem(f, i, depth))
    }

    const isDir = node.explicitDir || node.children.length > 0 || (node.files.length > 0 && node.name !== '')
    const isExpanded = expandedDirs.has(node.fullPath)
    const isLoadingDir = loadingDirectories?.has(node.fullPath) ?? false

    if (fileSearch && !expandedDirs.has(node.fullPath)) return null

    if (!isDir) return node.files.map((f, i) => renderFileItem(f, i, depth))

    return (
      <div key={node.fullPath}>
        <button
          type="button"
          onClick={() => toggleDir(node.fullPath)}
          className="w-full text-left flex items-center gap-1.5 py-1.5 cursor-pointer rounded-[var(--r-md)] mb-0.5 row-hover bg-transparent border-none font-[inherit]"
          style={{ paddingLeft: 8 + depth * 14, paddingRight: 12 }}
        >
          {isExpanded
            ? <ChevronDown size={11} className="text-[var(--text-3)] shrink-0" />
            : <ChevronRight size={11} className="text-[var(--text-3)] shrink-0" />
          }
          {isExpanded
            ? <FolderOpen size={13} className="shrink-0 text-[var(--text-3)]" />
            : <Folder size={13} className="shrink-0 text-[var(--text-3)]" />
          }
          <span className="truncate text-[0.6875rem] font-medium text-[var(--text-2)]">{node.name}</span>
          <span className="text-[0.625rem] text-[var(--text-3)] tabular-nums shrink-0">{isLoadingDir ? '...' : countAllFiles(node)}</span>
        </button>
        {isExpanded && (
          <>
            {node.children.map(child => renderTreeNode(child, depth + 1))}
            {node.files.map((f, i) => renderFileItem(f, i, depth + 1))}
          </>
        )}
      </div>
    )
  }

  return (
    <div className={clsx('flex flex-col min-h-0', className)}>
      {/* Search */}
      {artifacts.length > 3 && (
        <div className="p-2 shrink-0">
          <div className="flex items-center gap-1.5 bg-[var(--bg-surface)] border border-[var(--border)] rounded-[var(--r-sm)] px-2 py-[5px]">
            <Search size={11} className="text-[var(--text-3)] shrink-0" />
            <input
              type="text"
              placeholder="Filter..."
              aria-label="Filter files"
              value={fileSearch}
              onChange={e => setFileSearch(e.target.value)}
              className="border-none outline-none bg-transparent flex-1 text-[0.6875rem] text-[var(--text-1)] font-[inherit] min-w-0"
            />
          </div>
        </div>
      )}

      {/* Entries */}
      <div className="flex-1 overflow-y-auto px-2 py-1">
        {isLoading ? (
          <div className="flex flex-col gap-1.5 p-3">
            {[1, 2, 3].map(i => <Skeleton key={i} className="w-full h-11 rounded-[var(--r-md)]" />)}
          </div>
        ) : (fileSearch ? filteredArtifacts.length === 0 : tree.length === 0) ? (
          <div className="flex flex-col items-center px-5 py-12 text-center gap-2.5">
            <div className="w-12 h-12 rounded-xl bg-[var(--bg-surface)] border border-[var(--border)] flex items-center justify-center">
              <File size={20} className="text-[var(--text-3)]" />
            </div>
            <div className="text-[var(--text-3)] text-[0.8125rem]">
              {fileSearch ? 'No files match' : 'No files yet'}
            </div>
            {!fileSearch && (
              <div className="text-[var(--text-3)] text-[0.6875rem] leading-normal">
                Files will appear here as agents generate them
              </div>
            )}
          </div>
        ) : (
          tree.map(node => renderTreeNode(node, 0))
        )}
      </div>
    </div>
  )
}
