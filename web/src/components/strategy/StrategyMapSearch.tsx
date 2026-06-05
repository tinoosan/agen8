import { useEffect, useMemo, useRef, useState } from 'react'
import type { KeyboardEvent as ReactKeyboardEvent } from 'react'
import type { Node } from '@xyflow/react'
import { Search, Target, Diamond } from 'lucide-react'
import { Dialog, DialogContent } from '@/components/ui/dialog'
import { cn } from '@/lib/utils'
import { memberDisplayName } from '../../lib/memberDisplay'

interface StrategyMapSearchProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  nodes: Node[]
  onSelect: (nodeId: string) => void
}

interface SearchResult {
  node: Node
  title: string
  type: string
  typeLabel: string
  meta: ResultMeta
}

interface ResultMeta {
  status?: string
  statusTone?: string
  owner?: string
  progress?: number
  confidence?: number
  confidenceTone?: string
  phaseCount?: number
  todosDone?: number
  todosTotal?: number
  decisionCount?: number
}

const MAX_RESULTS = 60

// ── Type filter config ──────────────────────────────────────────────────

interface TypeFilter {
  key: string
  label: string
  prefix: string // prefix shortcut for typing (e.g., "t:" for tasks)
}

const TYPE_FILTERS: TypeFilter[] = [
  { key: 'mission', label: 'Mission', prefix: 'm:' },
  { key: 'keyResult', label: 'KR', prefix: 'kr:' },
  { key: 'task', label: 'Task', prefix: 't:' },
  { key: 'decision', label: 'Decision', prefix: 'd:' },
]

// ── Group ordering ──────────────────────────────────────────────────────

const TYPE_ORDER = ['mission', 'keyResult', 'task', 'decision']

function getTypeGroupLabel(type: string): string {
  switch (type) {
    case 'mission': return 'Missions'
    case 'keyResult': return 'Key Results'
    case 'decision': return 'Decisions'
    case 'task': return 'Tasks'
    default: return type
  }
}

// ── Data extraction ─────────────────────────────────────────────────────

function getNodeTitle(node: Node): string {
  const data = node.data as Record<string, unknown> | undefined
  if (!data) return '(untitled)'
  const mission = data.mission as { title?: string } | undefined
  if (mission?.title) return mission.title
  const kr = data.kr as { title?: string } | undefined
  if (kr?.title) return kr.title
  const plan = data.plan as { title?: string } | undefined
  if (plan?.title) return plan.title
  const decision = data.decision as { title?: string } | undefined
  if (decision?.title) return decision.title
  const task = data.task as { title?: string; description?: string } | undefined
  if (task?.title) return task.title
  if (task?.description) return task.description
  return '(untitled)'
}

function getNodeTypeLabel(type: string | undefined): string {
  switch (type) {
    case 'mission': return 'Mission'
    case 'keyResult': return 'Key Result'
    case 'decision': return 'Decision'
    case 'task': return 'Task'
    default: return ''
  }
}

function confidenceColor(value: number): string {
  if (value >= 0.8) return 'var(--green)'
  if (value >= 0.6) return 'var(--amber)'
  return 'var(--red)'
}

const STATUS_TONE: Record<string, string> = {
  pending: 'var(--text-3)',
  active: 'var(--accent)',
  delegated: 'var(--accent)',
  resumed: 'var(--accent)',
  blocked: 'var(--amber)',
  in_review: 'var(--purple, var(--accent))',
  succeeded: 'var(--green)',
  done: 'var(--green)',
  complete: 'var(--green)',
  completed: 'var(--green)',
  failed: 'var(--red)',
  canceled: 'var(--red)',
  suspended: 'var(--text-3)',
  draft: 'var(--text-3)',
  pending_approval: 'var(--amber)',
  abandoned: 'var(--red)',
  acknowledged: 'var(--accent)',
  in_progress: 'var(--accent)',
  pending_verification: 'var(--purple, var(--accent))',
  resolved: 'var(--green)',
  expired: 'var(--text-3)',
}

