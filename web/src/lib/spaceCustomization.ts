/**
 * Space customization helpers — icon registry, color palette resolution,
 * and shared shape definitions for the picker UI.
 *
 * Why curated: Lucide ships ~1700 icons. Importing them all bloats the
 * bundle by ~600KB (gzip ~150KB) for an "explorer" feature most users
 * touch once per space. This curated list of ~120 covers the top-of-mind
 * choices grouped by what spaces typically represent in agen8 (a
 * project, a process, a domain, a person), and search filters within
 * the curated set. If users hit the limit we can expand or layer
 * dynamic-import on top later.
 *
 * The PascalCase imports below match Lucide's named exports. Each entry
 * stores the kebab-case name (what we persist on the backend) alongside
 * the React component (what we render).
 */
import {
  // ── Work & projects
  Briefcase, FolderKanban, FolderOpen, Folder, FolderTree, ClipboardList,
  Target, Goal, Flag, Trophy, Bookmark, Tag, Tags,
  // ── Tech & engineering
  Code, Code2, Terminal, Cpu, Server, Database, Cloud, CloudCog,
  Bug, GitBranch, GitMerge, GitPullRequest, Wrench, Hammer, Cog, Settings, Settings2,
  Boxes, Container, Layers, Component, Workflow, Network, Plug, Webhook,
  Zap, Atom, Binary, Braces, FileCode, FileTerminal,
  // ── Communication & people
  MessageSquare, MessagesSquare, Mail, Send, Inbox,
  Users, User, UserPlus, UsersRound, Headphones, Megaphone, Speech,
  // ── Data & analytics
  ChartBar, ChartLine, ChartPie, ChartArea, TrendingUp, BarChart3, LineChart,
  Activity, Gauge, Calculator, Sigma, Table, FileSpreadsheet,
  // ── Content & writing
  FileText, FilePen, BookOpen, Book, Library, Newspaper, NotebookPen,
  PenLine, Pen, Highlighter, Quote, Type, Hash,
  // ── Science & research
  Microscope, FlaskConical, TestTube, Telescope, Lightbulb, Brain, GraduationCap, Search, Compass,
  // ── Money & business
  DollarSign, CreditCard, Wallet, PiggyBank, Banknote, Receipt, ShoppingCart, ShoppingBag, Store,
  // ── Time & planning
  Calendar, CalendarDays, Clock, Timer, Hourglass, AlarmClock,
  // ── Travel & places
  Map as MapIcon, MapPin, Globe, Plane, Car, Ship, Train,
  // ── Nature & weather
  Leaf, Trees, Sun, Moon, Star, Sparkles, Flame, Mountain, Waves, Cloud as CloudIcon,
  // ── Objects & symbols
  Rocket, Bookmark as BookmarkIcon, Heart, Lock, Key, Shield, ShieldCheck,
  Gift, Crown, Award, Diamond, Anchor, Puzzle, Music, Image as ImageIcon,
  Box,
} from 'lucide-react'
import type { LucideIcon } from 'lucide-react'

import { SPACE_COLOR_KEYS } from './types'
import type { SpaceColorKey, SpaceCustomization } from './types'

/* ------------------------------------------------------------------ */
/* Icon registry                                                       */
/* ------------------------------------------------------------------ */

/**
 * Category groupings for the picker's default grid view. Each entry
 * pairs the kebab-case Lucide name (persisted) with the imported
 * component (rendered). Adding to this list expands what the picker
 * shows; removing entries is safe because rendering always falls back
 * to the default Box icon when a name doesn't resolve.
 */
