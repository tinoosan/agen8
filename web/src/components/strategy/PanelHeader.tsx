import type { ReactNode } from 'react'
import { X } from 'lucide-react'

/* ── Dark header bar for strategy slide-over panels ── */

// The header frame repeated byte-for-byte across TaskPanel, KRPanel,
// MissionPanel, and DecisionPanel: a dark padded row with a flexible title
// area on the left and a round close button on the right. Each panel's title
// content (eyebrow, heading, id badges) genuinely differs, so it is passed as
// `children`; only the frame and the close affordance are shared.

export function PanelHeader({ onClose, children }: { onClose: () => void; children: ReactNode }) {
  return (
    <div
      className="flex items-start gap-2 shrink-0"
      style={{ background: 'var(--bg-app)', padding: '16px' }}
    >
      <div className="flex-1 min-w-0">{children}</div>
      <button
        onClick={onClose}
        className="shrink-0 text-muted-foreground hover:text-foreground transition-colors mt-0.5"
        style={{ padding: '4px', borderRadius: '50%', background: 'rgba(255,255,255,0.08)' }}
        aria-label="Close panel"
      >
        <X size={14} />
      </button>
    </div>
  )
}
