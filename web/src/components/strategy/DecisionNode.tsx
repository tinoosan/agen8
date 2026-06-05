import { memo } from 'react'
import { Handle, Position, type NodeProps } from '@xyflow/react'
import { cn } from '@/lib/utils'
import { Diamond } from 'lucide-react'
import type { DecisionView, DecisionSource } from '../../lib/types'
import { useStrategyMapStore } from './strategyMapStore'

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
  needsInput: boolean
}

function confidenceToColor(value: number): string {
  if (value >= 0.8) return 'var(--green)'
  if (value >= 0.6) return 'var(--amber)'
  return 'var(--red)'
}

function hasAnswer(decision: DecisionView, questionId: string): boolean {
  return (decision.answers ?? []).some((answer) => {
    if (answer.questionId !== questionId) return false
    return Boolean(answer.selectedOption?.trim() || answer.freeFormText?.trim())
  })
}

function relationCount(decision: DecisionView): number {
  return [
    decision.taskRef,
    decision.keyResultRef,
    decision.missionRef,
    decision.planRef,
    decision.correlationRef,
    decision.informedByRef,
  ].filter(Boolean).length
}

function decisionMeta(decision: DecisionView): DecisionNodeMeta {
  const kind = (decision.kind ?? '').trim().toLowerCase()
  const blockingQuestion = (decision.questions ?? []).some((question) =>
    question.blocking && !hasAnswer(decision, question.id),
  )
  const unansweredQuestion = (decision.questions ?? []).some((question) => !hasAnswer(decision, question.id))
  const needsInput = kind === 'ask_user' && (blockingQuestion || unansweredQuestion)
  const links = relationCount(decision)
  const confidence = decision.confidence > 0 ? `${Math.round(decision.confidence * 100)}%` : null

  const confColor = decision.confidence > 0 ? confidenceToColor(decision.confidence) : null

  if (decision.cancelled) {
    return {
      label: 'Cancelled',
      tone: 'var(--text-3)',
      sourceLabel: sourceLabel(decision),
      confidenceLabel: confidence,
      confidenceColor: confColor,
      linkLabel: links > 0 ? `${links} link${links === 1 ? '' : 's'}` : null,
      needsInput: false,
    }
  }
  if (needsInput) {
    return {
      label: blockingQuestion ? 'Input needed' : 'Question',
      tone: 'var(--amber)',
      sourceLabel: sourceLabel(decision),
      confidenceLabel: confidence,
      confidenceColor: confColor,
      linkLabel: links > 0 ? `${links} link${links === 1 ? '' : 's'}` : null,
      needsInput: true,
    }
  }
  return {
    label: kind === 'ask_user' ? 'Answered' : 'Decision',
    tone: SOURCE_COLOR[decision.source] ?? 'var(--border-strong)',
    sourceLabel: sourceLabel(decision),
    confidenceLabel: confidence,
    confidenceColor: confColor,
    linkLabel: links > 0 ? `${links} link${links === 1 ? '' : 's'}` : null,
    needsInput: false,
  }
}

function sourceLabel(decision: DecisionView): string {
  const name = decision.memberName?.trim()
  if (name) return name
  const actor = decision.sourceIdentity?.trim()
  if (actor) return actor
  return decision.source ? decision.source.replaceAll('_', ' ') : 'decision'
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
  const isActiveFromStore = useStrategyMapStore((s) => s.selectedNodeId === id)
  const isActive = selected || isActiveFromStore
  const isTraced = useStrategyMapStore((s) => s.activeFilter === 'trace') && !isDimmed
  const showNebula = isActive || isTraced
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
            background: meta.needsInput ? meta.tone : color,
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
            style={{ fontSize: '11px', lineHeight: '14px', maxWidth: 170 }}>
            {decision.title}
          </span>
          <span className="flex items-center gap-1 text-[9.5px] font-medium leading-none text-muted-foreground">
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
