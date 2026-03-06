import { useState } from 'react'
import { useStore } from '../lib/store'
import { useMail } from '../hooks/useMail'
import { X, ArrowUpRight, Inbox, Send, Mail } from 'lucide-react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
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
      className={selected ? '' : 'row-hover'}
      style={{
        padding: '9px 12px', cursor: 'pointer',
        borderRadius: 'var(--r-md)',
        background: selected ? 'var(--bg-active)' : 'transparent',
        display: 'flex', alignItems: 'center', gap: 9,
        margin: '1px 0',
      }}
    >
      <span style={{
        width: 7, height: 7, borderRadius: '50%', flexShrink: 0,
        background: dotColor,
      }} />
      <span className="truncate" style={{
        flex: 1, fontSize: 12, color: 'var(--text-1)',
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
          padding: '1px 6px', borderRadius: 999,
        }}>
          {task.assignedRole}
        </span>
      )}
    </div>
  )
}

function SectionLabel({ icon, label, count }: { icon: React.ReactNode; label: string; count: number }) {
  return (
    <div className="section-label" style={{ padding: '10px 12px 5px' }}>
      {icon}
      {label}
      <span style={{
        background: 'var(--bg-elevated)', border: '1px solid var(--border)',
        borderRadius: 999, padding: '0 6px',
        fontSize: 9, fontWeight: 700, letterSpacing: 0,
        textTransform: 'none',
        fontVariantNumeric: 'tabular-nums',
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
      className="slide-over-backdrop animate-slide-in-right"
      onClick={() => setMailOpen(false)}
    >
      <div
        onClick={e => e.stopPropagation()}
        style={{
          width: 420, height: '100%',
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
          flexShrink: 0,
        }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <span style={{ fontWeight: 600, fontSize: 14, color: 'var(--text-1)', letterSpacing: '-0.02em' }}>
              Mail
            </span>
            {(inbox.length + outbox.length) > 0 && (
              <span style={{
                fontSize: 11, color: 'var(--text-3)',
                background: 'var(--bg-elevated)',
                border: '1px solid var(--border)',
                borderRadius: 999, padding: '0 6px',
                fontVariantNumeric: 'tabular-nums',
              }}>
                {inbox.length + outbox.length}
              </span>
            )}
          </div>
          <button className="btn-ghost" onClick={() => setMailOpen(false)} aria-label="Close">
            <X size={16} />
          </button>
        </div>

        {/* Message list */}
        <div style={{ flex: 1, minHeight: 0, overflowY: 'auto', padding: '4px 8px' }}>
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
            <div style={{
              display: 'flex', flexDirection: 'column', alignItems: 'center',
              padding: '40px 20px', textAlign: 'center', gap: 8,
            }}>
              <Mail size={24} style={{ color: 'var(--text-3)' }} />
              <div style={{ color: 'var(--text-3)', fontSize: 13 }}>No messages</div>
              <div style={{ color: 'var(--text-3)', fontSize: 11, lineHeight: 1.5 }}>
                Inter-team messages will appear here
              </div>
            </div>
          )}
        </div>

        {/* Selected message detail */}
        {selected && (
          <div style={{
            flex: '0 0 55%',
            minHeight: 0,
            display: 'flex',
            flexDirection: 'column',
            borderTop: '1px solid var(--border)',
            background: 'var(--bg-surface)',
          }}>
            {/* Detail header */}
            <div style={{
              padding: '12px 16px 10px',
              borderBottom: '1px solid var(--border)',
              flexShrink: 0,
            }}>
              <div style={{ fontSize: 13, fontWeight: 600, color: 'var(--text-1)', letterSpacing: '-0.01em', lineHeight: 1.4 }}>
                {selected.task?.goal ?? selected.subject ?? selected.summary ?? selected.kind}
              </div>
            </div>

            {/* Scrollable body */}
            <div style={{ flex: 1, minHeight: 0, overflowY: 'auto', padding: '12px 16px' }}>
              {selected.summary && (
                <div className="md-prose" style={{ fontSize: 12, color: 'var(--text-2)', marginBottom: 10, lineHeight: 1.6 }}>
                  <ReactMarkdown remarkPlugins={[remarkGfm]}>{selected.summary}</ReactMarkdown>
                </div>
              )}
              {selected.bodyPreview && selected.bodyPreview !== selected.summary && (
                <div className="md-prose" style={{ fontSize: 12, color: 'var(--text-2)', marginBottom: 10, lineHeight: 1.6 }}>
                  <ReactMarkdown remarkPlugins={[remarkGfm]}>{selected.bodyPreview}</ReactMarkdown>
                </div>
              )}
              {(selected.task?.error ?? selected.error) && (
                <div style={{
                  fontSize: 11, color: 'var(--red)', marginBottom: 8,
                  background: 'var(--red-dim)', border: '1px solid rgba(239,68,68,0.2)',
                  padding: '8px 12px', borderRadius: 'var(--r-md)',
                  lineHeight: 1.5,
                }}>
                  {selected.task?.error ?? selected.error}
                </div>
              )}
            </div>

            {/* Action buttons */}
            <div style={{
              padding: '10px 16px',
              borderTop: '1px solid var(--border)',
              display: 'flex', gap: 8,
              flexShrink: 0,
            }}>
              {selected.canClaim && selected.taskId && (
                <button
                  className="btn-surface"
                  onClick={() => claim(selected.taskId!)}
                >
                  Claim
                </button>
              )}
              {selected.canComplete && selected.taskId && (
                <button
                  onClick={() => complete({ taskId: selected.taskId! })}
                  style={{
                    fontSize: 12, fontWeight: 500,
                    padding: '7px 16px', borderRadius: 'var(--r-md)',
                    border: 'none',
                    background: 'var(--green)',
                    color: '#fff', cursor: 'pointer',
                    fontFamily: 'inherit',
                    transition: 'opacity 0.15s',
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
