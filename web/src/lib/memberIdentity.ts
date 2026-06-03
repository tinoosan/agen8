/**
 * Deterministic visual identity for space members.
 *
 * Members don't yet have user-controlled icon/color customization (no
 * space.member.update RPC). To give them visual differentiation in
 * chat bubbles and channel rows, this module derives a stable
 * (color, icon) pair from the member's id via a fast string hash.
 *
 * Same memberId always produces the same identity, so a member looks
 * the same everywhere they appear. When user-controlled customization
 * lands later, the resolution will fall back to this when no
 * customization is set, so unconfigured members still differentiate.
 */
import type { LucideIcon } from 'lucide-react'
import {
  User, UserCircle, UserCog, UserRound, Bot, Brain, Sparkles, Zap,
} from 'lucide-react'

import { SPACE_COLOR_KEYS, type SpaceColorKey } from './types'
import { spaceColorVar } from './spaceCustomization'

/**
 * Curated set of member glyphs. Kept small so any single member is
 * easy to pick out from the others. Indexed by hash modulo length.
 */
const MEMBER_ICON_POOL: ReadonlyArray<LucideIcon> = [
  User,
  UserCircle,
  UserRound,
  UserCog,
  Bot,
  Brain,
  Sparkles,
  Zap,
]

/**
 * Color pool reuses the existing space palette so member identities
 * sit naturally next to space identities without introducing a second
 * vocabulary of colors.
 */
const MEMBER_COLOR_POOL: ReadonlyArray<SpaceColorKey> = SPACE_COLOR_KEYS

/**
 * 32-bit FNV-1a hash. Fast, deterministic, well-distributed across
 * short strings — exactly what we need for picking pool indices.
 */
function hashString(input: string): number {
  let hash = 0x811c9dc5
  for (let i = 0; i < input.length; i++) {
    hash ^= input.charCodeAt(i)
    hash = Math.imul(hash, 0x01000193) >>> 0
  }
  return hash >>> 0
}

export interface MemberIdentity {
  icon: LucideIcon
  colorKey: SpaceColorKey
  /** Resolved CSS color (theme-aware) for the colorKey. */
  colorVar: string
}

/**
 * Resolve a member's visual identity from their id. Falls back to the
 * first pool entry for empty/null ids so the UI never crashes on a
 * missing identity.
 */
export function resolveMemberIdentity(memberId: string | null | undefined): MemberIdentity {
  const id = (memberId ?? '').trim()
  if (!id) {
    return {
      icon: MEMBER_ICON_POOL[0],
      colorKey: MEMBER_COLOR_POOL[0],
      colorVar: spaceColorVar(MEMBER_COLOR_POOL[0]) ?? 'var(--text-3)',
    }
  }
  const h = hashString(id)
  // Use different bit-slices for icon vs color so members that
  // happen to land on the same icon don't also collide on color.
  const icon = MEMBER_ICON_POOL[h % MEMBER_ICON_POOL.length]
  const colorKey = MEMBER_COLOR_POOL[(h >>> 8) % MEMBER_COLOR_POOL.length]
  return {
    icon,
    colorKey,
    colorVar: spaceColorVar(colorKey) ?? 'var(--text-3)',
  }
}
