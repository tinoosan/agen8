import { useCallback, useMemo, useState } from 'react'
import { Link, useLocation, useSearch } from 'wouter'
import { useProjectTasks } from '../../hooks/useProjectTasks'
import { usePageParam } from '../../hooks/usePageParam'
import CreateTaskDialog from '../task/CreateTaskDialog'
import ListPager, { pageSlice } from '../ListPager'
import { taskDetailLink } from '../../lib/routing'
import { taskStatusLabel, taskStatusColor } from '../../lib/statusLabels'
import { taskAssignedMemberLabel } from '../../lib/taskMembers'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'
import { ListChecks, Plus, AlertCircle, Search, ExternalLink } from 'lucide-react'

/* Page size for the task list — same house pagination as the decisions panel. */
const TASKS_PAGE_SIZE = 60

const STATUS_FILTERS: { value: string; label: string }[] = [
  { value: 'all', label: 'All' },
  { value: 'pending', label: 'Queued' },
  { value: 'active', label: 'Working' },
  { value: 'in_review', label: 'In Review' },
  { value: 'blocked', label: 'Blocked' },
  { value: 'paused', label: 'Paused' },
  { value: 'succeeded', label: 'Done' },
  { value: 'failed', label: 'Failed' },
  { value: 'canceled', label: 'Canceled' },
]

function TasksSkeleton({ embedded = false }: { embedded?: boolean }) {
  return (
    <div className={cn('flex flex-col gap-0.5', embedded && 'px-1')}>
      {[1, 2, 3].map((i) => (
        <div key={i} className="flex items-center gap-3 px-2 py-3">
          <Skeleton className="h-2 w-2 rounded-full shrink-0" />
          <Skeleton className="h-3.5 flex-1 max-w-[240px]" />
          <Skeleton className="h-3.5 w-16 ml-auto" />
          <Skeleton className="h-3.5 w-3.5" />
        </div>
      ))}
    </div>
  )
}

interface DashboardTasksPanelProps {
  projectId: string | null
  focusedProjectRoot: string | null
  embedded?: boolean
}

