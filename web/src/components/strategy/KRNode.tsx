import { memo } from 'react'
import { Handle, Position, type NodeProps } from '@xyflow/react'
import { cn } from '@/lib/utils'
import type { KeyResultView } from '../../lib/types'
import { useStrategyMapStore } from './strategyMapStore'

// Mini progress bar icon — matches the KR legend glyph.
function KRIcon({ color, progress, active = false, size = 16 }: { color: string; progress: number; active?: boolean; size?: number }) {
  const w = size
  const h = size * 0.65
  const fill = Math.max(2, (progress / 100) * (w - 4))
  return (
    <div className="transition-all duration-200" style={{
      width: w, height: h,
      borderRadius: 3,
      border: `1.5px solid color-mix(in srgb, ${color} ${active ? '100' : '60'}%, transparent)`,
      background: `color-mix(in srgb, ${color} ${active ? '20' : '10'}%, transparent)`,
      overflow: 'hidden',
      position: 'relative',
      filter: active ? `drop-shadow(0 0 4px ${color})` : undefined,
    }}>
      <div style={{
        position: 'absolute', left: 0, top: 0, bottom: 0,
        width: fill,
        background: `color-mix(in srgb, ${color} 80%, transparent)`,
        borderRadius: 1,
      }} />
    </div>
  )
}

export interface KRNodeData {
  kr: KeyResultView
  clusterColor?: string
  linkedDecisionCount?: number
  [key: string]: unknown
}

export const KRNode = memo(function KRNode({ data, selected, id }: NodeProps) {
  const d = data as unknown as KRNodeData
  const { kr } = d
  // Cluster colour is used everywhere this node paints: nebula glow,
  // progress bar fill, active outline. Previously there was a secret
  // `isActive ? '#0071e3' : baseColor` override that swapped the colour
  // to hardcoded Apple Blue when selected — which defeated the whole
  // cluster-colour-identity goal and made active KRs always appear blue.
  const color = d.clusterColor ?? 'var(--text-3)'
  const progress = Math.min(100, Math.max(0, kr.progressPercent))
  const displayMode = useStrategyMapStore((s) => s.displayMode)
  const isZooming = useStrategyMapStore((s) => s.isZooming)
  const isDense = useStrategyMapStore((s) => s.isDense)
  const isDimmed = useStrategyMapStore((s) =>
    s.clusterNodeIds ? !s.clusterNodeIds.has(id) : false,
  )
  const isActiveFromStore = useStrategyMapStore((s) => s.selectedNodeId === id)
  const isActive = selected || isActiveFromStore
  const isTraced = useStrategyMapStore((s) => s.activeFilter === 'trace') && !isDimmed
  const nebulaOpacity = isActive ? 0.24 : isTraced ? 0.18 : isZooming ? 0.015 : 0.1
  const nebulaTransition = isZooming
    ? 'opacity 90ms linear, box-shadow 90ms linear'
    : 'opacity 280ms cubic-bezier(0.22, 1, 0.36, 1), box-shadow 360ms cubic-bezier(0.22, 1, 0.36, 1)'

  if (displayMode.missionKR === 'orbit') {
    return (
      <div className="cursor-pointer select-none transition-all duration-300 hover:scale-[1.5]"
        style={{ width: 14, height: 14, opacity: isDimmed ? 0.15 : 1 }}>
        <Handle type="target" position={Position.Top} style={{ left: '50%', top: '50%', opacity: 0, pointerEvents: 'none' }} />
        <Handle type="source" position={Position.Bottom} style={{ left: '50%', top: '50%', opacity: 0, pointerEvents: 'none' }} />
        <div className="w-full h-full rounded-full" style={{ background: color, opacity: 0.8 }} />
      </div>
    )
  }

  // KR nebulas are the biggest compositor cost on dense maps — 4-6 KRs per
  // mission means dozens of blurred composite layers. We drop the ambient
  // glow entirely on dense maps (keeping only the active node's nebula),
  // and also drop non-active nebulas during any zoom gesture. On sparse
  // maps the ambient constellation feel is preserved.
  const renderNebula = isActive || (!isZooming && !isDense)

  return (
    <div>
      {renderNebula && (
        <div className="absolute left-1/2 top-1/2 z-[-1] pointer-events-none">
          <div
            className="rounded-full heat-nebula"
            style={{
              width: '1px', height: '1px',
              boxShadow: `0 0 ${isZooming ? 130 : 180}px ${isZooming ? 85 : 120}px ${color}`,
              opacity: nebulaOpacity,
              transition: nebulaTransition,
            }}
          />
        </div>
      )}
      <div className={cn(
          'relative cursor-pointer select-none flex flex-col items-center',
          'group transition-all duration-200 hover:-translate-y-0.5',
          isActive ? 'z-50' : '',
      )} style={{
        padding: '4px 10px 6px',
        borderRadius: 6,
        background: 'var(--bg-panel)',
        transition: 'background 0.2s ease',
      }}>
        {/* Hover wash */}
        <div className="absolute inset-0 rounded-[6px] opacity-0 group-hover:opacity-100 transition-opacity duration-200 pointer-events-none"
          style={{ background: `color-mix(in srgb, ${color} 5%, transparent)` }} />
        <Handle type="target" position={Position.Top} style={{ left: '50%', top: '50%', opacity: 0, pointerEvents: 'none' }} />
        <Handle type="source" position={Position.Bottom} style={{ left: '50%', top: '50%', opacity: 0, pointerEvents: 'none' }} />

        <div className="flex flex-col items-center transition-opacity duration-300"
          style={{ opacity: isDimmed ? 0.15 : 1 }}>
          {/* KR icon — mini progress bar matching legend */}
          <KRIcon color={color} progress={progress} active={isActive} />

          <div className="flex flex-col items-center gap-[3px] mt-[2px]">
            <span className="text-foreground truncate text-center font-medium transition-opacity duration-200"
              style={{ fontSize: '0.71875rem', lineHeight: '14px', maxWidth: 180 }}>
              {kr.title}
            </span>
            <div className="flex items-center gap-2 w-full" style={{ maxWidth: 160 }}>
              <div className="flex-1 h-[3px] bg-muted/60 rounded-full overflow-hidden">
                <div className="h-full rounded-full transition-all duration-1000 ease-out" style={{ width: `${progress}%`, background: color }} />
              </div>
              <span className="shrink-0 tabular-nums text-[0.59375rem] font-semibold text-muted-foreground">
                {progress}%
              </span>
            </div>
            {d.linkedDecisionCount != null && d.linkedDecisionCount > 0 && (
              <span className="text-[0.59375rem] font-medium text-muted-foreground">
                {d.linkedDecisionCount} decision{d.linkedDecisionCount === 1 ? '' : 's'}
              </span>
            )}
          </div>
        </div>
      </div>
    </div>
  )
})

KRNode.displayName = 'KRNode'