function getNodeMeta(node: Node): ResultMeta {
  const data = node.data as Record<string, unknown> | undefined
  if (!data) return {}

  const mission = data.mission as { status?: string } | undefined
  if (mission) {
    const avgProgress = data.avgProgress as number | undefined
    const krCount = data.krCount as number | undefined
    return {
      status: mission.status,
      statusTone: STATUS_TONE[mission.status ?? ''],
      progress: avgProgress,
      decisionCount: krCount,
    }
  }

  const kr = data.kr as { progressPercent?: number; status?: string } | undefined
  if (kr) {
    return {
      status: kr.status,
      statusTone: STATUS_TONE[kr.status ?? ''],
      progress: kr.progressPercent,
      decisionCount: data.linkedDecisionCount as number | undefined,
    }
  }

  const plan = data.plan as { status?: string; mode?: string } | undefined
  if (plan) {
    return {
      status: plan.status,
      statusTone: STATUS_TONE[plan.status ?? ''],
      phaseCount: data.phaseCount as number | undefined,
      todosDone: data.todosDone as number | undefined,
      todosTotal: data.todosTotal as number | undefined,
    }
  }

  const decision = data.decision as { confidence?: number; source?: string; sourceIdentity?: string; memberName?: string } | undefined
  if (decision) {
    const conf = decision.confidence ?? 0
    return {
      owner: decision.memberName?.trim() || decision.sourceIdentity?.trim() || undefined,
      confidence: conf > 0 ? conf : undefined,
      confidenceTone: conf > 0 ? confidenceColor(conf) : undefined,
    }
  }

  const task = data.task as { status?: string; assignedTo?: string; assignedToLabel?: string } | undefined
  if (task) {
    return {
      status: task.status,
      statusTone: STATUS_TONE[task.status ?? ''],
      owner: memberDisplayName(task.assignedToLabel, task.assignedTo),
    }
  }

  return {}
}

// ── Component ───────────────────────────────────────────────────────────

