import { memo } from 'react'

interface PulseDotProps {
  status: 'active' | 'paused' | 'idle' | 'pending' | 'failed' | 'done' | 'stopped'
  size?: number
}

const colorMap = {
  active: 'var(--green)',
  paused: 'var(--amber)',
  idle: 'var(--text-3)',
  pending: 'var(--amber)',
  failed: 'var(--red)',
  done: 'var(--blue)',
  stopped: 'var(--text-3)',
}

export default memo(function PulseDot({ status, size = 8 }: PulseDotProps) {
  const color = colorMap[status] ?? colorMap.idle
  const shouldPulse = status === 'active'

  return (
    <span className="relative inline-flex items-center justify-center" style={{ width: size, height: size }}>
      {shouldPulse && (
        <span
          className="absolute inset-0 rounded-full"
          style={{ backgroundColor: color, animation: 'pulse-ring 2.5s ease-in-out infinite' }}
        />
      )}
      <span className="block relative z-[1] rounded-full" style={{ width: size, height: size, backgroundColor: color }} />
    </span>
  )
})
