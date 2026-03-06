import { useState } from 'react'
import { useStore } from '../lib/store'
import { useMail } from '../hooks/useMail'
import { X, ArrowUpRight, Inbox, Send } from 'lucide-react'
import type { MailMessage } from '../lib/types'

interface MailSlideOverProps {
  teamId: string
}

const statusColors: Record<string, string> = {
  pending: 'var(--amber)',
  failed: 'var(--red)',
  succeeded: 'var(--green)',
  acked: 'var(--green)',
}

function MailRow({ message, onSelect, selected }: { message: MailMessage; onSelect: () => void; selected: boolean }) {
  const task = message.task
  const isCrossTeam = task?.assignedToType === 'team' || (task?.teamId && task?.assignedTo && task.teamId !== task.assignedTo)
  const displayStatus = task?.status ?? message.status
  const label = task?.goal ?? message.subject ?? message.summary ?? message.bodyPreview ?? message.kind
  const dotColor = statusColors[displayStatus ?? ''] ?? 'var(--text-3)'

  return (
    <div
      onClick={onSelect}
      style={{
        padding: '9px 12px', cursor: 'pointer',
        borderRadius: 'var(--r-md)',
        background: selected ? 'var(--bg-active)' : 'transparent',
        display: 'flex', alignItems: 'center', gap: 9,
        transition: 'background 0.1s',
        margin: '1px 0',
      }}
      onMouseEnter={e => { if (!selected) e.currentTarget.style.background = 'var(--bg-hover)' }}
      onMouseLeave={e => { if (!selected) e.currentTarget.style.background = 'transparent' }}
    >
      <span style={{
        width: 7, height: 7, borderRadius: '50%', flexShrink: 0,
        background: dotColor,
      }} />
      <span style={{
        flex: 1, fontSize: 12, color: 'var(--text-1)',
        overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
        fontWeight: selected ? 500 : 400,
      }}>
        {label}
      </span>
      {isCrossTeam && (
        <ArrowUpRight size={11} style={{ color: 'var(--text-3)', flexShrink: 0 }} />
      )}
      {task?.assignedRole && (
        <span style={{
          fontSize: 10, color: 'var(--text-3)', flexShrink: 0,
          background: 'var(--bg-elevated)', border: '1px solid var(--border)',
          padding: '1px 5px', borderRadius: 999,
        }}>
          {task.assignedRole}
        </span>
      )}
    </div>
  )
}

function SectionLabel({ icon, label, count }: { icon: React.ReactNode; label: string; count: number }) {
  return (
    <div style={{
      display: 'flex', alignItems: 'center', gap: 6,
      padding: '10px 12px 5px',
      fontSize: 10, fontWeight: 600,
      letterSpacing: '0.08em', textTransform: 'uppercase',
      color: 'var(--text-3)',
    }}>
      {icon}
      {label}
      <span style={{
        background: 'var(--bg-elevated)', border: '1px solid var(--border)',
        borderRadius: 999, padding: '0px 5px',
        fontSize: 9, fontWeight: 700, letterSpacing: 0,
        textTransform: 'none',
      }}>{count}</span>
    </div>
  )
}

