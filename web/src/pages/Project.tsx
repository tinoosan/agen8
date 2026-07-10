import { useEffect, useMemo, useState } from 'react'
import { useLocation } from 'wouter'
import { useProjects } from '../hooks/useProjects'
import { rpcCall } from '../lib/rpc'
import { toast } from 'sonner'
import { useQueryClient } from '@tanstack/react-query'
import { qk } from '../lib/queryKeys'
import { projectDisplayName } from '@/lib/projectHelpers'
import { Plus, Search } from 'lucide-react'
import type { Project, ProjectClaudeMCPConfigureResult, ProjectCreateResult } from '../lib/types'
import LinkFolderDialog from '../components/projects/LinkFolderDialog'
import EditProjectDialog from '../components/projects/EditProjectDialog'
import CreateProjectDialog from '../components/projects/CreateProjectDialog'
import ProjectTableRow from '../components/projects/ProjectTableRow'
import RemoveProjectDialog, { type ProjectRemoveAction } from '../components/projects/RemoveProjectDialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table, TableBody, TableHead, TableHeader, TableRow,
} from '@/components/ui/table'
import { useStore } from '../lib/store'
import { cn } from '@/lib/utils'
import { brandIconFor } from '../lib/brandIcon'

/* ── Types ───────────────────────────────────────── */

type StatusFilter = 'all' | 'active' | 'archived'

/* ── FilterChip ──────────────────────────────────── */

function FilterChip({
  active,
  onClick,
  children,
}: {
  active: boolean
  onClick: () => void
  children: React.ReactNode
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        'inline-flex items-center gap-1 whitespace-nowrap rounded-full border px-2.5 py-1 text-[0.6875rem] font-medium transition-colors cursor-pointer',
        active
          ? 'border-[var(--accent)] bg-[var(--accent-dim)] text-[var(--accent)]'
          : 'border-[var(--border)] bg-transparent text-[var(--text-3)] hover:border-[var(--border-strong)] hover:text-[var(--text-2)]',
      )}
    >
      {children}
    </button>
  )
}

/* ── Main Project Page ───────────────────────────── */

