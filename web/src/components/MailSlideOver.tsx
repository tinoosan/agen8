import { useState, useMemo, useEffect, useRef } from 'react'
import { useStore } from '../lib/store'
import { useMail } from '../hooks/useMail'
import {
  X, ArrowRight, Inbox, Send, Mail, Search,
  CheckCircle2, Clock, AlertCircle, XCircle, Eye,
  Coins, Activity, FileText, Filter,
  Sparkles, ArrowUpRight
} from 'lucide-react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import type { MailMessage } from '../lib/types'

interface MailSlideOverProps {
  teamId: string
}

/* ── Helpers ──────────────────────────────────────── */

function timeAgo(dateStr: string): string {
  const seconds = Math.floor((Date.now() - new Date(dateStr).getTime()) / 1000)
  if (seconds < 10) return 'just now'
  if (seconds < 60) return `${seconds}s ago`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  const days = Math.floor(hours / 24)
  return `${days}d ago`
}

function extractSender(message: MailMessage): string {
  if (message.task?.createdBy) return message.task.createdBy
  if (message.task?.assignedRole && message.channel === 'outbox') return message.task.assignedRole
  return message.sourceTeamId ?? 'system'
}

function extractReceiver(message: MailMessage): string {
  if (message.task?.assignedRole) return message.task.assignedRole
  if (message.task?.assignedTo) return message.task.assignedTo
  return message.destinationTeamId ?? ''
}

function formatRole(role: string): string {
  return role
    .replace(/[_-]/g, ' ')
    .replace(/\b\w/g, c => c.toUpperCase())
}

type StatusKey = 'pending' | 'review_pending' | 'active' | 'succeeded' | 'acked' | 'failed' | 'canceled'

const statusConfig: Record<StatusKey, { label: string; color: string; bg: string; icon: typeof Clock }> = {
  pending: { label: 'Pending', color: 'var(--amber)', bg: 'var(--amber-dim)', icon: Clock },
  review_pending: { label: 'Review', color: 'var(--accent)', bg: 'color-mix(in srgb, var(--accent) 12%, transparent)', icon: Eye },
  active: { label: 'Active', color: 'var(--blue)', bg: 'color-mix(in srgb, var(--blue) 12%, transparent)', icon: Activity },
  succeeded: { label: 'Done', color: 'var(--green)', bg: 'color-mix(in srgb, var(--green) 12%, transparent)', icon: CheckCircle2 },
  acked: { label: 'Done', color: 'var(--green)', bg: 'color-mix(in srgb, var(--green) 12%, transparent)', icon: CheckCircle2 },
  failed: { label: 'Failed', color: 'var(--red)', bg: 'color-mix(in srgb, var(--red) 12%, transparent)', icon: AlertCircle },
  canceled: { label: 'Canceled', color: 'var(--text-3)', bg: 'var(--bg-elevated)', icon: XCircle },
}

function getStatus(message: MailMessage) {
  const key = (message.task?.status ?? message.taskStatus ?? message.status ?? 'pending') as StatusKey
  return statusConfig[key] ?? statusConfig.pending
}

function isUnread(message: MailMessage): boolean {
  const status = message.task?.status ?? message.taskStatus ?? message.status
  return status === 'pending' || status === 'review_pending'
}

function groupKey(message: MailMessage): string {
  const meta = message.task?.metadata as Record<string, unknown> | undefined
  if (meta?.batchParentTaskId) return String(meta.batchParentTaskId)
  if (meta?.batchWaveId) return String(meta.batchWaveId)
  return ''
}

/* ── Status Badge ──────────────────────────────── */

function StatusBadge({ message }: { message: MailMessage }) {
  const status = getStatus(message)
  const Icon = status.icon
  return (
    <span style={{
      display: 'inline-flex', alignItems: 'center', gap: 3,
      fontSize: 9, fontWeight: 700, textTransform: 'uppercase',
      letterSpacing: '0.04em',
      color: status.color, background: status.bg,
      padding: '2px 7px', borderRadius: 999,
      flexShrink: 0,
    }}>
      <Icon size={9} />
      {status.label}
    </span>
  )
}

/* ── Route indicator (sender → receiver) ──────── */

