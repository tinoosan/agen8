import { useMemo } from 'react'
import { useStore } from '../lib/store'
import { usePlan } from '../hooks/usePlan'
import { X, ListChecks, AlertCircle } from 'lucide-react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'

interface PlanSlideOverProps {
  teamId: string
  threadId: string | null
}

// TUI colors (Tokyo Night palette)
const PLAN_TEAL = '#56b6c2'
const OK_GREEN = '#98c379'
const DIM_GRAY = '#707070'

interface ChecklistItem {
  done: boolean
  text: string
  active: boolean // first unchecked item
}

const checklistRe = /^[\s]*[-*]\s*\[([ xX])\]\s*(.+)$/

function parseChecklistItems(md: string): { items: ChecklistItem[]; activeStep: string; done: number; total: number } {
  const items: ChecklistItem[] = []
  let foundActive = false
  let activeStep = ''
  let done = 0

  for (const line of md.split('\n')) {
    const m = line.match(checklistRe)
    if (m) {
      const isDone = m[1].toLowerCase() === 'x'
      const text = m[2].trim()
      const isActive = !isDone && !foundActive
      if (isDone) done++
      if (isActive) {
        foundActive = true
        activeStep = text
      }
      items.push({ done: isDone, text, active: isActive })
    }
  }
  return { items, activeStep, done, total: items.length }
}

/** Renders the checklist in TUI tree style with progress info */
function PlanChecklist({ items, activeStep, done, total }: {
  items: ChecklistItem[]
  activeStep: string
  done: number
  total: number
}) {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
      {/* Progress summary */}
      {total > 0 && (
        <div style={{ display: 'flex', alignItems: 'center', gap: 10, fontSize: 12 }}>
          <span style={{ color: 'var(--text-3)', fontStyle: 'italic' }}>
            Progress: {done}/{total} complete
          </span>
          {/* Mini progress bar */}
          <div style={{
            flex: 1, maxWidth: 120, height: 4,
            background: 'var(--bg-elevated)', borderRadius: 2, overflow: 'hidden',
          }}>
            <div style={{
              width: `${(done / total) * 100}%`, height: '100%',
              background: OK_GREEN, borderRadius: 2,
              transition: 'width 0.3s ease',
            }} />
          </div>
        </div>
      )}

      {/* Current step callout */}
      {activeStep && (
        <div style={{
          fontSize: 12, fontStyle: 'italic', color: PLAN_TEAL,
        }}>
          Current step: <strong>{activeStep}</strong>
        </div>
      )}

      {/* Tree */}
      <div className="mono" style={{ fontSize: 12, lineHeight: 1.8 }}>
        {items.map((item, i) => {
          const isLast = i === items.length - 1
          const branch = isLast ? '└─ ' : '├─ '
          return (
            <div key={i} style={{ display: 'flex', alignItems: 'baseline' }}>
              <span style={{ color: PLAN_TEAL, whiteSpace: 'pre', flexShrink: 0 }}>{branch}</span>
              {item.done ? (
                <span>
                  <span style={{ color: OK_GREEN }}>✓ </span>
                  <span style={{ color: DIM_GRAY, textDecoration: 'line-through' }}>{item.text}</span>
                </span>
              ) : item.active ? (
                <span>
                  <span style={{ color: PLAN_TEAL }}>● </span>
                  <span style={{ color: 'var(--text-1)', fontWeight: 700 }}>{item.text}</span>
                </span>
              ) : (
                <span>
                  <span style={{ color: DIM_GRAY }}>○ </span>
                  <span style={{ color: 'var(--text-2)' }}>{item.text}</span>
                </span>
              )}
            </div>
          )
        })}
      </div>
    </div>
  )
}

