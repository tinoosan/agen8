import { CircleCheck, Diamond, Target } from 'lucide-react'

export function StrategyMapLegend() {
  return (
    <div
      aria-label="Map Legend"
      className="absolute bottom-5 left-5 z-10 pointer-events-none flex flex-col gap-2 rounded-[8px] px-[13px] py-3"
      style={{
        background: 'color-mix(in srgb, var(--bg-panel) 78%, transparent)',
        border: '1px solid color-mix(in srgb, var(--border) 52%, transparent)',
        boxShadow: '0 14px 34px rgba(15, 23, 42, 0.06)',
        opacity: 0.9,
        backdropFilter: 'blur(10px)',
      }}
    >
      <h4 className="text-[0.5625rem] font-bold text-foreground/45 uppercase tracking-[0.6px] mb-1">Map Legend</h4>
      <div className="flex flex-col gap-2.5">
        <div className="flex items-center gap-3">
          <Target size={13} className="text-[var(--accent)]" strokeWidth={2.2} />
          <span className="text-[0.71875rem] font-medium text-foreground/85 tracking-tight">Mission</span>
        </div>
        <div className="flex items-center gap-3 w-full">
          <div
            className="w-[14px] h-[9px] flex items-center justify-start rounded-[2.5px] overflow-hidden relative left-[1px]"
            style={{
              border: '1px solid color-mix(in srgb, var(--accent) 45%, transparent)',
              background: 'color-mix(in srgb, var(--accent) 8%, transparent)',
            }}
          >
            <div className="h-full w-[8px] bg-[var(--accent)] opacity-70" />
          </div>
          <span className="text-[0.71875rem] font-medium text-foreground/85 tracking-tight pl-[3px]">Key Result</span>
        </div>
        <div className="flex items-center gap-3">
          <CircleCheck size={13} className="text-[var(--text-3)]" strokeWidth={2.2} />
          <span className="text-[0.71875rem] font-medium text-foreground/85 tracking-tight">Task</span>
        </div>
        <div className="flex items-center gap-3">
          <Diamond size={13} className="text-[var(--accent)]" strokeWidth={2.2} />
          <span className="text-[0.71875rem] font-medium text-foreground/85 tracking-tight">Decision</span>
        </div>
      </div>
    </div>
  )
}
