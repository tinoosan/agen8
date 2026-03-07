import { useEffect } from 'react'
import { useStore } from './lib/store'
import TopBar from './components/TopBar'
import Project from './pages/Project'
import Overview from './pages/Overview'
import Dashboard from './pages/Dashboard'
import Logs from './pages/Logs'
import TeamFocus from './pages/TeamFocus'
import CommandPalette from './components/CommandPalette'
import ModelPicker from './components/ModelPicker'
import ReasoningPicker from './components/ReasoningPicker'

export default function App() {
  const { focusedTeamId, focusedProjectRoot, paletteOpen, modelPickerTarget, reasoningPickerTarget, theme, activeView } = useStore()

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
        {focusedTeamId ? (
          <TeamFocus teamId={focusedTeamId} />
        ) : !focusedProjectRoot ? (
          <Project />
        ) : activeView === 'dashboard' ? (
          <Dashboard />
        ) : activeView === 'logs' ? (
          <Logs />
        ) : (
          <Overview />
        )}
      </main>
      {paletteOpen && <CommandPalette />}
      {modelPickerTarget && <ModelPicker />}
      {reasoningPickerTarget && <ReasoningPicker />}
    </div>
  )
}