export const SPACE_ICON_GROUPS: ReadonlyArray<{
  label: string
  icons: ReadonlyArray<{ name: string; component: LucideIcon }>
}> = [
  {
    label: 'Work',
    icons: [
      { name: 'briefcase', component: Briefcase },
      { name: 'folder-kanban', component: FolderKanban },
      { name: 'folder-open', component: FolderOpen },
      { name: 'folder', component: Folder },
      { name: 'folder-tree', component: FolderTree },
      { name: 'clipboard-list', component: ClipboardList },
      { name: 'target', component: Target },
      { name: 'goal', component: Goal },
      { name: 'flag', component: Flag },
      { name: 'trophy', component: Trophy },
      { name: 'bookmark', component: Bookmark },
      { name: 'tag', component: Tag },
      { name: 'tags', component: Tags },
    ],
  },
  {
    label: 'Engineering',
    icons: [
      { name: 'code', component: Code },
      { name: 'code-2', component: Code2 },
      { name: 'terminal', component: Terminal },
      { name: 'cpu', component: Cpu },
      { name: 'server', component: Server },
      { name: 'database', component: Database },
      { name: 'cloud', component: Cloud },
      { name: 'cloud-cog', component: CloudCog },
      { name: 'bug', component: Bug },
      { name: 'git-branch', component: GitBranch },
      { name: 'git-merge', component: GitMerge },
      { name: 'git-pull-request', component: GitPullRequest },
      { name: 'wrench', component: Wrench },
      { name: 'hammer', component: Hammer },
      { name: 'cog', component: Cog },
      { name: 'settings', component: Settings },
      { name: 'settings-2', component: Settings2 },
      { name: 'boxes', component: Boxes },
      { name: 'container', component: Container },
      { name: 'layers', component: Layers },
      { name: 'component', component: Component },
      { name: 'workflow', component: Workflow },
      { name: 'network', component: Network },
      { name: 'plug', component: Plug },
      { name: 'webhook', component: Webhook },
      { name: 'zap', component: Zap },
      { name: 'atom', component: Atom },
      { name: 'binary', component: Binary },
      { name: 'braces', component: Braces },
      { name: 'file-code', component: FileCode },
      { name: 'file-terminal', component: FileTerminal },
    ],
  },
  {
    label: 'People',
    icons: [
      { name: 'message-square', component: MessageSquare },
      { name: 'messages-square', component: MessagesSquare },
      { name: 'mail', component: Mail },
      { name: 'send', component: Send },
      { name: 'inbox', component: Inbox },
      { name: 'users', component: Users },
      { name: 'user', component: User },
      { name: 'user-plus', component: UserPlus },
      { name: 'users-round', component: UsersRound },
      { name: 'headphones', component: Headphones },
      { name: 'megaphone', component: Megaphone },
      { name: 'speech', component: Speech },
    ],
  },
  {
    label: 'Data',
    icons: [
      { name: 'chart-bar', component: ChartBar },
      { name: 'chart-line', component: ChartLine },
      { name: 'chart-pie', component: ChartPie },
      { name: 'chart-area', component: ChartArea },
      { name: 'trending-up', component: TrendingUp },
      { name: 'bar-chart-3', component: BarChart3 },
      { name: 'line-chart', component: LineChart },
      { name: 'activity', component: Activity },
      { name: 'gauge', component: Gauge },
      { name: 'calculator', component: Calculator },
      { name: 'sigma', component: Sigma },
      { name: 'table', component: Table },
      { name: 'file-spreadsheet', component: FileSpreadsheet },
    ],
  },
  {
    label: 'Writing',
    icons: [
      { name: 'file-text', component: FileText },
      { name: 'file-pen', component: FilePen },
      { name: 'book-open', component: BookOpen },
      { name: 'book', component: Book },
      { name: 'library', component: Library },
      { name: 'newspaper', component: Newspaper },
      { name: 'notebook-pen', component: NotebookPen },
      { name: 'pen-line', component: PenLine },
      { name: 'pen', component: Pen },
      { name: 'highlighter', component: Highlighter },
      { name: 'quote', component: Quote },
      { name: 'type', component: Type },
      { name: 'hash', component: Hash },
    ],
  },
  {
    label: 'Research',
    icons: [
      { name: 'microscope', component: Microscope },
      { name: 'flask-conical', component: FlaskConical },
      { name: 'test-tube', component: TestTube },
      { name: 'telescope', component: Telescope },
      { name: 'lightbulb', component: Lightbulb },
      { name: 'brain', component: Brain },
      { name: 'graduation-cap', component: GraduationCap },
      { name: 'search', component: Search },
      { name: 'compass', component: Compass },
    ],
  },
  {
    label: 'Money',
    icons: [
      { name: 'dollar-sign', component: DollarSign },
      { name: 'credit-card', component: CreditCard },
      { name: 'wallet', component: Wallet },
      { name: 'piggy-bank', component: PiggyBank },
      { name: 'banknote', component: Banknote },
      { name: 'receipt', component: Receipt },
      { name: 'shopping-cart', component: ShoppingCart },
      { name: 'shopping-bag', component: ShoppingBag },
      { name: 'store', component: Store },
    ],
  },
  {
    label: 'Time',
    icons: [
      { name: 'calendar', component: Calendar },
      { name: 'calendar-days', component: CalendarDays },
      { name: 'clock', component: Clock },
      { name: 'timer', component: Timer },
      { name: 'hourglass', component: Hourglass },
      { name: 'alarm-clock', component: AlarmClock },
    ],
  },
  {
    label: 'Travel',
    icons: [
      { name: 'map', component: MapIcon },
      { name: 'map-pin', component: MapPin },
      { name: 'globe', component: Globe },
      { name: 'plane', component: Plane },
      { name: 'car', component: Car },
      { name: 'ship', component: Ship },
      { name: 'train', component: Train },
    ],
  },
  {
    label: 'Nature',
    icons: [
      { name: 'leaf', component: Leaf },
      { name: 'trees', component: Trees },
      { name: 'sun', component: Sun },
      { name: 'moon', component: Moon },
      { name: 'star', component: Star },
      { name: 'sparkles', component: Sparkles },
      { name: 'flame', component: Flame },
      { name: 'mountain', component: Mountain },
      { name: 'waves', component: Waves },
      { name: 'cloud-icon', component: CloudIcon },
    ],
  },
  {
    label: 'Symbols',
    icons: [
      { name: 'rocket', component: Rocket },
      { name: 'bookmark-icon', component: BookmarkIcon },
      { name: 'heart', component: Heart },
      { name: 'lock', component: Lock },
      { name: 'key', component: Key },
      { name: 'shield', component: Shield },
      { name: 'shield-check', component: ShieldCheck },
      { name: 'gift', component: Gift },
      { name: 'crown', component: Crown },
      { name: 'award', component: Award },
      { name: 'diamond', component: Diamond },
      { name: 'anchor', component: Anchor },
      { name: 'puzzle', component: Puzzle },
      { name: 'music', component: Music },
      { name: 'image', component: ImageIcon },
    ],
  },
]

