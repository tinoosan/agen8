import ApproveRejectPanel from './human-input/ApproveRejectPanel'
import QuestionsPanel from './human-input/QuestionsPanel'
import type {
  ApproveRejectPayload,
  ApproveRejectResult,
  PendingHumanInput,
  QuestionsPayload,
  QuestionsResult,
} from '../hooks/useHumanInput'

interface Props {
  pending: PendingHumanInput | null
  busy: boolean
  onSubmit: (result: QuestionsResult | ApproveRejectResult) => void
  onCancel: () => void
  onCancelRequest?: () => void
}

export default function HumanInputPanel({ pending, busy, onSubmit, onCancel, onCancelRequest }: Props) {
  if (!pending) return null

  if (pending.primitive === 'questions') {
    return (
      <QuestionsPanel
        payload={pending.payload as QuestionsPayload}
        busy={busy}
        onSubmit={(answers) => onSubmit({ answers })}
        onCancel={onCancel}
        onCancelRequest={onCancelRequest}
      />
    )
  }

  if (pending.primitive === 'approve_reject') {
    return (
      <ApproveRejectPanel
        payload={pending.payload as ApproveRejectPayload}
        busy={busy}
        onSubmit={onSubmit}
        onCancel={onCancel}
        onCancelRequest={onCancelRequest}
      />
    )
  }

  return (
    <div className="rounded-2xl border border-[rgba(239,68,68,0.2)] bg-[var(--red-dim)] px-4 py-3 text-sm text-[var(--red)]">
      Unsupported human input primitive: {pending.primitive}
    </div>
  )
}
