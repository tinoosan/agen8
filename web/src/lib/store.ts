import { create } from 'zustand'

export type Theme = 'dark' | 'dim' | 'light'
export type DefaultProjectView = 'dashboard' | 'strategy'

interface AppStore {
  artifactsOpen: boolean
  setArtifactsOpen: (open: boolean) => void

  paletteOpen: boolean
  setPaletteOpen: (open: boolean) => void

  theme: Theme
  setTheme: (theme: Theme) => void

  defaultProjectView: DefaultProjectView
  setDefaultProjectView: (view: DefaultProjectView) => void

  /** Reset ephemeral UI state (called on navigation changes) */
  resetEphemeral: () => void
}

function loadTheme(): Theme {
  try {
    const stored = localStorage.getItem('agen8-theme')
    if (stored === 'dark' || stored === 'dim' || stored === 'light') return stored
  } catch {
    // Ignore localStorage access failures and fall back to the default theme.
  }
  return 'dark'
}

function loadDefaultProjectView(): DefaultProjectView {
  try {
    const stored = localStorage.getItem('agen8-default-project-view')
    if (stored === 'dashboard' || stored === 'strategy') return stored
  } catch {
    // Ignore localStorage access failures and fall back to dashboard.
  }
  return 'dashboard'
}

export const useStore = create<AppStore>((set) => ({
  artifactsOpen: false,
  setArtifactsOpen: (open) => set({ artifactsOpen: open }),

  paletteOpen: false,
  setPaletteOpen: (open) => set({ paletteOpen: open }),

  theme: loadTheme(),
  setTheme: (theme) => {
    localStorage.setItem('agen8-theme', theme)
    set({ theme })
  },

  defaultProjectView: loadDefaultProjectView(),
  setDefaultProjectView: (defaultProjectView) => {
    localStorage.setItem('agen8-default-project-view', defaultProjectView)
    set({ defaultProjectView })
  },

  resetEphemeral: () => set({
    artifactsOpen: false,
  }),
}))
