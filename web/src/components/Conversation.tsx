import { useRef, useEffect, useState } from 'react'
import { useConversation } from '../hooks/useConversation'
import { rpcCall } from '../lib/rpc'
import { Send } from 'lucide-react'
import type { Item } from '../lib/types'

interface ConversationProps {
  threadId: string | null
  coordinatorRole?: string
}

function MessageBubble({ item, isCoordinator }: { item: Item; isCoordinator: boolean }) {
  const isUser = item.role === 'user'
  const text = item.content ?? item.delta ?? ''
  if (!text) return null

  return (
    <div
      className="animate-fade-in"
      style={{
        display: 'flex',
        justifyContent: isUser ? 'flex-end' : 'flex-start',
        marginBottom: 12,
      }}
    >
      {!isUser && (
        <div style={{
          width: 24, height: 24, borderRadius: '50%',
          background: isCoordinator ? 'rgba(99,102,241,0.15)' : 'rgba(34,197,94,0.12)',
          display: 'flex', alignItems: 'center', justifyContent: 'center',
          fontSize: 9, fontWeight: 700, opacity: 0.8,
          flexShrink: 0, marginRight: 8, marginTop: 2,
        }}>
          {(item.role ?? 'a').slice(0, 2).toUpperCase()}
        </div>
      )}
      <div style={{ maxWidth: '72%' }}>
        {!isUser && (
          <div style={{ fontSize: 10, fontWeight: 600, opacity: 0.45, marginBottom: 3 }}>
            {item.role}
          </div>
        )}
        <div style={{
          padding: '8px 12px',
          borderRadius: isUser ? '12px 12px 3px 12px' : '3px 12px 12px 12px',
          background: isUser
            ? 'rgba(99,102,241,0.9)'
            : isCoordinator
              ? 'light-dark(rgba(0,0,0,0.05), rgba(255,255,255,0.06))'
              : 'light-dark(rgba(0,0,0,0.03), rgba(255,255,255,0.04))',
          color: isUser ? '#fff' : 'inherit',
          fontSize: 13,
          lineHeight: 1.5,
          opacity: isCoordinator || isUser ? 1 : 0.8,
          fontWeight: isCoordinator && !isUser ? 500 : 400,
          whiteSpace: 'pre-wrap',
          wordBreak: 'break-word',
        }}>
          {text}
          {item.status === 'in_progress' && (
            <span style={{ display: 'inline-flex', gap: 2, marginLeft: 6, verticalAlign: 'middle' }}>
              {[0, 1, 2].map(i => (
                <span
                  key={i}
                  style={{
                    width: 4, height: 4, borderRadius: '50%',
                    background: 'currentColor', opacity: 0.4,
                    animation: `fade-in 1s ${i * 0.2}s infinite alternate`,
                  }}
                />
              ))}
            </span>
          )}
        </div>
      </div>
    </div>
  )
}

export default function Conversation({ threadId, coordinatorRole }: ConversationProps) {
  const query = useConversation(threadId)
  const items = query.data ?? []
  const bottomRef = useRef<HTMLDivElement>(null)
  const [input, setInput] = useState('')
  const [sending, setSending] = useState(false)

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [items.length])

  async function sendMessage() {
    const text = input.trim()
    if (!text || !threadId || sending) return
    setSending(true)
    setInput('')
    try {
      await rpcCall('session.sendMessage', { threadId, content: text })
    } catch {
      setInput(text)
    } finally {
      setSending(false)
    }
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      {/* Messages */}
      <div style={{ flex: 1, minHeight: 0, overflowY: 'auto', padding: '20px 24px' }}>
        {!threadId ? (
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%', opacity: 0.3, fontSize: 13 }}>
            Loading conversation…
          </div>
        ) : items.length === 0 ? (
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%', opacity: 0.3, fontSize: 13 }}>
            No messages yet
          </div>
        ) : (
          items.map(item => (
            <MessageBubble
              key={item.id}
              item={item}
              isCoordinator={!item.role || item.role === coordinatorRole || item.role === 'coordinator'}
            />
          ))
        )}
        <div ref={bottomRef} />
      </div>

      {/* Input */}
      <div style={{
        padding: '12px 16px',
        borderTop: '1px solid light-dark(rgba(0,0,0,0.06), rgba(255,255,255,0.06))',
        display: 'flex', gap: 8,
      }}>
        <textarea
          value={input}
          onChange={e => setInput(e.target.value)}
          onKeyDown={e => {
            if (e.key === 'Enter' && !e.shiftKey) {
              e.preventDefault()
              sendMessage()
            }
          }}
          placeholder="Message the coordinator…"
          disabled={!threadId || sending}
          rows={1}
          style={{
            flex: 1, resize: 'none',
            padding: '8px 12px', borderRadius: 10,
            border: '1px solid light-dark(rgba(0,0,0,0.1), rgba(255,255,255,0.1))',
            background: 'light-dark(rgba(0,0,0,0.03), rgba(255,255,255,0.04))',
            color: 'inherit', fontSize: 13, lineHeight: 1.5,
            outline: 'none', fontFamily: 'inherit',
            maxHeight: 120, overflowY: 'auto',
          }}
        />
        <button
          onClick={sendMessage}
          disabled={!input.trim() || !threadId || sending}
          style={{
            width: 36, height: 36, alignSelf: 'flex-end',
            borderRadius: 10, border: 'none',
            background: input.trim() ? 'rgba(99,102,241,0.9)' : 'light-dark(rgba(0,0,0,0.06), rgba(255,255,255,0.06))',
            color: input.trim() ? '#fff' : 'inherit',
            cursor: input.trim() ? 'pointer' : 'default',
            display: 'flex', alignItems: 'center', justifyContent: 'center',
            opacity: (!input.trim() || !threadId) ? 0.4 : 1,
            transition: 'background 0.15s, opacity 0.15s',
            flexShrink: 0,
          }}
        >
          <Send size={14} />
        </button>
      </div>
    </div>
  )
}
