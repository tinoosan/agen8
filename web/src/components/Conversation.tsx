import { useEffect, useMemo, useRef, useState } from 'react'
import { useActivity } from '../hooks/useActivity'
import { useConversation } from '../hooks/useConversation'
import { useTaskHistory } from '../hooks/useTaskHistory'
import { rpcCall } from '../lib/rpc'
import { ArrowUp, Zap } from 'lucide-react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import type { ActivityEvent, Item, Task, UserMessageContent, AgentMessageContent } from '../lib/types'

interface ConversationProps {
  threadId: string | null
  teamId: string
  coordinatorRole: string | null
}

interface ChatEntry {
  id: string
  kind: 'user' | 'agent'
  text: string
  role?: string
  createdAt: number
}

interface ChatTurn {
  id: string
  kind: 'user' | 'agent'
  role?: string
  texts: string[]
}

function AgentBubble({ entry }: { entry: ChatEntry }) {
  return (
    <div
      className="animate-fade-in"
      style={{
        display: 'flex',
        justifyContent: 'flex-start',
        alignItems: 'flex-start',
        gap: 10,
        marginBottom: 16,
      }}
    >
      <div
        style={{
          width: 30,
          height: 30,
          borderRadius: 8,
          background: 'linear-gradient(135deg, var(--accent-dim), rgba(99,102,241,0.2))',
          border: '1px solid rgba(139,123,248,0.2)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          flexShrink: 0,
          marginTop: 1,
        }}
      >
        <Zap size={13} color="var(--accent)" fill="var(--accent)" strokeWidth={0} />
      </div>
      <div style={{ maxWidth: '78%', minWidth: 0 }}>
        {entry.role && (
          <div style={{ fontSize: 11, color: 'var(--text-3)', marginBottom: 5, textTransform: 'uppercase', letterSpacing: '0.05em' }}>
            {entry.role}
          </div>
        )}
        <div
          style={{
            padding: '10px 14px',
            borderRadius: '4px 14px 14px 14px',
            background: 'var(--bg-elevated)',
            border: '1px solid var(--border)',
            color: 'var(--text-1)',
            fontSize: 13.5,
            overflowWrap: 'break-word',
          }}
        >
          <div className="md-prose">
            <ReactMarkdown
              remarkPlugins={[remarkGfm]}
              components={{
                code({ children, className, ...props }) {
                  const isBlock = className?.startsWith('language-')
                  if (isBlock) {
                    return (
                      <pre
                        style={{
                          background: 'var(--bg-app)',
                          border: '1px solid var(--border)',
                          borderRadius: 8,
                          padding: '10px 14px',
                          overflow: 'auto',
                          fontSize: 12,
                          margin: '6px 0',
                        }}
                      >
                        <code className="mono" {...props}>
                          {children}
                        </code>
                      </pre>
                    )
                  }
                  return (
                    <code
                      className="mono"
                      style={{
                        background: 'var(--bg-surface)',
                        padding: '1px 5px',
                        borderRadius: 4,
                        fontSize: '0.88em',
                      }}
                      {...props}
                    >
                      {children}
                    </code>
                  )
                },
                a({ children, href }) {
                  return (
                    <a href={href} target="_blank" rel="noopener noreferrer" style={{ color: 'var(--accent)', textDecoration: 'underline' }}>
                      {children}
                    </a>
                  )
                },
              }}
            >
              {entry.text}
            </ReactMarkdown>
          </div>
        </div>
      </div>
    </div>
  )
}

function UserBubble({ entry }: { entry: ChatEntry }) {
  return (
    <div
      className="animate-fade-in"
      style={{
        display: 'flex',
        justifyContent: 'flex-end',
        alignItems: 'flex-start',
        gap: 10,
        marginBottom: 16,
      }}
    >
      <div style={{ maxWidth: '78%', minWidth: 0 }}>
        <div
          style={{
            padding: '10px 15px',
            borderRadius: '14px 14px 4px 14px',
            background: 'var(--accent-solid)',
            color: '#fff',
            fontSize: 13.5,
            lineHeight: 1.6,
            whiteSpace: 'pre-wrap',
            wordBreak: 'break-word',
            boxShadow: '0 1px 3px rgba(0,0,0,0.3)',
          }}
        >
          {entry.text}
        </div>
      </div>
    </div>
  )
}

function toChatEntry(event: ActivityEvent): ChatEntry | null {
  const kind = (event.kind ?? '').trim().toLowerCase()
  const text = extractActivityText(event)
  if (!text || isTaskDonePlaceholder(text)) return null

  if (kind === 'user_message') {
    return {
      id: event.id,
      kind: 'user',
      text,
      role: 'You',
      createdAt: getTimestampMs(event.startedAt),
    }
  }

  if (isAgentTextKind(kind)) {
    return {
      id: event.id,
      kind: 'agent',
      text,
      role: extractRole(event),
      createdAt: getTimestampMs(event.finishedAt ?? event.startedAt),
    }
  }

  return null
}

