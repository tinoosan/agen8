import { useCallback, useEffect, useRef, useState, type PointerEvent as ReactPointerEvent } from 'react'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs'
import { Sheet, SheetContent, SheetTitle, SheetDescription } from '@/components/ui/sheet'
import { cn } from '@/lib/utils'
import type { DashboardPanel } from '../../lib/routing'
import DashboardMissionsPanel from './DashboardMissionsPanel'
import DashboardTasksPanel from './DashboardTasksPanel'
import DashboardDecisionsPanel from './DashboardDecisionsPanel'

const CONTEXT_WIDTH_KEY = 'dashboard.context-panel-width'
const CONTEXT_DEFAULT_WIDTH = 460
const CONTEXT_MIN_WIDTH = 340
const CONTEXT_MAX_WIDTH = 920

function clampContextWidth(value: number): number {
  if (!Number.isFinite(value)) return CONTEXT_DEFAULT_WIDTH
  const viewportMax = typeof window === 'undefined'
    ? CONTEXT_MAX_WIDTH
    : Math.max(CONTEXT_MIN_WIDTH, window.innerWidth - 860)
  return Math.max(CONTEXT_MIN_WIDTH, Math.min(CONTEXT_MAX_WIDTH, viewportMax, Math.round(value)))
}

function readContextWidth(): number {
  try {
    const raw = localStorage.getItem(CONTEXT_WIDTH_KEY)
    if (!raw) return CONTEXT_DEFAULT_WIDTH
    return clampContextWidth(Number(raw))
  } catch {
    return CONTEXT_DEFAULT_WIDTH
  }
}

function writeContextWidth(value: number): void {
  try { localStorage.setItem(CONTEXT_WIDTH_KEY, String(clampContextWidth(value))) } catch { /* ignore */ }
}

interface DashboardContextPanelProps {
  open: boolean
  overlay: boolean
  panel: DashboardPanel
  projectId: string | null
  focusedProjectRoot: string | null
  onPanelChange: (panel: DashboardPanel) => void
  onOpenChange: (open: boolean) => void
}

