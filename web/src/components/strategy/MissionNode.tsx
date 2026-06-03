import { memo } from 'react'
import { Handle, Position, type NodeProps } from '@xyflow/react'
import { cn } from '@/lib/utils'
import { Target } from 'lucide-react'
import type { MissionView } from '../../lib/types'
import { useStrategyMapStore } from './strategyMapStore'

export interface MissionNodeData {
  mission: MissionView
  avgProgress: number
  krCount: number
  spaceName?: string
  clusterColor?: string
  [key: string]: unknown
}

const WIDTH = 220 

export const MissionNode = memo(function MissionNode({ data, selected, id }: NodeProps) {
  const d = data as unknown as MissionNodeData
  const { mission, avgProgress, krCount } = d
  const color = d.clusterColor ?? '#0071e3'
  // Field-level subscriptions — a change to e.g. `isInteracting` only
  // invalidates this selector (boolean unchanged → no re-render) instead
  // of tearing down every consumer of a monolithic context value.
  const displayMode = useStrategyMapStore((s) => s.displayMode)
  const isInteracting = useStrategyMapStore((s) => s.isInteracting)
  const isZooming = useStrategyMapStore((s) => s.isZooming)
  // Derived booleans — Object.is equality means this node only re-renders
  // when *its* dim/active flag actually flips, not whenever some other
  // node's selection changes.
  const isDimmed = useStrategyMapStore((s) =>
    s.clusterNodeIds ? !s.clusterNodeIds.has(id) : false,
  )
  const isActiveFromStore = useStrategyMapStore((s) => s.selectedNodeId === id)
  const orbit = displayMode.missionKR === 'orbit'
  const isActive = selected || isActiveFromStore
  const inactiveClass = isInteracting
    ? 'ring-1 ring-border shadow-none'
    : 'ring-1 ring-border hover:ring-[var(--border-strong)] hover:shadow-[0_6px_20px_rgba(0,0,0,0.1)] shadow-md'
  const nebulaOpacity = isActive ? 0.24 : isZooming ? 0.02 : 0.12
  const nebulaTransition = isZooming
    ? 'opacity 90ms linear, box-shadow 90ms linear'
    : 'opacity 280ms cubic-bezier(0.22, 1, 0.36, 1), box-shadow 360ms cubic-bezier(0.22, 1, 0.36, 1)'

  // Orbit: tiny dot
  if (orbit) {
    return (
      <div className="cursor-pointer select-none transition-all duration-300 hover:scale-110"
        style={{ width: 18, height: 18 }}>
        <Handle type="target" position={Position.Top} style={{ left: '50%', top: '50%', opacity: 0, pointerEvents: 'none' }} />
        <Handle type="source" position={Position.Bottom} style={{ left: '50%', top: '50%', opacity: 0, pointerEvents: 'none' }} />
        <div className="w-full h-full rounded-full opacity-80" style={{ background: color }} />
      </div>
    )
  }

  // Nebulas are GPU-expensive: every rendered nebula allocates its own
  // composited blur buffer. During zoom the ambient opacity collapses to
  // ~0.02 (effectively invisible), so unmounting non-active nebulas while
  // zooming buys a big compositor win with zero perceptible visual change.
  // The active node's nebula stays mounted to preserve its selection glow.
  const renderNebula = isActive || !isZooming

  // Pure typographical card with structural Apple Nebula cluster grouping
  return (
    <>
      {renderNebula && (
        <div className="absolute left-1/2 top-1/2 z-[-1] pointer-events-none">
          <div
            className="rounded-full heat-nebula"
            style={{
              width: '1px', height: '1px',
              boxShadow: `0 0 ${isZooming ? 200 : 300}px ${isZooming ? 130 : 220}px ${color}`,
              opacity: nebulaOpacity,
              transition: nebulaTransition,
            }}
          />
        </div>
      )}

      <div
        className={cn(
          "relative cursor-pointer select-none transition group flex flex-col items-start text-left hover:-translate-y-1 bg-panel",
          isActive ? "z-50" : inactiveClass
        )}
        style={{
          width: WIDTH,
          padding: '16px 20px',
          borderRadius: '12px',
          background: 'var(--bg-panel)',
        }}
      >
        <Handle type="target" position={Position.Top} style={{ left: '50%', top: '50%', opacity: 0, pointerEvents: 'none' }} />
        <Handle type="source" position={Position.Bottom} style={{ left: '50%', top: '50%', opacity: 0, pointerEvents: 'none' }} />

        {/* Content wrapper — dims when unfocused, background stays opaque */}
        <div className="w-full transition-opacity duration-300"
          style={{ opacity: isDimmed ? 0.15 : 1 }}>

          {/* Mission icon + space eyebrow */}
          <div className="flex w-full items-center gap-1.5 mb-2">
            <Target size={14} className="shrink-0 transition-all duration-200"
              style={{
                color: isActive ? color : 'var(--text-3)',
                opacity: isActive ? 1 : 0.6,
                filter: isActive ? `drop-shadow(0 0 4px ${color})` : undefined,
              }} />
            <span className="uppercase font-semibold text-foreground tracking-[0.4px] opacity-[0.65]" style={{ fontSize: '9px' }}>
              {d.spaceName ? d.spaceName : 'STRATEGY'}
            </span>
          </div>

          <div className="flex items-start justify-between w-full gap-3 mb-2.5">
            <h3 className="text-foreground w-full break-words"
              style={{
                fontWeight: 600,
                fontSize: '17px',
                letterSpacing: '-0.374px',
                lineHeight: 1.24
              }}>
              {mission.title}
            </h3>
          </div>

          <div className="flex items-center text-left"
            style={{ fontWeight: 400, fontSize: '14px', letterSpacing: '-0.224px', color: 'var(--text-3)' }}>
            <span>{krCount} KR{krCount !== 1 ? 's' : ''}</span>
            <span className="mx-2 opacity-60">·</span>
            <span>{avgProgress}% average</span>
          </div>
        </div>
    </div>
    </>
  )
})

MissionNode.displayName = 'MissionNode'
