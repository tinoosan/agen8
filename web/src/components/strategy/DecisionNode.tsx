import { memo } from 'react'
import { Handle, Position, type NodeProps } from '@xyflow/react'
import { cn } from '@/lib/utils'
import { Diamond } from 'lucide-react'
import type { DecisionView, DecisionSource } from '../../lib/types'
import { useStrategyMapStore } from './strategyMapStore'
import { confidenceColor, decisionActorDisplay } from '../../lib/decisionDisplay'

export interface DecisionNodeData {
  decision: DecisionView
  clusterColor?: string
  [key: string]: unknown
}

const SOURCE_COLOR: Record<DecisionSource, string> = {
  agent: 'var(--blue)',
}

type DecisionNodeMeta = {
  label: string
  tone: string
  sourceLabel: string
  confidenceLabel: string | null
  confidenceColor: string | null
  linkLabel: string | null
}

function relationCount(decision: DecisionView): number {
  return [
    decision.taskRef,
    decision.keyResultRef,
    decision.missionRef,
    decision.correlationRef,
    decision.informedByRef,
  ].filter(Boolean).length
}

function decisionMeta(decision: DecisionView): DecisionNodeMeta {
  const links = relationCount(decision)
  const confidence = decision.confidence > 0 ? `${Math.round(decision.confidence * 100)}%` : null

  const confColor = decision.confidence > 0 ? confidenceColor(decision.confidence) : null

  return {
    label: 'Decision',
    tone: SOURCE_COLOR[decision.source] ?? 'var(--border-strong)',
    sourceLabel: sourceLabel(decision),
    confidenceLabel: confidence,
    confidenceColor: confColor,
    linkLabel: links > 0 ? `${links} link${links === 1 ? '' : 's'}` : null,
  }
}

function sourceLabel(decision: DecisionView): string {
  return decisionActorDisplay(decision).label || 'decision'
}

export const DecisionNode = memo(function DecisionNode({ data, selected, id }: NodeProps) {
  const d = data as unknown as DecisionNodeData
  const { decision } = d
  const meta = decisionMeta(decision)
  const color = d.clusterColor ?? meta.tone
  const leafPhase = useStrategyMapStore((s) => s.leafPhase)
  const isInteracting = useStrategyMapStore((s) => s.isInteracting)
  const isDimmed = useStrategyMapStore((s) =>
    s.clusterNodeIds ? !s.clusterNodeIds.has(id) : false,
  )
  // Active glow follows the traversal cursor (focusNodeId), not the panel
  // (selectedNodeId), so the focused node stays lit while the panel is closed.
  const isActiveFromStore = useStrategyMapStore((s) => s.focusNodeId === id)
  const isActive = selected || isActiveFromStore
  const showNebula = isActive
  const showDot = !isActive && leafPhase === 'dot'
  const showFull = isActive || leafPhase !== 'dot'
  const isEntering = !isActive && leafPhase === 'toFull'
  const isExiting = !isActive && leafPhase === 'toDot'

  if (showDot) {
    return (
      <div className="cursor-pointer select-none transition-all duration-300 hover:scale-[1.5] flex items-center justify-center"
        style={{ width: 14, height: 14, opacity: isDimmed ? 0.15 : 1 }}>
        <Handle type="target" position={Position.Top}
          style={{ left: '50%', top: '50%', opacity: 0, pointerEvents: 'none' }} />
        <Handle type="source" position={Position.Bottom}
          style={{ left: '50%', top: '50%', opacity: 0, pointerEvents: 'none' }} />
        <div
          style={{
            width: 9,
            height: 9,
            background: color,
            opacity: 0.9,
            transform: 'rotate(45deg)',
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
        {/* Hover wash — fades in on hover, no hard border */}
        <div className="absolute inset-0 rounded-[6px] opacity-0 group-hover:opacity-100 transition-opacity duration-200 pointer-events-none"
          style={{ background: `color-mix(in srgb, ${meta.tone} 5%, transparent)` }} />
      <Handle type="target" position={Position.Top}
        style={{ left: '50%', top: '50%', opacity: 0, pointerEvents: 'none' }} />
      <Handle type="source" position={Position.Bottom}
        style={{ left: '50%', top: '50%', opacity: 0, pointerEvents: 'none' }} />

      {/* Content wrapper — dims when unfocused, background stays opaque to hide edges */}
      <div className="flex flex-col items-center transition-opacity duration-300"
        style={{ opacity: isDimmed ? 0.15 : 1 }}>
        <Diamond size={14} className="transition-all duration-200"
          style={{
            color,
            opacity: isActive ? 1 : 0.75,
            filter: isActive ? `drop-shadow(0 0 4px ${color})` : undefined,
          }}
          fill={isActive ? color : 'none'}
          strokeWidth={isActive ? 1.5 : 2}
        />
        <div className="flex flex-col items-center gap-[2px]">
          <span className="text-foreground truncate text-center font-medium transition-opacity duration-200"
            style={{ fontSize: '0.6875rem', lineHeight: '14px', maxWidth: 170 }}>
            {decision.title}
          </span>
          <span className="flex items-center gap-1 text-[0.59375rem] font-medium leading-none text-muted-foreground">
            <span className="shrink-0" style={{ color: meta.tone }}>{meta.label}</span>
            <span className="shrink-0 text-muted-foreground/50">·</span>
            <span className="truncate capitalize">{meta.sourceLabel}</span>
            {(meta.confidenceLabel || meta.linkLabel) && (
              <>
                <span className="shrink-0 text-muted-foreground/50">·</span>
                <span className="shrink-0" style={meta.confidenceLabel && meta.confidenceColor ? { color: meta.confidenceColor } : undefined}>
                  {meta.confidenceLabel ?? meta.linkLabel}
                </span>
              </>
            )}
          </span>
        </div>
      </div>
    </div>
    </div>
  )
})

DecisionNode.displayName = 'DecisionNode'
