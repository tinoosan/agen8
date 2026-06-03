import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useMemo, useCallback, useRef } from 'react'
import { projectAgentEvents } from '../lib/agentEventProjection'
import { rpcCall, onNotification } from '../lib/rpc'
import type {
  AgentEvent,
  SpaceDetailEntry,
  SpaceDetailResult,
} from '../lib/types'
import type { ChatEntry } from '../components/conversation/types'

interface ConversationAttachmentView {
  id: string
  name: string
  mediaType: string
  sizeBytes: number
  uri?: string
}

interface ConversationMessageView {
  id: string
  channelId: string
  spaceId: string
  memberId: string
  sessionId?: string
  turnId?: string
  direction: string
  senderType: string
  senderId?: string
  text: string
  attachments?: ConversationAttachmentView[]
  delivery?: string
  render: string
  createdAt: string
  updatedAt: string
  error?: string
}

interface ConversationListResult {
  messages: ConversationMessageView[]
  activities?: SpaceDetailEntry[]
}

function entryTime(entry: SpaceDetailEntry): number {
  const ms = Date.parse(entry.createdAt ?? '')
  return Number.isFinite(ms) ? ms : 0
}

function entryOrder(left: SpaceDetailEntry, right: SpaceDetailEntry): number {
  const leftTime = entryTime(left)
  const rightTime = entryTime(right)
  if (leftTime !== rightTime) return leftTime - rightTime
  const leftSeq = Number(left.sequence ?? left.data?.seq ?? 0)
  const rightSeq = Number(right.sequence ?? right.data?.seq ?? 0)
  if (
    Number.isFinite(leftSeq) &&
    Number.isFinite(rightSeq) &&
    leftSeq !== rightSeq
  ) {
    return leftSeq - rightSeq
  }
  return left.id.localeCompare(right.id)
}

function completedTime(entry: SpaceDetailEntry): number | undefined {
  const ms = Date.parse(entry.completedAt ?? '')
  return Number.isFinite(ms) ? ms : undefined
}

export function spaceDetailEntryToChatEntry(
  entry: SpaceDetailEntry,
): ChatEntry | null {
  const createdAt = entryTime(entry)
  switch (entry.kind) {
    case 'user_message':
      return {
        id: entry.id,
        kind: 'user',
        text: entry.text ?? '',
        attachments: (entry.data as Record<string, unknown> | undefined)
          ?.attachments as ChatEntry['attachments'],
        role: entry.member ?? 'You',
        channelId: entry.data?.channelId ?? null,
        turnId: entry.turnId,
        runId: entry.runId,
        createdAt,
        completedAt: completedTime(entry),
        status: entry.status,
        delivery:
          typeof entry.data?.delivery === 'string'
            ? entry.data.delivery
            : undefined,
        source: 'space-detail',
      }
    case 'agent_message':
      return {
        id: entry.id,
        kind: 'agent',
        text: entry.text ?? '',
        role: entry.member,
        channelId: entry.data?.channelId ?? null,
        turnId: entry.turnId,
        runId: entry.runId,
        createdAt,
        completedAt: completedTime(entry),
        live: !!entry.live,
        source: 'space-detail',
      }
    case 'thinking':
      return {
        id: entry.id,
        kind: 'thought',
        text: entry.text ?? '',
        role: entry.member,
        channelId: entry.data?.channelId ?? null,
        turnId: entry.turnId,
        runId: entry.runId,
        createdAt,
        completedAt: completedTime(entry),
        live: !!entry.live,
        source: 'space-detail',
      }
    case 'error':
      return {
        id: entry.id,
        kind: 'error',
        text: entry.text ?? entry.title ?? 'LLM request failed',
        role: entry.member,
        channelId: entry.data?.channelId ?? null,
        turnId: entry.turnId,
        runId: entry.runId,
        createdAt,
        completedAt: completedTime(entry),
        retryMessageId: entry.messageId,
        errorLabel: entry.title,
        source: 'space-detail',
      }
    case 'note':
      return {
        id: entry.id,
        kind: 'note',
        text: entry.text ?? '',
        role: entry.member,
        channelId: entry.data?.channelId ?? null,
        turnId: entry.turnId,
        runId: entry.runId,
        createdAt,
        completedAt: completedTime(entry),
        live: !!entry.live,
        source: 'space-detail',
      }
    default:
      return null
  }
}

function activityStatus(status: string | undefined): AgentEvent['status'] {
  const normalized = String(status ?? '')
    .trim()
    .toLowerCase()
  if (
    normalized === 'running' ||
    normalized === 'pending' ||
    normalized === 'started' ||
    normalized === 'streaming'
  ) {
    return 'pending'
  }
  if (normalized === 'error' || normalized === 'failed') return 'error'
  if (normalized === 'canceled' || normalized === 'cancelled') return 'canceled'
  return 'ok'
}

