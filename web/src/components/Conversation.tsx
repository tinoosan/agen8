import { useRef, useEffect, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useConversation } from '../hooks/useConversation'
import { rpcCall, isConnected } from '../lib/rpc'
import { ArrowUp, ChevronRight, Zap } from 'lucide-react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import {
  type Item,
  type UserMessageContent,
  type AgentMessageContent,
  type ToolExecutionContent,
  type ReasoningContent,
} from '../lib/types'

interface ConversationProps {
  threadId: string | null
}

// ──────────────────────────────────────────────
// ThinkingBlock — collapsible reasoning display
// ──────────────────────────────────────────────
function ThinkingBlock({ item }: { item: Item }) {
  const [open, setOpen] = useState(false)
  const content = item.content as ReasoningContent | undefined
  const text = content?.summary ?? ''
  const isStreaming = item.status === 'started' || item.status === 'streaming'
  const step = content?.step

  return (
    <div className="animate-fade-in" style={{ margin: '2px 0 2px 48px' }}>
      <button
        onClick={() => !isStreaming && setOpen((o) => !o)}
        style={{
          display: 'flex', alignItems: 'center', gap: 6,
          background: 'none', border: 'none',
          cursor: isStreaming ? 'default' : 'pointer',
          color: 'var(--text-3)', fontSize: 11.5, padding: '3px 0',
        }}
      >
        {isStreaming ? (
          <span style={{ color: 'var(--amber)', display: 'flex', alignItems: 'center', gap: 5 }}>
            <span
              className="streaming-cursor"
              style={{ width: 6, height: 6, background: 'var(--amber)', borderRadius: '50%', display: 'inline-block' }}
            />
            thinking…
          </span>
        ) : (
          <>
            <ChevronRight
              size={11}
              style={{
                transform: open ? 'rotate(90deg)' : 'rotate(0deg)',
                transition: 'transform 0.15s',
                color: 'var(--text-3)',
              }}
            />
            <span>thought{step != null ? ` ${step}` : ''}</span>
          </>
        )}
      </button>
      {open && text && (
        <div style={{
          marginTop: 4, padding: '8px 12px',
          borderLeft: '2px solid var(--border)',
          fontSize: 12, color: 'var(--text-3)',
          fontStyle: 'italic', whiteSpace: 'pre-wrap',
          lineHeight: 1.6, wordBreak: 'break-word',
          maxHeight: 240, overflowY: 'auto',
        }}>
          {text}
        </div>
      )}
    </div>
  )
}

