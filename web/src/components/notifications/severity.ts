import { AlertTriangle, AlertCircle, Info } from 'lucide-react'
import type { NotificationSeverity } from '../../lib/types'

/* Severity → icon + accent color. Single source of truth shared by the bell
 * inbox (NotificationInbox) and the dashboard "Needs attention" card so both
 * read with the same visual language as the rest of the app. */
export const SEVERITY_META: Record<
  NotificationSeverity,
  { Icon: typeof Info; color: string }
> = {
  info: { Icon: Info, color: 'var(--blue)' },
  warning: { Icon: AlertTriangle, color: 'var(--amber)' },
  critical: { Icon: AlertCircle, color: 'var(--red)' },
}