export default function MailSlideOver({ teamId }: MailSlideOverProps) {
  const { setMailOpen } = useStore()
  const { inbox, outbox, claim, complete } = useMail(teamId)
  const [selected, setSelected] = useState<MailMessage | null>(null)

  return (
    <div
      className="animate-slide-in-right"
      style={{
        position: 'absolute', inset: 0, zIndex: 50,
        display: 'flex', justifyContent: 'flex-end',
        background: 'rgba(0,0,0,0.45)',
        backdropFilter: 'blur(3px)',
      }}
      onClick={() => setMailOpen(false)}
    >
      <div
        onClick={e => e.stopPropagation()}
        style={{
          width: 390, height: '100%',
          background: 'var(--bg-panel)',
          borderLeft: '1px solid var(--border)',
          display: 'flex', flexDirection: 'column',
          boxShadow: '-12px 0 48px rgba(0,0,0,0.4)',
        }}
      >
        {/* Header */}
        <div style={{
          display: 'flex', alignItems: 'center', justifyContent: 'space-between',
          padding: '14px 16px',
          borderBottom: '1px solid var(--border)',
        }}>
          <span style={{ fontWeight: 600, fontSize: 14, color: 'var(--text-1)', letterSpacing: '-0.02em' }}>
            Mail
          </span>
          <button
            onClick={() => setMailOpen(false)}
            style={{
              background: 'none', border: 'none', cursor: 'pointer',
              color: 'var(--text-3)', padding: 5, borderRadius: 'var(--r-md)',
              display: 'flex', alignItems: 'center',
              transition: 'color 0.1s, background 0.1s',
            }}
            onMouseEnter={e => {
              e.currentTarget.style.color = 'var(--text-1)'
              e.currentTarget.style.background = 'var(--bg-hover)'
            }}
            onMouseLeave={e => {
              e.currentTarget.style.color = 'var(--text-3)'
              e.currentTarget.style.background = 'transparent'
            }}
          >
            <X size={16} />
          </button>
        </div>

        {/* Message list */}
        <div style={{ flex: 1, overflowY: 'auto', padding: '4px 8px' }}>
          {inbox.length > 0 && (
            <>
              <SectionLabel icon={<Inbox size={10} />} label="Inbox" count={inbox.length} />
              {inbox.map(message => (
                <MailRow
                  key={message.messageId}
                  message={message}
                  onSelect={() => setSelected(message)}
                  selected={selected?.messageId === message.messageId}
                />
              ))}
            </>
          )}

          {outbox.length > 0 && (
            <>
              <SectionLabel icon={<Send size={10} />} label="Sent" count={outbox.length} />
              {outbox.map(message => (
                <MailRow
                  key={message.messageId}
                  message={message}
                  onSelect={() => setSelected(message)}
                  selected={selected?.messageId === message.messageId}
                />
              ))}
            </>
          )}

          {inbox.length === 0 && outbox.length === 0 && (
            <div style={{ padding: 32, textAlign: 'center', color: 'var(--text-3)', fontSize: 13 }}>
              No messages
            </div>
          )}
        </div>

        {/* Selected message detail */}
        {selected && (
          <div style={{
            padding: 16,
            borderTop: '1px solid var(--border)',
            background: 'var(--bg-surface)',
          }}>
            <div style={{ fontSize: 13, fontWeight: 600, color: 'var(--text-1)', marginBottom: 6, letterSpacing: '-0.01em' }}>
              {selected.task?.goal ?? selected.subject ?? selected.summary ?? selected.kind}
            </div>
            {selected.summary && (
              <div style={{ fontSize: 12, color: 'var(--text-2)', marginBottom: 8, lineHeight: 1.5 }}>
                {selected.summary}
              </div>
            )}
            {selected.bodyPreview && selected.bodyPreview !== selected.summary && (
              <div style={{ fontSize: 12, color: 'var(--text-3)', marginBottom: 8, lineHeight: 1.5 }}>
                {selected.bodyPreview}
              </div>
            )}
            {(selected.task?.error ?? selected.error) && (
              <div style={{
                fontSize: 11, color: 'var(--red)', marginBottom: 8,
                background: 'var(--red-dim)', border: '1px solid rgba(239,68,68,0.2)',
                padding: '6px 10px', borderRadius: 'var(--r-md)',
              }}>
                {selected.task?.error ?? selected.error}
              </div>
            )}
            <div style={{ display: 'flex', gap: 8, marginTop: 10 }}>
              {selected.canClaim && selected.taskId && (
                <button
                  onClick={() => claim(selected.taskId!)}
                  style={{
                    fontSize: 12, fontWeight: 500,
                    padding: '6px 14px', borderRadius: 'var(--r-md)',
                    border: '1px solid var(--border-strong)',
                    background: 'var(--bg-elevated)',
                    cursor: 'pointer', color: 'var(--text-1)',
                    transition: 'border-color 0.1s',
                  }}
                >
                  Claim
                </button>
              )}
              {selected.canComplete && selected.taskId && (
                <button
                  onClick={() => complete({ taskId: selected.taskId! })}
                  style={{
                    fontSize: 12, fontWeight: 500,
                    padding: '6px 14px', borderRadius: 'var(--r-md)',
                    border: 'none',
                    background: 'var(--green)',
                    color: '#fff', cursor: 'pointer',
                  }}
                >
                  Complete
                </button>
              )}
              {(selected.readOnly || !selected.taskId) && (
                <span style={{ fontSize: 11, color: 'var(--text-3)', alignSelf: 'center' }}>
                  Read-only
                </span>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
