import { create } from 'zustand'

export type Theme = 'dark' | 'light' | 'dim'

interface AppStore {
  focusedTeamId: string | null
  setFocusedTeamId: (teamId: string | null) => void

  mailOpen: boolean
  setMailOpen: (open: boolean) => void

  artifactsOpen: boolean
  setArtifactsOpen: (open: boolean) => void

  paletteOpen: boolean
  setPaletteOpen: (open: boolean) => void

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
  focusedTeamId: null,
  setFocusedTeamId: (teamId) => set({ focusedTeamId: teamId, mailOpen: false, artifactsOpen: false }),

  mailOpen: false,
  setMailOpen: (open) => set({ mailOpen: open }),

  artifactsOpen: false,
  setArtifactsOpen: (open) => set({ artifactsOpen: open }),

  paletteOpen: false,
  setPaletteOpen: (open) => set({ paletteOpen: open }),

  theme: loadTheme(),
  setTheme: (theme) => {
    localStorage.setItem('agen8-theme', theme)
    set({ theme })
  },
}))
