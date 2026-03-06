import { useRef, useEffect, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useConversation } from '../hooks/useConversation'
import { rpcCall } from '../lib/rpc'
import { ArrowUp, Zap } from 'lucide-react'
import {
  getItemText,
  type Item,
  type UserMessageContent,
  type AgentMessageContent,
  type ToolExecutionContent,
} from '../lib/types'

interface ConversationProps {
  threadId: string | null
}

function ToolChip({ item }: { item: Item }) {
  const tc = item.content as ToolExecutionContent | undefined
  if (!tc) return null
  const isOk = tc.ok !== false
  const isPending = tc.ok === undefined

  return (
    <div
      className="animate-fade-in"
      style={{
        margin: '3px 0 3px 48px',
        display: 'inline-flex', alignItems: 'center', gap: 7,
        padding: '4px 10px',
        borderRadius: 'var(--r-md)',
        background: isPending ? 'var(--bg-elevated)' : isOk ? 'rgba(34,197,94,0.06)' : 'var(--red-dim)',
        border: `1px solid ${isPending ? 'var(--border)' : isOk ? 'rgba(34,197,94,0.15)' : 'rgba(239,68,68,0.2)'}`,
        fontSize: 11.5,
      }}
    >
      <span style={{ color: 'var(--text-3)' }} className="mono">›</span>
      <span style={{ fontWeight: 500, color: 'var(--text-2)', fontFamily: 'inherit' }} className="mono">
        {tc.toolName}
      </span>
      {tc.ok !== undefined && (
        <span style={{
          fontSize: 10, fontWeight: 600,
          color: tc.ok ? 'var(--green)' : 'var(--red)',
          letterSpacing: '0.04em',
        }}>
          {tc.ok ? 'OK' : 'ERR'}
        </span>
      )}
      {isPending && (
        <span style={{ color: 'var(--text-3)', fontSize: 10 }} className="streaming-cursor">▋</span>
      )}
    </div>
  )
}

function ReasoningBlock({ item }: { item: Item }) {
  const text = getItemText(item)
  if (!text) return null
  return (
    <div
      className="animate-fade-in"
      style={{
        margin: '3px 0 3px 48px',
        padding: '8px 12px',
        fontSize: 12,
        color: 'var(--text-3)',
        fontStyle: 'italic',
        borderLeft: '2px solid var(--border)',
        whiteSpace: 'pre-wrap',
        wordBreak: 'break-word',
        lineHeight: 1.6,
      }}
    >
      {text}
    </div>
  )
}

function MessageBubble({ item }: { item: Item }) {
  const isUser = item.type === 'user_message'
  const text = getItemText(item)
  const isStreaming =
    item.status === 'streaming' ||
    (item.type === 'agent_message' && (item.content as AgentMessageContent)?.isPartial)

  if (item.type === 'tool_execution') return <ToolChip item={item} />
  if (item.type === 'reasoning') return <ReasoningBlock item={item} />
  if (!text) return null

  return (
    <div
      className="animate-fade-in"
      style={{
        display: 'flex',
        justifyContent: isUser ? 'flex-end' : 'flex-start',
        alignItems: 'flex-start',
        gap: 10,
        marginBottom: 16,
      }}
    >
      {!isUser && (
        <div
          style={{
            width: 30, height: 30,
            borderRadius: 8,
            background: 'linear-gradient(135deg, var(--accent-dim), rgba(99,102,241,0.2))',
            border: '1px solid rgba(139,123,248,0.2)',
            display: 'flex', alignItems: 'center', justifyContent: 'center',
            flexShrink: 0,
            marginTop: 1,
          }}
        >
          <Zap size={13} color="var(--accent)" fill="var(--accent)" strokeWidth={0} />
        </div>
      )}

      <div style={{ maxWidth: '78%', minWidth: 0 }}>
        {isUser ? (
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
        ) : (
          <div style={{
            padding: '10px 14px',
            borderRadius: '4px 14px 14px 14px',
            background: 'var(--bg-elevated)',
            border: '1px solid var(--border)',
            color: 'var(--text-1)',
            fontSize: 13.5,
            lineHeight: 1.65,
            whiteSpace: 'pre-wrap',
            wordBreak: 'break-word',
          }}>
            {text}
            {isStreaming && <span className="streaming-cursor" />}
          </div>
        )}
      </div>
    </div>
  )
}

export default function Conversation({ threadId }: ConversationProps) {
  const query = useConversation(threadId)
  const items = query.data ?? []
  const queryClient = useQueryClient()
  const bottomRef = useRef<HTMLDivElement>(null)
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const [input, setInput] = useState('')
  const [sending, setSending] = useState(false)

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
    setInput('')
    try {
      const result = await rpcCall<{ turn: { id: string } }>('turn.create', {
        threadId,
        input: { text },
      })
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
    } catch {
      setInput(text)
    } finally {
      setSending(false)
    }
  }

  const canSend = !!input.trim() && !!threadId && !sending

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      {/* Message list */}
      <div
        style={{
          flex: 1,
          minHeight: 0,
          overflowY: 'auto',
          padding: '28px 32px 12px',
        }}
      >
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

      {/* Input area */}
      <div style={{
        padding: '10px 20px 16px',
        borderTop: '1px solid var(--border)',
      }}>
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