export default function DashboardContextPanel({
  open,
  overlay,
  panel,
  projectId,
  focusedProjectRoot,
  onPanelChange,
  onOpenChange,
}: DashboardContextPanelProps) {
  const [contextWidth, setContextWidth] = useState(readContextWidth)
  const contextRailRef = useRef<HTMLDivElement | null>(null)
  const resizeCleanupRef = useRef<(() => void) | null>(null)
  const resizeFrameRef = useRef<number | null>(null)
  const widthRef = useRef(contextWidth)

  useEffect(() => {
    widthRef.current = contextWidth
  }, [contextWidth])

  useEffect(() => () => {
    if (resizeCleanupRef.current) resizeCleanupRef.current()
  }, [])

  const beginResizeContextPanel = useCallback((event: ReactPointerEvent<HTMLDivElement>) => {
    if (event.button !== 0 || !open) return
    event.preventDefault()
    if (resizeCleanupRef.current) resizeCleanupRef.current()

    const startX = event.clientX
    const startWidth = widthRef.current
    let pendingWidth = startWidth
    const rail = contextRailRef.current
    if (!rail) return
    rail.style.transition = 'none'

    const queueWidthUpdate = (nextWidth: number) => {
      pendingWidth = clampContextWidth(nextWidth)
      if (resizeFrameRef.current !== null) return
      resizeFrameRef.current = requestAnimationFrame(() => {
        resizeFrameRef.current = null
        widthRef.current = pendingWidth
        if (!contextRailRef.current) return
        contextRailRef.current.style.width = `${pendingWidth}px`
      })
    }

    const cleanup = () => {
      if (resizeFrameRef.current !== null) {
        cancelAnimationFrame(resizeFrameRef.current)
        resizeFrameRef.current = null
      }
      const finalWidth = clampContextWidth(pendingWidth)
      widthRef.current = finalWidth
      setContextWidth((previous) => (previous === finalWidth ? previous : finalWidth))
      writeContextWidth(finalWidth)
      if (contextRailRef.current) {
        contextRailRef.current.style.width = `${finalWidth}px`
        contextRailRef.current.style.transition = ''
      }
      document.body.style.cursor = ''
      document.body.style.userSelect = ''
      window.removeEventListener('pointermove', onMove)
      window.removeEventListener('pointerup', onUp)
      window.removeEventListener('pointercancel', onUp)
      resizeCleanupRef.current = null
    }

    const onMove = (moveEvent: PointerEvent) => {
      const delta = startX - moveEvent.clientX
      queueWidthUpdate(startWidth + delta)
    }

    const onUp = () => cleanup()

    document.body.style.cursor = 'col-resize'
    document.body.style.userSelect = 'none'
    window.addEventListener('pointermove', onMove)
    window.addEventListener('pointerup', onUp)
    window.addEventListener('pointercancel', onUp)
    resizeCleanupRef.current = cleanup
  }, [open])

  const panelBody = (
    <Tabs value={panel} onValueChange={(value) => onPanelChange(value as DashboardPanel)} className="flex h-full flex-col">
      <div className="dashboard-context-header shrink-0 h-12 flex items-center px-[var(--dashboard-context-gutter)] border-b border-[color-mix(in_srgb,var(--border)_48%,transparent)]">
        <TabsList className="dashboard-context-tabs h-auto bg-transparent gap-0 p-0 rounded-none shrink-0">
          <TabsTrigger value="missions" className="dashboard-context-tab">Missions</TabsTrigger>
          <TabsTrigger value="tasks" className="dashboard-context-tab">Tasks</TabsTrigger>
          <TabsTrigger value="decisions" className="dashboard-context-tab">Decisions</TabsTrigger>
        </TabsList>
      </div>

      <TabsContent value="missions" className="flex-1 min-h-0 mt-0 overflow-hidden">
        <DashboardMissionsPanel projectId={projectId} focusedProjectRoot={focusedProjectRoot} embedded />
      </TabsContent>
      <TabsContent value="tasks" className="flex-1 min-h-0 mt-0 overflow-hidden">
        <DashboardTasksPanel projectId={projectId} focusedProjectRoot={focusedProjectRoot} embedded />
      </TabsContent>
      <TabsContent value="decisions" className="flex-1 min-h-0 mt-0 overflow-hidden">
        <DashboardDecisionsPanel projectId={projectId} focusedProjectRoot={focusedProjectRoot} embedded />
      </TabsContent>
    </Tabs>
  )

  if (overlay) {
    return (
      <Sheet open={open} onOpenChange={onOpenChange}>
        <SheetContent
          side="right"
          className="dashboard-context-drawer w-[92vw] sm:max-w-[400px] p-0 gap-0 flex flex-col border-l border-[color-mix(in_srgb,var(--border)_48%,transparent)] bg-[var(--bg-panel)]"
        >
          <SheetTitle className="sr-only">Context panel</SheetTitle>
          <SheetDescription className="sr-only">Missions, tasks, and decisions for this project</SheetDescription>
          {panelBody}
        </SheetContent>
      </Sheet>
    )
  }

  return (
    <div
      ref={contextRailRef}
      className={cn(
        'dashboard-context-rail shrink-0 min-h-0 overflow-hidden relative transition-[width] duration-300 ease-[cubic-bezier(0.22,1,0.36,1)] will-change-[width]',
      )}
      style={{ width: open ? contextWidth : 0 }}
    >
      {open && (
        <div
          className="absolute left-0 top-0 bottom-0 w-3 -translate-x-1/2 cursor-col-resize z-20 group"
          onPointerDown={beginResizeContextPanel}
          aria-hidden
        >
          <div className="absolute left-1/2 top-1/2 h-18 w-[2px] -translate-x-1/2 -translate-y-1/2 rounded-full bg-transparent group-hover:bg-[var(--border)] transition-colors duration-150" />
        </div>
      )}
      <div
        className={cn(
          'dashboard-context-surface h-full transition-[clip-path,opacity] duration-220 ease-out bg-[var(--bg-panel)]',
          open ? 'opacity-100 pointer-events-auto' : 'opacity-0 pointer-events-none',
        )}
        style={{ clipPath: open ? 'inset(0 0 0 0)' : 'inset(0 100% 0 0)' }}
      >
        {panelBody}
      </div>
    </div>
  )
}
