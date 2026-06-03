import type { AgentEvent } from '../../lib/types'

export interface ChatEntry {
  id: string
  kind: 'user' | 'agent' | 'thought' | 'note' | 'error'
  text: string
  role?: string
  channelId?: string | null
  taskId?: string
  turnId?: string
  runId?: string
  createdAt: number
  status?: string
  delivery?: string
  live?: boolean
  completedAt?: number
  source?: 'space-detail' | 'agent-event' | 'task' | 'thinking' | 'optimistic'
  /** Inbox message id for operator retry after deadletter (llm.error). */
  retryMessageId?: string
  errorLabel?: string
  /** Member type (coordinator, worker, etc.) — used by AgentBubble to
   *  render a role badge next to the speaker's name. */
  memberType?: string
  attachments?: ConversationAttachment[]
}

export interface ConversationAttachment {
  id: string
  name: string
  mediaType: string
  sizeBytes: number
  uri?: string
  previewUrl?: string
}

export interface PendingConversationAttachment {
  id: string
  file: File
  name: string
  mediaType: string
  sizeBytes: number
  previewUrl: string
}

/** Content-based comparison for ChatEntry — prevents re-renders when the turns
 *  memo produces a new entry object with identical content. */
export function areEntriesEqual(
  prev: { entry: ChatEntry },
  next: { entry: ChatEntry },
): boolean {
  const a = prev.entry
  const b = next.entry
  return (
    a.id === b.id &&
    a.text === b.text &&
    a.live === b.live &&
    a.role === b.role &&
    a.completedAt === b.completedAt &&
    a.status === b.status &&
    a.delivery === b.delivery &&
    a.kind === b.kind &&
    a.memberType === b.memberType &&
    areAttachmentsEqual(a.attachments, b.attachments)
  )
}

function areAttachmentsEqual(a?: ConversationAttachment[], b?: ConversationAttachment[]): boolean {
  if ((a?.length ?? 0) !== (b?.length ?? 0)) return false
  if (!a || !b) return true
  return a.every((attachment, index) => {
    const other = b[index]
    return (
      attachment.id === other.id &&
      attachment.name === other.name &&
      attachment.mediaType === other.mediaType &&
      attachment.sizeBytes === other.sizeBytes &&
      attachment.previewUrl === other.previewUrl
    )
  })
}

export interface RoleBannerData {
  role: string
  actionCount: number
  durationMs: number
  activities: AgentEvent[]
  live: boolean
}

export interface ChatTurn {
  id: string
  kind: 'user' | 'agent' | 'thought' | 'note' | 'error' | 'role-banner' | 'activity-only'
  role?: string
  turnId?: string
  runId?: string
  channelId?: string | null
  texts: string[]
  attachments?: ConversationAttachment[]
  live?: boolean
  completedAt?: number
  createdAt?: number
  bannerData?: RoleBannerData
  /** Inline activity events that occurred during/after this turn (shown as cards) */
  activities?: AgentEvent[]
  retryMessageId?: string
  errorLabel?: string
  /**
   * Pre-built ChatEntry for this turn. Computed once inside the turns
   * memo so the `entry` prop passed to AgentBubble / UserBubble / etc.
   * keeps referential stability across renders — without this, the
   * entry object was rebuilt inside the `.map()` on every render,
   * defeating React.memo on every bubble and causing every keystroke
   * in the input to re-render the entire chat list.
   */
  entry?: ChatEntry
}
