import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'

export interface ConfirmationDialogProps {
  open: boolean
  title: string
  message: string
  confirmLabel: string
  tone?: 'default' | 'danger'
  busy?: boolean
  onClose: () => void
  onConfirm: () => void
}

export default function ConfirmationDialog({
  open,
  title,
  message,
  confirmLabel,
  tone = 'default',
  busy = false,
  onClose,
  onConfirm,
}: ConfirmationDialogProps) {
  return (
    <AlertDialog open={open} onOpenChange={(v) => { if (!v && !busy) onClose() }}>
      <AlertDialogContent
        className="w-[min(92vw,420px)] bg-[var(--bg-panel)] border-[var(--border)] rounded-[var(--r-lg)] shadow-[var(--shadow-lg)] p-0 gap-0"
      >
        <AlertDialogHeader className="px-4 py-3 border-b border-[var(--border)] flex-row items-center justify-between space-y-0">
          <AlertDialogTitle className="font-semibold text-[0.8125rem] text-[var(--text-1)]">
            {title}
          </AlertDialogTitle>
        </AlertDialogHeader>

        <AlertDialogDescription
          className="px-4 py-4 text-[0.8125rem] text-[var(--text-2)] leading-[1.6]"
        >
          {message}
        </AlertDialogDescription>

        <AlertDialogFooter className="flex-row justify-end gap-2 px-4 pb-4 sm:space-x-0">
          <AlertDialogCancel
            onClick={onClose}
            disabled={busy}
            className="mt-0 sm:mt-0"
          >
            Cancel
          </AlertDialogCancel>
          <AlertDialogAction
            onClick={onConfirm}
            disabled={busy}
            className={
              tone === 'danger'
                ? 'bg-[var(--red)] border-[var(--red)] text-white hover:bg-[var(--red)]/90'
                : undefined
            }
          >
            {confirmLabel}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