function itemToChatEntry(item: Item): ChatEntry | null {
  if (item.type === 'user_message') {
    const content = item.content as UserMessageContent | undefined
    const text = content?.text?.trim()
    if (!text) return null
    return {
      id: item.id,
      kind: 'user',
      text,
      role: 'You',
      createdAt: getTimestampMs(item.createdAt),
    }
  }
  if (item.type === 'agent_message') {
    const content = item.content as AgentMessageContent | undefined
    const text = content?.text?.trim()
    if (!text) return null
    return {
      id: item.id,
      kind: 'agent',
      text,
      role: 'agent',
      createdAt: getTimestampMs(item.createdAt),
    }
  }
  return null
}

function taskToChatEntry(task: Task): ChatEntry | null {
  const summary = task.summary?.trim()
  if (!summary) return null
  const kind = (task.taskKind ?? '').trim().toLowerCase()
  if (kind === 'user_message') return null

  return {
    id: `task:${task.id}`,
    kind: 'agent',
    text: summary,
    role: task.assignedRole?.trim() || task.roleSnapshot?.trim() || 'agent',
    createdAt: getTimestampMs(task.completedAt || task.createdAt),
  }
}

function extractActivityText(event: ActivityEvent): string {
  const kind = (event.kind ?? '').trim().toLowerCase()
  if (kind === 'user_message' && event.textPreview?.trim()) return event.textPreview.trim()
  if ((kind === 'task.done' || kind === 'agent_speak' || kind === 'model_response') && event.outputPreview?.trim()) {
    return event.outputPreview.trim()
  }
  if (event.title?.trim()) return event.title.trim()
  if (event.textPreview?.trim()) return event.textPreview.trim()
  if (event.outputPreview?.trim()) return event.outputPreview.trim()
  return ''
}

function extractRole(event: ActivityEvent): string {
  return event.data?.role?.trim() || event.data?.agent_role?.trim() || 'agent'
}

function isAgentTextKind(kind: string): boolean {
  if (kind === 'task.done' || kind === 'agent_speak' || kind === 'model_response') return true
  return kind.endsWith('_message')
}

function isTaskDonePlaceholder(text: string): boolean {
  const trimmed = text.trim()
  return trimmed === '' || trimmed === 'Task finished' || trimmed === '(Task completed.)'
}

function getTimestampMs(value?: string): number {
  if (!value) return 0
  const ts = Date.parse(value)
  return Number.isFinite(ts) ? ts : 0
}

