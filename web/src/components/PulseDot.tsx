import { motion } from 'framer-motion'

interface PulseDotProps {
  status: 'active' | 'idle' | 'pending' | 'failed' | 'done'
  size?: number
}

const colorMap = {
  active: '#22c55e',
  idle: '#71717a',
  pending: '#f59e0b',
  failed: '#ef4444',
  done: '#3b82f6',
}

export default function PulseDot({ status, size = 8 }: PulseDotProps) {
  const color = colorMap[status] ?? colorMap.idle
  const shouldPulse = status === 'active' || status === 'pending'

  return (
    <span style={{ position: 'relative', display: 'inline-flex', alignItems: 'center', justifyContent: 'center', width: size, height: size }}>
      {shouldPulse && (
        <>
          <motion.span
            style={{
              position: 'absolute',
              inset: 0,
              borderRadius: '50%',
              backgroundColor: color,
              opacity: 0.6,
            }}
            animate={{ scale: [1, 2, 2], opacity: [0.6, 0, 0] }}
            transition={{ duration: 1.8, repeat: Infinity, ease: 'easeOut' }}
          />
          <motion.span
            style={{
              position: 'absolute',
              inset: 0,
              borderRadius: '50%',
              backgroundColor: color,
              opacity: 0.4,
            }}
            animate={{ scale: [1, 2.6, 2.6], opacity: [0.4, 0, 0] }}
            transition={{ duration: 1.8, repeat: Infinity, ease: 'easeOut', delay: 0.3 }}
          />
        </>
      )}
      <span style={{ width: size, height: size, borderRadius: '50%', backgroundColor: color, display: 'block', position: 'relative', zIndex: 1 }} />
    </span>
  )
}
