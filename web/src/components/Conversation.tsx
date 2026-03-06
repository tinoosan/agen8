import { useEffect, useMemo, useRef, useState } from 'react'
import { useActivity } from '../hooks/useActivity'
import { useConversation } from '../hooks/useConversation'
import { useTaskHistory } from '../hooks/useTaskHistory'
import { useThinkingEvents } from '../hooks/useThinkingEvents'
import { useArtifactFiles } from '../hooks/useArtifactFiles'
import { rpcCall } from '../lib/rpc'
import { ArrowUp, ChevronRight, Paperclip, Sparkles, X, Zap } from 'lucide-react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import type { ActivityEvent, ArtifactGetResult, ArtifactNode, EventRecord, Item, Task, UserMessageContent, AgentMessageContent } from '../lib/types'

interface ConversationProps {
  threadId: string | null
  teamId: string
  coordinatorRole: string | null
  coordinatorRunId: string | null
}

interface ChatEntry {
  id: string
  kind: 'user' | 'agent' | 'thought'
  text: string
  role?: string
  createdAt: number
  live?: boolean
  source?: 'item' | 'activity' | 'task' | 'thinking' | 'optimistic'
}

interface ChatTurn {
  id: string
  kind: 'user' | 'agent' | 'thought'
  role?: string
  texts: string[]
  live?: boolean
}

/* ── Agent Bubble ────────────────────────────────── */
function AgentBubble({ entry }: { entry: ChatEntry }) {
  return (
    <div
      className="animate-fade-in"
      style={{
        display: 'flex',
        justifyContent: 'flex-start',
        alignItems: 'flex-start',
        gap: 10,
        marginBottom: 20,
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
            padding: '12px 16px',
            borderRadius: '4px 16px 16px 16px',
            background: 'var(--bg-elevated)',
            borderLeft: '2px solid var(--accent-dim)',
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

/* ── Thinking Bubble ─────────────────────────────── */
function ThoughtBubble({ entry }: { entry: ChatEntry }) {
  const [open, setOpen] = useState(true) // default expanded

  useEffect(() => {
    if (entry.live) setOpen(true)
  }, [entry.live])

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
          background: entry.live ? 'var(--amber-dim)' : 'var(--bg-surface)',
          border: `1px solid ${entry.live ? 'rgba(245,158,11,0.2)' : 'var(--border)'}`,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          flexShrink: 0,
          marginTop: 1,
          transition: 'background 0.3s, border-color 0.3s',
        }}
      >
        <Sparkles size={13} color={entry.live ? 'var(--amber)' : 'var(--text-3)'} />
      </div>
      <div style={{ maxWidth: '78%', minWidth: 0, flex: 1 }}>
        <button
          onClick={() => setOpen((prev) => !prev)}
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 6,
            background: 'none',
            border: 'none',
            padding: 0,
            color: entry.live ? 'var(--amber)' : 'var(--text-3)',
            cursor: 'pointer',
            marginBottom: open ? 6 : 0,
            transition: 'color 0.15s',
          }}
        >
          <ChevronRight
            size={12}
            style={{
              transform: open ? 'rotate(90deg)' : 'rotate(0deg)',
              transition: 'transform 0.15s',
            }}
          />
          <span style={{ fontSize: 11, textTransform: 'uppercase', letterSpacing: '0.05em', fontWeight: 600 }}>
            {entry.role ? `${entry.role} thinking` : 'Thinking'}
          </span>
          {entry.live && (
            <span style={{
              fontSize: 10,
              fontWeight: 600,
              color: 'var(--amber)',
              background: 'var(--amber-dim)',
              padding: '1px 6px',
              borderRadius: 999,
              animation: 'pulse-soft 2s infinite',
            }}>
              live
            </span>
          )}
        </button>
        {open && (
          <div
            className={entry.live ? 'thinking-shimmer-bg' : ''}
            style={{
              padding: '10px 14px',
              borderRadius: '4px 12px 12px 12px',
              background: entry.live ? undefined : 'var(--bg-surface)',
              borderLeft: `2px solid ${entry.live ? 'var(--amber)' : 'var(--border)'}`,
              color: 'var(--text-2)',
              fontSize: 12.5,
              overflowWrap: 'break-word',
              lineHeight: 1.6,
              transition: 'border-color 0.3s',
            }}
          >
            <div className="md-prose" style={{ fontStyle: 'italic' }}>
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
                          fontSize: 12, margin: '6px 0', fontStyle: 'normal',
                        }}>
                          <code className="mono" {...props}>{children}</code>
                        </pre>
                      )
                    }
                    return (
                      <code className="mono" style={{
                        background: 'var(--bg-surface)', padding: '1px 5px',
                        borderRadius: 4, fontSize: '0.88em', fontStyle: 'normal',
                      }} {...props}>{children}</code>
                    )
                  },
                }}
              >
                {entry.text}
              </ReactMarkdown>
            </div>
            {entry.live && <span className="streaming-cursor" />}
          </div>
        )}
      </div>
    </div>
  )
}

