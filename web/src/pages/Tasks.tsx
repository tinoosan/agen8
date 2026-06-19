import { useCallback, useMemo, useState } from 'react'
import { Link, useLocation, useSearch } from 'wouter'
import {
  ListChecks,
  Plus,
  AlertCircle,
  Search,
  ExternalLink,
  Activity,
  Eye,
  Ban,
  CircleCheck,
  type LucideIcon,
} from 'lucide-react'
import { useNavigation, taskDetailLink } from '../lib/routing'
import { useProjectTasks } from '../hooks/useProjectTasks'
import { useMissions, useProjectKRs } from '../hooks/useMissions'
import { usePageParam } from '../hooks/usePageParam'
import { pageSlice } from '../components/listPaging'
import ListPager from '../components/ListPager'
import CreateTaskDialog from '../components/task/CreateTaskDialog'
import { computeBriefing } from '../lib/dashboardBriefing'
import { taskStatusLabel, taskStatusColor } from '../lib/statusLabels'
import { taskClaimedMemberLabel, taskAssignedMemberLabel } from '../lib/taskMembers'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'
import type { Task } from '../lib/types'

/*
 * Tasks — the project's tasks on their own page (formerly a wrapper around the
 * rail-era DashboardTasksPanel). It leads with aggregate tiles, gives each row
 * the mission it serves (the "why" the flat list lacked), and defaults to OPEN
 * work so landing here shows what's actionable instead of the whole done
 * backlog. The status filter + page live in the URL so the dashboard's briefing
 * chips (?status=active / in_review / succeeded / pending) deep-link straight in.
 */

const TASKS_PAGE_SIZE = 60

/* The actionable set — what "Open" means and the default landing view. */
const OPEN_STATUSES = ['pending', 'active', 'in_review', 'blocked']

const STATUS_FILTERS: { value: string; label: string }[] = [
  { value: 'open', label: 'Open' },
  { value: 'active', label: 'Working' },
  { value: 'in_review', label: 'In Review' },
  { value: 'blocked', label: 'Blocked' },
  { value: 'pending', label: 'Queued' },
  { value: 'succeeded', label: 'Done' },
  { value: 'paused', label: 'Paused' },
  { value: 'failed', label: 'Failed' },
  { value: 'canceled', label: 'Canceled' },
  { value: 'all', label: 'All' },
]

const WEEK_MS = 7 * 24 * 60 * 60 * 1000

function matchesFilter(status: string, filter: string): boolean {
  if (filter === 'all') return true
  if (filter === 'open') return OPEN_STATUSES.includes(status)
  return status === filter
}

/* ── Aggregate tile ───────────────────────────────────── */

function Tile({
  label,
  value,
  tone,
  icon: Icon,
  onClick,
  active,
}: {
  label: string
  value: number
  tone: string
  icon: LucideIcon
  onClick?: () => void
  active?: boolean
}) {
  const interactive = Boolean(onClick)
  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={interactive ? Boolean(active) : undefined}
      disabled={!interactive}
      className={cn(
        'flex flex-col gap-1.5 rounded-[14px] border bg-[var(--bg-elevated)] px-4 py-3 text-left',
        interactive ? 'cursor-pointer transition-colors hover:bg-[var(--bg-hover)]' : 'cursor-default',
        active ? 'border-[var(--accent)]' : 'border-[var(--border)]',
      )}
    >
      <div className="flex items-center gap-1.5">
        <Icon size={13} style={{ color: tone }} aria-hidden />
        <span className="text-[0.6875rem] font-semibold uppercase tracking-[0.05em] text-[var(--text-3)]">
          {label}
        </span>
      </div>
      <span className="text-[1.5rem] font-semibold leading-none tabular-nums" style={{ color: tone }}>
        {value}
      </span>
    </button>
  )
}

/* ── Page ─────────────────────────────────────────────── */