// ──────────────────────────────────────────────
// ToolCall — expandable tool execution chip
// ──────────────────────────────────────────────
function ToolCall({ item }: { item: Item }) {
  const [open, setOpen] = useState(false)
  const tc = item.content as ToolExecutionContent | undefined
  if (!tc) return null

  const isPending = item.status === 'started' || item.status === 'streaming' || tc.ok === undefined
  const isOk = tc.ok !== false
  const hasDetail = tc.input != null || tc.output != null

  function prettyJson(val: unknown): string {
    if (typeof val === 'string') return val
    try { return JSON.stringify(val, null, 2) } catch { return String(val) }
  }

  return (
    <div className="animate-fade-in" style={{ margin: '2px 0 2px 48px' }}>
      <button
        onClick={() => hasDetail && setOpen((o) => !o)}
        style={{
          display: 'inline-flex', alignItems: 'center', gap: 7,
          background: isPending ? 'var(--bg-elevated)' : isOk ? 'rgba(34,197,94,0.06)' : 'var(--red-dim)',
          border: `1px solid ${isPending ? 'var(--border)' : isOk ? 'rgba(34,197,94,0.2)' : 'rgba(239,68,68,0.2)'}`,
          borderRadius: 8, padding: '5px 10px',
          cursor: hasDetail ? 'pointer' : 'default',
          textAlign: 'left',
        }}
      >
        {isPending ? (
          <span style={{ fontSize: 11, color: 'var(--amber)' }}>◌</span>
        ) : (
          <span style={{ fontSize: 11, color: isOk ? 'var(--green)' : 'var(--red)', fontWeight: 700 }}>
            {isOk ? '✓' : '✗'}
          </span>
        )}
        <span className="mono" style={{ fontSize: 12, color: 'var(--text-2)', fontWeight: 500 }}>
          {tc.toolName}
        </span>
        {hasDetail && (
          <ChevronRight
            size={10}
            style={{
              color: 'var(--text-3)',
              transform: open ? 'rotate(90deg)' : 'rotate(0deg)',
              transition: 'transform 0.15s',
            }}
          />
        )}
      </button>

      {open && (
        <div style={{ marginTop: 4, paddingLeft: 12, borderLeft: '2px solid var(--border)' }}>
          {tc.input != null && (
            <>
              <div style={{
                fontSize: 9, color: 'var(--text-3)', fontWeight: 600,
                letterSpacing: '0.06em', textTransform: 'uppercase', marginBottom: 3,
              }}>
                Input
              </div>
              <pre className="mono" style={{
                fontSize: 11, color: 'var(--text-2)', background: 'var(--bg-elevated)',
                border: '1px solid var(--border)', borderRadius: 6, padding: '6px 10px',
                overflow: 'auto', maxHeight: 200, margin: '0 0 6px',
              }}>
                {prettyJson(tc.input)}
              </pre>
            </>
          )}
          {tc.output != null && (
            <>
              <div style={{
                fontSize: 9, color: 'var(--text-3)', fontWeight: 600,
                letterSpacing: '0.06em', textTransform: 'uppercase', marginBottom: 3,
              }}>
                Output
              </div>
              <pre className="mono" style={{
                fontSize: 11, color: 'var(--text-2)', background: 'var(--bg-elevated)',
                border: '1px solid var(--border)', borderRadius: 6, padding: '6px 10px',
                overflow: 'auto', maxHeight: 200, margin: 0,
              }}>
                {prettyJson(tc.output)}
              </pre>
            </>
          )}
        </div>
      )}
    </div>
  )
}

// ──────────────────────────────────────────────
// AgentBubble — markdown-rendered agent message
// ──────────────────────────────────────────────
function AgentBubble({ item }: { item: Item }) {
  const content = item.content as AgentMessageContent | undefined
  const text = content?.text ?? ''
  const isStreaming =
    item.status === 'streaming' || (item.status === 'started') || content?.isPartial === true

  return (
    <div
      className="animate-fade-in"
      style={{
        display: 'flex', justifyContent: 'flex-start',
        alignItems: 'flex-start', gap: 10, marginBottom: 16,
      }}
    >
      <div style={{
        width: 30, height: 30, borderRadius: 8,
        background: 'linear-gradient(135deg, var(--accent-dim), rgba(99,102,241,0.2))',
        border: '1px solid rgba(139,123,248,0.2)',
        display: 'flex', alignItems: 'center', justifyContent: 'center',
        flexShrink: 0, marginTop: 1,
      }}>
        <Zap size={13} color="var(--accent)" fill="var(--accent)" strokeWidth={0} />
      </div>
      <div style={{ maxWidth: '78%', minWidth: 0 }}>
        <div style={{
          padding: '10px 14px',
          borderRadius: '4px 14px 14px 14px',
          background: 'var(--bg-elevated)',
          border: '1px solid var(--border)',
          color: 'var(--text-1)',
          fontSize: 13.5,
          overflowWrap: 'break-word',
        }}>
          <div className="md-prose">
            <ReactMarkdown
              remarkPlugins={[remarkGfm]}
              components={{
                code({ children, className, ...props }) {
                  const isBlock = className?.startsWith('language-')
                  if (isBlock) {
                    return (
                      <pre style={{
                        background: 'var(--bg-app)', border: '1px solid var(--border)',
                        borderRadius: 8, padding: '10px 14px', overflow: 'auto',
                        fontSize: 12, margin: '6px 0',
                      }}>
                        <code className="mono" {...props}>{children}</code>
                      </pre>
                    )
                  }
                  return (
                    <code
                      className="mono"
                      style={{
                        background: 'var(--bg-surface)', padding: '1px 5px',
                        borderRadius: 4, fontSize: '0.88em',
                      }}
                      {...props}
                    >
                      {children}
                    </code>
                  )
                },
                a({ children, href }) {
                  return (
                    <a
                      href={href}
                      target="_blank"
                      rel="noopener noreferrer"
                      style={{ color: 'var(--accent)', textDecoration: 'underline' }}
                    >
                      {children}
                    </a>
                  )
                },
              }}
            >
              {text}
            </ReactMarkdown>
          </div>
          {isStreaming && <span className="streaming-cursor" />}
        </div>
      </div>
    </div>
  )
}