/* ── User Bubble ─────────────────────────────────── */
function UserBubble({ entry }: { entry: ChatEntry }) {
  return (
    <div
      className="animate-fade-in"
      style={{
        display: 'flex',
        justifyContent: 'flex-end',
        alignItems: 'flex-start',
        gap: 10,
        marginBottom: 20,
      }}
    >
      <div style={{ maxWidth: '78%', minWidth: 0 }}>
        <div
          style={{
            padding: '11px 16px',
            borderRadius: '16px 16px 4px 16px',
            background: 'linear-gradient(135deg, var(--accent-solid), var(--accent))',
            color: '#fff',
            fontSize: 13.5,
            lineHeight: 1.6,
            whiteSpace: 'pre-wrap',
            wordBreak: 'break-word',
            boxShadow: '0 2px 8px rgba(0,0,0,0.15)',
          }}
        >
          {entry.text}
        </div>
      </div>
    </div>
  )
}

/* ── Data helpers ─────────────────────────────────── */
function toChatEntry(event: ActivityEvent): ChatEntry | null {
  const kind = (event.kind ?? '').trim().toLowerCase()
  const text = extractActivityText(event)
  if (!text || isTaskDonePlaceholder(text)) return null

  if (kind === 'user_message') {
    return { id: event.id, kind: 'user', text, role: 'You', createdAt: getTimestampMs(event.startedAt), source: 'activity' }
  }
  if (isAgentTextKind(kind)) {
    return { id: event.id, kind: 'agent', text, role: extractRole(event), createdAt: getTimestampMs(event.finishedAt ?? event.startedAt), source: 'activity' }
  }
  return null
}

function itemToChatEntry(item: Item): ChatEntry | null {
  if (item.type === 'user_message') {
    const content = item.content as UserMessageContent | undefined
    const text = content?.text?.trim()
    if (!text) return null
    return { id: item.id, kind: 'user', text, role: 'You', createdAt: getTimestampMs(item.createdAt), source: 'item' }
  }
  if (item.type === 'agent_message') {
    const content = item.content as AgentMessageContent | undefined
    const text = content?.text?.trim()
    if (!text) return null
    return { id: item.id, kind: 'agent', text, role: 'agent', createdAt: getTimestampMs(item.createdAt), source: 'item' }
  }
  return null
}

function taskToChatEntry(task: Task): ChatEntry | null {
  const summary = task.summary?.trim()
  if (!summary) return null
  const kind = (task.taskKind ?? '').trim().toLowerCase()
  if (kind === 'user_message') return null
  return {
    id: `task:${task.id}`, kind: 'agent', text: summary,
    role: task.assignedRole?.trim() || task.roleSnapshot?.trim() || 'agent',
    createdAt: getTimestampMs(task.completedAt || task.createdAt), source: 'task',
  }
}

