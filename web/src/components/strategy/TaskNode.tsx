import { memo } from 'react'
import { Handle, Position, type NodeProps } from '@xyflow/react'
import { cn } from '@/lib/utils'
import type { Task } from '../../lib/types'
import { taskAssignedMemberLabel } from '../../lib/taskMembers'
import { useStrategyMapStore } from './strategyMapStore'

export interface TaskNodeData {
  task: Task
  clusterColor?: string
  [key: string]: unknown
}

interface StatusInfo {
  label: string
  tone: string
  progress: number // 0–1 ring fill
  centerGlyph: 'none' | 'check' | 'alert' | 'pause' | 'cross'
}

const STATUS_META: Record<string, StatusInfo> = {
  pending:        { label: 'Queued',    tone: 'var(--text-3)',                  progress: 0,    centerGlyph: 'pause' },
  active:         { label: 'Working',   tone: 'var(--accent)',                  progress: 0.45, centerGlyph: 'none' },
  delegated:      { label: 'Delegated', tone: 'var(--accent)',                  progress: 0.45, centerGlyph: 'none' },
  resumed:        { label: 'Working',   tone: 'var(--accent)',                  progress: 0.45, centerGlyph: 'none' },
  blocked:        { label: 'Blocked',   tone: 'var(--amber)',                   progress: 0.3,  centerGlyph: 'alert' },
  in_review: { label: 'Review',    tone: 'var(--purple, var(--accent))',    progress: 0.8,  centerGlyph: 'none' },
  succeeded:      { label: 'Done',      tone: 'var(--green)',                   progress: 1,    centerGlyph: 'check' },
  done:           { label: 'Done',      tone: 'var(--green)',                   progress: 1,    centerGlyph: 'check' },
  complete:       { label: 'Done',      tone: 'var(--green)',                   progress: 1,    centerGlyph: 'check' },
  failed:         { label: 'Failed',    tone: 'var(--red)',                     progress: 1,    centerGlyph: 'alert' },
  canceled:       { label: 'Canceled',  tone: 'var(--red)',                     progress: 0,    centerGlyph: 'cross' },
  suspended:      { label: 'Paused',    tone: 'var(--text-3)',                  progress: 0,    centerGlyph: 'pause' },
}

const RING_SIZE = 18
const RING_R = 7
const RING_C = 2 * Math.PI * RING_R // circumference ≈ 44

function ProgressRing({ progress, tone, glyph, active }: {
  progress: number; tone: string; glyph: StatusInfo['centerGlyph']; active: boolean
}) {
  const offset = RING_C * (1 - progress)
  return (
    <svg width={RING_SIZE} height={RING_SIZE} viewBox={`0 0 ${RING_SIZE} ${RING_SIZE}`}
      style={{ filter: active ? `drop-shadow(0 0 4px ${tone})` : undefined }}>
      {/* Track */}
      <circle cx={RING_SIZE / 2} cy={RING_SIZE / 2} r={RING_R}
        fill="none" stroke="currentColor" strokeWidth={2} opacity={0.3} />
      {/* Fill */}
      {progress > 0 && (
        <circle cx={RING_SIZE / 2} cy={RING_SIZE / 2} r={RING_R}
          fill="none" stroke={tone} strokeWidth={2}
          strokeDasharray={RING_C} strokeDashoffset={offset}
          strokeLinecap="round"
          transform={`rotate(-90 ${RING_SIZE / 2} ${RING_SIZE / 2})`}
          style={{ transition: 'stroke-dashoffset 0.4s ease' }} />
      )}
      {/* Center glyph */}
      {glyph === 'check' && (
        <polyline points="6.5,9.5 8.5,11.5 12,7.5" fill="none" stroke={tone}
          strokeWidth={1.5} strokeLinecap="round" strokeLinejoin="round" />
      )}
      {glyph === 'alert' && (
        <>
          <line x1={RING_SIZE / 2} y1={6} x2={RING_SIZE / 2} y2={10}
            stroke={tone} strokeWidth={1.5} strokeLinecap="round" />
          <circle cx={RING_SIZE / 2} cy={12.5} r={0.8} fill={tone} />
        </>
      )}
      {glyph === 'pause' && (
        <>
          <line x1={7} y1={6.5} x2={7} y2={11.5} stroke={tone} strokeWidth={1.5} strokeLinecap="round" />
          <line x1={11} y1={6.5} x2={11} y2={11.5} stroke={tone} strokeWidth={1.5} strokeLinecap="round" />
        </>
      )}
      {glyph === 'cross' && (
        <>
          <line x1={6.5} y1={6.5} x2={11.5} y2={11.5} stroke={tone} strokeWidth={1.5} strokeLinecap="round" />
          <line x1={11.5} y1={6.5} x2={6.5} y2={11.5} stroke={tone} strokeWidth={1.5} strokeLinecap="round" />
        </>
      )}
    </svg>
  )
}

function taskOwnerLabel(task: Task): string | null {
  return taskAssignedMemberLabel(task) ?? null
}

