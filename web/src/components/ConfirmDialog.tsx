import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
} from '@/components/ui/alert-dialog'

export function ConfirmDialog({ message, onConfirm, onCancel }: {
  message: string
  onConfirm: () => void
  onCancel: () => void
}) {
  return (
    <AlertDialog open={true} onOpenChange={(v) => { if (!v) onCancel() }}>
      <AlertDialogContent
        className="bg-[var(--bg-panel)] border-[var(--border)] rounded-[var(--r-lg)] shadow-[var(--shadow-lg)] p-5 max-w-sm w-[90%]"
      >
        <AlertDialogDescription className="text-[13px] text-[var(--text-1)] m-0">
          {message}
        </AlertDialogDescription>
        <AlertDialogFooter className="flex-row justify-end gap-2 sm:space-x-0">
          <AlertDialogCancel
            onClick={onCancel}
            className="mt-0 sm:mt-0"
          >
            Cancel
          </AlertDialogCancel>
          <AlertDialogAction
            onClick={onConfirm}
          >
            Continue
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