function thinkingEventsToEntries(events: EventRecord[]): ChatEntry[] {
  if (events.length === 0) return []
  const order = [...events].sort((a, b) => {
    const ts = getTimestampMs(a.timestamp) - getTimestampMs(b.timestamp)
    if (ts !== 0) return ts
    return a.eventId.localeCompare(b.eventId)
  })
  const byStep = new Map<string, ChatEntry>()
  for (const ev of order) {
    const type = (ev.type ?? '').trim()
    const step = (ev.data?.step ?? '').trim()
    if (!step) continue
    if (type === 'model.thinking.start') {
      if (!byStep.has(step)) {
        byStep.set(step, {
          id: `thinking:${step}`, kind: 'thought', text: '',
          role: ev.data?.role?.trim() || ev.data?.agent_role?.trim() || 'agent',
          createdAt: getTimestampMs(ev.timestamp), live: true, source: 'thinking',
        })
      }
      continue
    }
    const current = byStep.get(step)
    if (!current) continue
    if (type === 'model.thinking.summary') {
      const line = (ev.data?.text ?? '').trim()
      if (!line) continue
      current.text = current.text ? `${current.text}\n${line}` : line
      if (!current.role) current.role = ev.data?.role?.trim() || ev.data?.agent_role?.trim() || 'agent'
      continue
    }
    if (type === 'model.thinking.end') {
      if (!current.text.trim()) current.text = 'Reasoning used; provider did not return a summary.'
      current.live = false
    }
  }
  return [...byStep.values()]
    .map((entry) => {
      if (entry.text.trim()) return entry
      if (entry.live) return { ...entry, text: 'Thinking\u2026' }
      return entry
    })
    .filter((entry) => entry.text.trim() !== '')
}