export function spaceDetailEntryToActivity(
  entry: SpaceDetailEntry,
): AgentEvent | null {
  const data = { ...(entry.data ?? {}) }
  if (entry.runId && !data.runId) data.runId = entry.runId
  if (entry.turnId && !data.turnId) data.turnId = entry.turnId
  if (entry.toolCallId && !data.opId) data.opId = entry.toolCallId
  if (entry.member && !data.member) data.member = entry.member
  if (Number.isFinite(entry.sequence) && !data.seq)
    data.seq = String(entry.sequence)
  if (entry.kind === 'agent_message_received') {
    const status = activityStatus(entry.status)
    return {
      id: entry.id,
      kind: 'agent_message_received',
      title: entry.title ?? 'Message received',
      status,
      startedAt: entry.createdAt,
      completedAt: entry.completedAt,
      textPreview: entry.text,
      error: status === 'error' ? data.error : undefined,
      data,
    }
  }
  if (entry.kind !== 'tool_call') return null
  const lifecycleKind = internalHarnessLifecycleKind(entry, data)
  return {
    id: entry.id,
    kind: lifecycleKind || 'agent.tool.call',
    title: entry.title ?? data.toolName ?? data.toolNameRaw ?? 'Tool call',
    status: activityStatus(entry.status),
    startedAt: entry.createdAt,
    completedAt: entry.completedAt,
    duration: Number(data.durationMs ?? 0) || undefined,
    outputPreview: data.outputPreview,
    error: data.error,
    data,
  }
}

function conversationActivityToSpaceEntry(
  activity: SpaceDetailEntry,
): SpaceDetailEntry {
  if (activity.kind !== 'thinking') return activity
  const data = { ...(activity.data ?? {}) }
  if (activity.runId && !data.runId) data.runId = activity.runId
  if (activity.turnId && !data.turnId) data.turnId = activity.turnId
  if (activity.toolCallId && !data.toolCallId)
    data.toolCallId = activity.toolCallId
  if (activity.sequence !== undefined && !data.seq)
    data.seq = String(activity.sequence)
  return {
    ...activity,
    kind: 'thinking',
    runId: activity.runId ?? data.sessionId,
    toolCallId: activity.toolCallId ?? data.toolCallId,
    text: activity.text ?? '',
    member: activity.member ?? data.member,
    status: activity.status ?? 'completed',
    completedAt: activity.completedAt ?? activity.createdAt,
    data,
  }
}

function internalHarnessLifecycleKind(
  entry: SpaceDetailEntry,
  data: Record<string, string>,
): string {
  for (const value of [entry.title, data.toolName, data.kind]) {
    const normalized = String(value ?? '')
      .trim()
      .toLowerCase()
    if (normalized.startsWith('harness.run.')) {
      return normalized
    }
  }
  return ''
}

export function spaceDetailEntryToInspectorEvent(
  entry: SpaceDetailEntry,
): AgentEvent | null {
  const startedAt = entry.createdAt
  const completedAt = entry.completedAt
  const data = { ...(entry.data ?? {}) }
  if (entry.runId && !data.runId) data.runId = entry.runId
  if (entry.turnId && !data.turnId) data.turnId = entry.turnId
  if (entry.member && !data.member) data.member = entry.member

  switch (entry.kind) {
    case 'tool_call':
      return spaceDetailEntryToActivity(entry)
    case 'agent_message_received':
      return spaceDetailEntryToActivity(entry)
    case 'user_message':
      return {
        id: entry.id,
        kind: 'user_message',
        title: entry.title ?? 'User message',
        status: activityStatus(entry.status),
        startedAt,
        completedAt,
        textPreview: entry.text,
        data,
      }
    case 'agent_message':
      return {
        id: entry.id,
        kind: 'agent_message',
        title: entry.title ?? 'Agent message',
        status: activityStatus(entry.status),
        startedAt,
        completedAt,
        outputPreview: entry.text,
        data,
      }
    case 'thinking':
      return {
        id: entry.id,
        kind: 'model.thinking.completed',
        title: entry.title ?? 'Thinking',
        status: activityStatus(entry.status),
        startedAt,
        completedAt,
        outputPreview: entry.text,
        data,
      }
    case 'error':
      return {
        id: entry.id,
        kind: 'llm.error',
        title: entry.title ?? 'LLM request failed',
        status: 'error',
        startedAt,
        completedAt,
        error: entry.text ?? entry.title,
        data,
      }
    case 'note':
      return {
        id: entry.id,
        kind: 'note',
        title: entry.title ?? 'Note',
        status: activityStatus(entry.status),
        startedAt,
        completedAt,
        outputPreview: entry.text,
        data,
      }
    default:
      return null
  }
}

function conversationMessageToSpaceEntry(
  message: ConversationMessageView,
): SpaceDetailEntry {
  const direction = String(message.direction ?? '')
    .trim()
    .toLowerCase()
  const render = String(message.render ?? '')
    .trim()
    .toLowerCase()
  const delivery = String(message.delivery ?? '')
    .trim()
    .toLowerCase()
  const isOutbound = direction === 'outbound'
  const isError = render === 'error' || delivery === 'failed'
  const entry: SpaceDetailEntry = {
    id: message.id,
    kind: isError ? 'error' : isOutbound ? 'agent_message' : 'user_message',
    member: isOutbound ? 'Agent' : 'You',
    turnId: message.turnId,
    runId: message.sessionId,
    status: isError
      ? 'failed'
      : delivery === 'queued'
        ? 'pending'
        : 'completed',
    createdAt: message.createdAt,
    completedAt: message.updatedAt,
    text: isError ? message.error || message.text : message.text,
    title: isError ? 'Message delivery failed' : undefined,
    data: {
      channelId: message.channelId,
      spaceId: message.spaceId,
      memberId: message.memberId,
      sessionId: message.sessionId ?? '',
      delivery,
      render,
      direction,
    },
  }
  if (message.attachments?.length) {
    ;(entry.data as Record<string, unknown>)['attachments'] =
      message.attachments
  }
  return entry
}

