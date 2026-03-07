import { useStore } from '../lib/store'
import { usePlan } from '../hooks/usePlan'
import { X, ListChecks, AlertCircle } from 'lucide-react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'

interface PlanSlideOverProps {
  teamId: string
  threadId: string | null
}

export default function PlanSlideOver({ teamId, threadId }: PlanSlideOverProps) {
  const { setPlanOpen } = useStore()
  const planQuery = usePlan(teamId, threadId)
  const plan = planQuery.data

  const hasChecklist = plan && plan.checklist && !plan.checklistErr
  const hasDetails = plan && plan.details && !plan.detailsErr
  const isEmpty = !planQuery.isLoading && !hasChecklist && !hasDetails

  return (
    <div
      className="slide-over-backdrop animate-slide-in-right"
      onClick={() => setPlanOpen(false)}
    >
      <div
        onClick={e => e.stopPropagation()}
        style={{
          width: 520, height: '100%',
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
            <ListChecks size={16} style={{ color: 'var(--accent)' }} />
            <span style={{ fontWeight: 600, fontSize: 14, color: 'var(--text-1)', letterSpacing: '-0.02em' }}>
              Plan
            </span>
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
        <div style={{ flex: 1, overflowY: 'auto', padding: '16px' }}>
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
            <div style={{ display: 'flex', flexDirection: 'column', gap: 20 }}>
              {/* Checklist section */}
              {plan?.checklistErr && (
                <div style={{
                  display: 'flex', alignItems: 'center', gap: 6,
                  padding: '8px 12px', borderRadius: 'var(--r-md)',
                  background: 'var(--red-dim)', color: 'var(--red)',
                  fontSize: 11,
                }}>
                  <AlertCircle size={12} />
                  Checklist: {plan.checklistErr}
                </div>
              )}
              {hasChecklist && (
                <div>
                  <div style={{
                    fontSize: 11, fontWeight: 600, textTransform: 'uppercase',
                    letterSpacing: '0.06em', color: 'var(--text-3)', marginBottom: 8,
                  }}>
                    Checklist
                  </div>
                  <div className="plan-markdown">
                    <ReactMarkdown remarkPlugins={[remarkGfm]}>{plan!.checklist}</ReactMarkdown>
                  </div>
                </div>
              )}

              {/* Details section */}
              {plan?.detailsErr && (
                <div style={{
                  display: 'flex', alignItems: 'center', gap: 6,
                  padding: '8px 12px', borderRadius: 'var(--r-md)',
                  background: 'var(--red-dim)', color: 'var(--red)',
                  fontSize: 11,
                }}>
                  <AlertCircle size={12} />
                  Details: {plan.detailsErr}
                </div>
              )}
              {hasDetails && (
                <div>
                  <div style={{
                    fontSize: 11, fontWeight: 600, textTransform: 'uppercase',
                    letterSpacing: '0.06em', color: 'var(--text-3)', marginBottom: 8,
                  }}>
                    Details
                  </div>
                  <div className="plan-markdown">
                    <ReactMarkdown remarkPlugins={[remarkGfm]}>{plan!.details}</ReactMarkdown>
                  </div>
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