export const TaskNode = memo(function TaskNode({ data, selected, id }: NodeProps) {
  const d = data as unknown as TaskNodeData
  const { task } = d
  const color = d.clusterColor ?? 'var(--border-strong)'
  const status = task.status ?? 'pending'
  const statusMeta = STATUS_META[status] ?? { label: status.replaceAll('_', ' '), tone: 'var(--text-3)', progress: 0, centerGlyph: 'none' as const }
  const ownerLabel = taskOwnerLabel(task)
  const leafPhase = useStrategyMapStore((s) => s.leafPhase)
  const isInteracting = useStrategyMapStore((s) => s.isInteracting)
  const isDimmed = useStrategyMapStore((s) =>
    s.clusterNodeIds ? !s.clusterNodeIds.has(id) : false,
  )
  const isActiveFromStore = useStrategyMapStore((s) => s.selectedNodeId === id)
  const isActive = selected || isActiveFromStore
  const isTraced = useStrategyMapStore((s) => s.activeFilter === 'trace') && !isDimmed
  const showNebula = isActive || isTraced
  const showDot = !isActive && leafPhase === 'dot'
  const showFull = isActive || leafPhase !== 'dot'
  const isEntering = !isActive && leafPhase === 'toFull'
  const isExiting = !isActive && leafPhase === 'toDot'

  if (showDot) {
    // Dot-mode glyph: circle distinguishes tasks from decisions.
    // Failed/blocked tasks break cluster colour and go red at macro zoom.
    const dotColor =
      status === 'failed' || status === 'blocked' ? 'var(--red)' : color
    return (
      <div className="cursor-pointer select-none transition-all duration-300 hover:scale-[1.5] flex items-center justify-center"
        style={{ width: 14, height: 14, opacity: isDimmed ? 0.15 : 1 }}>
        <Handle type="target" position={Position.Top}
          style={{ left: '50%', top: '50%', opacity: 0, pointerEvents: 'none' }} />
        <Handle type="source" position={Position.Bottom}
          style={{ left: '50%', top: '50%', opacity: 0, pointerEvents: 'none' }} />
        <div
          className="rounded-full"
          style={{ width: 11, height: 11, background: dotColor, opacity: 0.9 }}
        />
      </div>
    )
  }
  if (!showFull) return null

  return (
    <div
      style={{
        opacity: isExiting ? 0 : 1,
        transform: isExiting ? 'scale(0.3)' : 'scale(1)',
        transition: isExiting
          ? 'opacity 120ms ease-out, transform 120ms ease-out'
          : 'opacity 200ms ease-out, transform 200ms ease-out',
        animation: isEntering ? 'leaf-node-enter 250ms ease-out' : undefined,
        pointerEvents: isExiting ? 'none' : undefined,
      }}
    >
      {showNebula && (
        <div className="absolute left-1/2 top-1/2 z-[-1] pointer-events-none">
          <div
            className="rounded-full heat-nebula"
            style={{
              width: '1px', height: '1px',
              boxShadow: `0 0 ${isInteracting ? 94 : 100}px ${isInteracting ? 66 : 70}px ${color}`,
              opacity: isActive ? 0.22 : isInteracting ? 0.045 : 0.05,
              transition: 'opacity 0.6s ease'
            }}
          />
        </div>
      )}
      <div className={cn(
        'relative cursor-pointer select-none flex flex-col items-center',
        'group transition-all duration-200 hover:-translate-y-0.5',
        isActive ? 'z-50' : '',
      )} style={{
        padding: '3px 8px 4px',
        borderRadius: 6,
        background: 'var(--bg-panel)',
        transition: 'background 0.2s ease',
      }}>
        {/* Hover wash */}
        <div className="absolute inset-0 rounded-[6px] opacity-0 group-hover:opacity-100 transition-opacity duration-200 pointer-events-none"
          style={{ background: `color-mix(in srgb, ${statusMeta.tone} 5%, transparent)` }} />
      <Handle type="target" position={Position.Top}
        style={{ left: '50%', top: '50%', opacity: 0, pointerEvents: 'none' }} />
      <Handle type="source" position={Position.Bottom}
        style={{ left: '50%', top: '50%', opacity: 0, pointerEvents: 'none' }} />

      <div className="flex flex-col items-center transition-opacity duration-300"
        style={{ opacity: isDimmed ? 0.15 : 1 }}>
        {/* SVG progress ring */}
        <div style={{ color: statusMeta.tone, opacity: isActive ? 1 : 0.75 }}>
          <ProgressRing progress={statusMeta.progress} tone={statusMeta.tone}
            glyph={statusMeta.centerGlyph} active={isActive} />
        </div>
        <div className="flex flex-col items-center gap-[2px]">
          <span className="text-foreground truncate text-center font-medium transition-opacity duration-200"
            style={{ fontSize: '0.6875rem', lineHeight: '14px', maxWidth: 170 }}>
            {task.title ?? task.description}
          </span>
          <span className="flex items-center gap-1 text-[0.59375rem] font-medium leading-none text-muted-foreground">
            <span className="shrink-0" style={{ color: statusMeta.tone }}>{statusMeta.label}</span>
            {ownerLabel && (
              <>
                <span className="shrink-0 text-muted-foreground/50">·</span>
                <span className="truncate">{ownerLabel}</span>
              </>
            )}
          </span>
        </div>
      </div>
    </div>
    </div>
  )
})

TaskNode.displayName = 'TaskNode'
