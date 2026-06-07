import { CollapsibleSection } from '../strategy/CollapsibleSection'
import { getAcceptanceCriteria } from '../../pages/boardHelpers'
import type { Task } from '../../lib/types'

/* ── Acceptance-criteria checklist ── */

// Computes the criteria, the done/total tally, and the accent color straight
// from the task, then renders the collapsible checklist. Renders nothing when
// the task has no acceptance criteria, so callers can drop it in unconditionally.
export function AcceptanceCriteriaList({ task }: { task: Task }) {
  const acceptanceCriteria = getAcceptanceCriteria(task)
  const acTotal = acceptanceCriteria.length
  if (acTotal === 0) return null

  const acDone = acceptanceCriteria.filter((c) => c.satisfied).length
  const acColor = acDone === acTotal && acTotal > 0 ? 'var(--green)' : acDone > 0 ? 'var(--amber)' : 'var(--text-3)'

  return (
    <CollapsibleSection
      storageKey="task-detail-ac"
      defaultOpen
      label={<>Acceptance Criteria <span style={{ fontWeight: 400, textTransform: 'none', letterSpacing: 0 }}>{acDone}/{acTotal}</span></>}
      accent={acColor}
    >
      <ul className="m-0 p-0 list-none flex flex-col" style={{ borderTop: '1px solid var(--border)' }}>
        {acceptanceCriteria.map((criterion, i) => {
          const checked = criterion.satisfied
          return (
            <li
              key={criterion.id || i}
              className="flex items-start gap-2"
              style={{
                paddingTop: '9px',
                paddingBottom: '9px',
                borderBottom: i < acceptanceCriteria.length - 1 ? '1px solid var(--border)' : 'none',
              }}
            >
              <span
                className="flex-shrink-0 flex items-center justify-center"
                style={{
                  width: 13,
                  height: 13,
                  marginTop: 2,
                  borderRadius: 3,
                  border: checked ? 'none' : '1px solid var(--border-strong)',
                  background: checked ? 'var(--green)' : 'transparent',
                }}
              >
                {checked && <span style={{ color: 'white', fontSize: '0.5rem', fontWeight: 700, lineHeight: 1 }}>✓</span>}
              </span>
              <span
                style={{
                  fontSize: '0.8125rem',
                  lineHeight: 1.47,
                  letterSpacing: '-0.08px',
                  color: checked ? 'var(--text-3)' : 'var(--text-1)',
                  textDecoration: checked ? 'line-through' : 'none',
                }}
              >
                {criterion.text}
              </span>
            </li>
          )
        })}
      </ul>
    </CollapsibleSection>
  )
}