// ──────────────────────────────────────────────
// MessageBubble — dispatcher
// ──────────────────────────────────────────────
function MessageBubble({ item }: { item: Item }) {
  if (item.type === 'tool_execution') return <ToolCall item={item} />
  if (item.type === 'reasoning') return <ThinkingBlock item={item} />

  if (item.type === 'agent_message') return <AgentBubble item={item} />

  if (item.type === 'user_message') {
    const content = item.content as UserMessageContent | undefined
    const text = content?.text ?? ''
    if (!text) return null
    return (
      <div
        className="animate-fade-in"
        style={{
          display: 'flex', justifyContent: 'flex-end',
          alignItems: 'flex-start', gap: 10, marginBottom: 16,
        }}
      >
        <div style={{ maxWidth: '78%', minWidth: 0 }}>
          <div style={{
            padding: '10px 15px',
            borderRadius: '14px 14px 4px 14px',
            background: 'var(--accent-solid)',
            color: '#fff',
            fontSize: 13.5,
            lineHeight: 1.6,
            whiteSpace: 'pre-wrap',
            wordBreak: 'break-word',
            boxShadow: '0 1px 3px rgba(0,0,0,0.3)',
          }}>
            {text}
          </div>
        </div>
      </div>
    )
  }

  return null
}

// ──────────────────────────────────────────────
// Main Conversation component
// ──────────────────────────────────────────────
export default function Conversation({ threadId }: ConversationProps) {
  const { query, registerTurnId } = useConversation(threadId)
  const items = query.data ?? []
  const queryClient = useQueryClient()
  const bottomRef = useRef<HTMLDivElement>(null)
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const [input, setInput] = useState('')
  const [sending, setSending] = useState(false)
  const [sendError, setSendError] = useState<string | null>(null)
  const [sseOk, setSseOk] = useState(false)

  // Poll SSE connection status every 1.5s
  useEffect(() => {
    setSseOk(isConnected())
    const t = setInterval(() => setSseOk(isConnected()), 1500)
    return () => clearInterval(t)
  }, [])

  // Scroll to bottom when new items arrive
  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [items.length])

  // Auto-resize textarea
  useEffect(() => {
    const el = textareaRef.current
    if (!el) return
    el.style.height = 'auto'
    el.style.height = Math.min(el.scrollHeight, 140) + 'px'
  }, [input])

  async function sendMessage() {
    const text = input.trim()
    if (!text || !threadId || sending) return
    setSending(true)
    setSendError(null)
    setInput('')
    try {
      const result = await rpcCall<{ turn: { id: string } }>('turn.create', {
        threadId,
        input: { text },
      })
      // Pre-register this turnId so item.started notifications are accepted
      // immediately — before SSE delivers items for this turn.
      registerTurnId(result.turn.id)
      const syntheticItem: Item = {
        id: `optimistic-${Date.now()}`,
        turnId: result.turn.id,
        type: 'user_message',
        status: 'completed',
        content: { text } as UserMessageContent,
      }
      queryClient.setQueryData<Item[]>(['item.list', threadId], (prev) => [
        ...(prev ?? []),
        syntheticItem,
      ])
    } catch (err) {
      setSendError(err instanceof Error ? err.message : 'Failed to send message')
      setInput(text)
    } finally {
      setSending(false)
    }
  }

  const canSend = !!input.trim() && !!threadId && !sending

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      {/* Message list */}
      <div style={{ flex: 1, minHeight: 0, overflowY: 'auto', padding: '28px 32px 12px' }}>
        {!threadId ? (
          <div style={{
            display: 'flex', alignItems: 'center', justifyContent: 'center',
            height: '100%', color: 'var(--text-3)', fontSize: 13,
          }}>
            Connecting…
          </div>
        ) : items.length === 0 ? (
          <div style={{
            display: 'flex', flexDirection: 'column',
            alignItems: 'center', justifyContent: 'center',
            height: '100%', gap: 14, textAlign: 'center',
          }}>
            <div style={{
              width: 52, height: 52, borderRadius: 14,
              background: 'var(--accent-dim)',
              border: '1px solid rgba(139,123,248,0.2)',
              display: 'flex', alignItems: 'center', justifyContent: 'center',
            }}>
              <Zap size={22} color="var(--accent)" fill="var(--accent)" strokeWidth={0} />
            </div>
            <div>
              <div style={{ fontSize: 14, fontWeight: 600, color: 'var(--text-1)', marginBottom: 5 }}>
                Coordinator ready
              </div>
              <div style={{ fontSize: 13, color: 'var(--text-3)' }}>
                Send a message to get started
              </div>
            </div>
          </div>
        ) : (
          items.map((item) => <MessageBubble key={item.id} item={item} />)
        )}
        <div ref={bottomRef} />
      </div>

      {/* Error banner */}
      {sendError && (
        <div style={{
          padding: '8px 20px', fontSize: 12, color: 'var(--red)',
          background: 'var(--red-dim)', borderTop: '1px solid rgba(239,68,68,0.15)',
          display: 'flex', justifyContent: 'space-between', alignItems: 'center',
          flexShrink: 0,
        }}>
          <span>{sendError}</span>
          <button
            onClick={() => setSendError(null)}
            style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'inherit', fontSize: 16, lineHeight: 1, padding: '0 0 0 8px' }}
          >
            ×
          </button>
        </div>
      )}

      {/* Input area */}
      <div style={{ padding: '10px 20px 16px', borderTop: '1px solid var(--border)', flexShrink: 0 }}>
        <div style={{
          display: 'flex', alignItems: 'flex-end', gap: 8,
          background: 'var(--bg-elevated)',
          border: '1px solid var(--border)',
          borderRadius: 'var(--r-xl)',
          padding: '8px 10px 8px 16px',
          transition: 'border-color 0.15s',
        }}>
          <textarea
            ref={textareaRef}
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault()
                sendMessage()
              }
            }}
            placeholder={sending ? 'Sending…' : 'Message the coordinator…'}
            disabled={!threadId || sending}
            rows={1}
            style={{
              flex: 1,
              resize: 'none',
              border: 'none',
              background: 'transparent',
              color: 'var(--text-1)',
              fontSize: 13.5,
              lineHeight: 1.55,
              outline: 'none',
              fontFamily: 'inherit',
              maxHeight: 140,
              overflowY: 'auto',
              padding: 0,
            }}
          />
          {/* SSE connection status dot */}
          <div
            title={sseOk ? 'Connected' : 'Reconnecting…'}
            style={{
              width: 7, height: 7, borderRadius: '50%', flexShrink: 0, alignSelf: 'center',
              background: sseOk ? 'var(--green)' : 'var(--text-3)',
              boxShadow: sseOk ? '0 0 6px var(--green)' : 'none',
              transition: 'background 0.3s, box-shadow 0.3s',
            }}
          />
          <button
            onClick={sendMessage}
            disabled={!canSend}
            style={{
              width: 32, height: 32,
              borderRadius: 10,
              border: 'none',
              background: canSend ? 'var(--accent-solid)' : 'var(--bg-active)',
              color: canSend ? '#fff' : 'var(--text-3)',
              cursor: canSend ? 'pointer' : 'default',
              display: 'flex', alignItems: 'center', justifyContent: 'center',
              flexShrink: 0,
              transition: 'background 0.15s, transform 0.1s',
              transform: canSend ? 'scale(1)' : 'scale(0.9)',
            }}
          >
            <ArrowUp size={15} />
          </button>
        </div>
        <div style={{
          textAlign: 'center', marginTop: 7,
          fontSize: 11, color: 'var(--text-3)',
        }}>
          Enter to send · Shift+Enter for newline
        </div>
      </div>
    </div>
  )
}
