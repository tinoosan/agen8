/**
 * Curated icon and color sets for project customization.
 *
 * Project.customization.icon stores a Lucide icon *name* (a key in
 * PROJECT_ICONS); customization.color stores a hex value from PROJECT_COLORS.
 * Both sets are intentionally curated rather than open-ended: a fixed Lucide
 * subset keeps project glyphs consistent with the app's line-icon style, and a
 * fixed palette guarantees swatches read legibly in both light and dark mode.
 *
 * Rendering and the picker share these sets, so anything the picker can set is
 * exactly what the avatar can render — an unknown/blank icon name falls back to
 * a monogram, and a blank color falls back to a neutral token.
 */
import {
  Folder, Rocket, Code, Terminal, Database, Cloud, Server, Cpu, GitBranch,
  Layers, FlaskConical, Bug, Hammer, BookOpen, Globe, Zap, Star, Flag,
  type LucideIcon,
} from 'lucide-react'

// The curated icon set, keyed by the name persisted in customization.icon.
// Keys are stable identifiers — renaming one would orphan existing projects, so
// treat them as a small append-only vocabulary.
export const PROJECT_ICONS: Record<string, LucideIcon> = {
  folder: Folder,
  rocket: Rocket,
  code: Code,
  terminal: Terminal,
  database: Database,
  cloud: Cloud,
  server: Server,
  cpu: Cpu,
  branch: GitBranch,
  layers: Layers,
  flask: FlaskConical,
  bug: Bug,
  hammer: Hammer,
  book: BookOpen,
  globe: Globe,
  zap: Zap,
  star: Star,
  flag: Flag,
}

// Ordered names for rendering the picker grid in a stable layout.
export const PROJECT_ICON_NAMES: string[] = Object.keys(PROJECT_ICONS)

export interface ProjectColor {
  name: string
  value: string
}

// Theme-friendly palette. Values are mid-tone hexes chosen to keep enough
// contrast as both the glyph color and a low-alpha background tint in light and
// dark mode (the avatar mixes the color with the surface at ~16%).
export const PROJECT_COLORS: ProjectColor[] = [
  { name: 'slate', value: '#64748b' },
  { name: 'red', value: '#ef4444' },
  { name: 'orange', value: '#f97316' },
  { name: 'amber', value: '#f59e0b' },
  { name: 'green', value: '#22c55e' },
  { name: 'teal', value: '#14b8a6' },
  { name: 'blue', value: '#3b82f6' },
  { name: 'indigo', value: '#6366f1' },
  { name: 'violet', value: '#8b5cf6' },
  { name: 'pink', value: '#ec4899' },
]

// The color an avatar uses when a project has no custom color set: a neutral
// token rather than a palette hue, so an un-customized project reads as neutral.
export const DEFAULT_PROJECT_COLOR = 'var(--text-3)'
