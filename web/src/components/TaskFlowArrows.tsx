import { useEffect, useRef, useState } from 'react'
import type { Task } from '../lib/types'

interface Arrow {
  fromTeamId: string
  toTeamId: string
  count: number
}

interface TaskFlowArrowsProps {
  tasks: Task[]
  cardRefs: Record<string, HTMLElement | null>
}

export default function TaskFlowArrows({ tasks, cardRefs }: TaskFlowArrowsProps) {
  const svgRef = useRef<SVGSVGElement>(null)
  const [size, setSize] = useState({ w: 0, h: 0 })

  useEffect(() => {
    function measure() {
      const parent = svgRef.current?.parentElement
      if (parent) setSize({ w: parent.clientWidth, h: parent.clientHeight })
    }
    measure()
    const ro = new ResizeObserver(measure)
    if (svgRef.current?.parentElement) ro.observe(svgRef.current.parentElement)
    return () => ro.disconnect()
  }, [])

  // Aggregate cross-team tasks
  const arrows: Arrow[] = []
  const arrowMap = new Map<string, Arrow>()
  for (const task of tasks) {
    if (!task.teamId || !task.assignedTo) continue
    if (task.teamId === task.assignedTo) continue
    const key = `${task.teamId}→${task.assignedTo}`
    const existing = arrowMap.get(key)
    if (existing) {
      existing.count++
    } else {
      const arrow = { fromTeamId: task.teamId, toTeamId: task.assignedTo, count: 1 }
      arrowMap.set(key, arrow)
      arrows.push(arrow)
    }
  }

  function getCardCenter(teamId: string): { x: number; y: number } | null {
    const el = cardRefs[teamId]
    if (!el || !svgRef.current) return null
    const svgRect = svgRef.current.getBoundingClientRect()
    const cardRect = el.getBoundingClientRect()
    return {
      x: cardRect.left - svgRect.left + cardRect.width / 2,
      y: cardRect.top - svgRect.top + cardRect.height / 2,
    }
  }

  return (
    <svg
      ref={svgRef}
      style={{ position: 'absolute', inset: 0, pointerEvents: 'none', zIndex: 1 }}
      width={size.w}
      height={size.h}
    >
      <defs>
        <marker id="arrowhead" markerWidth="6" markerHeight="4" refX="6" refY="2" orient="auto">
          <polygon points="0 0, 6 2, 0 4" fill="rgba(34,197,94,0.5)" />
        </marker>
      </defs>
      {arrows.map(arrow => {
        const from = getCardCenter(arrow.fromTeamId)
        const to = getCardCenter(arrow.toTeamId)
        if (!from || !to) return null
        const key = `${arrow.fromTeamId}→${arrow.toTeamId}`
        const mx = (from.x + to.x) / 2
        const my = (from.y + to.y) / 2
        return (
          <g key={key}>
            <line
              x1={from.x} y1={from.y}
              x2={to.x} y2={to.y}
              stroke="rgba(34,197,94,0.3)"
              strokeWidth={1.5}
              strokeDasharray="4 4"
              markerEnd="url(#arrowhead)"
            />
            <text
              x={mx} y={my - 6}
              textAnchor="middle"
              fontSize={10}
              fill="rgba(34,197,94,0.7)"
            >
              {arrow.count} task{arrow.count !== 1 ? 's' : ''}
            </text>
          </g>
        )
      })}
    </svg>
  )
}
