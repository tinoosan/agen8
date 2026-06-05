import { memo, useState } from 'react'
import { BaseEdge, EdgeLabelRenderer, getStraightPath, type EdgeProps } from '@xyflow/react'
import { useStrategyMapStore } from './strategyMapStore'
import { resolveEdgeStrokeColor } from './edgeColors'

export const ContextEdge = memo(function ContextEdge({
  id,
  source,
  target,
  sourceX,
  sourceY,
  targetX,
  targetY,
  data,
}: EdgeProps) {
  // Field-level store subscriptions. Each selector returns a primitive —
  // when focus changes, only edges whose `isDirect`/`isAmbient` flip actually
  // re-render, instead of every edge re-rendering from a single context blob.
  const isInteracting = useStrategyMapStore((s) => s.isInteracting)
  const isDirect = useStrategyMapStore((s) => !!s.directEdgeIds?.has(id))
  const isAmbient = useStrategyMapStore(
    (s) => !!s.clusterEdgeIds?.has(id) && !s.directEdgeIds?.has(id),
  )
  const hasFocus = useStrategyMapStore((s) => s.directEdgeIds != null)
  const focusNodeId = useStrategyMapStore((s) => s.focusNodeId)
  const opacity = !hasFocus ? 0.36 : isDirect ? 0.85 : isAmbient ? 0.5 : 0.28
  const strokeWidth = isDirect ? 2 : 1.5

  // Edge colour inherits from the source node's cluster so context links
  // visually belong to their source's cluster identity. Falls back to
  // neutral gray when the source has no cluster.
  const sourceColor = (data as { sourceColor?: string } | undefined)?.sourceColor
  const targetColor = (data as { targetColor?: string } | undefined)?.targetColor
  const strokeColor = resolveEdgeStrokeColor({
    isDirect,
    isAmbient,
    focusNodeId,
    source,
    target,
    sourceColor,
    targetColor,
  })

  const [path] = getStraightPath({
    sourceX,
    sourceY,
    targetX,
    targetY,
  })

  const [hovered, setHovered] = useState(false)
  const showLabel = hovered || isDirect

  const focusedAtTarget = focusNodeId === target
  const t = 0.5
  const labelX = sourceX + (targetX - sourceX) * t
  const labelY = sourceY + (targetY - sourceY) * t
  const angleDeg = Math.atan2(targetY - sourceY, targetX - sourceX) * (180 / Math.PI)
  const flipped = angleDeg > 90 || angleDeg < -90
  const textAngle = flipped ? angleDeg + 180 : angleDeg
  const pointsToTarget = !flipped

  // Context links: "informed by" when looking toward source, "informs" toward target
  const labelText = focusedAtTarget
    ? `${pointsToTarget ? '←' : '→'} informs`
    : `${pointsToTarget ? '→' : '←'} informed by`
  return (
    <>
      <g
        onMouseEnter={() => setHovered(true)}
        onMouseLeave={() => setHovered(false)}
      >
        <BaseEdge
          id={id}
          path={path}
          interactionWidth={20}
          style={{
            stroke: strokeColor,
            strokeWidth,
            opacity,
            strokeDasharray: isDirect ? '4 4' : '3 6',
            transition: isInteracting
              ? 'none'
              : 'opacity 0.25s ease, stroke-width 0.25s ease, stroke 0.25s ease',
            ...(isDirect && !isInteracting
              ? { animation: 'context-dash-flow 1.5s linear infinite' }
              : {}),
          }}
        />
      </g>
      {showLabel && (
        <EdgeLabelRenderer>
          <div
            className="nodrag nopan pointer-events-none"
            style={{
              position: 'absolute',
              transform: `translate(-50%, -50%) translate(${labelX}px, ${labelY}px) rotate(${textAngle}deg)`,
              background: 'var(--bg-panel)',
              padding: '2px 8px',
              borderRadius: 4,
              fontSize: '0.5625rem',
              fontWeight: 600,
              color: strokeColor,
              letterSpacing: '0.02em',
              opacity: 0.9,
              whiteSpace: 'nowrap',
            }}
          >
            {labelText}
          </div>
        </EdgeLabelRenderer>
      )}
    </>
  )
})

ContextEdge.displayName = 'ContextEdge'