function RouteIndicator({ message }: { message: MailMessage }) {
  const sender = formatRole(extractSender(message))
  const receiver = formatRole(extractReceiver(message))
  if (!sender && !receiver) return null
  return (
    <div style={{
      display: 'flex', alignItems: 'center', gap: 4,
      fontSize: 10, color: 'var(--text-3)',
      marginTop: 2,
    }}>
      <span style={{ fontWeight: 600, color: 'var(--text-2)' }}>{sender || '—'}</span>
      <ArrowRight size={9} style={{ opacity: 0.5 }} />
      <span style={{ fontWeight: 600, color: 'var(--accent)' }}>{receiver || '—'}</span>
    </div>
  )
}

/* ── Mail row ──────────────────────────────────── */

function MailRow({ message, onSelect, selected }: { message: MailMessage; onSelect: () => void; selected: boolean }) {
  const task = message.task
  const label = task?.goal ?? message.subject ?? message.summary ?? message.bodyPreview ?? message.kind
  const unread = isUnread(message)
  const isCrossTeam = task?.assignedToType === 'team' || (task?.teamId && task?.assignedTo && task.teamId !== task.assignedTo)
  const timestamp = message.createdAt

  return (
    <div
      onClick={onSelect}
      className={selected ? '' : 'row-hover'}
      style={{
        padding: '10px 12px', cursor: 'pointer',
        borderRadius: 'var(--r-md)',
        background: selected ? 'var(--bg-active)' : 'transparent',
        borderLeft: unread ? '3px solid var(--accent)' : '3px solid transparent',
        marginBottom: 2,
        transition: 'background 0.12s, border-color 0.15s',
      }}
    >
      {/* Top row: label + status + time */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
        <span className="truncate" style={{
          flex: 1, fontSize: 12,
          color: 'var(--text-1)',
          fontWeight: unread ? 600 : 400,
        }}>
          {label}
        </span>
        {isCrossTeam && (
          <ArrowUpRight size={10} style={{ color: 'var(--text-3)', flexShrink: 0 }} />
        )}
        <StatusBadge message={message} />
      </div>

      {/* Bottom row: route + timestamp */}
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginTop: 4 }}>
        <RouteIndicator message={message} />
        <span style={{
          fontSize: 10, color: 'var(--text-3)',
          fontVariantNumeric: 'tabular-nums',
          flexShrink: 0, marginLeft: 'auto',
        }}>
          {timeAgo(timestamp)}
        </span>
      </div>
    </div>
  )
}

/* ── Section header ────────────────────────────── */

function SectionLabel({ icon, label, count }: { icon: React.ReactNode; label: string; count: number }) {
  return (
    <div className="section-label" style={{ padding: '12px 12px 5px', display: 'flex', alignItems: 'center', gap: 6 }}>
      {icon}
      {label}
      <span style={{
        background: 'var(--bg-elevated)', border: '1px solid var(--border)',
        borderRadius: 999, padding: '0 6px',
        fontSize: 9, fontWeight: 700,
        fontVariantNumeric: 'tabular-nums',
        textTransform: 'none', letterSpacing: 0,
      }}>{count}</span>
    </div>
  )
}

/* ── Group header (batch wave) ─────────────────── */

function GroupHeader({ label, count }: { label: string; count: number }) {
  return (
    <div style={{
      display: 'flex', alignItems: 'center', gap: 6,
      padding: '6px 12px', margin: '6px 0 2px',
      fontSize: 10, fontWeight: 600, color: 'var(--text-3)',
      textTransform: 'uppercase', letterSpacing: '0.04em',
    }}>
      <Sparkles size={9} />
      {label}
      <span style={{
        fontSize: 9, fontWeight: 700,
        background: 'var(--bg-elevated)', border: '1px solid var(--border)',
        borderRadius: 999, padding: '0 5px',
      }}>{count}</span>
    </div>
  )
}

/* ── Filter bar ────────────────────────────────── */

