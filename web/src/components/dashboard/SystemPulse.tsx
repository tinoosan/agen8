import { memo, useCallback, useMemo } from 'react'
import { useCountUp } from '../../hooks/useCountUp'
import { cn } from '@/lib/utils'

type SystemState = 'idle' | 'active' | 'attention'

interface SystemPulseProps {
  spaceCount: number
  activeMissionCount: number
  pendingEscalationCount: number
  pendingOACount: number
  escalationUrgencies: string[]
  focusMode: boolean
}

function deriveState(activeSpaces: number, activeMissions: number, pendingEscalations: number): SystemState {
  if (pendingEscalations > 0) return 'attention'
  if (activeSpaces > 0 || activeMissions > 0) return 'active'
  return 'idle'
}

function pl(n: number, singular: string, plural?: string): string {
  return n === 1 ? singular : (plural ?? `${singular}s`)
}

export default memo(function SystemPulse({
  spaceCount,
  activeMissionCount,
  pendingEscalationCount,
  pendingOACount,
  escalationUrgencies,
  focusMode,
}: SystemPulseProps) {
  const activeSpaces = spaceCount
  const tasksDone: number = 0
  const tasksPending: number = 0

  const state = deriveState(activeSpaces, activeMissionCount, pendingEscalationCount)

  const formatInt = useCallback((n: number) => String(Math.round(n)), [])
  const animatedTasks = useCountUp(tasksDone, { format: formatInt })

  const sentence = useMemo(() => {
    const hasCritical = escalationUrgencies.includes('critical')

    if (pendingEscalationCount > 0) {
      return {
        text: `${pendingEscalationCount} ${pl(pendingEscalationCount, 'escalation')} ${pendingEscalationCount === 1 ? 'needs' : 'need'} you.`,
        className: hasCritical ? 'text-[var(--red)]' : 'text-[var(--amber)]',
      }
    }
    if (activeSpaces > 0) {
      return {
        text: `${activeSpaces} ${pl(activeSpaces, 'space')} ${activeSpaces === 1 ? 'is' : 'are'} moving.`,
        className: 'text-[var(--text-1)]',
      }
    }
    if (activeMissionCount > 0) {
      return {
        text: `${activeMissionCount} active ${pl(activeMissionCount, 'mission')} ${activeMissionCount === 1 ? 'is' : 'are'} moving.`,
        className: 'text-[var(--text-1)]',
      }
    }
    if (tasksPending > 0) {
      return {
        text: `${tasksPending} ${pl(tasksPending, 'task')} ${tasksPending === 1 ? 'is' : 'are'} queued.`,
        className: 'text-[var(--text-2)]',
      }
    }
    if (spaceCount > 0) {
      return {
        text: `All quiet. ${spaceCount} ${pl(spaceCount, 'space')} ${spaceCount === 1 ? 'is' : 'are'} ready.`,
        className: 'text-[var(--text-2)]',
      }
    }
    if (tasksDone > 0) {
      return {
        text: `All quiet. ${tasksDone} ${pl(tasksDone, 'task')} completed.`,
        className: 'text-[var(--text-2)]',
      }
    }
    return {
      text: 'Waiting for your first space to start.',
      className: 'text-[var(--text-3)]',
    }
  }, [activeMissionCount, activeSpaces, pendingEscalationCount, escalationUrgencies, spaceCount, tasksDone, tasksPending])

  const pulseFacts = useMemo(() => {
    const primary = (() => {
      if (activeSpaces > 0) {
        return {
          value: String(activeSpaces),
          label: 'moving',
          className: 'text-[var(--green)]',
          key: 'moving',
        }
      }
      if (activeMissionCount > 0) {
        return {
          value: String(activeMissionCount),
          label: activeMissionCount === 1 ? 'active mission' : 'active missions',
          className: 'text-[var(--green)]',
          key: 'active-missions',
        }
      }
      if (tasksPending > 0) {
        return {
          value: String(tasksPending),
          label: 'queued',
          className: 'text-[var(--amber)]',
          key: 'queued',
        }
      }
      if (spaceCount > 0) {
        return {
          value: String(spaceCount),
          label: spaceCount === 1 ? 'space ready' : 'spaces ready',
          className: 'text-[var(--text-2)]',
          key: 'ready',
        }
      }
      if (tasksDone > 0) {
        return {
          value: animatedTasks,
          label: 'done',
          className: 'text-[var(--text-2)]',
          key: 'done',
        }
      }
      return null
    })()

    const supporting: Array<{ value: string; label: string; className?: string }> = []

    if (pendingEscalationCount > 0 && escalationUrgencies.includes('critical')) {
      supporting.push({
        value: String(pendingEscalationCount),
        label: pendingEscalationCount === 1 ? 'critical blocker' : 'critical blockers',
        className: 'text-[var(--red)]',
      })
    }

    if (tasksDone > 0 && primary?.key !== 'done') {
      supporting.push({ value: animatedTasks, label: 'done', className: 'text-[var(--text-2)]' })
    }
    if (activeMissionCount > 0 && primary?.key !== 'active-missions') {
      supporting.push({
        value: String(activeMissionCount),
        label: activeMissionCount === 1 ? 'active mission' : 'active missions',
        className: 'text-[var(--green)]',
      })
    }
    if (spaceCount > 0 && primary?.key !== 'ready') {
      supporting.push({
        value: String(spaceCount),
        label: spaceCount === 1 ? 'space ready' : 'spaces ready',
        className: 'text-[var(--text-2)]',
      })
    }
    if (!focusMode && pendingOACount > 0) {
      supporting.push({
        value: String(pendingOACount),
        label: pl(pendingOACount, 'action'),
        className: 'text-[var(--accent)]',
      })
    }

    return { primary, supporting }
  }, [
    activeMissionCount,
    activeSpaces,
    animatedTasks,
    escalationUrgencies,
    focusMode,
    pendingEscalationCount,
    pendingOACount,
    spaceCount,
    tasksDone,
    tasksPending,
  ])

  const glowClass =
    state === 'attention'
      ? escalationUrgencies.includes('critical') ? 'system-glow-critical' : 'system-glow-amber'
      : state === 'active' ? 'system-glow-active'
      : 'system-glow-idle'

  return (
    <section className="dashboard-section dashboard-system-pulse relative dash-stagger dash-stagger-1">
      {/* Ambient glow — dashboard's version of strategy map nebulas */}
      <div
        className={cn(
          'dashboard-pulse-glow absolute pointer-events-none rounded-3xl system-glow-transition',
          glowClass,
        )}
      />

      <div className="dashboard-system-pulse-frame relative">
        <p className="dashboard-pulse-kicker m-0 text-[10px] font-semibold uppercase tracking-[0.09em] text-[var(--text-3)]/85">
          Pulse
        </p>

        {/* Status sentence */}
        <h2 className={cn('dashboard-pulse-sentence m-0 mt-1 text-[28px] sm:text-[31px] font-semibold tracking-[-0.04em] leading-[1.12]', sentence.className)}>
          {sentence.text}
        </h2>

        {(pulseFacts.primary || pulseFacts.supporting.length > 0) && (
          <div className="dashboard-pulse-facts flex flex-wrap items-start justify-between gap-x-6 gap-y-2 mt-3.5 pt-2.5 border-t border-[var(--border)]/55">
            {pulseFacts.primary && (
              <div className="dashboard-pulse-primary inline-flex items-baseline gap-2 min-w-0">
                <span className={cn('text-[16px] font-semibold tracking-[-0.03em]', pulseFacts.primary.className)}>
                  {pulseFacts.primary.value}
                </span>
                <span className={cn('text-[12px] font-medium tracking-[-0.01em]', pulseFacts.primary.className)}>
                  {pulseFacts.primary.label}
                </span>
              </div>
            )}

            {pulseFacts.supporting.length > 0 && (
              <div className="dashboard-pulse-supporting flex flex-wrap items-center gap-x-2 gap-y-1.5 text-[11px] text-[var(--text-3)] tabular-nums tracking-[-0.01em]">
                {pulseFacts.supporting.map((item, index) => (
                  <span key={`${item.label}-${index}`} className="inline-flex items-center gap-1.5">
                    <span className={cn('font-medium', item.className)}>{item.value}</span>
                    <span className={item.className}>{item.label}</span>
                    {index < pulseFacts.supporting.length - 1 && <span className="opacity-30">·</span>}
                  </span>
                ))}
              </div>
            )}
          </div>
        )}
      </div>
    </section>
  )
})
