import { useEffect, useMemo, useRef } from 'react'
import type { FitViewFn, SetViewportFn } from './strategyMapControls'

export type SavedViewport = { x: number; y: number; zoom: number }

/**
 * Per-project viewport persistence. We key by projectId so switching projects
 * doesn't restore the wrong pan/zoom, and store only the three numbers React
 * Flow needs. Failures are logged but non-fatal — if localStorage is
 * unavailable (private browsing, quota), the map falls back to fit-view and
 * the user loses only the persistence nicety, not functionality.
 */
function viewportStorageKey(projectId: string): string {
  return `agen8:strategyMap:viewport:${projectId}`
}

export function loadSavedViewport(projectId: string): SavedViewport | null {
  let raw: string | null
  try {
    raw = localStorage.getItem(viewportStorageKey(projectId))
  } catch (e) {
    console.warn('[StrategyMap] localStorage.getItem failed', e)
    return null
  }
  if (!raw) return null
  let parsed: unknown
  try {
    parsed = JSON.parse(raw)
  } catch (e) {
    console.warn('[StrategyMap] saved viewport parse failed', e)
    return null
  }
  if (
    !parsed ||
    typeof parsed !== 'object' ||
    typeof (parsed as SavedViewport).x !== 'number' ||
    typeof (parsed as SavedViewport).y !== 'number' ||
    typeof (parsed as SavedViewport).zoom !== 'number'
  ) {
    console.warn('[StrategyMap] saved viewport shape invalid, ignoring')
    return null
  }
  const v = parsed as SavedViewport
  return { x: v.x, y: v.y, zoom: v.zoom }
}

export function saveViewport(projectId: string, viewport: SavedViewport): void {
  try {
    localStorage.setItem(
      viewportStorageKey(projectId),
      JSON.stringify({ x: viewport.x, y: viewport.y, zoom: viewport.zoom }),
    )
  } catch (e) {
    console.warn('[StrategyMap] localStorage.setItem failed', e)
  }
}

/**
 * Restores the saved viewport (or animates a welcome fit-to-all on first
 * visit) once nodes are available, and re-arms that restore when the project
 * changes within the same mounted component. Returns the synchronously-loaded
 * initial viewport so ReactFlow's `defaultViewport` can avoid a first-frame
 * flash.
 */
export function useStrategyMapViewport({
  projectId,
  displayNodeCount,
  fitView,
  setViewport,
}: {
  projectId: string
  displayNodeCount: number
  fitView: FitViewFn
  setViewport: SetViewportFn
}): { initialViewport: SavedViewport | null } {
  // Synchronous load so `defaultViewport` on ReactFlow can use it without a
  // first-frame flash. Rememoizes when the user switches projects.
  const initialViewport = useMemo(() => loadSavedViewport(projectId), [projectId])
  const hasRestoredRef = useRef(false)

  useEffect(() => {
    if (displayNodeCount === 0 || hasRestoredRef.current) return
    hasRestoredRef.current = true
    if (initialViewport) {
      // Restore the user's saved pan/zoom instantly — setViewport without a
      // duration option is synchronous and animation-free, so there is no
      // jitter between the default viewport and the restored one.
      setViewport(initialViewport)
      return
    }
    // First-time visit (or cleared storage) — animate a welcome fit-to-all.
    const t = setTimeout(() => fitView({ padding: 0.18, duration: 800 }), 80)
    return () => clearTimeout(t)
  }, [displayNodeCount, fitView, setViewport, initialViewport])

  // Switching projects within the same mounted component needs to re-trigger
  // the restore-or-fit path. Resetting the guard allows the effect above to
  // run a second time with the new project's initialViewport.
  useEffect(() => {
    hasRestoredRef.current = false
  }, [projectId])

  return { initialViewport }
}
