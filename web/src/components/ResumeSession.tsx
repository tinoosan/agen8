import { Popover, PopoverTrigger, PopoverContent } from '@/components/ui/popover'
import { Button } from '@/components/ui/button'
import { Terminal, Copy, ExternalLink } from 'lucide-react'
import { toast } from 'sonner'
import { copyText } from '@/lib/utils'
import { buildSessionResume } from '../lib/sessionResume'

/**
 * ResumeSession — a small "Resume" affordance for a member whose harness session
 * can be reopened. Renders NOTHING when the session isn't resumable (see
 * buildSessionResume / decision dec-ab7c6f66), so callers can drop it in
 * unconditionally.
 *
 * There's no universal resume deep link (Claude Code has none), so the primary
 * action is a copy-the-command popover: `claude --resume <id>` run in the project
 * dir, or `codex resume <id>`. Codex also gets a best-effort "Open in Codex app"
 * link.
 */
export default function ResumeSession({
  harnessKind,
  nativeSessionRef,
  projectRoot,
  label = true,
}: {
  harnessKind: string | undefined
  nativeSessionRef: string | undefined
  projectRoot?: string | null
  /** Show the "Resume" text label next to the icon (false = icon-only). */
  label?: boolean
}) {
  const info = buildSessionResume(harnessKind, nativeSessionRef, projectRoot)
  if (!info) return null

  const handleCopy = async () => {
    try {
      await copyText(info.command)
      toast.success('Resume command copied')
    } catch {
      toast.error('Copy failed')
    }
  }

  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          aria-label={`Resume this ${info.harnessLabel} session`}
          className="h-7 gap-1.5 px-2 text-[var(--text-3)] hover:text-[var(--text-1)]"
        >
          <Terminal size={13} aria-hidden />
          {label && <span className="text-[0.75rem]">Resume</span>}
        </Button>
      </PopoverTrigger>
      <PopoverContent
        align="end"
        className="w-[320px] rounded-[var(--r-md)] border border-[var(--border)] bg-[var(--bg-panel)] p-3 shadow-[var(--shadow-lg)]"
      >
        <div className="mb-2 flex items-center justify-between gap-2">
          <span className="text-[0.8125rem] font-semibold text-[var(--text-1)]">
            Resume in {info.harnessLabel}
          </span>
          <Button type="button" variant="ghost" size="sm" className="h-6 gap-1 px-1.5" onClick={() => void handleCopy()}>
            <Copy size={12} />
            <span className="text-[0.6875rem]">Copy</span>
          </Button>
        </div>
        <code className="block w-full overflow-x-auto whitespace-pre rounded-[var(--r-sm)] bg-[var(--bg-app)] px-2.5 py-2 text-[0.71875rem] leading-relaxed text-[var(--text-1)]">
          {info.command}
        </code>
        {info.cwdNote && (
          <p className="m-0 mt-2 text-[0.6875rem] leading-relaxed text-[var(--text-3)]">
            {info.cwdNote}
          </p>
        )}
        {info.appDeepLink && (
          <a
            href={info.appDeepLink}
            className="mt-2 inline-flex items-center gap-1 text-[0.6875rem] text-[var(--accent)] no-underline hover:underline"
          >
            <ExternalLink size={11} aria-hidden />
            Open in the Codex app
          </a>
        )}
      </PopoverContent>
    </Popover>
  )
}
