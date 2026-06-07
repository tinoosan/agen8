import { useState, type ReactNode } from 'react'
import { useRoute, useLocation, Link } from 'wouter'
import { toast } from 'sonner'
import {
  ArrowLeft,
  Clock,
  Trash2,
  ChevronRight,
  Tag,
} from 'lucide-react'
import { useDecision, useDeleteDecision } from '../hooks/useDecisions'
import { useMissions, useProjectKRs } from '../hooks/useMissions'
import { useProjectTasks } from '../hooks/useProjectTasks'
import { formatRelative } from '@/lib/format'
import { confidenceColor } from '@/lib/decisionDisplay'
import {
  decisionsPanelLink,
  missionDetailLink,
  taskDetailLink,
  strategyMapLink,
} from '../lib/routing'
import { sanitizeDecisionTitle, safeReferenceLabel } from '../lib/displaySanitizers'
import { decisionActorDisplay } from '../lib/decisionDisplay'
import { CollapsibleSection } from '../components/strategy/CollapsibleSection'
import { StatItem } from '../components/detail/StatItem'
import { DetailNotFound, DetailError } from '../components/detail/DetailStates'
import { DetailSkeleton } from '../components/detail/DetailSkeleton'
import DecisionDetails from '../components/decision/DecisionDetails'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'

// Raw refs are opaque ids — shorten them so a missing title doesn't blow out the row.
function shortRef(ref: string): string {
  return ref.length > 16 ? `${ref.slice(0, 10)}…` : ref
}

/* ── Plain text section (Context / Outcome) ── */

function TextSection({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-1.5">
      <span className="uppercase" style={{ fontSize: '0.625rem', fontWeight: 500, letterSpacing: '0.08em', color: 'var(--text-3)' }}>
        {label}
      </span>
      <p className="m-0 text-[var(--text-2)] whitespace-pre-wrap" style={{ fontSize: '0.875rem', letterSpacing: '-0.14px', lineHeight: 1.55 }}>
        {children}
      </p>
    </div>
  )
}

/* ── Loading skeleton ── */

function DecisionDetailSkeleton() {
  return (
    <DetailSkeleton>
      <div className="flex flex-col gap-4">
        <Skeleton className="h-28 w-full rounded-[var(--r-md)]" />
        <Skeleton className="h-20 w-full rounded-[var(--r-md)]" />
      </div>
    </DetailSkeleton>
  )
}

/* ── Main component ── */

