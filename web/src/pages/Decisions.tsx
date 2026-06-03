import { useMemo, useState } from 'react'
import { ArrowLeft, ScrollText, Download, ChevronDown, ChevronUp, Clock, Link2, Trash2 } from 'lucide-react'
import { decisionsPanelLink, useNavigation } from '../lib/routing'
import { useDecisionLog, useExportDecisions, useDeleteDecision } from '../hooks/useDecisions'
import { useProjectSpaces } from '../hooks/useProjectSpaces'
import { spaceDisplayName } from '../lib/spaceDisplayName'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '@/components/ui/alert-dialog'
import { CustomSelect } from '../components/fields'
import type { DecisionView } from '../lib/types'
import DecisionDetails from '../components/decision/DecisionDetails'
import { toast } from 'sonner'
import { Link } from 'wouter'

const PAGE_SIZE = 20

function timeAgo(iso: string): string {
  const diffMs = Date.now() - new Date(iso).getTime()
  if (diffMs < 0) return 'just now'
  const minutes = Math.floor(diffMs / 60_000)
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  return `${Math.floor(hours / 24)}d ago`
}

function downloadBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  document.body.appendChild(link)
  link.click()
  link.remove()
  URL.revokeObjectURL(url)
}

function exportCsv(decisions: DecisionView[]) {
  const header = 'id,title,source,spaceId,confidence,createdAt,taskRef,keyResultRef,operatorActionRef,rationale,alternativesRejected\n'
  const rows = decisions.map(decision => [
    decision.id,
    decision.title,
    decision.source,
    decision.spaceId ?? '',
    String(decision.confidence ?? ''),
    decision.createdAt,
    decision.taskRef ?? '',
    decision.keyResultRef ?? '',
    decision.operatorActionRef ?? '',
    decision.rationale,
    decision.alternativesRejected ?? '',
  ].map(value => `"${String(value).replace(/"/g, '""')}"`).join(',')).join('\n')
  downloadBlob(new Blob([header + rows], { type: 'text/csv;charset=utf-8' }), 'decisions.csv')
}

function DecisionRow({ decision, spaceName }: { decision: DecisionView; spaceName?: string }) {
  const [expanded, setExpanded] = useState(false)
  const deleteDecision = useDeleteDecision()
  const refs = [
    decision.missionRef ? `Mission: ${decision.missionRef}` : null,
    decision.taskRef ? `Task: ${decision.taskRef}` : null,
    decision.keyResultRef ? `KR: ${decision.keyResultRef}` : null,
    decision.operatorActionRef ? `OA: ${decision.operatorActionRef}` : null,
  ].filter(Boolean) as string[]

  const handleDelete = async () => {
    try {
      await deleteDecision.mutateAsync(decision.id)
      toast.success('Decision deleted')
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to delete decision')
    }
  }

  return (
    <div>
      <div className="flex items-center gap-2 px-5 py-2.5 hover:bg-[var(--bg-hover)] transition-colors">
        <button
          type="button"
          aria-label={`Toggle details for decision ${decision.title}`}
          className="min-w-0 flex-1 bg-transparent border-0 cursor-pointer text-left p-0"
          onClick={() => setExpanded(prev => !prev)}
        >
          <div className="flex items-center gap-3">
            <span className="text-[var(--text-3)] shrink-0">
              {expanded ? <ChevronUp size={14} /> : <ChevronDown size={14} />}
            </span>
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2 flex-wrap">
                <span className="text-sm font-medium text-[var(--text-1)]">{decision.title}</span>
                <Badge variant={decision.source === 'operator' ? 'success' : 'info'} className="text-[10px] px-1.5 py-0">
                  {decision.source}
                </Badge>
                {decision.spaceId && (
                  <Badge variant="secondary" className="text-[10px] px-1.5 py-0">
                    {spaceName ?? 'Space'}
                  </Badge>
                )}
              </div>
              <div className="flex items-center gap-3 text-[11px] text-[var(--text-3)] mt-1 flex-wrap">
                <span>{Math.round((decision.confidence ?? 0) * 100)}% confidence</span>
                <span className="inline-flex items-center gap-1"><Clock size={10} />{timeAgo(decision.createdAt)}</span>
                <span>{new Date(decision.createdAt).toLocaleDateString()}</span>
                <span>{refs.length} linked</span>
              </div>
            </div>
          </div>
        </button>
        <AlertDialog>
          <AlertDialogTrigger asChild>
            <Button
              type="button"
              variant="ghost-danger"
              size="icon"
              className="h-7 w-7 shrink-0 opacity-70 hover:opacity-100"
              aria-label={`Delete decision ${decision.title}`}
            >
              <Trash2 size={13} />
            </Button>
          </AlertDialogTrigger>
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>Delete decision?</AlertDialogTitle>
              <AlertDialogDescription>
                This removes the decision from the log and clears its graph links. This cannot be undone.
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel>Cancel</AlertDialogCancel>
              <AlertDialogAction
                className="bg-[var(--red)] text-white hover:bg-[var(--red)]/90"
                onClick={() => void handleDelete()}
              >
                Delete decision
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      </div>

      {expanded && (
        <div className="px-12 pb-3">
          <DecisionDetails decision={decision} />
          {refs.length > 0 && (
            <>
              <div className="text-[11px] font-semibold uppercase tracking-[0.04em] text-[var(--text-3)] mt-3 mb-1">Linked entities</div>
              <div className="flex flex-wrap gap-2">
                {refs.map(ref => (
                  <span key={ref} className="inline-flex items-center gap-1 text-xs text-[var(--accent)]">
                    <Link2 size={10} />
                    {ref}
                  </span>
                ))}
              </div>
            </>
          )}
        </div>
      )}
      <div className="h-px bg-[var(--border)]" />
    </div>
  )
}