export default function Tasks() {
  const { projectId } = useNavigation()
  const rawSearch = useSearch()
  const [location, navigate] = useLocation()
  const searchParams = useMemo(() => new URLSearchParams(rawSearch), [rawSearch])
  const rawStatus = searchParams.get('status') ?? 'open'
  const statusFilter = STATUS_FILTERS.some((f) => f.value === rawStatus) ? rawStatus : 'open'

  const setStatusFilter = useCallback(
    (value: string) => {
      if (!projectId) return
      const params = new URLSearchParams(rawSearch)
      params.delete('page')
      // 'open' is the default landing view, so keep it out of the URL.
      if (value === 'open') params.delete('status')
      else params.set('status', value)
      const qs = params.toString()
      navigate(`${location}${qs ? `?${qs}` : ''}`)
    },
    [navigate, projectId, rawSearch, location],
  )

  const [searchQuery, setSearchQuery] = useState('')
  const [createDialogOpen, setCreateDialogOpen] = useState(false)
  const [page, setPage] = usePageParam()

  const changeSearchQuery = (value: string) => {
    setPage(1)
    setSearchQuery(value)
  }

  const { data: allTasks, isLoading, isError, error, dataUpdatedAt } = useProjectTasks(projectId)
  const { data: missions } = useMissions(projectId)
  const { data: krMap } = useProjectKRs(projectId)

  // Aggregate tiles reuse the tested briefing math (it already splits the
  // statuses); decisions/missions are irrelevant here so pass empties.
  const overview = useMemo(
    () => computeBriefing(allTasks ?? [], [], [], dataUpdatedAt || 0, WEEK_MS),
    [allTasks, dataUpdatedAt],
  )

  // Resolve the mission each task serves: direct missionRef, else via its KR.
  const missionTitleByTask = useMemo(() => {
    const titleById = new Map<string, string>()
    for (const m of missions ?? []) titleById.set(m.id, m.title)
    const resolve = (task: Task): string | null => {
      if (task.missionRef) {
        const ref = task.missionRef.startsWith('mission:')
          ? task.missionRef.slice('mission:'.length)
          : task.missionRef
        const title = titleById.get(ref)
        if (title) return title
      }
      if (task.keyResultRef && krMap) {
        const kr = krMap.get(task.keyResultRef)
        if (kr) return titleById.get(kr.missionId) ?? null
      }
      return null
    }
    const out = new Map<string, string>()
    for (const t of allTasks ?? []) {
      const title = resolve(t)
      if (title) out.set(t.id, title)
    }
    return out
  }, [allTasks, missions, krMap])

  const statusCounts = useMemo(() => {
    const counts: Record<string, number> = {}
    for (const t of allTasks ?? []) {
      const s = t.status ?? ''
      counts[s] = (counts[s] ?? 0) + 1
    }
    counts.open = OPEN_STATUSES.reduce((n, s) => n + (counts[s] ?? 0), 0)
    counts.all = allTasks?.length ?? 0
    return counts
  }, [allTasks])

  const filteredTasks = useMemo(() => {
    if (!allTasks) return []
    let list = allTasks.filter((t) => matchesFilter(t.status ?? '', statusFilter))
    if (searchQuery.trim()) {
      const q = searchQuery.toLowerCase()
      list = list.filter(
        (t) => (t.title ?? '').toLowerCase().includes(q) || t.description.toLowerCase().includes(q),
      )
    }
    return list
  }, [allTasks, statusFilter, searchQuery])

  const totalPages = Math.max(1, Math.ceil(filteredTasks.length / TASKS_PAGE_SIZE))
  const visibleTasks = useMemo(() => pageSlice(filteredTasks, page, TASKS_PAGE_SIZE), [filteredTasks, page])

  if (!projectId) {
    return (
      <div className="flex h-full items-center justify-center p-8">
        <div className="rounded-[var(--r-lg)] border border-dashed border-[var(--border)] p-8 text-center text-[0.8125rem] text-[var(--text-3)]">
          Select a project to view its tasks.
        </div>
      </div>
    )
  }

  const hasTasks = (allTasks?.length ?? 0) > 0

  return (
    <div className="h-full overflow-y-auto">
      <div className="mx-auto w-full max-w-[960px] px-6 pt-8 pb-12">
        {/* Header */}
        <div className="mb-5 flex items-start justify-between gap-3">
          <h1 className="m-0 text-[1.75rem] font-bold leading-[1.14] tracking-[-0.03em] text-[var(--text-1)]">
            Tasks
          </h1>
          <Button
            variant="outline"
            size="sm"
            onClick={() => setCreateDialogOpen(true)}
            className="dashboard-action-button dashboard-action-button-accent"
          >
            <Plus size={13} className="mr-1" />
            New Task
          </Button>
        </div>

        {/* Aggregate tiles */}
        {hasTasks && !isError && (
          <div className="@container mb-6">
            <div className="grid grid-cols-2 gap-3 @min-[640px]:grid-cols-4">
              <Tile
                label="In flight"
                value={overview.inFlight}
                tone="var(--accent)"
                icon={Activity}
                onClick={() => setStatusFilter(statusFilter === 'active' ? 'open' : 'active')}
                active={statusFilter === 'active'}
              />
              <Tile
                label="Needs review"
                value={overview.inReview}
                tone={overview.inReview > 0 ? 'var(--amber)' : 'var(--text-3)'}
                icon={Eye}
                onClick={() => setStatusFilter(statusFilter === 'in_review' ? 'open' : 'in_review')}
                active={statusFilter === 'in_review'}
              />
              <Tile
                label="Blocked"
                value={overview.blocked}
                tone={overview.blocked > 0 ? 'var(--red)' : 'var(--text-3)'}
                icon={Ban}
                onClick={() => setStatusFilter(statusFilter === 'blocked' ? 'open' : 'blocked')}
                active={statusFilter === 'blocked'}
              />
              <Tile
                label="Done this week"
                value={overview.completed}
                tone="var(--green,var(--accent))"
                icon={CircleCheck}
              />
            </div>
          </div>
        )}

        {/* Filters + search */}
        {hasTasks && !isError && (
          <div className="mb-4">
            <div className="flex flex-wrap items-center gap-x-1 gap-y-1.5">
              {STATUS_FILTERS.map((filter) => {
                const isActive = statusFilter === filter.value
                const count = statusCounts[filter.value] ?? 0
                return (
                  <button
                    key={filter.value}
                    onClick={() => setStatusFilter(filter.value)}
                    aria-pressed={isActive}
                    className={cn(
                      'inline-flex items-center gap-1 rounded-full border-none px-3 py-1 text-[0.8125rem] transition-colors',
                      isActive
                        ? 'bg-[color-mix(in_srgb,var(--accent-dim)_18%,var(--bg-panel)_82%)] font-semibold text-[var(--text-1)]'
                        : 'cursor-pointer bg-transparent text-[var(--text-2)] hover:text-[var(--text-1)]',
                    )}
                  >
                    {filter.label}
                    {count > 0 && (
                      <span className="text-[0.6875rem] tabular-nums text-[var(--text-3)]">{count}</span>
                    )}
                  </button>
                )
              })}
            </div>
            <div className="relative mt-3">
              <Search
                size={12}
                className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-[var(--text-3)]"
              />
              <input
                value={searchQuery}
                onChange={(e) => changeSearchQuery(e.target.value)}
                placeholder="Search tasks…"
                className="w-full rounded-[var(--r-md)] border border-[color-mix(in_srgb,var(--border)_54%,transparent)] bg-[var(--bg-elevated)] py-1.5 pl-7 pr-3 text-[0.8125rem] text-[var(--text-1)] outline-none transition-colors placeholder:text-[var(--text-3)] focus:border-[var(--accent)]/40"
              />
            </div>
          </div>
        )}

        {/* List */}
        {isLoading && (
          <div className="flex flex-col gap-2">
            {[1, 2, 3, 4, 5].map((i) => (
              <Skeleton key={i} className="h-[3.25rem] rounded-[var(--r-md)]" />
            ))}
          </div>
        )}

        {isError && (
          <div className="flex items-center gap-2 rounded-[8px] bg-[var(--bg-elevated)] px-4 py-3 text-[0.8125rem] text-[var(--red)]">
            <AlertCircle size={16} />
            <span>Failed to load tasks: {error instanceof Error ? error.message : 'Unknown error'}</span>
          </div>
        )}

        {!isLoading && !isError && visibleTasks.length === 0 && (
          <div className="flex flex-col items-center justify-center py-16 text-center">
            <ListChecks size={36} className="mb-4 text-[var(--text-3)] opacity-25" />
            {!hasTasks ? (
              <>
                <h3 className="mb-1.5 text-[1.0625rem] font-semibold tracking-[-0.01em] text-[var(--text-1)]">
                  No tasks yet
                </h3>
                <p className="mb-5 max-w-sm text-[0.875rem] text-[var(--text-3)]">
                  Tasks are the concrete units of work members pick up. Create one to start tracking
                  progress and acceptance criteria.
                </p>
                <Button
                  onClick={() => setCreateDialogOpen(true)}
                  className="dashboard-action-button dashboard-action-button-accent"
                >
                  <Plus size={14} className="mr-1.5" />
                  Create your first task
                </Button>
              </>
            ) : (
              <>
                <h3 className="mb-1.5 text-[1.0625rem] font-semibold tracking-[-0.01em] text-[var(--text-1)]">
                  Nothing here
                </h3>
                <p className="mb-5 text-[0.875rem] text-[var(--text-3)]">
                  {statusFilter === 'open'
                    ? 'No open work right now — try the Done or All filter.'
                    : 'Try a different filter or search term.'}
                </p>
                <Button variant="secondary" onClick={() => { setStatusFilter('open'); changeSearchQuery('') }}>
                  Show open work
                </Button>
              </>
            )}
          </div>
        )}

        {!isLoading && !isError && visibleTasks.length > 0 && (
          <div className="overflow-hidden rounded-[14px] border border-[var(--border)] bg-[var(--bg-elevated)]">
            {visibleTasks.map((task, i) => {
              const who = taskClaimedMemberLabel(task) ?? taskAssignedMemberLabel(task)
              const missionTitle = missionTitleByTask.get(task.id)
              const color = taskStatusColor(task.status)
              return (
                <Link
                  key={task.id}
                  to={taskDetailLink(projectId, task.id)}
                  className={cn(
                    'group flex items-center gap-3 px-4 py-2.5 no-underline transition-colors hover:bg-[var(--bg-hover)]',
                    i > 0 && 'border-t border-[var(--border)]',
                  )}
                >
                  <span
                    className="mt-1.5 h-2 w-2 shrink-0 self-start rounded-full"
                    style={{ background: color }}
                    title={taskStatusLabel(task.status)}
                  />
                  <div className="min-w-0 flex-1">
                    <div className="truncate text-[0.875rem] font-medium tracking-[-0.01em] text-[var(--text-1)]">
                      {task.title || task.description}
                    </div>
                    <div className="flex items-center gap-1.5 text-[0.75rem] text-[var(--text-3)]">
                      {missionTitle && <span className="truncate">{missionTitle}</span>}
                      {missionTitle && who && <span aria-hidden>·</span>}
                      {who && <span className="shrink-0 truncate max-w-[160px]">{who}</span>}
                      {!missionTitle && !who && <span>No mission</span>}
                    </div>
                  </div>
                  <span className="shrink-0 text-[0.6875rem] font-medium" style={{ color }}>
                    {taskStatusLabel(task.status)}
                  </span>
                  <ExternalLink
                    size={12}
                    className="shrink-0 text-[var(--text-3)] opacity-0 transition-opacity group-hover:opacity-100"
                  />
                </Link>
              )
            })}
          </div>
        )}

        {!isLoading && !isError && totalPages > 1 && (
          <div className="mt-4">
            <ListPager page={page} totalPages={totalPages} onPageChange={setPage} />
          </div>
        )}
      </div>

      <CreateTaskDialog projectId={projectId} open={createDialogOpen} onOpenChange={setCreateDialogOpen} />
    </div>
  )
}
