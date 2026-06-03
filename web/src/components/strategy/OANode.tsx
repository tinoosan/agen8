import { memo } from 'react'
import { Handle, Position, type NodeProps } from '@xyflow/react'
import { cn } from '@/lib/utils'
import { UserCog } from 'lucide-react'
import type { OpActionView } from '../../lib/types'
import { useStrategyMapStore } from './strategyMapStore'

export interface OANodeData {
  oa: OpActionView
  clusterColor?: string
  [key: string]: unknown
}

interface OAStatusInfo {
  label: string
  tone: string
}

const STATUS_META: Record<string, OAStatusInfo> = {
  pending:              { label: 'Pending',      tone: 'var(--amber)' },
  acknowledged:         { label: 'Acknowledged',  tone: 'var(--accent)' },
  in_progress:          { label: 'In Progress',   tone: 'var(--accent)' },
  pending_verification: { label: 'Verifying',     tone: 'var(--purple, var(--accent))' },
  completed:            { label: 'Done',           tone: 'var(--green)' },
}


export const OANode = memo(function OANode({ data, selected, id }: NodeProps) {
  const d = data as unknown as OANodeData
  const { oa } = d
  const color = d.clusterColor ?? 'var(--amber)'
  const status = oa.status ?? 'pending'
  const statusMeta = STATUS_META[status] ?? { label: status.replaceAll('_', ' '), tone: 'var(--amber)' }
  const isCompleted = status === 'completed'
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
    // Dot-mode glyph: square distinguishes operator actions from tasks
    // (circle), decisions (diamond), and escalations (triangle). Blocking
    // OAs break cluster colour and go red so they stand out at macro zoom —
    // these are the ones holding up other work.
    const dotColor = oa.blocking ? 'var(--red)' : color
    return (
      <div className="cursor-pointer select-none transition-all duration-300 hover:scale-[1.5] flex items-center justify-center"
        style={{ width: 14, height: 14, opacity: isDimmed ? 0.15 : 1 }}>
        <Handle type="target" position={Position.Top}
          style={{ left: '50%', top: '50%', opacity: 0, pointerEvents: 'none' }} />
        <Handle type="source" position={Position.Bottom}
          style={{ left: '50%', top: '50%', opacity: 0, pointerEvents: 'none' }} />
        <div
          style={{ width: 10, height: 10, background: dotColor, opacity: 0.9 }}
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
        'relative cursor-pointer select-none',
        'group transition-all duration-200 hover:-translate-y-0.5',
        isActive ? 'z-50' : '',
      )}>
      <Handle type="target" position={Position.Top}
        style={{ left: '50%', top: '50%', opacity: 0, pointerEvents: 'none' }} />
      <Handle type="source" position={Position.Bottom}
        style={{ left: '50%', top: '50%', opacity: 0, pointerEvents: 'none' }} />

      {/* Speech bubble SVG container */}
      <svg width={190} height={68} viewBox="0 0 190 68" className="overflow-visible">
        {/* Bubble body + tail */}
        <path
          d={`M8,0 L182,0 Q190,0 190,8 L190,48 Q190,56 182,56 L168,56 L175,66 L158,56 L8,56 Q0,56 0,48 L0,8 Q0,0 8,0 Z`}
          fill="var(--bg-panel)"
          stroke={isActive ? color : 'none'}
          strokeWidth={isActive ? 1.5 : 0}
          className="transition-all duration-200"
          style={{ filter: isActive ? `drop-shadow(0 0 12px color-mix(in srgb, ${color} 30%, transparent))` : undefined }}
        />
        {/* Hover wash */}
        <path
          d={`M8,0 L182,0 Q190,0 190,8 L190,48 Q190,56 182,56 L168,56 L175,66 L158,56 L8,56 Q0,56 0,48 L0,8 Q0,0 8,0 Z`}
          fill={statusMeta.tone} opacity={0}
          className="transition-opacity duration-200 group-hover:opacity-[0.05]"
        />
        {/* Content */}
        <foreignObject x={8} y={2} width={174} height={52}>
          <div className="flex flex-col items-center gap-[1px] h-full justify-center transition-opacity duration-300"
            style={{ opacity: isDimmed ? 0.15 : 1 }}>
            <UserCog size={16} style={{ color: statusMeta.tone, opacity: isActive ? 1 : 0.75 }} />
            <span className="text-foreground truncate text-center font-medium block w-full"
              style={{ fontSize: '11px', lineHeight: '14px', maxWidth: 160 }}>
              {oa.title}
            </span>
            <span className="flex items-center gap-1 text-[9.5px] font-medium leading-none text-muted-foreground">
              <span className="shrink-0" style={{ color: statusMeta.tone }}>{statusMeta.label}</span>
              {oa.blocking && !isCompleted && (
                <>
                  <span className="shrink-0 text-muted-foreground/50">·</span>
                  <span className="shrink-0" style={{ color: 'var(--red)' }}>Blocking</span>
                </>
              )}
              {oa.sourceMemberLabel && (
                <>
                  <span className="shrink-0 text-muted-foreground/50">·</span>
                  <span className="truncate capitalize">{oa.sourceMemberLabel.replaceAll('_', ' ')}</span>
                </>
              )}
            </span>
          </div>
        </foreignObject>
      </svg>
    </div>
    </div>
  )
})

OANode.displayName = 'OANode'
