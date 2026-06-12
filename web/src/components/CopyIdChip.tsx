import { Hash } from 'lucide-react'
import { toast } from 'sonner'
import { copyText } from '@/lib/utils'

/**
 * CopyIdChip — the short-id chip on detail pages, made useful: clicking copies
 * the FULL entity id. That id is the handle the human pastes when telling an
 * agent what to work on ("pick up task-x"), and until this chip the UI never
 * surfaced it anywhere copyable.
 */
export default function CopyIdChip({ id, shortId }: { id: string; shortId: string }) {
  async function handleCopy() {
    try {
      await copyText(id)
      toast.success(`${id} copied`)
    } catch {
      toast.error('Copy failed')
    }
  }

  return (
    <button
      type="button"
      onClick={() => void handleCopy()}
      title={`Copy ${id}`}
      aria-label={`Copy id ${id}`}
      className="flex items-center gap-0.5 border-none bg-transparent p-0 cursor-pointer text-[var(--text-3)] hover:text-[var(--text-1)] transition-colors"
    >
      <Hash size={9} />
      <span style={{ fontSize: '0.625rem', fontFamily: 'monospace' }}>{shortId}</span>
    </button>
  )
}