export default function PlanSlideOver({ teamId, threadId }: PlanSlideOverProps) {
  const { setPlanOpen } = useStore()
  const planQuery = usePlan(teamId, threadId)
  const plan = planQuery.data

  const parsed = useMemo(
    () => plan?.checklist ? parseChecklistItems(plan.checklist) : { items: [], activeStep: '', done: 0, total: 0 },
    [plan?.checklist],
  )

  const hasChecklist = parsed.items.length > 0 && !plan?.checklistErr
  const hasDetails = !!plan?.details && !plan?.detailsErr
  const isEmpty = !planQuery.isLoading && !hasChecklist && !hasDetails

  return (
    <div
      className="slide-over-backdrop animate-slide-in-right"
      onClick={() => setPlanOpen(false)}
    >
      <div
        onClick={e => e.stopPropagation()}
        style={{
          width: 540, height: '100%',
          background: 'var(--bg-panel)',
          borderLeft: '1px solid var(--border)',
          display: 'flex', flexDirection: 'column',
          boxShadow: '-12px 0 48px rgba(0,0,0,0.4)',
        }}
      >
        {/* Header */}
        <div style={{
          display: 'flex', alignItems: 'center', justifyContent: 'space-between',
          padding: '14px 16px',
          borderBottom: '1px solid var(--border)',
          flexShrink: 0,
        }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <ListChecks size={16} style={{ color: PLAN_TEAL }} />
            <span style={{ fontWeight: 600, fontSize: 14, color: 'var(--text-1)', letterSpacing: '-0.02em' }}>
              Plan
            </span>
            {parsed.total > 0 && (
              <span style={{
                fontSize: 11, color: 'var(--text-3)',
                background: 'var(--bg-elevated)',
                border: '1px solid var(--border)',
                borderRadius: 999, padding: '0 6px',
                fontVariantNumeric: 'tabular-nums',
              }}>
                {parsed.done}/{parsed.total}
              </span>
            )}
          </div>
          <button
            onClick={() => setPlanOpen(false)}
            style={{
              background: 'none', border: 'none', cursor: 'pointer',
              color: 'var(--text-3)', display: 'flex', padding: 4,
            }}
          >
            <X size={16} />
          </button>
        </div>

        {/* Content */}
        <div style={{ flex: 1, overflowY: 'auto', padding: '16px 20px' }}>
          {planQuery.isLoading ? (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
              <div className="skeleton" style={{ width: '60%', height: 20 }} />
              <div className="skeleton" style={{ width: '100%', height: 14 }} />
              <div className="skeleton" style={{ width: '100%', height: 14 }} />
              <div className="skeleton" style={{ width: '80%', height: 14 }} />
              <div className="skeleton" style={{ width: '100%', height: 14 }} />
            </div>
          ) : isEmpty ? (
            <div style={{ textAlign: 'center', padding: '40px 20px', color: 'var(--text-3)' }}>
              <ListChecks size={32} style={{ opacity: 0.3, marginBottom: 12 }} />
              <div style={{ fontSize: 13 }}>No plan available yet</div>
              <div style={{ fontSize: 11, marginTop: 4 }}>The team plan will appear here once created</div>
            </div>
          ) : (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 0 }}>
              {/* Error banners */}
              {plan?.detailsErr && (
                <div style={{
                  display: 'flex', alignItems: 'center', gap: 6,
                  padding: '8px 12px', marginBottom: 12, borderRadius: 'var(--r-md)',
                  background: 'var(--red-dim)', color: 'var(--red)',
                  fontSize: 11,
                }}>
                  <AlertCircle size={12} />
                  Details: {plan.detailsErr}
                </div>
              )}
              {plan?.checklistErr && (
                <div style={{
                  display: 'flex', alignItems: 'center', gap: 6,
                  padding: '8px 12px', marginBottom: 12, borderRadius: 'var(--r-md)',
                  background: 'var(--red-dim)', color: 'var(--red)',
                  fontSize: 11,
                }}>
                  <AlertCircle size={12} />
                  Checklist: {plan.checklistErr}
                </div>
              )}

              {/* Section 1: Plan Details (HEAD.md) — shown first like the TUI */}
              {hasDetails && (
                <div style={{ marginBottom: hasChecklist ? 20 : 0 }}>
                  <div style={{
                    fontSize: 11, fontWeight: 600, textTransform: 'uppercase',
                    letterSpacing: '0.06em', color: PLAN_TEAL, marginBottom: 10,
                  }}>
                    Plan Details
                  </div>
                  <div className="plan-markdown" style={{
                    fontSize: 13, color: 'var(--text-2)', lineHeight: 1.65,
                    paddingLeft: 2,
                  }}>
                    <ReactMarkdown remarkPlugins={[remarkGfm]}>{plan!.details}</ReactMarkdown>
                  </div>
                </div>
              )}

              {/* Divider between sections */}
              {hasDetails && hasChecklist && (
                <div style={{ height: 1, background: 'var(--border)', marginBottom: 20 }} />
              )}

              {/* Section 2: Checklist (CHECKLIST.md) */}
              {hasChecklist && (
                <div>
                  <div style={{
                    fontSize: 11, fontWeight: 600, textTransform: 'uppercase',
                    letterSpacing: '0.06em', color: PLAN_TEAL, marginBottom: 10,
                  }}>
                    Checklist
                  </div>
                  <PlanChecklist
                    items={parsed.items}
                    activeStep={parsed.activeStep}
                    done={parsed.done}
                    total={parsed.total}
                  />
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
