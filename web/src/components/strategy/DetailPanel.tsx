import { useCallback, useEffect, useRef, useState } from 'react'
import { motion } from 'framer-motion'
import type { Node } from '@xyflow/react'
import { nodeTypeRegistry } from './registry'

interface Props {
  node: Node
  projectId: string
  projectRoot?: string | null
  onClose: () => void
}

const STORAGE_KEY = 'strategy-detail-panel-width'
const DEFAULT_WIDTH = 420
const MIN_WIDTH = 320
const MAX_WIDTH = 700

function loadWidth(): number {
  try {
    const saved = localStorage.getItem(STORAGE_KEY)
    if (saved) {
      const n = Number(saved)
      if (n >= MIN_WIDTH && n <= MAX_WIDTH) return n
    }
  } catch { /* ignore */ }
  return DEFAULT_WIDTH
}

/**
 * Dispatches to the correct panel component based on the selected node's type.
 * Renders as an absolute overlay on the right side so the ReactFlow canvas
 * stays full-width underneath — no reflow gap when the panel opens/closes.
 *
 * Width is resizable via a drag handle on the left edge and persisted
 * to localStorage.
 */
export function DetailPanel({ node, projectId, projectRoot, onClose }: Props) {
  const descriptor = nodeTypeRegistry[node.type ?? '']
  const [width, setWidth] = useState(loadWidth)
  const dragging = useRef(false)
  const startX = useRef(0)
  const startWidth = useRef(width)

  // Persist width changes
  useEffect(() => {
    try { localStorage.setItem(STORAGE_KEY, String(width)) } catch { /* ignore */ }
  }, [width])

  const onMouseDown = useCallback((e: React.MouseEvent) => {
    e.preventDefault()
    dragging.current = true
    startX.current = e.clientX
    startWidth.current = width

    const onMouseMove = (ev: MouseEvent) => {
      if (!dragging.current) return
      const delta = startX.current - ev.clientX
      const newWidth = Math.min(MAX_WIDTH, Math.max(MIN_WIDTH, startWidth.current + delta))
      setWidth(newWidth)
    }

    const onMouseUp = () => {
      dragging.current = false
      document.removeEventListener('mousemove', onMouseMove)
      document.removeEventListener('mouseup', onMouseUp)
      document.body.style.cursor = ''
      document.body.style.userSelect = ''
    }

    document.body.style.cursor = 'col-resize'
    document.body.style.userSelect = 'none'
    document.addEventListener('mousemove', onMouseMove)
    document.addEventListener('mouseup', onMouseUp)
  }, [width])

  if (!descriptor) return null

  return (
    <motion.div
      className="absolute top-0 right-0 bottom-0 z-20 border-l border-border bg-card overflow-hidden flex flex-col"
      style={{ width }}
      initial={{ x: width, opacity: 0 }}
      animate={{ x: 0, opacity: 1 }}
      exit={{ x: width, opacity: 0 }}
      transition={{ type: 'spring', stiffness: 400, damping: 38 }}
    >
      {/* Resize handle */}
      <div
        className="absolute left-0 top-0 bottom-0 z-50 cursor-col-resize group"
        style={{ width: 6 }}
        onMouseDown={onMouseDown}
      >
        <div
          className="absolute left-0 top-0 bottom-0 w-[2px] opacity-0 group-hover:opacity-100 transition-opacity duration-150"
          style={{ background: 'var(--accent)' }}
        />
      </div>

      <descriptor.Panel
        data={node.data}
        projectId={projectId}
        projectRoot={projectRoot}
        onClose={onClose}
      />
    </motion.div>
  )
}
