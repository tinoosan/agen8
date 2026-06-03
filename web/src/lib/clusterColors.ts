/**
 * Cluster colour palette shared between the strategy map and dashboard.
 * Each mission cluster receives a distinct hue. KRs, tasks, and decisions
 * inherit their parent's colour, creating visual coherence across views.
 */
export const CLUSTER_PALETTE = [
  'var(--blue)',
  'var(--green)',
  'var(--amber)',
  'hsl(280, 55%, 65%)',   // purple
  'var(--red)',
  'hsl(175, 50%, 50%)',   // teal
  'hsl(330, 55%, 60%)',   // rose
  'hsl(200, 60%, 55%)',   // sky
]

function hashString(str: string): number {
  let hash = 0
  for (let i = 0; i < str.length; i++) {
    hash = ((hash << 5) - hash) + str.charCodeAt(i)
    hash |= 0
  }
  return Math.abs(hash)
}

/** Deterministic mapping from an identity string to a cluster colour. */
export function sourceToClusterColor(identity: string): string {
  return CLUSTER_PALETTE[hashString(identity) % CLUSTER_PALETTE.length]
}
