import { vscDarkPlus, oneLight } from 'react-syntax-highlighter/dist/esm/styles/prism'
import { isLightTheme, type Theme } from '../../lib/store'

/**
 * Prism style matching the app theme. LIGHT_THEMES (light/sepia/solarized)
 * get a light token palette; every dark theme keeps vscDarkPlus. Keyed off
 * isLightTheme so new themes categorize automatically.
 */
export function prismStyleFor(theme: Theme) {
  return isLightTheme(theme) ? oneLight : vscDarkPlus
}
