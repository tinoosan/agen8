import { create } from 'zustand'

export type Theme = 'dark' | 'light' | 'dim'
export type ActiveView = 'project' | 'overview' | 'dashboard' | 'logs'

interface AppStore {
  focusedProjectRoot: string | null
  setFocusedProjectRoot: (root: string | null) => void

  focusedTeamId: string | null
  setFocusedTeamId: (teamId: string | null) => void

  activeView: ActiveView
  setActiveView: (view: ActiveView) => void

  focusedRole: string | null
  setFocusedRole: (role: string | null) => void

  mailOpen: boolean
  setMailOpen: (open: boolean) => void

  artifactsOpen: boolean
  setArtifactsOpen: (open: boolean) => void

  paletteOpen: boolean
  setPaletteOpen: (open: boolean) => void

  planOpen: boolean
  setPlanOpen: (open: boolean) => void

  modelPickerTarget: { role: string; threadId: string } | null
  setModelPickerTarget: (target: { role: string; threadId: string } | null) => void

  reasoningPickerTarget: { role: string | null; threadId: string } | null
  setReasoningPickerTarget: (target: { role: string | null; threadId: string } | null) => void

  theme: Theme
  setTheme: (theme: Theme) => void
}

function loadTheme(): Theme {
  try {
    const stored = localStorage.getItem('agen8-theme')
    if (stored === 'dark' || stored === 'light' || stored === 'dim') return stored
  } catch {}
  return 'dark'
}

export const useStore = create<AppStore>((set) => ({
  focusedProjectRoot: null,
  setFocusedProjectRoot: (root) => set({ focusedProjectRoot: root, focusedTeamId: null, focusedRole: null, activeView: root ? 'overview' : 'project' }),

  focusedTeamId: null,
  setFocusedTeamId: (teamId) => set({ focusedTeamId: teamId, focusedRole: null, mailOpen: false, artifactsOpen: false, planOpen: false, modelPickerTarget: null, reasoningPickerTarget: null }),

  focusedRole: null,
  setFocusedRole: (role) => set({ focusedRole: role }),

  activeView: 'project',
  setActiveView: (view) => set({ activeView: view }),

  mailOpen: false,
  setMailOpen: (open) => set({ mailOpen: open }),

  artifactsOpen: false,
  setArtifactsOpen: (open) => set({ artifactsOpen: open }),

  paletteOpen: false,
  setPaletteOpen: (open) => set({ paletteOpen: open }),

  planOpen: false,
  setPlanOpen: (open) => set({ planOpen: open }),

  modelPickerTarget: null,
  setModelPickerTarget: (target) => set({ modelPickerTarget: target }),

  reasoningPickerTarget: null,
  setReasoningPickerTarget: (target) => set({ reasoningPickerTarget: target }),

  theme: loadTheme(),
  setTheme: (theme) => {
    localStorage.setItem('agen8-theme', theme)
    set({ theme })
  },
}))
