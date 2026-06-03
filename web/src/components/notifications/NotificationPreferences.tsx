import { useState } from 'react'
import {
  useNotificationRules,
  useNotificationSources,
  useSaveRule,
  useDeleteRule,
} from '../../hooks/useNotifications'
import type { NotificationRule, NotificationSeverity } from '../../lib/types'
import { CustomSelect } from '../fields'
import { Switch } from '@/components/ui/switch'
import { Label } from '@/components/ui/label'

interface NotificationPreferencesProps {
  userId: string | null
}

const severityOptions: NotificationSeverity[] = ['info', 'warning', 'critical']

/**
 * Notification preferences UI. Shows a source x trigger x channel matrix
 * with toggles. Auto-populated from registered evaluators via the
 * notifications.sources.list RPC.
 */
export default function NotificationPreferences({ userId }: NotificationPreferencesProps) {
  const { data: rules = [], isLoading: rulesLoading } = useNotificationRules(userId)
  const { data: sourcesData } = useNotificationSources()
  const saveRule = useSaveRule()
  const deleteRule = useDeleteRule()

  const [soundEnabled, setSoundEnabled] = useState(() =>
    localStorage.getItem(`notification_sound_${userId}`) === 'true'
  )

  function handleSoundToggle(checked: boolean) {
    setSoundEnabled(checked)
    localStorage.setItem(`notification_sound_${userId}`, String(checked))
  }

  const sources = sourcesData?.sources ?? []
  const channels = sourcesData?.channels ?? []

  function handleToggleChannel(rule: NotificationRule, channel: string) {
    const newChannels = rule.channels.includes(channel)
      ? rule.channels.filter((c) => c !== channel)
      : [...rule.channels, channel]
    saveRule.mutate({ ...rule, channels: newChannels })
  }

  function handleToggleEnabled(rule: NotificationRule) {
    saveRule.mutate({ ...rule, enabled: !rule.enabled })
  }

  function handleSeverityChange(rule: NotificationRule, severity: NotificationSeverity) {
    saveRule.mutate({ ...rule, minSeverity: severity })
  }

  function handleWebhookURLChange(rule: NotificationRule, url: string) {
    saveRule.mutate({ ...rule, webhookUrl: url })
  }

  if (!userId) {
    return (
      <div style={{ padding: 24, color: 'var(--text-tertiary)' }}>
        Select a profile to configure notification preferences.
      </div>
    )
  }

  if (rulesLoading) {
    return (
      <div style={{ padding: 24, textAlign: 'center' }}>
        <span className="spinner spinner-sm" />
      </div>
    )
  }

  // Group rules by source
  const rulesBySource = new Map<string, NotificationRule[]>()
  for (const rule of rules) {
    const key = rule.source === '*' ? 'All sources' : rule.source
    if (!rulesBySource.has(key)) rulesBySource.set(key, [])
    rulesBySource.get(key)!.push(rule)
  }

  return (
    <div style={{ padding: '16px 0' }}>
      <h3 style={{ fontSize: '16px', fontWeight: 600, marginBottom: 16 }}>Notification Preferences</h3>

      <div className="flex items-center justify-between py-3 border-b border-[var(--border)]">
        <div>
          <Label htmlFor="sound-alerts" className="text-sm font-medium">Sound alerts</Label>
          <p className="text-xs text-[var(--text-secondary)] mt-0.5">Play a chime for critical and high urgency items</p>
        </div>
        <Switch id="sound-alerts" checked={soundEnabled} onCheckedChange={handleSoundToggle} />
      </div>

      {sources.length === 0 && rules.length === 0 && (
        <div style={{ color: 'var(--text-tertiary)', fontSize: '13px' }}>
          No notification sources registered yet. Sources appear here as evaluators register.
        </div>
      )}

      {Array.from(rulesBySource.entries()).map(([source, sourceRules]) => (
        <div key={source} style={{ marginBottom: 24 }}>
          <h4 style={{
            fontSize: '14px',
            fontWeight: 600,
            textTransform: 'capitalize',
            marginBottom: 8,
            color: 'var(--text-primary)',
          }}>
            {source}
          </h4>

          <div style={{
            border: '1px solid var(--border, #e5e7eb)',
            borderRadius: 8,
            overflow: 'hidden',
          }}>
            {sourceRules.map((rule, idx) => (
              <div
                key={rule.id}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 12,
                  padding: '10px 16px',
                  borderBottom: idx < sourceRules.length - 1 ? '1px solid var(--border-subtle, #f3f4f6)' : 'none',
                  opacity: rule.enabled ? 1 : 0.5,
                }}
              >
                {/* Enable/disable toggle */}
                <label style={{ display: 'flex', alignItems: 'center', cursor: 'pointer' }}>
                  <input
                    type="checkbox"
                    checked={rule.enabled}
                    onChange={() => handleToggleEnabled(rule)}
                    style={{ marginRight: 8 }}
                  />
                </label>

                {/* Trigger name */}
                <div style={{ flex: 1, minWidth: 0 }}>
                  <span style={{ fontSize: '13px', fontWeight: 500 }}>
                    {rule.trigger === '*' ? 'All triggers' : formatTriggerName(rule.trigger)}
                  </span>
                  <div style={{ fontSize: '11px', color: 'var(--text-tertiary)' }}>
                    Min severity: {rule.minSeverity}
                  </div>
                </div>

                {/* Severity selector */}
                <CustomSelect
                  value={rule.minSeverity}
                  onChange={v => handleSeverityChange(rule, v as NotificationSeverity)}
                  className="text-xs px-1.5 py-0.5 border border-[var(--border)] rounded bg-[var(--bg-surface)] text-[var(--text-1)] flex items-center gap-2 cursor-pointer"
                  options={severityOptions.map(s => ({ value: s, label: s }))}
                />

                {/* Channel toggles */}
                {channels.map((ch) => (
                  <label
                    key={ch}
                    style={{
                      display: 'flex',
                      alignItems: 'center',
                      gap: 4,
                      fontSize: '12px',
                      cursor: 'pointer',
                    }}
                  >
                    <input
                      type="checkbox"
                      checked={rule.channels.includes(ch)}
                      onChange={() => handleToggleChannel(rule, ch)}
                    />
                    {formatChannelName(ch)}
                  </label>
                ))}

                {/* Webhook URL input — shown when webhook channel is active */}
                {rule.channels.includes('webhook') && (
                  <input
                    type="url"
                    placeholder="Webhook URL"
                    value={rule.webhookUrl ?? ''}
                    onChange={(e) => handleWebhookURLChange(rule, e.target.value)}
                    onBlur={(e) => handleWebhookURLChange(rule, e.target.value.trim())}
                    style={{
                      fontSize: '12px',
                      padding: '2px 6px',
                      borderRadius: 4,
                      border: '1px solid var(--border, #e5e7eb)',
                      background: 'var(--bg-surface)',
                      width: 180,
                    }}
                  />
                )}

                {/* Delete button */}
                <button
                  onClick={() => deleteRule.mutate(rule.id)}
                  title="Delete rule"
                  style={{
                    background: 'none',
                    border: 'none',
                    cursor: 'pointer',
                    color: 'var(--danger, #ef4444)',
                    fontSize: '12px',
                    padding: '2px 4px',
                    opacity: 0.6,
                  }}
                >
                  Remove
                </button>
              </div>
            ))}
          </div>
        </div>
      ))}
    </div>
  )
}

function formatTriggerName(trigger: string): string {
  return trigger
    .replace(/_/g, ' ')
    .replace(/\b\w/g, (c) => c.toUpperCase())
}

function formatChannelName(channel: string): string {
  switch (channel) {
    case 'in_app': return 'Bell'
    case 'webhook': return 'Webhook'
    default: return channel
  }
}