function extractActivityText(event: ActivityEvent): string {
  const kind = (event.kind ?? '').trim().toLowerCase()
  if (kind === 'user_message' && event.textPreview?.trim()) return event.textPreview.trim()
  if ((kind === 'task.done' || kind === 'agent_speak' || kind === 'model_response') && event.outputPreview?.trim()) return event.outputPreview.trim()
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

function formatAttachedMessage(text: string, files: Array<{ artifact: ArtifactNode; content: string; truncated: boolean }>): string {
  if (files.length === 0) return text
  const blocks = files.map(({ artifact, content, truncated }) => {
    const label = artifact.displayName?.trim() || artifact.vpath?.trim() || artifact.artifactId?.trim() || artifact.label.trim()
    const suffix = truncated ? '\n[truncated]' : ''
    return `File: ${label}\n\`\`\`\n${content}\n\`\`\`${suffix}`
  })
  return `${text}\n\nAttached file context:\n\n${blocks.join('\n\n')}`
}

function normalizeText(value: string): string {
  return value.trim().replace(/\s+/g, ' ')
}

function shouldDeduplicate(prev: ChatEntry, next: ChatEntry): boolean {
  if (prev.kind !== next.kind) return false
  if (normalizeText(prev.text) !== normalizeText(next.text)) return false
  if ((prev.role ?? '').trim().toLowerCase() !== (next.role ?? '').trim().toLowerCase()) return false
  if (prev.kind === 'thought') return true
  const pair = new Set([prev.source, next.source])
  return (pair.has('task') && pair.has('activity')) || (pair.has('item') && pair.has('activity'))
}

/* ── Main Component ──────────────────────────────── */
export default function Conversation({ threadId, teamId, coordinatorRole, coordinatorRunId }: ConversationProps) {
  const { query: conversationQuery } = useConversation(threadId)
  const activityQuery = useActivity({ threadId, teamId, includeChildRuns: true, limit: 500 })
  const taskHistoryQuery = useTaskHistory({ threadId, teamId, limit: 500 })
  const thinkingQuery = useThinkingEvents(coordinatorRunId, 2000)
  const artifactFilesQuery = useArtifactFiles(threadId, teamId)
  const [input, setInput] = useState('')
  const [sending, setSending] = useState(false)
  const [sendError, setSendError] = useState<string | null>(null)
  const [optimistic, setOptimistic] = useState<ChatEntry[]>([])
  const [pickerOpen, setPickerOpen] = useState(false)
  const [selectedFiles, setSelectedFiles] = useState<ArtifactNode[]>([])
  const bottomRef = useRef<HTMLDivElement>(null)
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  const entries = useMemo(() => {
    const fromItems = (conversationQuery.data ?? []).map(itemToChatEntry).filter((entry): entry is ChatEntry => entry !== null)
    const fromActivities = (activityQuery.data ?? []).map(toChatEntry).filter((entry): entry is ChatEntry => entry !== null)
    const fromTasks = (taskHistoryQuery.data ?? []).map(taskToChatEntry).filter((entry): entry is ChatEntry => entry !== null)
    const fromThinking = thinkingEventsToEntries(thinkingQuery.data ?? [])
    const byID = new Map<string, ChatEntry>()
    for (const entry of [...fromItems, ...fromTasks, ...fromActivities, ...fromThinking]) {
      const prev = byID.get(entry.id)
      if (!prev || entry.createdAt >= prev.createdAt) byID.set(entry.id, entry)
    }
    const userSeen = new Set(
      [...byID.values()].filter((entry) => entry.kind === 'user').map((entry) => entry.text),
    )
    for (const entry of optimistic) {
      if (!userSeen.has(entry.text)) byID.set(entry.id, entry)
    }
    const sorted = [...byID.values()].sort((a, b) => {
      if (a.createdAt !== b.createdAt) return a.createdAt - b.createdAt
      return a.id.localeCompare(b.id)
    })
    const deduped: ChatEntry[] = []
    for (const entry of sorted) {
      const prev = deduped[deduped.length - 1]
      if (!prev || !shouldDeduplicate(prev, entry)) { deduped.push(entry); continue }
      if ((prev.source === 'activity' && entry.source !== 'activity') || (prev.live && !entry.live)) {
        deduped[deduped.length - 1] = entry
      }
    }
    return deduped
  }, [conversationQuery.data, activityQuery.data, optimistic, taskHistoryQuery.data, thinkingQuery.data])

  const turns = useMemo(() => {
    const grouped: ChatTurn[] = []
    for (const entry of entries) {
      const prev = grouped[grouped.length - 1]
      if (prev && prev.kind === entry.kind && prev.role === entry.role && (entry.kind === 'agent' || entry.kind === 'thought')) {
        prev.texts.push(entry.text)
        if (entry.live) prev.live = true
        continue
      }
      grouped.push({ id: entry.id, kind: entry.kind, role: entry.role, texts: [entry.text], live: entry.live })
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
      activityQuery.data.map(toChatEntry).filter((entry): entry is ChatEntry => entry !== null && entry.kind === 'user').map((entry) => entry.text),
    )
    setOptimistic((prev) => prev.filter((entry) => !userTexts.has(entry.text)))
  }, [activityQuery.data])

  async function sendMessage() {
    const text = input.trim()
    if (!text || !threadId || !teamId || !coordinatorRole || sending) return
    setSending(true)
    setSendError(null)
    const pendingFiles = [...selectedFiles]
    setInput('')
    setSelectedFiles([])
    setPickerOpen(false)
    setOptimistic((prev) => [...prev, {
      id: `optimistic-${Date.now()}`,
      kind: 'user',
      text: pendingFiles.length === 0 ? text : `${text}\n\nAttached files: ${pendingFiles.map((file) => file.displayName ?? file.vpath ?? file.label).join(', ')}`,
      role: 'You',
      createdAt: Date.now(),
      source: 'optimistic',
    }])
    try {
      const attachedFiles = await Promise.all(
        pendingFiles.map(async (file) => {
          const res = await rpcCall<ArtifactGetResult>('artifact.get', {
            threadId,
            teamId,
            artifactId: file.artifactId,
            vpath: file.vpath,
            maxBytes: 12 * 1024,
          })
          return { artifact: res.artifact, content: res.content, truncated: res.truncated }
        }),
      )
      await rpcCall('task.create', {
        threadId,
        teamId,
        goal: formatAttachedMessage(text, attachedFiles),
        taskKind: 'user_message',
        assignedRole: coordinatorRole,
      })
      await activityQuery.refetch()
    } catch (err) {
      setSendError(err instanceof Error ? err.message : 'Failed to send message')
      setInput(text)
      setSelectedFiles(pendingFiles)
      setOptimistic((prev) => prev.filter((entry) => entry.text !== text))
    } finally {
      setSending(false)
    }
  }

  const canSend = !!input.trim() && !!threadId && !!teamId && !!coordinatorRole && !sending

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      {/* Messages */}
      <div style={{ flex: 1, minHeight: 0, overflowY: 'auto', padding: '28px 32px 12px' }}>
        {!threadId ? (
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%', color: 'var(--text-3)', fontSize: 13 }}>
            <span className="spinner spinner-sm" style={{ marginRight: 8 }} />
            Connecting&hellip;
          </div>
        ) : turns.length === 0 ? (
          <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', height: '100%', gap: 14, textAlign: 'center' }}>
            <div style={{
              width: 52, height: 52, borderRadius: 14,
              background: 'var(--accent-dim)', border: '1px solid rgba(139,123,248,0.2)',
              display: 'flex', alignItems: 'center', justifyContent: 'center',
            }}>
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
              id: turn.id, kind: turn.kind, role: turn.role,
              text: turn.texts.join('\n\n'), createdAt: 0, live: turn.live,
            }
            if (turn.kind === 'thought') return <ThoughtBubble key={turn.id} entry={entry} />
            return turn.kind === 'user' ? <UserBubble key={turn.id} entry={entry} /> : <AgentBubble key={turn.id} entry={entry} />
          })
        )}
        <div ref={bottomRef} />
      </div>

      {/* Error bar */}
      {sendError && (
        <div style={{
          padding: '8px 20px', fontSize: 12, color: 'var(--red)', background: 'var(--red-dim)',
          borderTop: '1px solid rgba(239,68,68,0.15)', display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexShrink: 0,
        }}>
          <span>{sendError}</span>
          <button onClick={() => setSendError(null)} style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'inherit', fontSize: 16, lineHeight: 1, padding: '0 0 0 8px' }}>
            &times;
          </button>
        </div>
      )}

      {/* Input */}
      <div style={{ padding: '10px 20px 16px', borderTop: '1px solid var(--border)', flexShrink: 0 }}>
        {selectedFiles.length > 0 && (
          <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', marginBottom: 10 }}>
            {selectedFiles.map((file) => {
              const label = file.displayName ?? file.vpath ?? file.label
              return (
                <div
                  key={file.nodeKey}
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 6,
                    padding: '4px 8px',
                    background: 'var(--bg-elevated)',
                    border: '1px solid var(--border)',
                    borderRadius: 999,
                    fontSize: 11,
                    color: 'var(--text-2)',
                  }}
                >
                  <span>{label}</span>
                  <button
                    type="button"
                    onClick={() => setSelectedFiles((prev) => prev.filter((entry) => entry.nodeKey !== file.nodeKey))}
                    style={{ background: 'none', border: 'none', color: 'inherit', cursor: 'pointer', padding: 0, display: 'flex' }}
                    aria-label={`Remove ${label}`}
                  >
                    <X size={12} />
                  </button>
                </div>
              )
            })}
          </div>
        )}
        <div style={{
          display: 'flex', alignItems: 'flex-end', gap: 8,
          background: 'var(--bg-elevated)', border: '1px solid var(--border)',
          borderRadius: 'var(--r-xl)', padding: '8px 10px 8px 16px',
          transition: 'border-color 0.15s',
        }}>
          <textarea
            ref={textareaRef}
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={(e) => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); sendMessage() } }}
            placeholder={sending ? 'Sending\u2026' : 'Message the coordinator\u2026'}
            disabled={!threadId || sending}
            rows={1}
            style={{
              flex: 1, resize: 'none', border: 'none', background: 'transparent',
              color: 'var(--text-1)', fontSize: 13.5, lineHeight: 1.55, outline: 'none',
              fontFamily: 'inherit', maxHeight: 140, overflowY: 'auto', padding: 0,
            }}
          />
          <div style={{ position: 'relative', flexShrink: 0 }}>
            <button
              type="button"
              className="btn-ghost"
              onClick={() => setPickerOpen((prev) => !prev)}
              aria-label="Attach file"
              style={{ width: 32, height: 32, padding: 0 }}
            >
              <Paperclip size={14} />
            </button>
            {pickerOpen && (
              <div
                style={{
                  position: 'absolute',
                  right: 0,
                  bottom: 40,
                  width: 320,
                  maxHeight: 280,
                  overflowY: 'auto',
                  background: 'var(--bg-panel)',
                  border: '1px solid var(--border)',
                  borderRadius: 'var(--r-lg)',
                  boxShadow: '0 12px 40px rgba(0,0,0,0.35)',
                  padding: 8,
                  zIndex: 30,
                }}
              >
                {artifactFilesQuery.isLoading ? (
                  <div style={{ padding: 10, fontSize: 12, color: 'var(--text-3)' }}>Loading files…</div>
                ) : (artifactFilesQuery.data ?? []).length === 0 ? (
                  <div style={{ padding: 10, fontSize: 12, color: 'var(--text-3)' }}>No files available</div>
                ) : (
                  (artifactFilesQuery.data ?? []).slice(0, 50).map((file) => {
                    const label = file.displayName ?? file.vpath ?? file.label
                    const selected = selectedFiles.some((entry) => entry.nodeKey === file.nodeKey)
                    return (
                      <button
                        key={file.nodeKey}
                        type="button"
                        onClick={() => {
                          setSelectedFiles((prev) => {
                            if (prev.some((entry) => entry.nodeKey === file.nodeKey)) {
                              return prev.filter((entry) => entry.nodeKey !== file.nodeKey)
                            }
                            return [...prev, file]
                          })
                        }}
                        style={{
                          width: '100%',
                          textAlign: 'left',
                          background: selected ? 'var(--bg-elevated)' : 'transparent',
                          border: 'none',
                          borderRadius: 'var(--r-md)',
                          padding: '8px 10px',
                          cursor: 'pointer',
                          color: 'var(--text-1)',
                        }}
                      >
                        <div className="truncate" style={{ fontSize: 12, fontWeight: 500 }}>{label}</div>
                        <div style={{ fontSize: 10, color: 'var(--text-3)', marginTop: 2 }}>
                          {file.role ?? 'file'}{file.producedAt ? ` · ${new Date(file.producedAt).toLocaleString()}` : ''}
                        </div>
                      </button>
                    )
                  })
                )}
              </div>
            )}
          </div>
          <button
            onClick={sendMessage}
            disabled={!canSend}
            style={{
              width: 32, height: 32, borderRadius: 10, border: 'none',
              background: canSend ? 'var(--accent-solid)' : 'var(--bg-active)',
              color: canSend ? '#fff' : 'var(--text-3)',
              cursor: canSend ? 'pointer' : 'default',
              display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0,
              transition: 'background 0.15s, transform 0.1s',
              transform: canSend ? 'scale(1)' : 'scale(0.9)',
            }}
          >
            <ArrowUp size={15} />
          </button>
        </div>
        <div style={{ textAlign: 'center', marginTop: 7, fontSize: 11, color: 'var(--text-3)' }}>Enter to send &middot; Shift+Enter for newline</div>
      </div>
    </div>
  )
}
