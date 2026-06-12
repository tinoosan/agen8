import { useEffect } from 'react'
import { useLocation } from 'wouter'
import { useStore } from '../lib/store'
import { useNavigation, strategyMapLink } from '../lib/routing'
import { useStrategyGraph } from './strategy/useStrategyGraph'
import { StrategyMapSearch } from './strategy/StrategyMapSearch'

/**
 * GlobalNodeSearch — makes the existing node search reachable from every page.
 *
 * The search dialog itself (StrategyMapSearch over the strategy graph's
 * tasks/missions/KRs/decisions) was only mounted on the dashboard and the
 * Context Map, so the header search button silently did nothing anywhere else.
 * This wrapper, mounted once in App:
 *
 *  - registers the global shortcuts (Cmd/Ctrl+K, Cmd/Ctrl+/, and bare "/"
 *    outside text fields) that flip the shared strategySearchOpen flag, and
 *  - renders the dialog on pages that have no local mount. Dashboard and the
 *    map keep their own instances (the map centers in place on select), so
 *    this renders nothing there to avoid double dialogs on the shared flag.
 *
 * Selecting a result deep-links to that node on the Context Map.
 */
export default function GlobalNodeSearch() {
  const [location, navigate] = useLocation()
  const { projectId, focusedProjectRoot } = useNavigation()
  const searchOpen = useStore((s) => s.strategySearchOpen)
  const setSearchOpen = useStore((s) => s.setStrategySearchOpen)

  // Pages with their own StrategyMapSearch mount; the global one stands down.
  const hasLocalMount =
    /\/strategy(\/|$|\?)/.test(location) ||
    /\/dashboard(\/|$|\?)/.test(location) ||
    (projectId !== null && location === `/project/${encodeURIComponent(projectId)}`)

  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      const target = e.target as HTMLElement | null
      const typing =
        !!target &&
        (target.tagName === 'INPUT' ||
          target.tagName === 'TEXTAREA' ||
          target.isContentEditable)
      const combo = (e.metaKey || e.ctrlKey) && (e.key === 'k' || e.key === '/')
      const bareSlash = e.key === '/' && !e.metaKey && !e.ctrlKey && !e.altKey && !typing
      if (!combo && !bareSlash) return
      if (combo && typing && e.key === 'k') return // never steal Cmd+K mid-edit
      e.preventDefault()
      useStore.getState().setStrategySearchOpen(true)
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [])

  const { nodes } = useStrategyGraph(
    !hasLocalMount && searchOpen ? projectId : null,
    focusedProjectRoot,
  )

  if (!projectId || hasLocalMount) return null

  return (
    <StrategyMapSearch
      open={searchOpen}
      onOpenChange={setSearchOpen}
      nodes={nodes}
      onSelect={(nodeId) => {
        setSearchOpen(false)
        navigate(strategyMapLink(projectId, nodeId))
      }}
    />
  )
}
