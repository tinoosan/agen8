import { useNavigation } from '../lib/routing'

/**
 * Returns the routing user identity for the notifications inbox.
 *
 * Backend evaluators write `notifications.user_id` from
 * `event.Data["userId"]` if stamped, else fall back to
 * `event.Data["projectId"]`. In single-user-local mode (where this
 * codebase runs today) those collapse to the project ID — so this
 * hook returns the project ID and the read/write sides agree.
 *
 * Hosted multi-user mode will replace this with a real auth-context
 * user lookup. The notification surfaces should call this hook
 * (not `useProfileId`, which resolves an agent persona slug — the
 * conflation that motivated the identity-model refactor).
 */
export function useNotificationUserId(): string | null {
  const { projectId } = useNavigation()
  const trimmed = (projectId ?? '').trim()
  return trimmed === '' ? null : trimmed
}