function FilterBar({ search, setSearch, statusFilter, setStatusFilter, roleFilter, setRoleFilter, roles }: {
  search: string; setSearch: (s: string) => void
  statusFilter: string; setStatusFilter: (s: string) => void
  roleFilter: string; setRoleFilter: (s: string) => void
  roles: string[]
}) {
  const [showFilters, setShowFilters] = useState(false)
  return (
    <div style={{ padding: '8px 12px', borderBottom: '1px solid var(--border)', flexShrink: 0 }}>
      {/* Search */}
      <div style={{
        display: 'flex', alignItems: 'center', gap: 6,
        background: 'var(--bg-surface)', border: '1px solid var(--border)',
        borderRadius: 'var(--r-md)', padding: '6px 10px',
      }}>
        <Search size={12} style={{ color: 'var(--text-3)', flexShrink: 0 }} />
        <input
          type="text"
          placeholder="Search messages…"
          value={search}
          onChange={e => setSearch(e.target.value)}
          style={{
            border: 'none', outline: 'none', background: 'transparent',
            flex: 1, fontSize: 12, color: 'var(--text-1)',
            fontFamily: 'inherit',
          }}
        />
        <button
          onClick={() => setShowFilters(f => !f)}
          style={{
            display: 'flex', alignItems: 'center', gap: 3,
            border: 'none', background: showFilters ? 'var(--bg-active)' : 'none',
            cursor: 'pointer', padding: '2px 6px', borderRadius: 'var(--r-sm)',
            color: showFilters ? 'var(--accent)' : 'var(--text-3)',
            fontSize: 10, fontWeight: 600, fontFamily: 'inherit',
            transition: 'color 0.12s, background 0.12s',
          }}
        >
          <Filter size={10} />
          Filters
        </button>
      </div>

      {/* Filter pills */}
      {showFilters && (
        <div className="animate-fade-in" style={{
          display: 'flex', gap: 6, marginTop: 8, flexWrap: 'wrap',
        }}>
          {/* Status filter */}
          <select
            value={statusFilter}
            onChange={e => setStatusFilter(e.target.value)}
            style={{
              fontSize: 11, fontFamily: 'inherit', fontWeight: 500,
              border: '1px solid var(--border)', borderRadius: 'var(--r-sm)',
              background: 'var(--bg-surface)', color: 'var(--text-2)',
              padding: '3px 8px', cursor: 'pointer', outline: 'none',
            }}
          >
            <option value="">All statuses</option>
            <option value="pending">Pending</option>
            <option value="review_pending">Review</option>
            <option value="active">Active</option>
            <option value="succeeded">Done</option>
            <option value="failed">Failed</option>
          </select>

          {/* Role filter */}
          {roles.length > 0 && (
            <select
              value={roleFilter}
              onChange={e => setRoleFilter(e.target.value)}
              style={{
                fontSize: 11, fontFamily: 'inherit', fontWeight: 500,
                border: '1px solid var(--border)', borderRadius: 'var(--r-sm)',
                background: 'var(--bg-surface)', color: 'var(--text-2)',
                padding: '3px 8px', cursor: 'pointer', outline: 'none',
              }}
            >
              <option value="">All roles</option>
              {roles.map(r => <option key={r} value={r}>{formatRole(r)}</option>)}
            </select>
          )}

          {(statusFilter || roleFilter) && (
            <button
              onClick={() => { setStatusFilter(''); setRoleFilter('') }}
              style={{
                fontSize: 10, fontWeight: 600, fontFamily: 'inherit',
                border: 'none', background: 'none', cursor: 'pointer',
                color: 'var(--red)', padding: '3px 6px',
              }}
            >
              Clear
            </button>
          )}
        </div>
      )}
    </div>
  )
}

/* ── Detail panel ──────────────────────────────── */