export default function Conversation({ threadId, teamId, coordinatorRole }: ConversationProps) {
  const { query: conversationQuery } = useConversation(threadId)
  const activityQuery = useActivity({ threadId, teamId, includeChildRuns: true, limit: 500 })
  const taskHistoryQuery = useTaskHistory({ threadId, teamId, limit: 500 })
  const [input, setInput] = useState('')
  const [sending, setSending] = useState(false)
  const [sendError, setSendError] = useState<string | null>(null)
  const [optimistic, setOptimistic] = useState<ChatEntry[]>([])
  const bottomRef = useRef<HTMLDivElement>(null)
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  const entries = useMemo(() => {
    const fromItems = (conversationQuery.data ?? []).map(itemToChatEntry).filter((entry): entry is ChatEntry => entry !== null)
    const fromActivities = (activityQuery.data ?? []).map(toChatEntry).filter((entry): entry is ChatEntry => entry !== null)
    const fromTasks = (taskHistoryQuery.data ?? []).map(taskToChatEntry).filter((entry): entry is ChatEntry => entry !== null)
    const byID = new Map<string, ChatEntry>()
    for (const entry of [...fromItems, ...fromTasks, ...fromActivities]) {
      const prev = byID.get(entry.id)
      if (!prev || entry.createdAt >= prev.createdAt) {
        byID.set(entry.id, entry)
      }
    }
    const userSeen = new Set(
      [...byID.values()]
        .filter((entry) => entry.kind === 'user')
        .map((entry) => entry.text),
    )
    for (const entry of optimistic) {
      if (!userSeen.has(entry.text)) {
        byID.set(entry.id, entry)
      }
    }
    return [...byID.values()].sort((a, b) => {
      if (a.createdAt !== b.createdAt) return a.createdAt - b.createdAt
      return a.id.localeCompare(b.id)
    })
  }, [conversationQuery.data, activityQuery.data, optimistic, taskHistoryQuery.data])

  const turns = useMemo(() => {
    const grouped: ChatTurn[] = []
    for (const entry of entries) {
      const prev = grouped[grouped.length - 1]
      if (prev && prev.kind === entry.kind && prev.role === entry.role && entry.kind === 'agent') {
        prev.texts.push(entry.text)
        continue
      }
      grouped.push({
        id: entry.id,
        kind: entry.kind,
        role: entry.role,
        texts: [entry.text],
      })
    }
    return grouped
  }, [entries])

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [entries.length])

  useEffect(() => {
    const el = textareaRef.current
    if (!el) return
    el.style.height = 'auto'
    el.style.height = Math.min(el.scrollHeight, 140) + 'px'
  }, [input])

  useEffect(() => {
    if (!activityQuery.data) return
    const userTexts = new Set(
      activityQuery.data
        .map(toChatEntry)
        .filter((entry): entry is ChatEntry => entry !== null && entry.kind === 'user')
        .map((entry) => entry.text),
    )
    setOptimistic((prev) => prev.filter((entry) => !userTexts.has(entry.text)))
  }, [activityQuery.data])

  async function sendMessage() {
    const text = input.trim()
    if (!text || !threadId || !teamId || !coordinatorRole || sending) return
    setSending(true)
    setSendError(null)
    setInput('')
    setOptimistic((prev) => [
      ...prev,
        {
          id: `optimistic-${Date.now()}`,
          kind: 'user',
          text,
          role: 'You',
          createdAt: Date.now(),
        },
    ])
    try {
      await rpcCall('task.create', {
        threadId,
        teamId,
        goal: text,
        taskKind: 'user_message',
        assignedRole: coordinatorRole,
      })
      await activityQuery.refetch()
    } catch (err) {
      setSendError(err instanceof Error ? err.message : 'Failed to send message')
      setInput(text)
      setOptimistic((prev) => prev.filter((entry) => entry.text !== text))
    } finally {
      setSending(false)
    }
  }

  const canSend = !!input.trim() && !!threadId && !!teamId && !!coordinatorRole && !sending

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      <div style={{ flex: 1, minHeight: 0, overflowY: 'auto', padding: '28px 32px 12px' }}>
        {!threadId ? (
          <div
            style={{
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              height: '100%',
              color: 'var(--text-3)',
              fontSize: 13,
            }}
          >
            Connecting…
          </div>
        ) : turns.length === 0 ? (
          <div
            style={{
              display: 'flex',
              flexDirection: 'column',
              alignItems: 'center',
              justifyContent: 'center',
              height: '100%',
              gap: 14,
              textAlign: 'center',
            }}
          >
            <div
              style={{
                width: 52,
                height: 52,
                borderRadius: 14,
                background: 'var(--accent-dim)',
                border: '1px solid rgba(139,123,248,0.2)',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
              }}
            >
              <Zap size={22} color="var(--accent)" fill="var(--accent)" strokeWidth={0} />
            </div>
            <div>
              <div style={{ fontSize: 14, fontWeight: 600, color: 'var(--text-1)', marginBottom: 5 }}>Coordinator ready</div>
              <div style={{ fontSize: 13, color: 'var(--text-3)' }}>Send a message to get started</div>
            </div>
          </div>
        ) : (
          turns.map((turn) => {
            const entry: ChatEntry = {
              id: turn.id,
              kind: turn.kind,
              role: turn.role,
              text: turn.texts.join('\n\n'),
              createdAt: 0,
            }
            return turn.kind === 'user' ? <UserBubble key={turn.id} entry={entry} /> : <AgentBubble key={turn.id} entry={entry} />
          })
        )}
        <div ref={bottomRef} />
      </div>

      {sendError && (
        <div
          style={{
            padding: '8px 20px',
            fontSize: 12,
            color: 'var(--red)',
            background: 'var(--red-dim)',
            borderTop: '1px solid rgba(239,68,68,0.15)',
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            flexShrink: 0,
          }}
        >
          <span>{sendError}</span>
          <button
            onClick={() => setSendError(null)}
            style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'inherit', fontSize: 16, lineHeight: 1, padding: '0 0 0 8px' }}
          >
            ×
          </button>
        </div>
      )}

      <div style={{ padding: '10px 20px 16px', borderTop: '1px solid var(--border)', flexShrink: 0 }}>
        <div
          style={{
            display: 'flex',
            alignItems: 'flex-end',
            gap: 8,
            background: 'var(--bg-elevated)',
            border: '1px solid var(--border)',
            borderRadius: 'var(--r-xl)',
            padding: '8px 10px 8px 16px',
            transition: 'border-color 0.15s',
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
              width: 32,
              height: 32,
              borderRadius: 10,
              border: 'none',
              background: canSend ? 'var(--accent-solid)' : 'var(--bg-active)',
              color: canSend ? '#fff' : 'var(--text-3)',
              cursor: canSend ? 'pointer' : 'default',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              flexShrink: 0,
              transition: 'background 0.15s, transform 0.1s',
              transform: canSend ? 'scale(1)' : 'scale(0.9)',
            }}
          >
            <ArrowUp size={15} />
          </button>
        </div>
        <div style={{ textAlign: 'center', marginTop: 7, fontSize: 11, color: 'var(--text-3)' }}>Enter to send · Shift+Enter for newline</div>
      </div>
    </div>
  )
}
