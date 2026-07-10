import { create } from 'zustand'

export type Theme =
  | 'dark'
  | 'midnight'
  | 'dim'
  | 'nebula'
  | 'nord'
  | 'rose'
  | 'forest'
  | 'ember'
  | 'light'
  | 'sepia'
  | 'solarized'
export type DefaultProjectView = 'dashboard' | 'strategy'
export type FontFamily =
  | 'inter'
  | 'geist'
  | 'figtree'
  | 'space-grotesk'
  | 'atkinson'
  | 'system'
  | 'serif'
  | 'lora'
  | 'fraunces'
  | 'mono'

export const THEMES: Theme[] = [
  'dark', 'midnight', 'dim', 'nebula', 'nord', 'rose', 'forest', 'ember',
  'light', 'sepia', 'solarized',
]

/** Themes that render on a light canvas. Everything else is treated as dark.
 *  Single source of truth for icon-variant selection and the dark/light
 *  toggle, so consumers never hardcode `theme === 'light'` (which silently
 *  miscategorizes sepia/solarized). */
export const LIGHT_THEMES: Theme[] = ['light', 'sepia', 'solarized']

export function isLightTheme(theme: Theme): boolean {
  return (LIGHT_THEMES as string[]).includes(theme)
}
export const FONT_FAMILIES: FontFamily[] = [
  'inter', 'geist', 'figtree', 'space-grotesk', 'atkinson',
  'system', 'serif', 'lora', 'fraunces', 'mono',
]

/** Root font size, in px. The UI exposes this as a stepper; everything that
 *  scales with `rem` follows it. Clamped to [MIN, MAX] so the layout never
 *  breaks regardless of what lands in localStorage. */
export const FONT_SCALE_MIN = 13
export const FONT_SCALE_MAX = 20
export const FONT_SCALE_DEFAULT = 16
export const FONT_SCALE_STEP = 1

export interface UserPreferences {
  theme?: Theme
  lastDarkTheme?: Theme
  lastLightTheme?: Theme
  defaultProjectView?: DefaultProjectView
  fontFamily?: FontFamily
  fontScale?: number
}

function clampFontScale(value: number): number {
  if (!Number.isFinite(value)) return FONT_SCALE_DEFAULT
  return Math.min(FONT_SCALE_MAX, Math.max(FONT_SCALE_MIN, Math.round(value)))
}

interface AppStore {
  artifactsOpen: boolean
  setArtifactsOpen: (open: boolean) => void

  /** Context-map node search open state. Lifted into the store so the mobile
   *  top bar, dashboard search affordance, and GlobalNodeSearch keyboard
   *  wrapper can all open the same StrategyMapSearch UI. Pages with a local
   *  mount render it in place; other pages use the global wrapper. */
  strategySearchOpen: boolean
  setStrategySearchOpen: (open: boolean) => void

  theme: Theme
  setTheme: (theme: Theme) => void

  /** Last theme picked in each mode. The sidebar dark/light toggle restores
   *  these so flipping out of (say) nebula and back returns to nebula, not a
   *  generic 'dark'. */
  lastDarkTheme: Theme
  lastLightTheme: Theme
  toggleThemeMode: () => void

  defaultProjectView: DefaultProjectView
  setDefaultProjectView: (view: DefaultProjectView) => void

  fontFamily: FontFamily
  setFontFamily: (fontFamily: FontFamily) => void

  fontScale: number
  setFontScale: (fontScale: number) => void
  stepFontScale: (delta: number) => void
  resetFontScale: () => void

  applyUserPreferences: (preferences: UserPreferences) => void

  /** Reset ephemeral UI state (called on navigation changes) */
  resetEphemeral: () => void
}

function loadTheme(): Theme {
  try {
    const stored = localStorage.getItem('agen8-theme')
    if (stored && (THEMES as string[]).includes(stored)) return stored as Theme
  } catch {
    // Ignore localStorage access failures and fall back to the default theme.
  }
  return 'dark'
}

/** Resolve the remembered theme for one mode. Falls back to `seed` if it
 *  matches the requested mode, else to the generic theme for that mode — so a
 *  user whose only-ever theme is dark still gets a sensible 'light' to flip to. */
