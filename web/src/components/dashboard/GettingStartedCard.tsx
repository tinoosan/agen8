import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { Check, Copy, Rocket, X } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { cn, copyText } from '@/lib/utils'
import { createAPIKey } from '../../lib/authClient'
import {
  buildMCPSetup,
  CLAUDE_SKILL_COMMAND,
  CODEX_SKILL_COMMAND,
  type MCPSetupSnippets,
} from '../../lib/mcpSetup'
import {
  deriveGettingStarted,
  readGettingStartedDismissed,
  writeGettingStartedDismissed,
} from '../../lib/gettingStarted'
import { qk } from '../../lib/queryKeys'
import { useMissions } from '../../hooks/useMissions'
import { useProjectMembers } from '../../hooks/useProjectMembers'
import { useProjectTasks } from '../../hooks/useProjectTasks'

/**
 * GettingStartedCard — the fresh-project checklist (mission-1d7062c6).
 *
 * Steps self-tick from live project state (members/missions/tasks queries,
 * already SSE-invalidated), so the payoff moment — "I ran `claude` in my
 * terminal and the dashboard reacted" — needs no reload. The card renders
 * nothing until all three queries have resolved, so an established project
 * never flashes it, and it disappears for good once complete or dismissed.
 *
 * Hooks are deliberately NOT a step: the daemon installs them when the project
 * is created, so they appear only as a pre-ticked footnote.
 */

type Harness = 'claude' | 'codex'

export default function GettingStartedCard({ projectId }: { projectId: string | null }) {
  const membersQuery = useProjectMembers(projectId)
  const missionsQuery = useMissions(projectId)
  const tasksQuery = useProjectTasks(projectId)

  const [dismissed, setDismissed] = useState(() =>
    projectId ? readGettingStartedDismissed(projectId) : false,
  )
  const [harness, setHarness] = useState<Harness>('claude')
  const [snippets, setSnippets] = useState<MCPSetupSnippets | null>(null)
  const [generating, setGenerating] = useState(false)
  const queryClient = useQueryClient()

  if (!projectId || dismissed) return null
  if (!membersQuery.isSuccess || !missionsQuery.isSuccess || !tasksQuery.isSuccess) return null

  const state = deriveGettingStarted({
    memberCount: (membersQuery.data ?? []).length,
    missionCount: (missionsQuery.data ?? []).length,
    taskCount: (tasksQuery.data ?? []).length,
  })
  if (state.complete) return null

  const handleDismiss = () => {
    writeGettingStartedDismissed(projectId)
    setDismissed(true)
  }

  const handleGenerateToken = async () => {
    setGenerating(true)
    try {
      const result = await createAPIKey('Agen8 MCP key')
      setSnippets(buildMCPSetup(result.secret))
      await queryClient.invalidateQueries({ queryKey: qk.apiKeys })
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to generate token')
    } finally {
      setGenerating(false)
    }
  }

  const connectCommand = snippets
    ? harness === 'claude' ? snippets.claudeCommand : snippets.codexCommand
    : null
  const skillCommand = harness === 'claude' ? CLAUDE_SKILL_COMMAND : CODEX_SKILL_COMMAND

  return (
    <div className="mb-8">
      <section className="dashboard-section @container" aria-label="Getting started">
        <div className="dashboard-section-heading mb-2">
          <div className="dashboard-section-heading-main">
            <div className="flex items-center gap-2">
              <Rocket size={14} className="text-[var(--accent)]" aria-hidden />
              <span className="dashboard-section-title">Getting started</span>
            </div>
          </div>
          <div className="dashboard-section-meta flex items-center gap-3">
            <HarnessToggle value={harness} onChange={setHarness} />
            <button
              type="button"
              onClick={handleDismiss}
              aria-label="Dismiss getting started checklist"
              className="cursor-pointer border-none bg-transparent p-0.5 text-[var(--text-3)] transition-colors hover:text-[var(--text-1)]"
            >
              <X size={14} />
            </button>
          </div>
        </div>

        <div className="max-w-[720px] rounded-[18px] border border-[var(--border)] bg-[var(--bg-elevated)] px-4 py-3">
          <StepRow index={1} done title="Project created" />

          <StepRow index={2} done={state.done.connect} title="Connect your harness">
            {connectCommand ? (
              <>
                <p className="m-0 mb-1.5 text-[0.75rem] text-[var(--text-3)]">
                  This token is shown once — the command below already includes it.
                </p>
                <CommandLine value={connectCommand} />
              </>
            ) : (
              <div className="flex flex-wrap items-center gap-2">
                <Button type="button" size="sm" onClick={() => void handleGenerateToken()} disabled={generating}>
                  {generating ? 'Generating…' : 'Generate connect command'}
                </Button>
                <span className="text-[0.75rem] text-[var(--text-3)]">
                  Mints an MCP token and builds the command for you.
                </span>
              </div>
            )}
          </StepRow>

          <StepRow index={3} done={state.done.skill} title="Install the agen8 skill">
            {harness === 'claude' ? (
              <p className="m-0 text-[0.75rem] text-[var(--text-3)]">
                Included in the Claude setup command above.
              </p>
            ) : (
              <CommandLine value={skillCommand} />
            )}
          </StepRow>

          <StepRow index={4} done={state.done.agent} title="Start an agent in this project folder">
            <p className="m-0 text-[0.75rem] text-[var(--text-3)]">
              Run {harness === 'claude' ? 'claude' : 'codex'} in this project's directory and ask it to
              register with agen8 — it will appear here the moment it does.
            </p>
          </StepRow>

          <StepRow index={5} done={state.done.work} title="Give it work" last>
            <p className="m-0 text-[0.75rem] text-[var(--text-3)]">
              Ask the agent to set up a mission with a first task. This dashboard fills in as it works.
            </p>
          </StepRow>

          <div className="mt-2 flex items-center gap-1.5 border-t border-[var(--border)] pt-2 text-[0.6875rem] text-[var(--text-3)]">
            <Check size={11} className="text-[var(--green)]" aria-hidden />
            <span>
              {harness === 'claude'
                ? 'The Claude setup command installs project attention hooks.'
                : <>Install attention reporting with <code className="text-[var(--text-2)]">agen8 hooks install</code>.</>}
            </span>
          </div>
        </div>
      </section>
    </div>
  )
}

