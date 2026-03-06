import { useEffect } from 'react'
import { useStore } from './lib/store'
import TopBar from './components/TopBar'
import Overview from './pages/Overview'
import TeamFocus from './pages/TeamFocus'
import CommandPalette from './components/CommandPalette'

export default function App() {
  const { focusedTeamId, paletteOpen, theme } = useStore()

  useEffect(() => {
    document.documentElement.setAttribute('data-theme', theme)
  }, [theme])

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
        e.preventDefault()
        useStore.getState().setPaletteOpen(true)
      }
      if (e.key === 'Escape') {
        useStore.getState().setPaletteOpen(false)
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  return (
    <div className="app-shell">
      <TopBar />
      <main className="app-main">
        {focusedTeamId ? <TeamFocus teamId={focusedTeamId} /> : <Overview />}
      </main>
      {paletteOpen && <CommandPalette />}
    </div>
  )
}
