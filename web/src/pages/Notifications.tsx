import { useState } from 'react'
import { useLocation } from 'wouter'
import { Bell, Filter, Settings, CheckCheck } from 'lucide-react'
import { CustomSelect } from '../components/fields'
import { useNotificationUserId } from '../hooks/useNotificationUserId'
import {
  useNotifications,
  useMarkRead,
  useMarkAllRead,
  useDismissNotification,
} from '../hooks/useNotifications'
import NotificationPreferences from '../components/notifications/NotificationPreferences'
import type { NotificationItem, NotificationSeverity } from '../lib/types'

const severityColors: Record<NotificationSeverity, string> = {
  critical: 'var(--danger, #ef4444)',
  warning: 'var(--warning, #f59e0b)',
  info: 'var(--info, #3b82f6)',
}

function formatTriggerLabel(trigger: string): string | null {
  switch (trigger) {
    case 'schedule_expiring':
      return 'Schedule expiring'
    case 'schedule_expired':
      return 'Schedule expired'
    case 'schedule_completed':
      return 'Schedule completed'
    default:
      return null
  }
}

function SeverityBadge({ severity }: { severity: NotificationSeverity }) {
  return (
    <span
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: 4,
        padding: '2px 8px',
        borderRadius: 9999,
        fontSize: '11px',
        fontWeight: 600,
        background: `${severityColors[severity]}20`,
        color: severityColors[severity],
        textTransform: 'uppercase',
      }}
    >
      <span style={{
        width: 6, height: 6, borderRadius: '50%',
        background: severityColors[severity],
      }} />
      {severity}
    </span>
  )
}

function relativeTime(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime()
  const seconds = Math.floor(diff / 1000)
  if (seconds < 60) return 'just now'
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  const days = Math.floor(hours / 24)
  return `${days}d ago`
}

/**
 * Full notification history page with filtering by source, severity, and read state.
 * Includes a preferences tab for rule configuration.
 */
