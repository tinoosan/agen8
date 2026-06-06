import type { Theme } from './store'

/**
 * Per-theme agen8 brand icon. Each `agen8-icon-<theme>.svg` is the same mark
 * geometry with the theme's background, hexagon, and accent colors swapped in.
 *
 * Vite resolves this glob to hashed asset URLs at build time (eager, so there's
 * no async import), giving us a `theme → url` map that works in dev and prod.
 */
const icons = import.meta.glob('../assets/brand/agen8-icon-*.svg', {
  eager: true,
  query: '?url',
  import: 'default',
}) as Record<string, string>

const byTheme: Record<string, string> = {}
for (const [path, url] of Object.entries(icons)) {
  const match = path.match(/agen8-icon-(.+)\.svg$/)
  if (match) byTheme[match[1]] = url
}

/** Brand icon URL for a theme, falling back to the default 'dark' variant. */
export function brandIconFor(theme: Theme): string {
  return byTheme[theme] ?? byTheme.dark
}
