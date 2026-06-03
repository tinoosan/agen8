import { Check, AlertTriangle, X } from 'lucide-react'

export type BannerKind = 'success' | 'warning' | 'error'

export interface BannerState { kind: BannerKind; message: string }

export function InlineBanner({ banner, onDismiss }: { banner: BannerState; onDismiss: () => void }) {
  const c = {
    success: { bg: 'var(--green-dim)', border: 'var(--green)', text: 'var(--green)', icon: <Check size={13} /> },
    warning: { bg: 'var(--amber-dim)', border: 'var(--amber)', text: 'var(--amber)', icon: <AlertTriangle size={13} /> },
    error: { bg: 'var(--red-dim)', border: 'var(--red)', text: 'var(--red)', icon: <X size={13} /> },
  }[banner.kind]
  return (
    <div
      className="flex items-start gap-2 px-3.5 py-2.5 rounded-[var(--r-md)] text-xs font-medium mb-4"
      style={{ background: c.bg, border: `1px solid ${c.border}`, color: c.text }}
    >
      <span className="shrink-0 mt-px">{c.icon}</span>
      <span className="flex-1 leading-normal">{banner.message}</span>
      <button
        type="button"
        onClick={onDismiss}
        className="bg-transparent border-none cursor-pointer p-px flex shrink-0"
        style={{ color: c.text }}
      >
        <X size={12} />
      </button>
    </div>
  )
}
