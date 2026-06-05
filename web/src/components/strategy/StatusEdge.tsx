import { memo, useState } from 'react'
import { BaseEdge, EdgeLabelRenderer, getStraightPath, type EdgeProps } from '@xyflow/react'
import { useStrategyMapStore } from './strategyMapStore'
import { resolveEdgeStrokeColor } from './edgeColors'

// Forward = looking from source toward target (down the hierarchy)
// Reverse = looking from target toward source (up the hierarchy)
function edgeLabelForward(target: string): string {
  if (target.startsWith('task:')) return 'task'
  if (target.startsWith('decision:')) return 'decision'
  return 'key result'  // KR IDs have no prefix
}

function edgeLabelReverse(source: string): string {
  if (source.startsWith('task:')) return 'spawned by'
  if (source.startsWith('decision:')) return 'decided by'
  return 'serves'
}

export const StatusEdge = memo(function StatusEdge({
  id,
  source,
  target,
  sourceX,
  sourceY,
  targetX,
  targetY,
  data,
}: EdgeProps) {
  // Field-level store subscriptions — see ContextEdge for the rationale.
  const isInteracting = useStrategyMapStore((s) => s.isInteracting)
  const isDirect = useStrategyMapStore((s) => !!s.directEdgeIds?.has(id))
  const isAmbient = useStrategyMapStore(
    (s) => !!s.clusterEdgeIds?.has(id) && !s.directEdgeIds?.has(id),
  )
  const hasFocus = useStrategyMapStore((s) => s.directEdgeIds != null)
  const focusNodeId = useStrategyMapStore((s) => s.focusNodeId)
  const isContextEdge = id.startsWith('cl:')
  const opacity = !hasFocus ? (isContextEdge ? 0.38 : 0.46) : isDirect ? 0.85 : isAmbient ? 0.52 : 0.3
  const strokeWidth = isDirect ? 2 : isAmbient ? 1.9 : 1.5

  // Edge colour inherits from the source node's cluster so edges visually
  // belong to their cluster identity. Falls back to a neutral gray for
  // edges whose source has no cluster (shouldn't happen in practice).
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

  // Position label away from the focused node so it's near the destination.
  // If source is focused → 65% (closer to target). Target focused → 35%.
  // Hover with no focus → 50%. Avoids labels hidden behind the focused node.
  const focusedAtSource = focusNodeId === source
  const focusedAtTarget = focusNodeId === target
  const t = 0.5
  const labelX = sourceX + (targetX - sourceX) * t
  const labelY = sourceY + (targetY - sourceY) * t
  const angleDeg = Math.atan2(targetY - sourceY, targetX - sourceX) * (180 / Math.PI)
  const flipped = angleDeg > 90 || angleDeg < -90
  const textAngle = flipped ? angleDeg + 180 : angleDeg

  // Contextual label: changes meaning based on which node is selected
  const pointsToTarget = !flipped
  let labelText: string
  if (focusedAtSource) {
    labelText = `${pointsToTarget ? '→' : '←'} ${edgeLabelForward(target)}`
  } else if (focusedAtTarget) {
    labelText = `${pointsToTarget ? '←' : '→'} ${edgeLabelReverse(source)}`
  } else {
    labelText = `${pointsToTarget ? '→' : '←'} ${edgeLabelForward(target)}`
  }
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
            strokeWidth: isDirect ? 2.5 : strokeWidth,
            opacity,
            transition: isInteracting
              ? 'none'
              : 'opacity 0.25s ease, stroke-width 0.25s ease, stroke 0.25s ease',
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

StatusEdge.displayName = 'StatusEdge'