function HarnessToggle({ value, onChange }: { value: Harness; onChange: (h: Harness) => void }) {
  return (
    <div role="group" aria-label="Harness" className="flex overflow-hidden rounded-full border border-[var(--border)]">
      {(['claude', 'codex'] as const).map((h) => (
        <button
          key={h}
          type="button"
          onClick={() => onChange(h)}
          aria-pressed={value === h}
          className={cn(
            'cursor-pointer border-none px-2.5 py-0.5 text-[0.6875rem] font-medium transition-colors',
            value === h
              ? 'bg-[var(--accent)] text-[var(--bg-app)]'
              : 'bg-transparent text-[var(--text-3)] hover:text-[var(--text-1)]',
          )}
        >
          {h === 'claude' ? 'Claude Code' : 'Codex'}
        </button>
      ))}
    </div>
  )
}

function StepRow({
  index,
  done,
  title,
  last = false,
  children,
}: {
  index: number
  done: boolean
  title: string
  last?: boolean
  children?: React.ReactNode
}) {
  return (
    <div className={cn('flex gap-3 py-2', !last && 'border-b border-[var(--border)]')}>
      <div
        aria-hidden
        className={cn(
          'mt-px flex size-[18px] shrink-0 items-center justify-center rounded-full text-[0.625rem] font-semibold',
          done
            ? 'bg-[color-mix(in_srgb,var(--green)_18%,transparent)] text-[var(--green)]'
            : 'border border-[var(--border)] text-[var(--text-3)]',
        )}
      >
        {done ? <Check size={11} /> : index}
      </div>
      <div className="min-w-0 flex-1">
        <div
          className={cn(
            'text-[0.8125rem] font-medium',
            done ? 'text-[var(--text-3)] line-through decoration-[1px]' : 'text-[var(--text-1)]',
          )}
        >
          {title}
          <span className="sr-only">{done ? ' — done' : ' — to do'}</span>
        </div>
        {!done && children && <div className="mt-1.5">{children}</div>}
      </div>
    </div>
  )
}

function CommandLine({ value }: { value: string }) {
  const handleCopy = async () => {
    try {
      await copyText(value)
      toast.success('Copied')
    } catch {
      toast.error('Copy failed')
    }
  }
  return (
    <div className="flex items-start gap-1.5">
      <code className="block min-w-0 flex-1 overflow-auto whitespace-pre-wrap break-all rounded-[var(--r-sm)] bg-[var(--bg-app)] px-2.5 py-1.5 text-[0.71875rem] leading-relaxed text-[var(--text-1)]">
        {value}
      </code>
      <button
        type="button"
        onClick={() => void handleCopy()}
        aria-label="Copy command"
        className="mt-1 cursor-pointer border-none bg-transparent p-1 text-[var(--text-3)] transition-colors hover:text-[var(--text-1)]"
      >
        <Copy size={13} />
      </button>
    </div>
  )
}
