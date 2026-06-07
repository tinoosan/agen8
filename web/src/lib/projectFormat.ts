import type { ExecutionLocation } from './types'

/* ── Project + execution-location display formatters ──────────
   Shared by the projects list page (Project.tsx), its table rows,
   and the create-project wizard. Kept in one place so the path
   shortening and location-label logic stay consistent. */

export function shortenPath(path: string): string {
  const parts = path.split('/')
  if (parts.length <= 3) return path
  return '…/' + parts.slice(-2).join('/')
}

export function locationLabel(location: ExecutionLocation): string {
  if (location.label?.trim()) return location.label.trim()
  if (location.kind === 'local') return 'This machine'
  if (location.address?.host) return location.address.host
  return location.id
}

export function locationDescription(location: ExecutionLocation): string {
  if (location.kind === 'local') return 'Daemon machine'
  const address = location.address
  if (address?.host) {
    const user = address.username ? `${address.username}@` : ''
    const port = address.port ? `:${address.port}` : ''
    return `${user}${address.host}${port}`
  }
  if (location.kind === 'managed') return 'Managed location'
  return location.kind
}

export function locationStatusLabel(location: ExecutionLocation): string {
  if (location.ready) return 'Ready'
  if (location.lastProbe?.failureCode) return location.lastProbe.failureCode.replaceAll('_', ' ')
  return location.status.replaceAll('_', ' ')
}
