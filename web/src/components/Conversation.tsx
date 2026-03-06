import { useRef, useEffect, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useConversation } from '../hooks/useConversation'
import { rpcCall } from '../lib/rpc'
import { Send } from 'lucide-react'
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

function MessageBubble({ item }: { item: Item }) {
  const isUser = item.type === 'user_message'
  const text = getItemText(item)
  const isStreaming =
    item.status === 'streaming' ||
    (item.type === 'agent_message' && (item.content as AgentMessageContent)?.isPartial)

  // Tool execution — compact chip
  if (item.type === 'tool_execution') {
    const tc = item.content as ToolExecutionContent | undefined
    if (!tc) return null
    return (
      <div
        className="animate-fade-in"
        style={{
          margin: '4px 0 4px 38px',
          padding: '5px 10px',
          borderRadius: 8,
          background: 'light-dark(rgba(0,0,0,0.03), rgba(255,255,255,0.04))',
          border: '1px solid light-dark(rgba(0,0,0,0.06), rgba(255,255,255,0.06))',
          fontSize: 12,
          fontFamily: '"SF Mono", "Fira Code", "Cascadia Code", monospace',
          opacity: 0.7,
          display: 'flex',
          alignItems: 'center',
          gap: 6,
        }}
      >
        <span style={{ opacity: 0.4, fontSize: 10 }}>{'>'}</span>
        <span style={{ fontWeight: 600 }}>{tc.toolName}</span>
        {tc.ok !== undefined && (
          <span
            style={{
              color: tc.ok ? '#22c55e' : '#ef4444',
              fontSize: 10,
              fontWeight: 600,
            }}
          >
            {tc.ok ? 'ok' : 'err'}
          </span>
        )}
      </div>
    )
  }

  // Reasoning — dimmed italic with left border
  if (item.type === 'reasoning') {
    if (!text) return null
    return (
      <div
        className="animate-fade-in"
        style={{
          margin: '4px 0 4px 38px',
          padding: '4px 10px',
          fontSize: 12,
          opacity: 0.4,
          fontStyle: 'italic',
          borderLeft: '2px solid light-dark(rgba(0,0,0,0.1), rgba(255,255,255,0.1))',
          whiteSpace: 'pre-wrap',
          wordBreak: 'break-word',
        }}
      >
        {text}
      </div>
    )
  }

  if (!text) return null

  return (
    <div
      className="animate-fade-in"
      style={{
        display: 'flex',
        justifyContent: isUser ? 'flex-end' : 'flex-start',
        marginBottom: 14,
      }}
    >
      {!isUser && (
        <div
          style={{
            width: 28,
            height: 28,
            borderRadius: '50%',
            background: 'light-dark(rgba(99,102,241,0.08), rgba(99,102,241,0.15))',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            fontSize: 10,
            fontWeight: 700,
            flexShrink: 0,
            marginRight: 10,
            marginTop: 2,
            color: 'rgb(99,102,241)',
          }}
        >
          AI
        </div>
      )}
      <div style={{ maxWidth: '75%', minWidth: 0 }}>
        <div
          style={{
            padding: '10px 14px',
            borderRadius: isUser ? '14px 14px 4px 14px' : '4px 14px 14px 14px',
            background: isUser
              ? 'rgba(99,102,241,0.9)'
              : 'light-dark(rgba(0,0,0,0.04), rgba(255,255,255,0.06))',
            color: isUser ? '#fff' : 'inherit',
            fontSize: 13.5,
            lineHeight: 1.6,
            whiteSpace: 'pre-wrap',
            wordBreak: 'break-word',
          }}
        >
          {text}
          {isStreaming && (
            <span
              className="streaming-cursor"
              style={{
                display: 'inline-block',
                width: 2,
                height: '1em',
                background: 'currentColor',
                marginLeft: 2,
                verticalAlign: 'text-bottom',
              }}
            />
          )}
        </div>
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

  // Auto-resize textarea.
  useEffect(() => {
    const el = textareaRef.current
    if (!el) return
    el.style.height = 'auto'
    el.style.height = Math.min(el.scrollHeight, 120) + 'px'
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
      // Optimistic: add user message to cache immediately.
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
      setInput(text) // restore on failure
    } finally {
      setSending(false)
    }
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      {/* Messages */}
      <div
        style={{
          flex: 1,
          minHeight: 0,
          overflowY: 'auto',
          padding: '24px 28px',
        }}
      >
        {!threadId ? (
          <div
            style={{
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              height: '100%',
              opacity: 0.3,
              fontSize: 13,
            }}
          >
            Loading conversation…
          </div>
        ) : items.length === 0 ? (
          <div
            style={{
              display: 'flex',
              flexDirection: 'column',
              alignItems: 'center',
              justifyContent: 'center',
              height: '100%',
              gap: 12,
              opacity: 0.35,
            }}
          >
            <div style={{ fontSize: 32 }}>{'>'}_</div>
            <div style={{ fontSize: 13 }}>Send a message to the coordinator</div>
          </div>
        ) : (
          items.map((item) => <MessageBubble key={item.id} item={item} />)
        )}
        <div ref={bottomRef} />
      </div>

      {/* Input */}
      <div
        style={{
          padding: '12px 20px 16px',
          borderTop: '1px solid light-dark(rgba(0,0,0,0.06), rgba(255,255,255,0.06))',
          display: 'flex',
          gap: 8,
          alignItems: 'flex-end',
        }}
      >
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
          placeholder="Message the coordinator…"
          disabled={!threadId || sending}
          rows={1}
          style={{
            flex: 1,
            resize: 'none',
            padding: '10px 14px',
            borderRadius: 12,
            border: '1px solid light-dark(rgba(0,0,0,0.1), rgba(255,255,255,0.1))',
            background: 'light-dark(rgba(0,0,0,0.02), rgba(255,255,255,0.04))',
            color: 'inherit',
            fontSize: 13.5,
            lineHeight: 1.5,
            outline: 'none',
            fontFamily: 'inherit',
            maxHeight: 120,
            overflowY: 'auto',
            transition: 'border-color 0.15s, box-shadow 0.15s',
          }}
        />
        <button
          onClick={sendMessage}
          disabled={!input.trim() || !threadId || sending}
          style={{
            width: 38,
            height: 38,
            borderRadius: 12,
            border: 'none',
            background: input.trim()
              ? 'rgba(99,102,241,0.9)'
              : 'light-dark(rgba(0,0,0,0.06), rgba(255,255,255,0.06))',
            color: input.trim() ? '#fff' : 'inherit',
            cursor: input.trim() ? 'pointer' : 'default',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            opacity: !input.trim() || !threadId ? 0.4 : 1,
            transition: 'background 0.15s, opacity 0.15s',
            flexShrink: 0,
          }}
        >
          <Send size={15} />
        </button>
      </div>
    </div>
  )
}