function loadModeTheme(key: string, seed: Theme, wantLight: boolean): Theme {
  try {
    const stored = localStorage.getItem(key)
    if (stored && (THEMES as string[]).includes(stored) && isLightTheme(stored as Theme) === wantLight) {
      return stored as Theme
    }
  } catch {
    // Ignore localStorage access failures and fall back below.
  }
  if (isLightTheme(seed) === wantLight) return seed
  return wantLight ? 'light' : 'dark'
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

function loadFontFamily(): FontFamily {
  try {
    const stored = localStorage.getItem('agen8-font-family')
    if (stored && (FONT_FAMILIES as string[]).includes(stored)) return stored as FontFamily
    // Migrate the legacy 'sans' value to the bundled Inter typeface.
    if (stored === 'sans') return 'inter'
  } catch {
    // Ignore localStorage access failures and fall back to the default font.
  }
  return 'inter'
}

function loadFontScale(): number {
  try {
    const stored = localStorage.getItem('agen8-font-scale')
    if (stored !== null) return clampFontScale(Number(stored))
    // Migrate the legacy sm/md/lg enum to its px equivalent.
    const legacy = localStorage.getItem('agen8-font-size')
    if (legacy === 'sm') return 15
    if (legacy === 'md') return 16
    if (legacy === 'lg') return 18
  } catch {
    // Ignore localStorage access failures and fall back to the default size.
  }
  return FONT_SCALE_DEFAULT
}

function persistFontScale(value: number) {
  try {
    localStorage.setItem('agen8-font-scale', String(value))
  } catch {
    // Ignore localStorage write failures — state still lives in memory.
  }
}

export const useStore = create<AppStore>((set, get) => ({
  artifactsOpen: false,
  setArtifactsOpen: (open) => set({ artifactsOpen: open }),

  strategySearchOpen: false,
  setStrategySearchOpen: (open) => set({ strategySearchOpen: open }),

  theme: loadTheme(),
  lastDarkTheme: loadModeTheme('agen8-theme-dark', loadTheme(), false),
  lastLightTheme: loadModeTheme('agen8-theme-light', loadTheme(), true),
  setTheme: (theme) => {
    localStorage.setItem('agen8-theme', theme)
    if (isLightTheme(theme)) {
      localStorage.setItem('agen8-theme-light', theme)
      set({ theme, lastLightTheme: theme })
    } else {
      localStorage.setItem('agen8-theme-dark', theme)
      set({ theme, lastDarkTheme: theme })
    }
  },
  toggleThemeMode: () => {
    const { theme, lastDarkTheme, lastLightTheme, setTheme } = get()
    setTheme(isLightTheme(theme) ? lastDarkTheme : lastLightTheme)
  },

  defaultProjectView: loadDefaultProjectView(),
  setDefaultProjectView: (defaultProjectView) => {
    localStorage.setItem('agen8-default-project-view', defaultProjectView)
    set({ defaultProjectView })
  },

  fontFamily: loadFontFamily(),
  setFontFamily: (fontFamily) => {
    localStorage.setItem('agen8-font-family', fontFamily)
    set({ fontFamily })
  },

  fontScale: loadFontScale(),
  setFontScale: (fontScale) => {
    const next = clampFontScale(fontScale)
    persistFontScale(next)
    set({ fontScale: next })
  },
  stepFontScale: (delta) => {
    const next = clampFontScale(get().fontScale + delta)
    persistFontScale(next)
    set({ fontScale: next })
  },
  resetFontScale: () => {
    persistFontScale(FONT_SCALE_DEFAULT)
    set({ fontScale: FONT_SCALE_DEFAULT })
  },

  applyUserPreferences: (preferences) => {
    const next: Partial<AppStore> = {}
    if (preferences.theme && (THEMES as string[]).includes(preferences.theme)) {
      next.theme = preferences.theme
      try {
        localStorage.setItem('agen8-theme', preferences.theme)
      } catch {
        // Ignore localStorage write failures — state still lives in memory.
      }
    }
    if (preferences.lastDarkTheme && (THEMES as string[]).includes(preferences.lastDarkTheme) && !isLightTheme(preferences.lastDarkTheme)) {
      next.lastDarkTheme = preferences.lastDarkTheme
      try {
        localStorage.setItem('agen8-theme-dark', preferences.lastDarkTheme)
      } catch {
        // Ignore localStorage write failures — state still lives in memory.
      }
    }
    if (preferences.lastLightTheme && (THEMES as string[]).includes(preferences.lastLightTheme) && isLightTheme(preferences.lastLightTheme)) {
      next.lastLightTheme = preferences.lastLightTheme
      try {
        localStorage.setItem('agen8-theme-light', preferences.lastLightTheme)
      } catch {
        // Ignore localStorage write failures — state still lives in memory.
      }
    }
    if (preferences.defaultProjectView === 'dashboard' || preferences.defaultProjectView === 'strategy') {
      next.defaultProjectView = preferences.defaultProjectView
      try {
        localStorage.setItem('agen8-default-project-view', preferences.defaultProjectView)
      } catch {
        // Ignore localStorage write failures — state still lives in memory.
      }
    }
    if (preferences.fontFamily && (FONT_FAMILIES as string[]).includes(preferences.fontFamily)) {
      next.fontFamily = preferences.fontFamily
      try {
        localStorage.setItem('agen8-font-family', preferences.fontFamily)
      } catch {
        // Ignore localStorage write failures — state still lives in memory.
      }
    }
    if (preferences.fontScale !== undefined) {
      const fontScale = clampFontScale(preferences.fontScale)
      next.fontScale = fontScale
      persistFontScale(fontScale)
    }
    set(next)
  },

  resetEphemeral: () => set({
    artifactsOpen: false,
    strategySearchOpen: false,
  }),
}))