export default function Decisions() {
  const { projectId } = useNavigation()
  const [query, setQuery] = useState('')
  const [source, setSource] = useState<'' | 'agent' | 'operator'>('')
  const [spaceId, setSpaceId] = useState('')
  const [fromDate, setFromDate] = useState('')
  const [toDate, setToDate] = useState('')
  const [sort, setSort] = useState<'newest' | 'oldest'>('newest')
  const [page, setPage] = useState(1)
  const spacesQuery = useProjectSpaces(projectId ?? null)
  const exportDecisions = useExportDecisions()

  const logQuery = useDecisionLog(projectId, {
    page,
    pageSize: PAGE_SIZE,
    query,
    source: source || undefined,
    spaceId: spaceId || undefined,
    since: fromDate ? new Date(`${fromDate}T00:00:00Z`).toISOString() : undefined,
    until: toDate ? new Date(`${toDate}T23:59:59Z`).toISOString() : undefined,
    sort,
  })

  const total = logQuery.data?.total ?? 0
  const decisions = logQuery.data?.decisions ?? []
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))
  const spaces = useMemo(() => spacesQuery.data ?? [], [spacesQuery.data])
  const spaceNameMap = useMemo(() => {
    const map = new Map<string, string>()
    for (const t of spaces) {
      map.set(t.spaceId, spaceDisplayName(t.spaceId, t.spaceName))
    }
    return map
  }, [spaces])

  if (!projectId) {
    return (
      <div className="flex flex-col items-center justify-center h-full text-center p-8">
        <ScrollText size={40} className="text-[var(--text-3)] opacity-40 mb-4" />
        <h2 className="text-lg font-semibold text-[var(--text-1)] mb-1">No project selected</h2>
        <p className="text-sm text-[var(--text-3)]">Select a project from the sidebar to view the decision log.</p>
      </div>
    )
  }

  const onExportCsv = async () => {
    const exported = await exportDecisions.mutateAsync({
      projectId,
      query: query || undefined,
      source: source || undefined,
      spaceId: spaceId || undefined,
      since: fromDate ? new Date(`${fromDate}T00:00:00Z`).toISOString() : undefined,
      until: toDate ? new Date(`${toDate}T23:59:59Z`).toISOString() : undefined,
      sort,
    })
    exportCsv(exported)
  }

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="flex items-center gap-3 px-6 py-4 border-b border-[var(--border)] shrink-0">
        <Button asChild variant="ghost" size="sm" className="gap-1.5">
          <Link to={decisionsPanelLink(projectId)}>
            <ArrowLeft size={14} />
            Dashboard
          </Link>
        </Button>
        <ScrollText size={20} className="text-[var(--accent)]" />
        <h1 className="text-lg font-semibold text-[var(--text-1)] tracking-[-0.02em] m-0">Decision Log</h1>
        {total > 0 && (
          <Badge variant="secondary" className="text-xs">{total}</Badge>
        )}
        <div className="flex-1" />
        <Button variant="ghost" size="sm" onClick={() => {
          setSort(prev => prev === 'newest' ? 'oldest' : 'newest')
          setPage(1)
        }}>
          {sort === 'newest' ? 'Newest first' : 'Oldest first'}
        </Button>
        <Button variant="ghost" size="sm" onClick={onExportCsv} disabled={exportDecisions.isPending}>
          <Download size={12} /> CSV
        </Button>
      </div>

      {/* Filter bar */}
      <div className="flex items-end gap-3 px-6 py-3 border-b border-[var(--border)] shrink-0 flex-wrap">
        <div className="min-w-[180px] flex-1 max-w-[280px]">
          <Label htmlFor="decision-search" className="text-[11px] text-[var(--text-3)]">Search</Label>
          <Input
            id="decision-search"
            placeholder="Search decisions..."
            value={query}
            onChange={(event) => {
              setQuery(event.target.value)
              setPage(1)
            }}
          />
        </div>
        <div className="w-[140px]">
          <Label htmlFor="decision-from-date" className="text-[11px] text-[var(--text-3)]">From</Label>
          <Input
            id="decision-from-date"
            type="date"
            value={fromDate}
            onChange={(event) => {
              setFromDate(event.target.value)
              setPage(1)
            }}
          />
        </div>
        <div className="w-[140px]">
          <Label htmlFor="decision-to-date" className="text-[11px] text-[var(--text-3)]">To</Label>
          <Input
            id="decision-to-date"
            type="date"
            value={toDate}
            onChange={(event) => {
              setToDate(event.target.value)
              setPage(1)
            }}
          />
        </div>
        <div className="w-[160px]">
          <Label htmlFor="decision-space-filter" className="text-[11px] text-[var(--text-3)]">Space</Label>
          <CustomSelect
            value={spaceId}
            onChange={(value) => {
              setSpaceId(value)
              setPage(1)
            }}
            options={[
              { value: '', label: 'All spaces' },
              ...spaces.map(space => ({ value: space.spaceId, label: spaceDisplayName(space.spaceId, space.spaceName) })),
            ]}
          />
        </div>
        <div className="flex items-center gap-1 pb-0.5">
          <Button variant={source === '' ? 'secondary' : 'ghost'} size="xs" onClick={() => { setSource(''); setPage(1) }}>All</Button>
          <Button variant={source === 'agent' ? 'secondary' : 'ghost'} size="xs" onClick={() => { setSource('agent'); setPage(1) }}>Agent</Button>
          <Button variant={source === 'operator' ? 'secondary' : 'ghost'} size="xs" onClick={() => { setSource('operator'); setPage(1) }}>Operator</Button>
        </div>
      </div>

      {/* Content */}
      <div className="flex-1 overflow-y-auto">
        {logQuery.isLoading ? (
          <div className="px-6 py-8 text-sm text-[var(--text-3)]">Loading decisions...</div>
        ) : decisions.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-16 text-center">
            <ScrollText size={40} className="text-[var(--text-3)] opacity-30 mb-4" />
            <h3 className="text-base font-semibold text-[var(--text-1)] mb-1">No decisions found</h3>
            <p className="text-sm text-[var(--text-3)]">
              {query || source || spaceId || fromDate || toDate
                ? 'Try adjusting your filters.'
                : 'Decisions are logged here as agents make choices during their work.'}
            </p>
          </div>
        ) : (
          <div className="max-w-4xl">
            {decisions.map(decision => <DecisionRow key={decision.id} decision={decision} spaceName={decision.spaceId ? spaceNameMap.get(decision.spaceId) : undefined} />)}
          </div>
        )}
      </div>

      {/* Pagination */}
      {totalPages > 1 && (
        <div className="flex items-center justify-between px-6 py-2.5 border-t border-[var(--border)] shrink-0 text-sm text-[var(--text-3)]">
          <span>Page {page} of {totalPages}</span>
          <div className="flex items-center gap-2">
            <Button variant="ghost" size="sm" onClick={() => setPage(prev => Math.max(1, prev - 1))} disabled={page <= 1}>
              Previous
            </Button>
            <Button variant="ghost" size="sm" onClick={() => setPage(prev => Math.min(totalPages, prev + 1))} disabled={page >= totalPages}>
              Next
            </Button>
          </div>
        </div>
      )}
    </div>
  )
}