export default function DecisionDetail() {
  const [, params] = useRoute('/project/:projectId/decisions/:decisionId')
  const [, navigate] = useLocation()
  const projectId = params?.projectId ? decodeURIComponent(params.projectId) : null
  const decisionId = params?.decisionId ? decodeURIComponent(params.decisionId) : null

  const [deleteOpen, setDeleteOpen] = useState(false)

  const { data: decision, isLoading, isError, error } = useDecision(decisionId)
  const deleteDecision = useDeleteDecision()

  // Ref-resolution lookups — only meaningful once the decision is loaded.
  const missionsQuery = useMissions(projectId)
  const krsQuery = useProjectKRs(projectId)
  const tasksQuery = useProjectTasks(projectId)

  if (!projectId || !decisionId) {
    return <DetailNotFound entity="decision" />
  }

  if (isLoading) return <DecisionDetailSkeleton />

  if (isError) {
    return <DetailError entity="decision" message={error instanceof Error ? error.message : 'Unknown error'} />
  }

  if (!decision) {
    return <DetailNotFound entity="decision" />
  }

  const title = sanitizeDecisionTitle(decision.title)
  const actor = decisionActorDisplay(decision)
  const confidence = decision.confidence ?? 0
  const confidencePct = Math.round(confidence * 100)
  const confColor = confidenceColor(confidence)

  const missions = missionsQuery.data ?? []
  const krMap = krsQuery.data ?? new Map()
  const tasks = tasksQuery.data ?? []

  const missionRef = decision.missionRef?.trim()
  const krRef = decision.keyResultRef?.trim()
  const taskRef = decision.taskRef?.trim()

  const kr = krRef ? krMap.get(krRef) : undefined
  const krMission = kr ? missions.find((m) => m.id === kr.missionId) : undefined

  const related: Array<{ key: string; label: string; title: string; to: string; suffix?: ReactNode }> = []
  if (missionRef) {
    const t = missions.find((m) => m.id === missionRef)?.title
    related.push({
      key: 'mission',
      label: 'Mission',
      title: t || safeReferenceLabel(decision.metadata?.missionTitle) || shortRef(missionRef),
      to: missionDetailLink(projectId, missionRef),
    })
  }
  if (krRef) {
    related.push({
      key: 'kr',
      label: 'Key Result',
      title: kr?.title || safeReferenceLabel(decision.metadata?.keyResultTitle) || shortRef(krRef),
      to: krMission ? missionDetailLink(projectId, krMission.id) : strategyMapLink(projectId, krRef),
      suffix: kr && kr.progressPercent > 0 ? `${Math.round(kr.progressPercent)}%` : undefined,
    })
  }
  if (taskRef) {
    const t = tasks.find((x) => x.id === taskRef)
    related.push({
      key: 'task',
      label: 'Task',
      title: t?.title || t?.description || safeReferenceLabel(decision.metadata?.taskTitle) || shortRef(taskRef),
      to: taskDetailLink(projectId, taskRef),
    })
  }

  const tags = decision.tags ?? []

  async function handleDelete() {
    if (!decision) return
    try {
      await deleteDecision.mutateAsync(decision.id)
      toast.success('Decision deleted')
      navigate(decisionsPanelLink(projectId!))
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to delete decision')
    }
  }

  return (
    <div className="flex flex-col h-full overflow-y-auto">
      {/* Sticky header */}
      <div className="sticky top-0 z-10 bg-[var(--bg-app)] border-b border-[var(--border)]/60 w-full">
        <div className="px-6 pt-6 pb-4 max-w-4xl mx-auto w-full">
          <Link
            to={decisionsPanelLink(projectId)}
            className="inline-flex items-center gap-1.5 text-[var(--text-3)] hover:text-[var(--text-1)] transition-colors no-underline mb-5"
            style={{ fontSize: '0.8125rem', letterSpacing: '-0.08px' }}
          >
            <ArrowLeft size={13} />
            Decisions
          </Link>

          <div className="flex items-start gap-3">
            <div className="flex-1 min-w-0">
              <div className="flex items-center gap-2 mb-2">
                <span className="uppercase" style={{ fontSize: '0.625rem', fontWeight: 500, letterSpacing: '0.08em', color: 'var(--text-3)' }}>
                  Decision
                </span>
                <span style={{ fontSize: '0.625rem', color: 'var(--text-3)' }}>·</span>
                <span style={{ fontSize: '0.6875rem', color: 'var(--text-2)' }}>{actor.label}</span>
              </div>
              <h1
                className="m-0 text-[var(--text-1)]"
                style={{ fontSize: '1.75rem', fontWeight: 700, letterSpacing: '-0.56px', lineHeight: 1.14 }}
              >
                {title}
              </h1>
              <div className="flex items-center gap-2 mt-3 flex-wrap">
                <Badge
                  variant="outline"
                  className="gap-1.5"
                  style={{ borderRadius: '4px', fontSize: '0.75rem', fontWeight: 600, letterSpacing: '-0.12px' }}
                >
                  <span style={{ width: 6, height: 6, borderRadius: '50%', background: confColor, display: 'inline-block' }} />
                  {confidencePct}% confidence
                </Badge>
                <span className="flex items-center gap-1 text-[var(--text-3)]" style={{ fontSize: '0.75rem', letterSpacing: '-0.08px' }}>
                  <Clock size={11} />
                  {formatRelative(decision.createdAt, { fallback: 'unknown' })}
                </span>
              </div>
            </div>

            {/* Actions */}
            <div className="flex items-center gap-2 shrink-0">
              <Button
                variant="outline"
                size="sm"
                onClick={() => setDeleteOpen(true)}
                className="dashboard-action-button"
                style={{ letterSpacing: '-0.12px', color: 'var(--red)' }}
              >
                <Trash2 size={12} className="mr-1" />
                Delete
              </Button>
            </div>
          </div>
        </div>
      </div>

      {/* Content */}
      <div className="px-6 py-5 max-w-4xl mx-auto w-full flex flex-col gap-5">
        {/* Context */}
        {decision.context && <TextSection label="Context">{decision.context}</TextSection>}

        {/* Rationale / Alternatives / Invalidation conditions */}
        <div className="rounded-[var(--r-md)] bg-[var(--bg-surface)] border border-[var(--border)]/60 px-4 py-3.5">
          <DecisionDetails decision={decision} />
        </div>

        {/* Outcome */}
        {decision.outcome && <TextSection label="Outcome">{decision.outcome}</TextSection>}

        {/* Stats grid */}
        <div className="rounded-[var(--r-md)] bg-[var(--bg-surface)] border border-[var(--border)]/60 px-4 py-3.5">
          <div className="grid grid-cols-2 sm:grid-cols-3 gap-4">
            <StatItem label="Confidence" value={<span style={{ color: confColor, fontWeight: 600 }}>{confidencePct}%</span>} />
            <StatItem
              label="Logged"
              value={formatRelative(decision.createdAt, { fallback: 'unknown' })}
              icon={<Clock size={11} style={{ color: 'var(--text-3)' }} />}
            />
            <StatItem label="By" value={actor.label} />
            {decision.kind && <StatItem label="Kind" value={decision.kind} />}
          </div>
        </div>

        {/* Related */}
        {related.length > 0 && (
          <CollapsibleSection storageKey="decision-detail-related" defaultOpen label="Related">
            <div className="flex flex-col" style={{ borderTop: '1px solid var(--border)' }}>
              {related.map((item, i) => (
                <Link
                  key={item.key}
                  to={item.to}
                  className="flex items-center gap-2 py-2.5 no-underline group"
                  style={{ borderBottom: i < related.length - 1 ? '1px solid var(--border)' : 'none' }}
                >
                  <span style={{ fontSize: '0.625rem', fontWeight: 600, letterSpacing: '0.04em', textTransform: 'uppercase', color: 'var(--text-3)', width: 78 }}>
                    {item.label}
                  </span>
                  <span className="flex-1 min-w-0 truncate text-[var(--text-1)] group-hover:text-[var(--accent)] transition-colors" style={{ fontSize: '0.8125rem', letterSpacing: '-0.08px' }}>
                    {item.title}
                  </span>
                  {item.suffix && (
                    <span className="shrink-0 tabular-nums text-[var(--text-3)]" style={{ fontSize: '0.6875rem' }}>
                      {item.suffix}
                    </span>
                  )}
                  <ChevronRight size={13} className="shrink-0 text-[var(--text-3)]" />
                </Link>
              ))}
            </div>
          </CollapsibleSection>
        )}

        {/* Tags */}
        {tags.length > 0 && (
          <div className="flex flex-wrap items-center gap-1.5">
            {tags.map((tag) => (
              <span
                key={tag}
                className="inline-flex items-center gap-1 rounded-full bg-[var(--bg-elevated)] px-2 py-0.5 text-[0.6875rem] text-[var(--text-2)]"
              >
                <Tag size={9} className="opacity-70" />
                {tag}
              </span>
            ))}
          </div>
        )}
      </div>

      <AlertDialog open={deleteOpen} onOpenChange={setDeleteOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete decision?</AlertDialogTitle>
            <AlertDialogDescription>
              This removes the decision from the log and clears its graph links. This cannot be undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <Button
              variant="outline"
              onClick={() => setDeleteOpen(false)}
              className="dashboard-action-button dashboard-action-button-neutral border-0"
            >
              Cancel
            </Button>
            <Button
              onClick={handleDelete}
              disabled={deleteDecision.isPending}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              {deleteDecision.isPending ? 'Deleting…' : 'Delete decision'}
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