export default function DashboardTasksPanel({
  projectId,
  embedded = false,
}: DashboardTasksPanelProps) {
  // Status filter lives in the URL (?status=) so dashboard tiles can deep-link
  // straight to a filtered list and the view is shareable. 'all' is the default,
  // so it's encoded as the absence of the param.
  const rawSearch = useSearch()
  const [, navigate] = useLocation()
  const searchParams = useMemo(() => new URLSearchParams(rawSearch), [rawSearch])
  const rawStatus = searchParams.get('status') ?? 'all'
  const statusFilter = STATUS_FILTERS.some((f) => f.value === rawStatus) ? rawStatus : 'all'
  const setStatusFilter = useCallback(
    (value: string) => {
      if (!projectId) return
      const params = new URLSearchParams(rawSearch)
      params.set('panel', 'tasks')
      // A new filter is a new list: the page param must not survive it. The
      // page reset rides THIS navigation — calling setPage(1) separately races
      // it (two navigations from stale closures resurrect the old page).
      params.delete('page')
      if (value === 'all') params.delete('status')
      else params.set('status', value)
      const qs = params.toString()
      navigate(`/project/${encodeURIComponent(projectId)}/dashboard${qs ? `?${qs}` : ''}`)
    },
    [navigate, projectId, rawSearch],
  )
	const [searchQuery, setSearchQuery] = useState('')
	const [createDialogOpen, setCreateDialogOpen] = useState(false)
	const [page, setPage] = usePageParam()

	const changeStatusFilter = (value: string) => {
		setStatusFilter(value)
	}

	const changeSearchQuery = (value: string) => {
		setPage(1)
		setSearchQuery(value)
	}

	const { data: allTasks, isLoading, isError, error } = useProjectTasks(projectId)

  const statusCounts = useMemo(
    () =>
      allTasks
        ? allTasks.reduce<Record<string, number>>((acc, task) => {
            acc[task.status] = (acc[task.status] ?? 0) + 1
            return acc
          }, {})
        : {},
    [allTasks],
  )

  const filteredTasks = useMemo(() => {
    if (!allTasks) return []

    let list = statusFilter === 'all'
      ? allTasks
      : allTasks.filter((task) => task.status === statusFilter)

    if (searchQuery.trim()) {
      const q = searchQuery.toLowerCase()
      list = list.filter((task) =>
        (task.title ?? '').toLowerCase().includes(q) || task.description.toLowerCase().includes(q),
      )
    }

    return list
  }, [allTasks, statusFilter, searchQuery])

	// House pagination (same pattern as the decisions panel): filters and search
	// above operate on the FULL set; the page only bounds the DOM. Filter/search
	// changes reset to page 1 in the handlers above.
	const totalPages = Math.max(1, Math.ceil(filteredTasks.length / TASKS_PAGE_SIZE))
	const visibleTasks = useMemo(() => pageSlice(filteredTasks, page, TASKS_PAGE_SIZE), [filteredTasks, page])

  if (!projectId) {
    return (
      <div className="flex flex-col items-center justify-center h-full text-center p-8">
        <ListChecks size={36} className="text-[var(--text-3)] opacity-30 mb-4" />
        <h2 className="text-base font-semibold text-[var(--text-1)] mb-1">No project selected</h2>
        <p className="text-sm text-[var(--text-3)]">Select a project to work with tasks.</p>
      </div>
    )
  }

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className={cn('shrink-0', embedded ? 'px-[var(--dashboard-context-gutter)] pt-5 pb-3 border-b border-[color-mix(in_srgb,var(--border)_56%,transparent)]' : 'px-6 pt-8 pb-2 max-w-4xl mx-auto w-full')}>
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <h1
              className="m-0 text-[var(--text-1)]"
              style={{ fontSize: embedded ? '1.1875rem' : '1.75rem', fontWeight: 700, letterSpacing: embedded ? '-0.36px' : '-0.56px', lineHeight: embedded ? 1.18 : 1.14 }}
            >
              Tasks
            </h1>
          </div>
          <Button
            variant="outline"
            size="sm"
            onClick={() => setCreateDialogOpen(true)}
            style={{ letterSpacing: '-0.12px' }}
            className="dashboard-action-button dashboard-action-button-accent"
          >
            <Plus size={13} className="mr-1" />
            New Task
          </Button>
        </div>
      </div>

      <div className={cn('shrink-0', embedded ? 'px-[var(--dashboard-context-gutter)] py-3 border-b border-[color-mix(in_srgb,var(--border)_42%,transparent)]' : 'px-6 pt-4 pb-2 max-w-4xl mx-auto w-full')}>
        <div className="flex items-center gap-3">
          <div className="flex flex-wrap items-center gap-x-0.5 gap-y-1.5 flex-1 min-w-0">
            {STATUS_FILTERS.map((filter) => {
              const isActive = statusFilter === filter.value
              const count = filter.value === 'all' ? (allTasks?.length ?? 0) : (statusCounts[filter.value] ?? 0)
              return (
                <button
                  key={filter.value}
									onClick={() => changeStatusFilter(filter.value)}
                  aria-pressed={isActive}
                  className="inline-flex items-center gap-1 border-none cursor-pointer transition-colors duration-150 whitespace-nowrap"
                  style={{
                    padding: embedded ? '4px 10px' : '4px 12px',
                    borderRadius: '980px',
                    fontSize: embedded ? '0.75rem' : '0.8125rem',
                    fontWeight: isActive ? 600 : 400,
                    letterSpacing: '-0.08px',
                    background: isActive ? 'color-mix(in srgb, var(--accent-dim) 18%, var(--bg-panel) 82%)' : 'transparent',
                    color: isActive ? 'var(--text-1)' : 'var(--text-2)',
                  }}
                >
                  {filter.label}
                  {count > 0 && (
                    <span
                      className="tabular-nums"
                      style={{ fontSize: '0.6875rem', letterSpacing: '-0.06px', color: 'var(--text-3)' }}
                    >
                      {count}
                    </span>
                  )}
                </button>
              )
            })}
          </div>
        </div>

        {!isLoading && !isError && allTasks && allTasks.length > 0 && (
          <div className="mt-3 flex items-center gap-2">
            <div className="relative flex-1">
              <Search size={11} className="absolute left-2.5 top-1/2 -translate-y-1/2 text-[var(--text-3)] pointer-events-none" />
              <input
                value={searchQuery}
									onChange={(e) => changeSearchQuery(e.target.value)}
                placeholder="Search tasks…"
                className={cn(
                  'w-full pl-7 pr-3 py-1.5 rounded-[var(--r-md)] outline-none',
                  'bg-[var(--dashboard-subsurface-bg)] border border-[color-mix(in_srgb,var(--border)_54%,transparent)]',
                  'text-[var(--text-1)] placeholder:text-[var(--text-3)]',
                  'focus:border-[var(--accent)]/40 transition-colors',
                )}
                style={{ fontSize: '0.75rem', letterSpacing: '-0.08px' }}
              />
            </div>
          </div>
        )}
      </div>

      <div className={cn('flex-1 min-h-0 overflow-y-auto', embedded ? 'px-[var(--dashboard-context-gutter)] py-4' : 'px-6 py-2 max-w-4xl mx-auto w-full')}>
        {isLoading && <TasksSkeleton embedded={embedded} />}

        {isError && (
          <div className="flex items-center gap-2 px-4 py-3 rounded-[8px] bg-[var(--dashboard-subsurface-bg)] text-[0.8125rem] text-[var(--red)]" style={{ letterSpacing: '-0.08px' }}>
            <AlertCircle size={16} />
            <span>Failed to load tasks: {error instanceof Error ? error.message : 'Unknown error'}</span>
          </div>
        )}

        {!isLoading && !isError && visibleTasks.length === 0 && (
          <div className="flex flex-col items-center justify-center py-16 text-center">
            <ListChecks size={36} className="text-[var(--text-3)] opacity-25 mb-4" />
            {allTasks && allTasks.length === 0 ? (
              <>
                <h3 className="text-[var(--text-1)] mb-1.5" style={{ fontSize: '1.0625rem', fontWeight: 600, letterSpacing: '-0.24px', lineHeight: 1.24 }}>
                  No tasks yet
                </h3>
                <p className="text-[var(--text-3)] mb-5 max-w-sm" style={{ fontSize: '0.875rem', letterSpacing: '-0.224px', lineHeight: 1.47 }}>
                  Tasks are the concrete units of work members pick up. Create one to start tracking progress and acceptance criteria.
                </p>
                <Button
                  onClick={() => setCreateDialogOpen(true)}
                  style={{ letterSpacing: '-0.12px' }}
                  className="dashboard-action-button dashboard-action-button-accent"
                >
                  <Plus size={14} className="mr-1.5" />
                  Create your first task
                </Button>
              </>
            ) : (
              <>
                <h3 className="text-[var(--text-1)] mb-1.5" style={{ fontSize: '1.0625rem', fontWeight: 600, letterSpacing: '-0.24px', lineHeight: 1.24 }}>
                  No results
                </h3>
                <p className="text-[var(--text-3)] mb-5" style={{ fontSize: '0.875rem', letterSpacing: '-0.224px', lineHeight: 1.47 }}>
                  Try a different filter or search term.
                </p>
								<Button variant="secondary" onClick={() => { changeStatusFilter('all'); changeSearchQuery('') }} style={{ letterSpacing: '-0.12px' }}>
                  Clear filters
                </Button>
              </>
            )}
          </div>
        )}

        {!isLoading && !isError && visibleTasks.length > 0 && (
          <div className="flex flex-col gap-0.5">
				{visibleTasks.map((task) => {
					const assigneeLabel = taskAssignedMemberLabel(task)
					return (
                <Link
                  key={task.id}
                  to={taskDetailLink(projectId, task.id)}
                  className="dashboard-queue-row flex items-center gap-2 px-3 py-2.5 rounded-[var(--r-md)] hover:bg-[var(--bg-hover)] transition-colors no-underline"
                >
                  <span
                    className="shrink-0 w-2 h-2 rounded-full"
                    style={{ background: taskStatusColor(task.status) }}
                    title={taskStatusLabel(task.status)}
                  />
                  <span className="text-[0.8125rem] font-semibold text-[var(--text-1)] tracking-[-0.02em] truncate flex-1">
                    {task.title || task.description}
                  </span>
                  {assigneeLabel && (
                    <span className="shrink-0 text-[0.6875rem] text-[var(--text-3)] truncate max-w-[110px]">
                      {assigneeLabel}
                    </span>
                  )}
                  <span
                    className="shrink-0 text-[0.625rem] font-medium tracking-[-0.04px]"
                    style={{ color: taskStatusColor(task.status) }}
                  >
                    {taskStatusLabel(task.status)}
                  </span>
                  <ExternalLink size={11} className="shrink-0 text-[var(--text-3)]" />
                </Link>
					)
				})}
				<div className="mt-3 px-1">
					<ListPager page={page} totalPages={totalPages} onPageChange={setPage} />
				</div>
			</div>
		)}
      </div>

      <CreateTaskDialog
        projectId={projectId}
        open={createDialogOpen}
        onOpenChange={setCreateDialogOpen}
      />
    </div>
  )
}