export default function Notifications() {
  const [, navigate] = useLocation()
  const userId = useNotificationUserId()

  const [tab, setTab] = useState<'history' | 'preferences'>('history')
  const [sourceFilter, setSourceFilter] = useState('')
  const [severityFilter, setSeverityFilter] = useState('')
  const [unreadOnly, setUnreadOnly] = useState(false)

  const { data: notifications = [], isLoading } = useNotifications({
    userId,
    source: sourceFilter || undefined,
    severity: severityFilter || undefined,
    unread: unreadOnly || undefined,
    limit: 100,
  })

  const markRead = useMarkRead()
  const markAllRead = useMarkAllRead()
  const dismissNotification = useDismissNotification()

  function handleClick(n: NotificationItem) {
    if (!n.readAt) markRead.mutate(n.id)
    if (n.link?.url) navigate(n.link.url)
  }

  return (
    <div style={{ padding: '24px 32px', maxWidth: 900 }}>
      {/* Page header */}
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 24 }}>
        <h1 style={{ fontSize: '20px', fontWeight: 600, display: 'flex', alignItems: 'center', gap: 8 }}>
          <Bell size={20} />
          Notifications
        </h1>
        <div style={{ display: 'flex', gap: 8 }}>
          <button
            onClick={() => setTab('history')}
            style={{
              padding: '6px 12px', borderRadius: 6, fontSize: '13px', cursor: 'pointer',
              background: tab === 'history' ? 'var(--accent, #3b82f6)' : 'transparent',
              color: tab === 'history' ? 'white' : 'var(--text-secondary)',
              border: tab === 'history' ? 'none' : '1px solid var(--border)',
            }}
          >
            History
          </button>
          <button
            onClick={() => setTab('preferences')}
            style={{
              padding: '6px 12px', borderRadius: 6, fontSize: '13px', cursor: 'pointer',
              display: 'flex', alignItems: 'center', gap: 4,
              background: tab === 'preferences' ? 'var(--accent, #3b82f6)' : 'transparent',
              color: tab === 'preferences' ? 'white' : 'var(--text-secondary)',
              border: tab === 'preferences' ? 'none' : '1px solid var(--border)',
            }}
          >
            <Settings size={13} />
            Preferences
          </button>
        </div>
      </div>

      {tab === 'preferences' && (
        <NotificationPreferences userId={userId} />
      )}

      {tab === 'history' && (
        <>
          {/* Filters */}
          <div style={{ display: 'flex', gap: 12, marginBottom: 16, alignItems: 'center', flexWrap: 'wrap' }}>
            <Filter size={14} style={{ color: 'var(--text-tertiary)' }} />

            <CustomSelect
              value={sourceFilter}
              onChange={setSourceFilter}
              className="text-xs px-2 py-1 bg-[var(--bg-surface)] border border-[var(--border)] rounded-[6px] flex items-center gap-2 cursor-pointer"
              options={[
                { value: '', label: 'All sources' },
                { value: 'heartbeat', label: 'Heartbeat' },
                { value: 'task', label: 'Task' },
              ]}
            />

            <CustomSelect
              value={severityFilter}
              onChange={setSeverityFilter}
              className="text-xs px-2 py-1 bg-[var(--bg-surface)] border border-[var(--border)] rounded-[6px] flex items-center gap-2 cursor-pointer"
              options={[
                { value: '', label: 'All severities' },
                { value: 'critical', label: 'Critical' },
                { value: 'warning', label: 'Warning' },
                { value: 'info', label: 'Info' },
              ]}
            />

            <label style={{ display: 'flex', alignItems: 'center', gap: 4, fontSize: '12px', cursor: 'pointer' }}>
              <input
                type="checkbox"
                checked={unreadOnly}
                onChange={(e) => setUnreadOnly(e.target.checked)}
              />
              Unread only
            </label>

            <div style={{ flex: 1 }} />

            <button
              onClick={() => userId && markAllRead.mutate(userId)}
              disabled={!userId}
              style={{
                display: 'flex', alignItems: 'center', gap: 4,
                background: 'none', border: '1px solid var(--border)',
                borderRadius: 6, padding: '4px 10px', cursor: 'pointer',
                fontSize: '12px', color: 'var(--text-secondary)',
              }}
            >
              <CheckCheck size={13} />
              Mark all read
            </button>
          </div>

          {/* Notification list */}
          {isLoading && (
            <div style={{ padding: 32, textAlign: 'center' }}>
              <span className="spinner spinner-md" />
            </div>
          )}

          {!isLoading && notifications.length === 0 && (
            <div style={{
              padding: '48px 24px', textAlign: 'center',
              color: 'var(--text-tertiary)', fontSize: '14px',
              border: '1px dashed var(--border)', borderRadius: 8,
            }}>
              <Bell size={32} style={{ marginBottom: 12, opacity: 0.3 }} />
              <div>No notifications match your filters</div>
              <div style={{ fontSize: '12px', marginTop: 4 }}>
                Notifications appear here when heartbeat events require operator attention.
              </div>
            </div>
          )}

          <div style={{
            border: notifications.length > 0 ? '1px solid var(--border)' : 'none',
            borderRadius: 8,
            overflow: 'hidden',
          }}>
            {notifications.map((n, idx) => (
              <div
                key={n.id}
                onClick={() => handleClick(n)}
                style={{
                  display: 'flex',
                  gap: 12,
                  padding: '12px 16px',
                  cursor: n.link?.url ? 'pointer' : 'default',
                  borderBottom: idx < notifications.length - 1 ? '1px solid var(--border-subtle, #f3f4f6)' : 'none',
                  background: n.readAt ? 'transparent' : 'var(--bg-muted, #f9fafb)',
                  transition: 'background 0.15s',
                }}
                onMouseEnter={(e) => { if (n.link?.url) e.currentTarget.style.background = 'var(--bg-hover, #f3f4f6)' }}
                onMouseLeave={(e) => { e.currentTarget.style.background = n.readAt ? 'transparent' : 'var(--bg-muted, #f9fafb)' }}
              >
                <div style={{ paddingTop: 2 }}>
                  <SeverityBadge severity={n.severity} />
                </div>
                <div style={{ flex: 1, minWidth: 0 }}>
                  <div style={{ fontSize: '13px', fontWeight: n.readAt ? 400 : 600, lineHeight: 1.4 }}>
                    {n.title}
                  </div>
                  {formatTriggerLabel(n.trigger) && (
                    <div style={{ marginTop: 4 }}>
                      <span
                        style={{
                          display: 'inline-flex',
                          alignItems: 'center',
                          gap: 4,
                          padding: '2px 8px',
                          borderRadius: 9999,
                          fontSize: '10px',
                          fontWeight: 600,
                          textTransform: 'uppercase',
                          background: 'color-mix(in srgb, var(--warning, #f59e0b) 12%, transparent)',
                          color: 'var(--warning, #f59e0b)',
                        }}
                      >
                        {formatTriggerLabel(n.trigger)}
                      </span>
                    </div>
                  )}
                  {n.body && (
                    <div style={{
                      fontSize: '12px', color: 'var(--text-secondary)', marginTop: 2,
                      lineHeight: 1.4, maxHeight: '2.8em', overflow: 'hidden',
                    }}>
                      {n.body}
                    </div>
                  )}
                  <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginTop: 6, fontSize: '11px', color: 'var(--text-tertiary)' }}>
                    <span>{relativeTime(n.createdAt)}</span>
                    <span style={{ opacity: 0.4 }}>|</span>
                    <span>{n.source}</span>
                    {n.link?.url && (
                      <>
                        <span style={{ opacity: 0.4 }}>|</span>
                        <span>View in {n.link.surface}</span>
                      </>
                    )}
                  </div>
                </div>
                <button
                  onClick={(e) => { e.stopPropagation(); dismissNotification.mutate(n.id) }}
                  title="Dismiss"
                  style={{
                    background: 'none', border: 'none', cursor: 'pointer',
                    color: 'var(--text-tertiary)', fontSize: '11px', padding: '4px 8px',
                    opacity: 0.5, alignSelf: 'flex-start',
                  }}
                  onMouseEnter={(e) => { e.currentTarget.style.opacity = '1' }}
                  onMouseLeave={(e) => { e.currentTarget.style.opacity = '0.5' }}
                >
                  Dismiss
                </button>
              </div>
            ))}
          </div>
        </>
      )}
    </div>
  )
}
