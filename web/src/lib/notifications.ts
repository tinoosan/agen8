import type { NotificationItem } from './types'

/* Standing alerts are condition-based nudges that the backend auto-clears once
 * the underlying condition resolves (high backlog, a task stuck in the queue, a
 * task overrunning its time threshold) — as opposed to one-off events like
 * "task completed" or "task entered review".
 *
 * This list mirrors the backend's IsStandingTrigger
 * (internal/services/notification/domain/notification.go). Keep the two in sync:
 * if the backend adds a standing trigger, add it here too or the dashboard
 * "Needs attention" card will silently skip it. */
export const STANDING_TRIGGERS = new Set<string>([
  'backlog.high',
  'task.stale_queued',
  'task.overrun',
])

/** Reports whether a notification is a standing (condition-based) alert. */
export function isStandingNotification(n: NotificationItem): boolean {
  return STANDING_TRIGGERS.has(n.trigger)
}
