import { Check, Copy, Terminal } from 'lucide-react'
import { toast } from 'sonner'
import { copyText } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'

export default function ClientSetupDialog({
  command,
  projectName,
  onDone,
}: {
  command: string
  projectName: string
  onDone: () => void
}) {
  const copyCommand = async () => {
    try {
      await copyText(command)
      toast.success('Setup command copied')
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'Could not copy setup command')
    }
  }

  return (
    <Dialog open onOpenChange={(open) => { if (!open) onDone() }}>
      <DialogContent className="max-w-[min(92vw,620px)] gap-0 overflow-hidden rounded-[var(--r-lg)] border-[var(--border)] bg-[var(--bg-panel)] p-0">
        <DialogHeader className="border-b border-[var(--border)] px-5 py-4 text-left">
          <DialogTitle className="flex items-center gap-2 text-[0.9375rem]">
            <Terminal size={15} className="text-[var(--accent)]" />
            Finish Claude setup
          </DialogTitle>
          <DialogDescription className="text-[0.75rem] leading-5 text-[var(--text-3)]">
            Run this once from the local folder for {projectName}. It installs Agen8 skills, attention hooks, and the project-local Claude MCP connection.
          </DialogDescription>
        </DialogHeader>
        <div className="px-5 py-4">
          <pre className="max-h-[220px] overflow-auto whitespace-pre-wrap break-all rounded-[var(--r-sm)] border border-[var(--border)] bg-[var(--bg-surface)] p-3 font-mono text-[0.75rem] leading-5 text-[var(--text-1)]">
            {command}
          </pre>
          <p className="mt-2 text-[0.6875rem] leading-4 text-[var(--text-3)]">
            This credential is shown once. Closing this dialog removes it from the page.
          </p>
        </div>
        <DialogFooter className="flex-row justify-end gap-2 border-t border-[var(--border)] px-5 py-3">
          <Button type="button" variant="outline" onClick={() => { void copyCommand() }}>
            <Copy size={13} />
            Copy command
          </Button>
          <Button type="button" onClick={onDone}>
            <Check size={13} />
            Done
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