export function StrategyMapSearch({
  open,
  onOpenChange,
  nodes,
  onSelect,
}: StrategyMapSearchProps) {
  const [query, setQuery] = useState('')
  const [selectedIdx, setSelectedIdx] = useState(0)
  const [activeTypes, setActiveTypes] = useState<Set<string>>(new Set())
  const [prevOpen, setPrevOpen] = useState(open)
  const inputRef = useRef<HTMLInputElement>(null)
  const listRef = useRef<HTMLDivElement>(null)

  if (open !== prevOpen) {
    setPrevOpen(open)
    if (open) {
      setQuery('')
      setSelectedIdx(0)
      setActiveTypes(new Set())
    }
  }

  useEffect(() => {
    if (open) {
      const t = window.setTimeout(() => inputRef.current?.focus(), 40)
      return () => window.clearTimeout(t)
    }
  }, [open])

  const toggleType = (key: string) => {
    setActiveTypes((prev) => {
      const next = new Set(prev)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      return next
    })
    setSelectedIdx(0)
  }

  const allResults = useMemo<SearchResult[]>(() => {
    return nodes.map((node) => ({
      node,
      title: getNodeTitle(node),
      type: node.type ?? '',
      typeLabel: getNodeTypeLabel(node.type),
      meta: getNodeMeta(node),
    }))
  }, [nodes])

  // Parse prefix shortcuts from query (e.g., "t:search term")
  const { effectiveQuery, prefixType } = useMemo(() => {
    const q = query.trim()
    for (const tf of TYPE_FILTERS) {
      if (q.toLowerCase().startsWith(tf.prefix)) {
        return { effectiveQuery: q.slice(tf.prefix.length).trim(), prefixType: tf.key }
      }
    }
    return { effectiveQuery: q, prefixType: null }
  }, [query])

  const filteredResults = useMemo<SearchResult[]>(() => {
    let items = allResults

    // Apply type filter (chips or prefix)
    const typeFilter = prefixType ? new Set([prefixType]) : activeTypes
    if (typeFilter.size > 0) {
      items = items.filter((r) => typeFilter.has(r.type))
    }

    // Apply text search
    const q = effectiveQuery.toLowerCase()
    if (q) {
      items = items.filter((r) =>
        r.title.toLowerCase().includes(q) ||
        (r.meta.owner && r.meta.owner.toLowerCase().includes(q)) ||
        (r.meta.status && r.meta.status.toLowerCase().includes(q)),
      )
    }

    return items.slice(0, MAX_RESULTS)
  }, [allResults, activeTypes, effectiveQuery, prefixType])

  // Group results by type
  const groupedResults = useMemo(() => {
    const groups: { type: string; label: string; items: SearchResult[] }[] = []
    const byType = new Map<string, SearchResult[]>()

    for (const r of filteredResults) {
      const list = byType.get(r.type) ?? []
      list.push(r)
      byType.set(r.type, list)
    }

    for (const type of TYPE_ORDER) {
      const items = byType.get(type)
      if (items && items.length > 0) {
        groups.push({ type, label: getTypeGroupLabel(type), items })
      }
    }

    return groups
  }, [filteredResults])

  // Flat list for keyboard navigation
  const flatResults = useMemo(() => groupedResults.flatMap((g) => g.items), [groupedResults])
  const effectiveSelectedIdx = Math.min(selectedIdx, Math.max(0, flatResults.length - 1))

  useEffect(() => {
    const el = listRef.current?.querySelector<HTMLElement>(`[data-idx="${effectiveSelectedIdx}"]`)
    el?.scrollIntoView({ block: 'nearest' })
  }, [effectiveSelectedIdx])

  const handleKeyDown = (e: ReactKeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setSelectedIdx(Math.min(effectiveSelectedIdx + 1, flatResults.length - 1))
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      setSelectedIdx(Math.max(effectiveSelectedIdx - 1, 0))
    } else if (e.key === 'Enter') {
      e.preventDefault()
      const picked = flatResults[effectiveSelectedIdx]
      if (picked) {
        onSelect(picked.node.id)
        onOpenChange(false)
      }
    }
  }

  // Count nodes per type for filter chip badges
  const typeCounts = useMemo(() => {
    const counts = new Map<string, number>()
    for (const r of allResults) {
      counts.set(r.type, (counts.get(r.type) ?? 0) + 1)
    }
    return counts
  }, [allResults])

  let flatIdx = 0

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="sm:max-w-[620px] p-0 overflow-hidden gap-0 border-0 [&>button]:hidden"
        style={{
          borderRadius: '12px',
          boxShadow: 'rgba(0, 0, 0, 0.22) 3px 5px 30px 0px',
          background: 'var(--bg-panel)',
        }}
      >
        {/* Search input */}
        <div
          className="flex items-center"
          style={{ padding: '16px 20px', gap: '12px', borderBottom: '1px solid var(--border)' }}
        >
          <Search size={16} style={{ color: 'var(--text-3)', flexShrink: 0 }} />
          <input
            ref={inputRef}
            value={query}
            onChange={(e) => { setQuery(e.target.value); setSelectedIdx(0) }}
            onKeyDown={handleKeyDown}
            placeholder="Search nodes... (t: d: kr: m: for type filters)"
            className="flex-1 bg-transparent outline-none focus:outline-none focus-visible:outline-none"
            style={{
              outline: 'none',
              boxShadow: 'none',
              fontSize: '15px',
              fontWeight: 400,
              lineHeight: 1.47,
              letterSpacing: '-0.2px',
              color: 'var(--text-1)',
            }}
          />
          <kbd
            className="inline-flex items-center justify-center shrink-0"
            style={{
              minWidth: '24px', height: '22px', padding: '0 7px',
              borderRadius: '5px', border: '1px solid var(--border)',
              background: 'var(--bg-surface)', color: 'var(--text-3)',
              fontFamily: 'SF Mono, Menlo, monospace',
              fontSize: '11px', fontWeight: 500, lineHeight: 1, letterSpacing: '-0.12px',
            }}
          >
            Esc
          </kbd>
        </div>

        {/* Type filter chips */}
        <div className="flex items-center gap-1.5 flex-wrap" style={{ padding: '10px 20px 6px' }}>
          {TYPE_FILTERS.map((tf) => {
            const count = typeCounts.get(tf.key) ?? 0
            if (count === 0) return null
            const isActive = activeTypes.has(tf.key)
            return (
              <button
                key={tf.key}
                type="button"
                onClick={() => toggleType(tf.key)}
                className={cn(
                  'inline-flex items-center gap-1 transition-colors duration-150',
                  'focus:outline-none focus-visible:outline-none',
                )}
                style={{
                  padding: '3px 8px',
                  borderRadius: 12,
                  fontSize: '11px',
                  fontWeight: 600,
                  letterSpacing: '0.02em',
                  border: `1px solid ${isActive ? 'var(--accent)' : 'var(--border)'}`,
                  background: isActive ? 'color-mix(in srgb, var(--accent) 12%, transparent)' : 'transparent',
                  color: isActive ? 'var(--accent)' : 'var(--text-3)',
                }}
              >
                {tf.label}
                <span style={{ opacity: 0.6 }}>{count}</span>
              </button>
            )
          })}
        </div>

        {/* Results */}
        <div
          ref={listRef}
          className="overflow-y-auto"
          style={{ maxHeight: '380px', padding: '4px 8px 8px' }}
        >
          {flatResults.length === 0 ? (
            <div className="text-center" style={{
              padding: '40px 20px', fontSize: '14px', fontWeight: 400,
              lineHeight: 1.43, letterSpacing: '-0.224px', color: 'var(--text-3)',
            }}>
              No matching nodes
            </div>
          ) : (
            groupedResults.map((group) => (
              <div key={group.type}>
                {/* Group header */}
                <div className="flex items-center gap-2" style={{
                  padding: '8px 12px 4px',
                  fontSize: '10px', fontWeight: 600, letterSpacing: '0.08em',
                  color: 'var(--text-3)', textTransform: 'uppercase',
                }}>
                  {group.label}
                  <span style={{ opacity: 0.5 }}>{group.items.length}</span>
                </div>

                {/* Group items */}
                {group.items.map((result) => {
                  const idx = flatIdx++
                  const isSelected = idx === effectiveSelectedIdx
                  return (
                    <button
                      key={result.node.id}
                      data-idx={idx}
                      type="button"
                      className={cn(
                        'w-full text-left flex items-center',
                        'transition-colors duration-150',
                        'focus:outline-none focus-visible:outline-none',
                      )}
                      style={{
                        padding: '8px 12px',
                        borderRadius: '8px',
                        background: isSelected ? 'var(--bg-surface)' : 'transparent',
                        gap: '10px',
                        outline: 'none',
                      }}
                      onClick={() => { onSelect(result.node.id); onOpenChange(false) }}
                      onMouseEnter={() => setSelectedIdx(idx)}
                    >
                      {/* Type icon */}
                      <ResultIcon type={result.type} />

                      {/* Title + metadata */}
                      <div className="flex-1 min-w-0 flex flex-col gap-[1px]">
                        <span className="truncate" style={{
                          fontSize: '13px', fontWeight: 500, lineHeight: 1.3,
                          letterSpacing: '-0.1px', color: 'var(--text-1)',
                        }}>
                          {result.title}
                        </span>
                        <ResultMetaRow meta={result.meta} type={result.type} />
                      </div>

                      {/* Progress bar for KRs/missions */}
                      {result.meta.progress != null && (
                        <div className="flex items-center gap-1.5 shrink-0" style={{ width: 60 }}>
                          <div className="flex-1 h-[3px] rounded-full overflow-hidden" style={{ background: 'var(--border)' }}>
                            <div className="h-full rounded-full" style={{
                              width: `${Math.min(100, result.meta.progress)}%`,
                              background: result.meta.progress >= 100 ? 'var(--green)' : 'var(--accent)',
                            }} />
                          </div>
                          <span style={{ fontSize: '10px', fontWeight: 600, color: 'var(--text-3)' }}>
                            {Math.round(result.meta.progress)}%
                          </span>
                        </div>
                      )}
                    </button>
                  )
                })}
              </div>
            ))
          )}
        </div>
      </DialogContent>
    </Dialog>
  )
}

