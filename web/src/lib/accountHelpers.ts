/**
 * Account display helpers — pure functions for resolving user identity
 * labels from auth state. Extracted from the sidebar so any component
 * can resolve a display name without importing the sidebar itself.
 */
import type { AuthUser } from './types'

export function accountDisplayName(user: AuthUser | null | undefined): string {
  const name = user?.name?.trim() ?? ''
  const email = user?.email?.trim() ?? ''
  if (name) return name
  if (email) return email
  return 'Account'
}
