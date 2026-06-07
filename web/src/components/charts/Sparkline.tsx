/**
 * Sparkline — a tiny, dependency-free SVG bar chart for inline trends.
 *
 * Hand-rolled rather than pulled from a chart library: the only charting dep in
 * the app is d3-force (for the strategy graph), and a full chart lib would dwarf
 * this 20px glyph. It's a BAR chart, not a connected line, on purpose — the
 * series it renders (tasks completed per day) is bursty and frequently zero, and
 * bars show an empty day honestly instead of interpolating a slope across it.
 *
 * Bars scale to the series max, so the shape shows relative trend, not absolute
 * volume — read the adjacent count for magnitude. Accessible via role="img" + an
 * aria-label; the bars themselves are aria-hidden decoration.
 */
export interface SparklineProps {
  /** One value per bucket, oldest → newest. */
  data: number[]
  width?: number
  height?: number
  className?: string
  /** Bar fill; any CSS color (defaults to the accent token). */
  color?: string
  /** Accessible description of the trend. */
  label?: string
}

export default function Sparkline({
  data,
  width = 72,
  height = 20,
  className,
  color = 'var(--accent)',
  label,
}: SparklineProps) {
  const max = data.reduce((m, v) => Math.max(m, v), 0)

  // Nothing to show: a flat baseline reads as "no activity" without faking bars.
  if (data.length === 0 || max <= 0) {
    return (
      <svg
        width={width}
        height={height}
        className={className}
        role="img"
        aria-label={label ?? 'No recent activity'}
      >
        <line
          x1={0}
          y1={height - 0.5}
          x2={width}
          y2={height - 0.5}
          stroke="var(--border)"
          strokeWidth={1}
        />
      </svg>
    )
  }

  const n = data.length
  const gap = n > 1 ? 1 : 0
  const barW = (width - gap * (n - 1)) / n
  const total = data.reduce((s, v) => s + v, 0)

  return (
    <svg
      width={width}
      height={height}
      className={className}
      role="img"
      aria-label={label ?? `${total} completed across ${n} days`}
    >
      <g aria-hidden="true">
        {data.map((v, i) => {
          if (v <= 0) return null
          // Floor a non-zero bar at 1px so a single completion is still visible.
          const h = Math.max(1, (v / max) * (height - 1))
          const x = i * (barW + gap)
          const y = height - h
          return (
            <rect
              key={i}
              x={x}
              y={y}
              width={barW}
              height={h}
              rx={Math.min(1, barW / 2)}
              fill={color}
            />
          )
        })}
      </g>
    </svg>
  )
}