// ── Subcomponents ────────────────────────────────────────────────────────

function ResultIcon({ type }: { type: string }) {
  const size = 13
  const style = { color: 'var(--text-3)', flexShrink: 0 } as const
  switch (type) {
    case 'mission': return <Target size={size} style={style} />
    case 'keyResult': return <div style={{ width: 13, height: 8, borderRadius: 2, border: '1.5px solid var(--text-3)', flexShrink: 0 }} />
    case 'decision': return <Diamond size={size} style={style} />
    case 'task': return <div style={{ width: 11, height: 11, borderRadius: '50%', border: '1.5px solid var(--text-3)', flexShrink: 0 }} />
    default: return null
  }
}

function ResultMetaRow({ meta, type }: { meta: ResultMeta; type: string }) {
  const items: { text: string; color?: string }[] = []

  if (meta.status) {
    items.push({
      text: meta.status.replaceAll('_', ' '),
      color: meta.statusTone,
    })
  }

  if (meta.owner) {
    items.push({ text: meta.owner })
  }

  if (meta.confidence != null) {
    items.push({
      text: `${Math.round(meta.confidence * 100)}%`,
      color: meta.confidenceTone,
    })
  }

  if (type === 'mission' && meta.decisionCount != null) {
    items.push({ text: `${meta.decisionCount} KRs` })
  }

  if (type === 'keyResult' && meta.decisionCount != null && meta.decisionCount > 0) {
    items.push({ text: `${meta.decisionCount} decisions` })
  }

  if (meta.phaseCount != null && meta.phaseCount > 0) {
    items.push({ text: `${meta.phaseCount} phases` })
  }

  if (meta.todosTotal != null && meta.todosTotal > 0) {
    items.push({ text: `${meta.todosDone}/${meta.todosTotal} todos` })
  }

  if (items.length === 0) return null

  return (
    <span className="flex items-center gap-1 text-[10px] font-medium leading-none text-muted-foreground">
      {items.map((item, i) => (
        <span key={i} className="flex items-center gap-1">
          {i > 0 && <span className="text-muted-foreground/40">·</span>}
          <span className="capitalize" style={item.color ? { color: item.color } : undefined}>
            {item.text}
          </span>
        </span>
      ))}
    </span>
  )
}