export function useSpaceDetail(
  spaceId: string | null,
  channelId?: string | null,
) {
  const queryClient = useQueryClient()
  const normalizedChannelId = useMemo(
    () => channelId?.trim() || null,
    [channelId],
  )
  const key = useMemo(
    () =>
      normalizedChannelId
        ? ['message.conversation.list', normalizedChannelId]
        : ['space.detail', spaceId],
    [normalizedChannelId, spaceId],
  )
  const legacyKey = useMemo(() => ['space.detail', spaceId], [spaceId])
  const refreshTimer = useRef<number | null>(null)
  const emptyResult = useMemo<SpaceDetailResult>(
    () => ({ space: { id: spaceId ?? '' }, entries: [] }),
    [spaceId],
  )

  const query = useQuery<SpaceDetailResult>({
    queryKey: key,
    queryFn: async () => {
      if (!normalizedChannelId) {
        return rpcCall<SpaceDetailResult>('space.detail', {
          spaceId,
          limit: 2000,
        })
      }
      const result = await rpcCall<ConversationListResult>(
        'message.conversation.list',
        {
          channelId: normalizedChannelId,
          limit: 2000,
        },
      )
      return {
        space: { id: spaceId ?? '' },
        entries: [
          ...(result.messages ?? []).map(conversationMessageToSpaceEntry),
          ...(result.activities ?? []).map(conversationActivityToSpaceEntry),
        ].sort(entryOrder),
      }
    },
    enabled: !!spaceId,
    placeholderData: (previous) =>
      previous ??
      queryClient.getQueryData<SpaceDetailResult>(legacyKey) ??
      emptyResult,
    staleTime: Infinity,
    retry: false,
  })

  useEffect(() => {
    if (!spaceId) return
    const refresh = (notification?: Record<string, unknown>) => {
      if (
        !spaceDetailNotificationMatches(
          notification,
          spaceId,
          normalizedChannelId,
        )
      )
        return
      if (refreshTimer.current !== null) return
      refreshTimer.current = window.setTimeout(() => {
        refreshTimer.current = null
        queryClient.invalidateQueries({ queryKey: key })
      }, 250)
    }
    const unsubs = [onNotification('event.append', refresh)]
    return () => {
      unsubs.forEach((unsub) => unsub())
      if (refreshTimer.current !== null) {
        window.clearTimeout(refreshTimer.current)
        refreshTimer.current = null
      }
    }
  }, [key, normalizedChannelId, queryClient, spaceId])

  const registerTurnId = useCallback(
    (turnId: string) => {
      void turnId
      queryClient.invalidateQueries({ queryKey: key })
    },
    [key, queryClient],
  )

  const chatEntries = useMemo(
    () =>
      (query.data?.entries ?? [])
        .map(spaceDetailEntryToChatEntry)
        .filter((entry): entry is ChatEntry => entry !== null),
    [query.data?.entries],
  )

  const activities = useMemo(
    () =>
      projectAgentEvents(
        (query.data?.entries ?? [])
          .map(spaceDetailEntryToActivity)
          .filter((entry): entry is AgentEvent => entry !== null),
      ),
    [query.data?.entries],
  )

  const inspectorEvents = useMemo(
    () =>
      projectAgentEvents(
        (query.data?.entries ?? [])
          .map(spaceDetailEntryToInspectorEvent)
          .filter((entry): entry is AgentEvent => entry !== null),
      ),
    [query.data?.entries],
  )

  return {
    query,
    registerTurnId,
    chatEntries,
    activities,
    inspectorEvents,
    context: query.data?.context ?? null,
  }
}

function spaceDetailNotificationMatches(
  notification: Record<string, unknown> | undefined,
  spaceId: string,
  channelId: string | null,
): boolean {
  const params =
    notificationRecord(notification?.params) ?? notificationRecord(notification)
  const event = notificationRecord(params?.event)
  const eventSpaceId =
    notificationString(params?.spaceId) || notificationString(event?.spaceId)
  const eventChannelId =
    notificationString(params?.channelId) ||
    notificationString(event?.channelId)

  if (channelId) {
    if (eventChannelId) return eventChannelId === channelId
    if (eventSpaceId) return false
    return true
  }
  if (eventSpaceId) return eventSpaceId === spaceId
  return true
}

function notificationRecord(
  value: unknown,
): Record<string, unknown> | undefined {
  if (!value || typeof value !== 'object' || Array.isArray(value))
    return undefined
  return value as Record<string, unknown>
}

function notificationString(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}