function DetailPanel({ message, onClaim, onComplete, claiming, completing }: {
  message: MailMessage
  onClaim: () => void
  onComplete: () => void
  claiming: boolean
  completing: boolean
}) {
  const task = message.task
  const sender = formatRole(extractSender(message))
  const receiver = formatRole(extractReceiver(message))
  const costUSD = task?.costUSD ?? 0
  const totalTokens = task?.totalTokens ?? 0
  const artifacts = task?.artifacts ?? []

  return (
    <div style={{
      flex: '0 0 55%', minHeight: 0, display: 'flex', flexDirection: 'column',
      borderTop: '1px solid var(--border)', background: 'var(--bg-surface)',
    }}>
      {/* Detail header */}
      <div style={{
        padding: '12px 16px 10px', borderBottom: '1px solid var(--border)', flexShrink: 0,
      }}>
        <div style={{
          display: 'flex', alignItems: 'flex-start', gap: 8, marginBottom: 8,
        }}>
          <div style={{ flex: 1 }}>
            <div style={{
              fontSize: 13, fontWeight: 600, color: 'var(--text-1)',
              letterSpacing: '-0.01em', lineHeight: 1.4,
            }}>
              {task?.goal ?? message.subject ?? message.summary ?? message.kind}
            </div>
          </div>
          <StatusBadge message={message} />
        </div>

        {/* Route + timestamp */}
        <div style={{
          display: 'flex', alignItems: 'center', gap: 12,
          fontSize: 11, color: 'var(--text-3)',
        }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
            <span style={{ fontWeight: 600, color: 'var(--text-2)' }}>{sender}</span>
            <ArrowRight size={10} style={{ opacity: 0.5 }} />
            <span style={{ fontWeight: 600, color: 'var(--accent)' }}>{receiver}</span>
          </div>
          <span style={{ fontVariantNumeric: 'tabular-nums' }}>
            {timeAgo(message.createdAt)}
          </span>
        </div>

        {/* Metadata chips */}
        {(costUSD > 0 || totalTokens > 0 || artifacts.length > 0) && (
          <div style={{
            display: 'flex', gap: 10, marginTop: 10, flexWrap: 'wrap',
          }}>
            {costUSD > 0 && (
              <span style={{
                display: 'inline-flex', alignItems: 'center', gap: 4,
                fontSize: 10, fontWeight: 600,
                color: 'var(--amber)', background: 'var(--amber-dim)',
                padding: '2px 8px', borderRadius: 999,
              }}>
                <Coins size={10} />
                ${costUSD.toFixed(4)}
              </span>
            )}
            {totalTokens > 0 && (
              <span style={{
                display: 'inline-flex', alignItems: 'center', gap: 4,
                fontSize: 10, fontWeight: 600,
                color: 'var(--text-2)', background: 'var(--bg-elevated)',
                padding: '2px 8px', borderRadius: 999,
                border: '1px solid var(--border)',
              }}>
                <Activity size={10} />
                {totalTokens.toLocaleString()} tok
              </span>
            )}
            {artifacts.length > 0 && (
              <span style={{
                display: 'inline-flex', alignItems: 'center', gap: 4,
                fontSize: 10, fontWeight: 600,
                color: 'var(--text-2)', background: 'var(--bg-elevated)',
                padding: '2px 8px', borderRadius: 999,
                border: '1px solid var(--border)',
              }}>
                <FileText size={10} />
                {artifacts.length} artifact{artifacts.length !== 1 ? 's' : ''}
              </span>
            )}
          </div>
        )}
      </div>

      {/* Scrollable body */}
      <div style={{ flex: 1, minHeight: 0, overflowY: 'auto', padding: '12px 16px' }}>
        {message.summary && (
          <div className="md-prose" style={{ fontSize: 12, color: 'var(--text-2)', marginBottom: 10, lineHeight: 1.6 }}>
            <ReactMarkdown remarkPlugins={[remarkGfm]}>{message.summary}</ReactMarkdown>
          </div>
        )}
        {message.bodyPreview && message.bodyPreview !== message.summary && (
          <div className="md-prose" style={{ fontSize: 12, color: 'var(--text-2)', marginBottom: 10, lineHeight: 1.6 }}>
            <ReactMarkdown remarkPlugins={[remarkGfm]}>{message.bodyPreview}</ReactMarkdown>
          </div>
        )}
        {(task?.error ?? message.error) && (
          <div style={{
            fontSize: 11, color: 'var(--red)', marginBottom: 8,
            background: 'color-mix(in srgb, var(--red) 8%, transparent)',
            border: '1px solid color-mix(in srgb, var(--red) 20%, transparent)',
            padding: '8px 12px', borderRadius: 'var(--r-md)', lineHeight: 1.5,
          }}>
            {task?.error ?? message.error}
          </div>
        )}

        {/* Artifacts list */}
        {artifacts.length > 0 && (
          <div style={{ marginTop: 8 }}>
            <div style={{
              fontSize: 9, fontWeight: 700, textTransform: 'uppercase',
              letterSpacing: '0.06em', color: 'var(--text-3)', marginBottom: 6,
            }}>
              Artifacts
            </div>
            {artifacts.map((art, i) => (
              <div key={i} style={{
                display: 'flex', alignItems: 'center', gap: 6,
                padding: '4px 8px', marginBottom: 2,
                background: 'var(--bg-elevated)', borderRadius: 'var(--r-sm)',
                fontSize: 11, color: 'var(--text-2)',
              }}>
                <FileText size={10} style={{ color: 'var(--text-3)', flexShrink: 0 }} />
                <span className="truncate mono">{typeof art === 'string' ? art.split('/').pop() : String(art)}</span>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Action buttons */}
      <div style={{
        padding: '10px 16px', borderTop: '1px solid var(--border)',
        display: 'flex', gap: 8, flexShrink: 0, alignItems: 'center',
      }}>
        {message.canClaim && message.taskId && (
          <button
            className="btn-surface"
            onClick={onClaim}
            disabled={claiming}
            style={{ opacity: claiming ? 0.6 : 1, transition: 'opacity 0.15s' }}
          >
            {claiming ? (
              <><span className="spinner" style={{ width: 10, height: 10, borderWidth: 1.5 }} /> Claiming…</>
            ) : 'Claim'}
          </button>
        )}
        {message.canComplete && message.taskId && (
          <button
            onClick={onComplete}
            disabled={completing}
            style={{
              fontSize: 12, fontWeight: 600, padding: '7px 16px',
              borderRadius: 'var(--r-md)', border: 'none',
              background: completing ? 'color-mix(in srgb, var(--green) 60%, transparent)' : 'var(--green)',
              color: '#fff', cursor: completing ? 'default' : 'pointer',
              fontFamily: 'inherit', transition: 'opacity 0.15s, background 0.15s',
              display: 'flex', alignItems: 'center', gap: 5,
            }}
          >
            {completing ? (
              <><span className="spinner" style={{ width: 10, height: 10, borderWidth: 1.5, borderColor: '#fff', borderTopColor: 'transparent' }} /> Completing…</>
            ) : (
              <><CheckCircle2 size={12} /> Complete</>
            )}
          </button>
        )}
        {(message.readOnly || !message.taskId) && (
          <span style={{
            fontSize: 11, color: 'var(--text-3)',
            display: 'flex', alignItems: 'center', gap: 4,
          }}>
            <Eye size={10} /> Read-only
          </span>
        )}
        <div style={{ flex: 1 }} />
        {message.task?.assignedRole && (
          <span style={{
            fontSize: 10, color: 'var(--text-3)', fontWeight: 500,
            background: 'var(--bg-elevated)', border: '1px solid var(--border)',
            padding: '2px 8px', borderRadius: 999,
          }}>
            {formatRole(message.task.assignedRole)}
          </span>
        )}
      </div>
    </div>
  )
}

/* ── Empty state ───────────────────────────────── */

function EmptyState() {
  return (
    <div style={{
      display: 'flex', flexDirection: 'column', alignItems: 'center',
      justifyContent: 'center', padding: '60px 32px', textAlign: 'center', gap: 16,
      flex: 1,
    }}>
      <div style={{
        width: 56, height: 56, borderRadius: 16,
        background: 'linear-gradient(135deg, var(--bg-elevated), var(--bg-surface))',
        border: '1px solid var(--border)',
        display: 'flex', alignItems: 'center', justifyContent: 'center',
      }}>
        <Mail size={24} style={{ color: 'var(--text-3)' }} />
      </div>
      <div>
        <div style={{ fontWeight: 600, fontSize: 14, color: 'var(--text-1)', marginBottom: 4 }}>
          No messages yet
        </div>
        <div style={{ color: 'var(--text-3)', fontSize: 12, lineHeight: 1.6, maxWidth: 240 }}>
          When agents delegate tasks or submit reviews, their messages will appear here.
        </div>
      </div>
      <div style={{
        display: 'flex', flexDirection: 'column', gap: 6, marginTop: 8, width: '100%', maxWidth: 220,
      }}>
        {[
          { icon: <Send size={10} />, text: 'Delegated tasks' },
          { icon: <Inbox size={10} />, text: 'Review callbacks' },
          { icon: <ArrowUpRight size={10} />, text: 'Cross-team messages' },
        ].map((hint, i) => (
          <div key={i} style={{
            display: 'flex', alignItems: 'center', gap: 8,
            padding: '6px 10px', borderRadius: 'var(--r-sm)',
            background: 'var(--bg-surface)', border: '1px solid var(--border)',
            fontSize: 11, color: 'var(--text-2)',
          }}>
            <span style={{ color: 'var(--text-3)' }}>{hint.icon}</span>
            {hint.text}
          </div>
        ))}
      </div>
    </div>
  )
}

/* ── Main component ────────────────────────────── */

export default function MailSlideOver({ teamId }: MailSlideOverProps) {
  const { setMailOpen } = useStore()
  const { inbox, outbox, claim, complete } = useMail(teamId)
  const [selected, setSelected] = useState<MailMessage | null>(null)
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState('')
  const [roleFilter, setRoleFilter] = useState('')
  const [claiming, setClaiming] = useState(false)
  const [completing, setCompleting] = useState(false)
  const [justCompleted, setJustCompleted] = useState<string | null>(null)
  const prevCountRef = useRef(inbox.length)
  const [flashNew, setFlashNew] = useState(false)

  // Detect new messages
  useEffect(() => {
    if (inbox.length > prevCountRef.current) {
      setFlashNew(true)
      setTimeout(() => setFlashNew(false), 1500)
    }
    prevCountRef.current = inbox.length
  }, [inbox.length])

  // Unique roles for filter
  const allRoles = useMemo(() => {
    const roles = new Set<string>()
      ;[...inbox, ...outbox].forEach(m => {
        if (m.task?.assignedRole) roles.add(m.task.assignedRole)
        if (m.task?.createdBy) roles.add(m.task.createdBy)
      })
    return [...roles].sort()
  }, [inbox, outbox])

  // Filter messages
  const filterMessages = (messages: MailMessage[]) => {
    return messages.filter(m => {
      if (search) {
        const haystack = [
          m.task?.goal, m.subject, m.summary, m.bodyPreview, m.kind,
          m.task?.assignedRole, m.task?.createdBy,
        ].filter(Boolean).join(' ').toLowerCase()
        if (!haystack.includes(search.toLowerCase())) return false
      }
      if (statusFilter) {
        const status = m.task?.status ?? m.taskStatus ?? m.status
        if (status !== statusFilter) return false
      }
      if (roleFilter) {
        const role = m.task?.assignedRole ?? m.task?.createdBy
        if (role !== roleFilter) return false
      }
      return true
    })
  }

  const filteredInbox = filterMessages(inbox)
  const filteredOutbox = filterMessages(outbox)

  // Group inbox by batch wave
  const groupedInbox = useMemo(() => {
    const groups = new Map<string, MailMessage[]>()
    const ungrouped: MailMessage[] = []
    for (const m of filteredInbox) {
      const key = groupKey(m)
      if (key) {
        if (!groups.has(key)) groups.set(key, [])
        groups.get(key)!.push(m)
      } else {
        ungrouped.push(m)
      }
    }
    return { groups, ungrouped }
  }, [filteredInbox])

  function handleClaim() {
    if (!selected?.taskId) return
    setClaiming(true)
    claim(selected.taskId, {
      onSettled: () => setClaiming(false),
    } as any)
  }

  function handleComplete() {
    if (!selected?.taskId) return
    setCompleting(true)
    complete({ taskId: selected.taskId }, {
      onSettled: () => {
        setCompleting(false)
        setJustCompleted(selected?.taskId ?? null)
        setTimeout(() => setJustCompleted(null), 2000)
      },
    } as any)
  }

  const totalMessages = inbox.length + outbox.length
  const unreadCount = inbox.filter(isUnread).length

  return (
    <div
      className="slide-over-backdrop animate-slide-in-right"
      onClick={() => setMailOpen(false)}
    >
      <div
        onClick={e => e.stopPropagation()}
        style={{
          width: 460, height: '100%',
          background: 'var(--bg-panel)',
          borderLeft: '1px solid var(--border)',
          display: 'flex', flexDirection: 'column',
          boxShadow: '-12px 0 48px rgba(0,0,0,0.4)',
        }}
      >
        {/* Header */}
        <div style={{
          display: 'flex', alignItems: 'center', justifyContent: 'space-between',
          padding: '14px 16px', borderBottom: '1px solid var(--border)', flexShrink: 0,
        }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <div style={{
              width: 28, height: 28, borderRadius: 8,
              background: 'linear-gradient(135deg, var(--accent), color-mix(in srgb, var(--accent) 70%, var(--blue)))',
              display: 'flex', alignItems: 'center', justifyContent: 'center',
            }}>
              <Mail size={14} color="#fff" />
            </div>
            <span style={{ fontWeight: 700, fontSize: 15, color: 'var(--text-1)', letterSpacing: '-0.02em' }}>
              Mail
            </span>
            {totalMessages > 0 && (
              <span style={{
                fontSize: 11, color: 'var(--text-3)',
                background: 'var(--bg-elevated)', border: '1px solid var(--border)',
                borderRadius: 999, padding: '0 7px', fontVariantNumeric: 'tabular-nums',
              }}>
                {totalMessages}
              </span>
            )}
            {unreadCount > 0 && (
              <span style={{
                fontSize: 10, fontWeight: 700,
                color: '#fff', background: 'var(--accent)',
                borderRadius: 999, padding: '1px 7px',
                animation: flashNew ? 'pulse 0.5s ease-in-out 2' : undefined,
              }}>
                {unreadCount} new
              </span>
            )}
          </div>
          <button className="btn-ghost" onClick={() => setMailOpen(false)} aria-label="Close">
            <X size={16} />
          </button>
        </div>

        {/* Filter bar */}
        <FilterBar
          search={search} setSearch={setSearch}
          statusFilter={statusFilter} setStatusFilter={setStatusFilter}
          roleFilter={roleFilter} setRoleFilter={setRoleFilter}
          roles={allRoles}
        />

        {/* Success toast */}
        {justCompleted && (
          <div className="animate-fade-in" style={{
            margin: '8px 12px 0', padding: '8px 12px',
            background: 'color-mix(in srgb, var(--green) 12%, transparent)',
            border: '1px solid color-mix(in srgb, var(--green) 25%, transparent)',
            borderRadius: 'var(--r-md)',
            display: 'flex', alignItems: 'center', gap: 6,
            fontSize: 12, fontWeight: 500, color: 'var(--green)',
          }}>
            <CheckCircle2 size={14} />
            Task completed successfully
          </div>
        )}

        {/* Message list */}
        <div style={{ flex: selected ? undefined : 1, minHeight: selected ? 180 : 0, overflowY: 'auto', padding: '4px 8px' }}>
          {filteredInbox.length > 0 && (
            <>
              <SectionLabel icon={<Inbox size={10} />} label="Inbox" count={filteredInbox.length} />

              {/* Ungrouped */}
              {groupedInbox.ungrouped.map(message => (
                <MailRow
                  key={message.messageId}
                  message={message}
                  onSelect={() => setSelected(message)}
                  selected={selected?.messageId === message.messageId}
                />
              ))}

              {/* Grouped by batch */}
              {[...groupedInbox.groups.entries()].map(([key, messages]) => (
                <div key={key}>
                  <GroupHeader label={`Batch · ${messages.length} items`} count={messages.length} />
                  {messages.map(message => (
                    <MailRow
                      key={message.messageId}
                      message={message}
                      onSelect={() => setSelected(message)}
                      selected={selected?.messageId === message.messageId}
                    />
                  ))}
                </div>
              ))}
            </>
          )}

          {filteredOutbox.length > 0 && (
            <>
              <SectionLabel icon={<Send size={10} />} label="Sent" count={filteredOutbox.length} />
              {filteredOutbox.map(message => (
                <MailRow
                  key={message.messageId}
                  message={message}
                  onSelect={() => setSelected(message)}
                  selected={selected?.messageId === message.messageId}
                />
              ))}
            </>
          )}

          {filteredInbox.length === 0 && filteredOutbox.length === 0 && (
            search || statusFilter || roleFilter ? (
              <div style={{
                padding: '24px 16px', textAlign: 'center',
                color: 'var(--text-3)', fontSize: 12,
              }}>
                No messages match your filters
              </div>
            ) : (
              <EmptyState />
            )
          )}
        </div>

        {/* Selected message detail */}
        {selected && (
          <DetailPanel
            message={selected}
            onClaim={handleClaim}
            onComplete={handleComplete}
            claiming={claiming}
            completing={completing}
          />
        )}
      </div>
    </div>
  )
}
