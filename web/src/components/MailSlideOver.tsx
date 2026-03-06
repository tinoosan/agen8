import { useState } from 'react'
import { useStore } from '../lib/store'
import { useMail } from '../hooks/useMail'
import { X, ArrowUpRight } from 'lucide-react'
import type { MailMessage } from '../lib/types'

interface MailSlideOverProps {
  teamId: string
}

function MailRow({ message, onSelect, selected }: { message: MailMessage; onSelect: () => void; selected: boolean }) {
  const task = message.task
  const isCrossTeam = task?.assignedToType === 'team' || (task?.teamId && task?.assignedTo && task.teamId !== task.assignedTo)
  const displayStatus = task?.status ?? message.status
  const label = task?.goal ?? message.subject ?? message.summary ?? message.bodyPreview ?? message.kind

  return (
    <div
      onClick={onSelect}
      style={{
        padding: '8px 12px', cursor: 'pointer', borderRadius: 8,
        background: selected ? 'light-dark(rgba(0,0,0,0.06), rgba(255,255,255,0.06))' : 'transparent',
        display: 'flex', alignItems: 'center', gap: 8,
      }}
    >
      <span style={{
        fontSize: 8, width: 8, height: 8, borderRadius: '50%', flexShrink: 0,
        background: displayStatus === 'pending' ? '#f59e0b' : displayStatus === 'failed' ? '#ef4444' : displayStatus === 'succeeded' || displayStatus === 'acked' ? '#22c55e' : '#71717a',
      }} />
      <span style={{ flex: 1, fontSize: 12, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
        {label}
      </span>
      {isCrossTeam && (
        <ArrowUpRight size={10} style={{ opacity: 0.5, flexShrink: 0 }} />
      )}
      {task?.assignedRole && (
        <span style={{ fontSize: 10, opacity: 0.45, flexShrink: 0 }}>
          {task.assignedRole}
        </span>
      )}
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
        background: 'rgba(0,0,0,0.3)',
      }}
      onClick={() => setMailOpen(false)}
    >
      <div
        onClick={e => e.stopPropagation()}
        style={{
          width: 380, height: '100%',
          background: 'light-dark(#ffffff, #111113)',
          display: 'flex', flexDirection: 'column',
          boxShadow: '-8px 0 40px rgba(0,0,0,0.2)',
        }}
      >
        {/* Header */}
        <div style={{
          display: 'flex', alignItems: 'center', justifyContent: 'space-between',
          padding: '16px 16px', borderBottom: '1px solid light-dark(rgba(0,0,0,0.08), rgba(255,255,255,0.08))',
        }}>
          <span style={{ fontWeight: 700, fontSize: 15 }}>Mail</span>
          <button onClick={() => setMailOpen(false)} style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'inherit', opacity: 0.5 }}>
            <X size={18} />
          </button>
        </div>

        {/* Tasks list */}
        <div style={{ flex: 1, overflowY: 'auto', padding: '12px 8px' }}>
          {inbox.length > 0 && (
            <>
              <div style={{ fontSize: 10, fontWeight: 700, letterSpacing: '0.08em', textTransform: 'uppercase', opacity: 0.35, padding: '0 12px 6px' }}>
                Inbox ({inbox.length})
              </div>
              {inbox.map(message => (
                <MailRow key={message.messageId} message={message} onSelect={() => setSelected(message)} selected={selected?.messageId === message.messageId} />
              ))}
            </>
          )}

          {outbox.length > 0 && (
            <>
              <div style={{ fontSize: 10, fontWeight: 700, letterSpacing: '0.08em', textTransform: 'uppercase', opacity: 0.35, padding: '12px 12px 6px' }}>
                Outbox ({outbox.length})
              </div>
              {outbox.map(message => (
                <MailRow key={message.messageId} message={message} onSelect={() => setSelected(message)} selected={selected?.messageId === message.messageId} />
              ))}
            </>
          )}

          {inbox.length === 0 && outbox.length === 0 && (
            <div style={{ padding: 24, textAlign: 'center', opacity: 0.3, fontSize: 13 }}>No messages</div>
          )}
        </div>

        {/* Message detail */}
        {selected && (
          <div style={{
            padding: 16, borderTop: '1px solid light-dark(rgba(0,0,0,0.08), rgba(255,255,255,0.08))',
          }}>
            <div style={{ fontSize: 13, fontWeight: 600, marginBottom: 6 }}>
              {selected.task?.goal ?? selected.subject ?? selected.summary ?? selected.kind}
            </div>
            {selected.summary && (
              <div style={{ fontSize: 12, opacity: 0.6, marginBottom: 8 }}>{selected.summary}</div>
            )}
            {selected.bodyPreview && selected.bodyPreview !== selected.summary && (
              <div style={{ fontSize: 12, opacity: 0.55, marginBottom: 8 }}>{selected.bodyPreview}</div>
            )}
            {(selected.task?.error ?? selected.error) && (
              <div style={{ fontSize: 11, color: '#ef4444', marginBottom: 8 }}>{selected.task?.error ?? selected.error}</div>
            )}
            <div style={{ display: 'flex', gap: 8 }}>
              {selected.canClaim && selected.taskId && (
                <button
                  onClick={() => claim(selected.taskId!)}
                  style={{ fontSize: 12, padding: '5px 12px', borderRadius: 8, border: '1px solid currentColor', background: 'none', cursor: 'pointer', color: 'inherit' }}
                >
                  Claim
                </button>
              )}
              {selected.canComplete && selected.taskId && (
                <button
                  onClick={() => complete({ taskId: selected.taskId! })}
                  style={{ fontSize: 12, padding: '5px 12px', borderRadius: 8, border: 'none', background: '#22c55e', color: '#fff', cursor: 'pointer' }}
                >
                  Complete
                </button>
              )}
              {(selected.readOnly || !selected.taskId) && (
                <span style={{ fontSize: 11, opacity: 0.45, alignSelf: 'center' }}>Read-only message</span>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
