const ROLE_COLOR_PALETTE = [
  '#0f766e',
  '#2563eb',
  '#7c3aed',
  '#c2410c',
  '#be185d',
  '#0891b2',
  '#65a30d',
  '#b45309',
  '#dc2626',
  '#4338ca',
]

function hashRole(role: string): number {
  let hash = 0
  for (let i = 0; i < role.length; i += 1) {
    hash = ((hash << 5) - hash + role.charCodeAt(i)) | 0
  }
  return Math.abs(hash)
}

export function getRoleColor(role?: string): string {
  if (!role) return 'var(--accent)'
  return ROLE_COLOR_PALETTE[hashRole(role) % ROLE_COLOR_PALETTE.length]
}

export function getRoleTint(role?: string, strength = 14): string {
  const color = getRoleColor(role)
  return `color-mix(in srgb, ${color} ${strength}%, transparent)`
}
