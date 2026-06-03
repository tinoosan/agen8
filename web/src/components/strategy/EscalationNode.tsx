import { memo } from 'react'
import { Handle, Position, type NodeProps } from '@xyflow/react'
import { cn } from '@/lib/utils'
import { AlertTriangle } from 'lucide-react'
import type { EscalationView, OperatorUrgency } from '../../lib/types'
import { useStrategyMapStore } from './strategyMapStore'

export interface EscalationNodeData {
  escalation: EscalationView
  clusterColor?: string
  [key: string]: unknown
}

const URGENCY_COLOR: Record<OperatorUrgency, string> = {
  low: 'var(--green)',
  medium: 'var(--amber)',
  high: 'var(--amber)',
  critical: 'var(--red)',
}

export const EscalationNode = memo(function EscalationNode({ data, selected, id }: NodeProps) {
  const d = data as unknown as EscalationNodeData
  const { escalation } = d
  const color = d.clusterColor ?? 'var(--amber)'
  const urgency = (escalation.urgency as OperatorUrgency) ?? 'low'
  const iconColor = urgency === 'low' ? color : (URGENCY_COLOR[urgency] ?? color)
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
    // Dot-mode glyph: triangle distinguishes escalations from tasks (circle),
    // decisions (diamond), and operator actions (square). Critical urgency
    // breaks cluster colour and goes red so crises stand out at macro zoom;
    // high urgency goes amber; low/medium stay cluster-coloured.
    //
    // Clip-path uses (50% 0%, 7% 75%, 93% 75%) instead of the usual
    // (50% 0%, 0% 100%, 100% 100%) so the triangle's CENTROID lands at
    // (50%, 50%) of the container — otherwise the visual centroid sits
    // ~17% below the bounding-box center and edges appear to miss the
    // triangle tip. See https://en.wikipedia.org/wiki/Centroid#Triangle
    const dotColor =
      urgency === 'critical' ? 'var(--red)' :
      urgency === 'high' ? 'var(--amber)' :
      color
    return (
      <div className="cursor-pointer select-none transition-all duration-300 hover:scale-[1.5] flex items-center justify-center"
        style={{ width: 14, height: 14, opacity: isDimmed ? 0.15 : 1 }}>
        <Handle type="target" position={Position.Top}
          style={{ left: '50%', top: '50%', opacity: 0, pointerEvents: 'none' }} />
        <Handle type="source" position={Position.Bottom}
          style={{ left: '50%', top: '50%', opacity: 0, pointerEvents: 'none' }} />
        <div
          style={{
            width: 14,
            height: 14,
            background: dotColor,
            opacity: 0.9,
            clipPath: 'polygon(50% 0%, 7% 75%, 93% 75%)',
          }}
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
          style={{ background: `color-mix(in srgb, ${iconColor} 5%, transparent)` }} />
      <Handle type="target" position={Position.Top}
        style={{ left: '50%', top: '50%', opacity: 0, pointerEvents: 'none' }} />
      <Handle type="source" position={Position.Bottom}
        style={{ left: '50%', top: '50%', opacity: 0, pointerEvents: 'none' }} />

      <div className="flex flex-col items-center transition-opacity duration-300"
        style={{ opacity: isDimmed ? 0.15 : 1 }}>
        <AlertTriangle size={16} className="transition-all duration-200"
          style={{
            color: iconColor,
            opacity: isActive ? 1 : 0.75,
            filter: isActive ? `drop-shadow(0 0 4px ${iconColor})` : undefined,
          }} />
        <div className="flex flex-col items-center gap-[2px]">
          <span className="text-foreground truncate text-center font-medium transition-opacity duration-200"
            style={{ fontSize: '11px', lineHeight: '14px', maxWidth: 170 }}>
            {escalation.title}
          </span>
          <span className="flex items-center gap-1 text-[9.5px] font-medium leading-none text-muted-foreground">
            <span className="shrink-0 capitalize" style={{ color: iconColor }}>{escalation.status}</span>
            <span className="shrink-0 text-muted-foreground/50">·</span>
            <span className="shrink-0 capitalize" style={{ color: URGENCY_COLOR[urgency] }}>{urgency}</span>
          </span>
        </div>
      </div>
    </div>
    </div>
  )
})

EscalationNode.displayName = 'EscalationNode'