/**
 * Flat lookup: kebab-case name → component. Built once at module load
 * by walking SPACE_ICON_GROUPS.
 */
export const SPACE_ICON_REGISTRY: ReadonlyMap<string, LucideIcon> = new Map(
  SPACE_ICON_GROUPS.flatMap(group =>
    group.icons.map(({ name, component }) => [name, component] as const),
  ),
)

/**
 * Default icon shown when a customization is unset or the stored name
 * doesn't resolve. Kept as a separate export so the sidebar can render
 * the default in muted text color while a real picked icon renders in
 * the chosen accent color.
 */
export const DEFAULT_SPACE_ICON: LucideIcon = Box

/**
 * Resolve a kebab-case Lucide name to a component, falling back to the
 * default Box icon. Returns the default for nullish/empty names.
 */
export function resolveSpaceIcon(name: string | undefined | null): LucideIcon {
  if (!name) return DEFAULT_SPACE_ICON
  return SPACE_ICON_REGISTRY.get(name) ?? DEFAULT_SPACE_ICON
}

/* ------------------------------------------------------------------ */
/* Color palette                                                       */
/* ------------------------------------------------------------------ */

/**
 * Display label + visual swatch CSS for each palette key. Labels are
 * shown as accessible names in the picker; the CSS variable resolves
 * against the active theme in index.css.
 */
export const SPACE_COLOR_PALETTE: ReadonlyArray<{
  key: SpaceColorKey
  label: string
}> = [
  { key: 'slate', label: 'Slate' },
  { key: 'blue', label: 'Blue' },
  { key: 'violet', label: 'Violet' },
  { key: 'green', label: 'Green' },
  { key: 'amber', label: 'Amber' },
  { key: 'orange', label: 'Orange' },
  { key: 'rose', label: 'Rose' },
  { key: 'pink', label: 'Pink' },
]

/**
 * Resolve a color key to a CSS color value. Returns undefined for
 * unrecognized keys (callers should treat undefined as "no accent").
 */
export function spaceColorVar(key: string | undefined | null): string | undefined {
  if (!key) return undefined
  if (!isSpaceColorKey(key)) return undefined
  return `var(--space-color-${key})`
}

export function isSpaceColorKey(value: string): value is SpaceColorKey {
  return (SPACE_COLOR_KEYS as ReadonlyArray<string>).includes(value)
}

/* ------------------------------------------------------------------ */
/* Shared types                                                        */
/* ------------------------------------------------------------------ */

export type { SpaceCustomization, SpaceColorKey }

/**
 * Filter the curated icon registry by a search query. Empty query
 * returns null (caller renders the grouped default view); non-empty
 * returns a flat list of matches by kebab-case substring match.
 */
export function searchSpaceIcons(query: string): Array<{ name: string; component: LucideIcon }> | null {
  const trimmed = query.trim().toLowerCase()
  if (!trimmed) return null
  const results: Array<{ name: string; component: LucideIcon }> = []
  for (const group of SPACE_ICON_GROUPS) {
    for (const entry of group.icons) {
      if (entry.name.includes(trimmed)) {
        results.push({ name: entry.name, component: entry.component })
      }
    }
  }
  return results
}
