import type { OpActionStatus } from '../../lib/types'

export function statusConfig(status: OpActionStatus): { label: string; color: string; bg: string } {
  switch (status) {
    case 'pending': return { label: 'Pending', color: 'var(--amber)', bg: 'var(--amber-dim)' }
    case 'acknowledged': return { label: 'Seen', color: 'var(--blue)', bg: 'var(--blue-dim, rgba(96,165,250,0.12))' }
    case 'in_progress': return { label: 'In Progress', color: 'var(--accent)', bg: 'rgba(59,130,246,0.12)' }
    case 'pending_verification': return { label: 'Awaiting Verification', color: 'var(--amber)', bg: 'var(--amber-dim)' }
    case 'completed': return { label: 'Completed', color: 'var(--green)', bg: 'var(--green-dim)' }
    case 'blocked': return { label: 'Blocked', color: 'var(--red)', bg: 'var(--red-dim)' }
    case 'canceled': return { label: 'Canceled', color: 'var(--text-3)', bg: 'var(--bg-surface)' }
    default: return { label: status, color: 'var(--text-3)', bg: 'var(--bg-surface)' }
  }
}

export function timeAgo(iso: string | undefined): string {
  if (!iso) return ''
  const diffMs = Date.now() - new Date(iso).getTime()
  if (diffMs < 0) return 'just now'
  const seconds = Math.floor(diffMs / 1000)
  if (seconds < 60) return `${seconds}s ago`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  return `${Math.floor(hours / 24)}d ago`
}