export default function ProjectPage() {
  const theme = useStore((s) => s.theme)
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('all')
  const [searchQuery, setSearchQuery] = useState('')
  const projectsQuery = useProjects({ includeArchived: true })
  const allProjects = useMemo(() => projectsQuery.data ?? [], [projectsQuery.data])
  const isLoading = projectsQuery.isLoading
  const isError = projectsQuery.isError
  const queryClient = useQueryClient()
  const [, navigate] = useLocation()
  const [createOpen, setCreateOpen] = useState(() =>
    new URLSearchParams(window.location.search).get('new') === 'true'
  )
  const [removeTarget, setRemoveTarget] = useState<{ project: Project; action: ProjectRemoveAction } | null>(null)
  const [linkTarget, setLinkTarget] = useState<Project | null>(null)
  const [editTarget, setEditTarget] = useState<Project | null>(null)

  // Filter projects by status + search
  const filteredProjects = useMemo(() => {
    let list = allProjects
    if (statusFilter === 'active') list = list.filter(p => p.status === 'open')
    else if (statusFilter === 'archived') list = list.filter(p => p.status === 'archived')
    if (searchQuery.trim()) {
      const q = searchQuery.toLowerCase().trim()
      list = list.filter(p =>
        projectDisplayName(p).toLowerCase().includes(q) ||
        p.root.toLowerCase().includes(q)
      )
    }
    return list
  }, [allProjects, statusFilter, searchQuery])

  const hasArchived = allProjects.some(p => p.status === 'archived')

  useEffect(() => {
    if (new URLSearchParams(window.location.search).get('new') === 'true') {
      window.history.replaceState({}, '', '/')
    }
  }, [])

  const handleCreateSuccess = ({ project, setup }: ProjectCreateResult) => {
    queryClient.invalidateQueries({ queryKey: qk.projectsAll })
    toast.success(`Project "${projectDisplayName(project)}" created`)
    if (setup?.claudeMcpConfigured && setup.hooksInstalled) {
      toast.success('Claude MCP and attention hooks configured')
    } else if (setup && (setup.attempted || (setup.warnings?.length ?? 0) > 0)) {
      toast.warning(setup.warnings?.join(' ') || 'Some local harness setup could not be completed')
    }
    if (project.id) {
      navigate(`/project/${encodeURIComponent(project.id)}`)
    }
  }

  const configureClaudeMCP = async (project: Project) => {
    try {
      const result = await rpcCall<ProjectClaudeMCPConfigureResult>('project.claudeMCP.configure', { projectId: project.id })
      if (!result.installed) throw new Error('Claude MCP configuration was not installed')
      toast.success(`Claude MCP configured for "${projectDisplayName(project)}"`)
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'Failed to configure Claude MCP')
    }
  }

  return (
    <div className="h-full overflow-y-auto">
      <div className="mx-auto w-full max-w-[960px] px-4 py-6 sm:px-6 md:px-10 md:py-8">
        {/* Header */}
        <div className="flex items-center gap-3 mb-5 flex-wrap">
          <h1 className="m-0 text-[1.125rem] font-semibold tracking-[-0.03em] text-[var(--text-1)] leading-[1.1] sm:text-[1.25rem] hidden md:block">
            Projects
          </h1>
          <div className="flex-1" />
          {allProjects.length > 0 && (
            <Button
              variant="ghost"
              className="gap-[5px] text-[var(--text-3)]"
              onClick={() => setCreateOpen(true)}
              data-tour="new-project"
            >
              <Plus size={13} />
              New project
            </Button>
          )}
        </div>

        {/* Loading state */}
        {isLoading && (
          <div className="rounded-[var(--r-lg)] border border-[var(--border)] overflow-hidden">
            <div className="bg-[var(--bg-surface)] px-4 py-2.5">
              <Skeleton className="h-4 w-[200px] rounded" />
            </div>
            {[1, 2, 3].map(i => (
              <div key={i} className="flex items-center gap-3 px-4 py-3 border-t border-[var(--border)]">
                <Skeleton className="h-[6px] w-[6px] rounded-full" />
                <div className="flex-1 space-y-1.5">
                  <Skeleton className="h-3.5 w-[140px] rounded" />
                  <Skeleton className="h-3 w-[200px] rounded" />
                </div>
                <Skeleton className="h-3 w-[50px] rounded" />
              </div>
            ))}
          </div>
        )}

        {!isLoading && isError && (
          <div className="flex flex-col items-center justify-center gap-3 py-16 text-center">
            <div className="font-semibold text-[0.9375rem] text-[var(--text-1)]">Projects could not be loaded</div>
            <div className="max-w-[360px] text-[0.8125rem] text-[var(--text-3)]">
              {projectsQuery.error instanceof Error ? projectsQuery.error.message : 'The project service did not respond.'}
            </div>
            <Button variant="outline" size="sm" onClick={() => projectsQuery.refetch()}>
              Try again
            </Button>
          </div>
        )}

        {/* Empty state */}
        {!isLoading && !isError && allProjects.length === 0 && (
          <div className="flex flex-col items-center justify-center py-16 gap-5 text-center sm:py-20">
            <div className="flex h-14 w-14 items-center justify-center rounded-2xl bg-[var(--bg-elevated)]">
              <img
                src={brandIconFor(theme)}
                alt=""
                className="w-7 h-7 rounded-[5px]"
                aria-hidden="true"
              />
            </div>
            <div>
              <div className="font-semibold text-[1rem] text-[var(--text-1)] mb-1.5 tracking-[-0.02em]">
                Create your first project
              </div>
              <div className="text-[0.8125rem] text-[var(--text-3)] leading-[1.6] max-w-[360px] mx-auto">
                A project connects agen8 to a directory on your machine.
                Missions, key results, tasks, decisions, and graph context live inside a project.
              </div>
            </div>
            <Button
              onClick={() => setCreateOpen(true)}
              className="gap-1.5 mt-1"
              data-tour="new-project"
            >
              <Plus size={14} />
              New project
            </Button>
          </div>
        )}

        {/* Project table */}
        {!isLoading && !isError && allProjects.length > 0 && (
          <>
            {/* Filter bar: search + status chips */}
            <div className="flex items-center gap-2 mb-4 flex-wrap">
              <div className="relative flex-1 min-w-[140px] max-w-[260px]">
                <Search
                  size={13}
                  className="absolute left-2.5 top-1/2 -translate-y-1/2 text-[var(--text-3)] pointer-events-none"
                />
                <Input
                  value={searchQuery}
                  onChange={e => setSearchQuery(e.target.value)}
                  placeholder="Search projects..."
                  className="h-[32px] pl-8 text-[0.75rem]"
                />
              </div>
              <FilterChip active={statusFilter === 'all'} onClick={() => setStatusFilter('all')}>
                All
              </FilterChip>
              <FilterChip active={statusFilter === 'active'} onClick={() => setStatusFilter('active')}>
                Active
              </FilterChip>
              {hasArchived && (
                <FilterChip active={statusFilter === 'archived'} onClick={() => setStatusFilter('archived')}>
                  Archived
                </FilterChip>
              )}
            </div>

            {/* Table */}
            <div className="rounded-[var(--r-lg)] border border-[var(--border)] overflow-hidden bg-[var(--bg-panel)] @container">
              <Table className="w-full">
                <TableHeader>
                  <TableRow className="border-[var(--border)] bg-[var(--bg-surface)] hover:bg-[var(--bg-surface)]">
                    <TableHead className="w-[28px] h-8 px-3 text-[0.625rem] font-semibold uppercase tracking-[0.06em] text-[var(--text-2)]" />
                    <TableHead className="h-8 px-3 text-[0.625rem] font-semibold uppercase tracking-[0.06em] text-[var(--text-2)]">
                      Project
                    </TableHead>
                    <TableHead className="hidden w-[80px] h-8 px-3 text-center text-[0.625rem] font-semibold uppercase tracking-[0.06em] text-[var(--text-2)] @[640px]:table-cell">
                      Status
                    </TableHead>
                    <TableHead className="w-[80px] h-8 px-3 text-right text-[0.625rem] font-semibold uppercase tracking-[0.06em] text-[var(--text-2)]">
                      Activity
                    </TableHead>
                    <TableHead className="w-[36px] h-8 px-2" />
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {filteredProjects.map(project => (
                    <ProjectTableRow
                      key={project.id}
                      project={project}
                      onRemove={(action) => setRemoveTarget({ project, action })}
                      onLink={() => setLinkTarget(project)}
                      onEdit={() => setEditTarget(project)}
                      onConfigureClaudeMCP={() => { void configureClaudeMCP(project) }}
                    />
                  ))}
                </TableBody>
              </Table>

              {/* No results for current filter */}
              {filteredProjects.length === 0 && (
                <div className="flex flex-col items-center justify-center py-12 text-center gap-2">
                  <div className="text-[0.8125rem] text-[var(--text-3)]">
                    {searchQuery.trim()
                      ? `No projects matching "${searchQuery}"`
                      : statusFilter === 'archived'
                        ? 'No archived projects'
                        : 'No active projects'
                    }
                  </div>
                  {searchQuery.trim() && (
                    <Button
                      variant="ghost"
                      size="sm"
                      className="text-[var(--text-3)]"
                      onClick={() => setSearchQuery('')}
                    >
                      Clear search
                    </Button>
                  )}
                </div>
              )}
            </div>
          </>
        )}

        {/* Create project dialog */}
        {createOpen && (
          <CreateProjectDialog
            open={createOpen}
            onOpenChange={setCreateOpen}
            onSuccess={handleCreateSuccess}
          />
        )}

        {removeTarget && (
          <RemoveProjectDialog
            project={removeTarget.project}
            action={removeTarget.action}
            onClose={() => setRemoveTarget(null)}
            onRemoved={() => {
              const removed = removeTarget
              setRemoveTarget(null)
              queryClient.invalidateQueries({ queryKey: qk.projectsAll })
              toast.success(`Project "${projectDisplayName(removed.project)}" ${removed.action === 'delete' ? 'deleted' : 'archived'}`)
            }}
          />
        )}

        {linkTarget && (
          <LinkFolderDialog
            project={linkTarget}
            onClose={() => setLinkTarget(null)}
          />
        )}

        {editTarget && (
          <EditProjectDialog
            project={editTarget}
            onClose={() => setEditTarget(null)}
            onSaved={(updated) => {
              setEditTarget(null)
              queryClient.invalidateQueries({ queryKey: qk.projectsAll })
              toast.success(`Project renamed to "${projectDisplayName(updated)}"`)
            }}
          />
        )}
      </div>
    </div>
  )
}
