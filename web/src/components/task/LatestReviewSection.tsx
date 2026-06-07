import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { CollapsibleSection } from '../strategy/CollapsibleSection'
import { getLatestReview } from '../../pages/boardHelpers'
import { formatRelative } from '@/lib/format'
import type { Task } from '../../lib/types'

/* ── Latest-review summary ── */

// Tone styling keyed by review decision. Module-level so it is built once
// rather than on every render.
const reviewTone: Record<string, { fg: string; bg: string; label: string }> = {
  approved: { fg: 'var(--green)', bg: 'color-mix(in srgb, var(--green) 12%, transparent)', label: 'Approved' },
  retry: { fg: 'var(--amber)', bg: 'color-mix(in srgb, var(--amber) 12%, transparent)', label: 'Retry Requested' },
  failed: { fg: 'var(--red)', bg: 'color-mix(in srgb, var(--red) 10%, transparent)', label: 'Failed' },
  fail: { fg: 'var(--red)', bg: 'color-mix(in srgb, var(--red) 10%, transparent)', label: 'Failed' },
}

// Renders the most recent review for the task. Renders nothing when the task
// has no review yet, so callers can drop it in unconditionally.
export function LatestReviewSection({ task }: { task: Task }) {
  const latestReview = getLatestReview(task)
  if (!latestReview) return null

  const tone = reviewTone[latestReview.decision] ?? { fg: 'var(--text-1)', bg: 'var(--bg-elevated)', label: latestReview.decision }

  return (
    <CollapsibleSection storageKey="task-detail-review" defaultOpen label="Latest Review">
      <div style={{ borderTop: '1px solid var(--border)', paddingTop: 10 }}>
        <div className="flex items-center gap-2 flex-wrap">
          <span
            style={{
              fontSize: '0.625rem',
              fontWeight: 700,
              letterSpacing: '0.06em',
              textTransform: 'uppercase',
              padding: '2px 8px',
              borderRadius: 980,
              color: tone.fg,
              background: tone.bg,
            }}
          >
            {tone.label}
          </span>
          {(latestReview.reviewerRole || latestReview.reviewedBy) && (
            <span style={{ fontSize: '0.6875rem', color: 'var(--text-3)' }}>
              by {latestReview.reviewerRole || latestReview.reviewedBy}
            </span>
          )}
          {latestReview.reviewedAt && (
            <span style={{ fontSize: '0.6875rem', color: 'var(--text-3)' }}>{formatRelative(latestReview.reviewedAt, { fallback: 'unknown' })}</span>
          )}
        </div>
        {latestReview.feedback && (
          <div className="md-prose" style={{ fontSize: '0.8125rem', lineHeight: 1.47, color: 'var(--text-2)', marginTop: 8 }}>
            <ReactMarkdown remarkPlugins={[remarkGfm]}>{latestReview.feedback}</ReactMarkdown>
          </div>
        )}
      </div>
    </CollapsibleSection>
  )
}
