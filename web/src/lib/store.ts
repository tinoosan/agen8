import { create } from 'zustand'

interface AppStore {
  focusedTeamId: string | null
  setFocusedTeamId: (teamId: string | null) => void

  mailOpen: boolean
  setMailOpen: (open: boolean) => void

  artifactsOpen: boolean
  setArtifactsOpen: (open: boolean) => void

  paletteOpen: boolean
  setPaletteOpen: (open: boolean) => void
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
}))
