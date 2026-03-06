import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef, useCallback } from 'react'
import { rpcCall, onNotification } from '../lib/rpc'
import type {
  Item,
  ItemDeltaParams,
  ItemNotificationParams,
  AgentMessageContent,
  ReasoningContent,
} from '../lib/types'

interface ItemListResult {
  items: Item[]
  cursor?: string
}

export function useConversation(threadId: string | null) {
  const queryClient = useQueryClient()
  const key = ['item.list', threadId]

  // Persistent ref so registerTurnId() can add to it from outside the effect,
  // allowing callers to pre-register a turnId immediately after turn.create returns
  // (before SSE delivers item.started for that turn).
  const knownTurnIdsRef = useRef<Set<string>>(new Set())

  const query = useQuery<Item[]>({
    queryKey: key,
    queryFn: async () => {
      const res = await rpcCall<ItemListResult>('item.list', {
        threadId,
        limit: 200,
      })
      const items = res.items ?? []
      // Seed knownTurnIds from initial data so existing items are accepted.
      for (const item of items) {
        if (item.turnId) knownTurnIdsRef.current.add(item.turnId)
      }
      return items
    },
    enabled: !!threadId,
    staleTime: Infinity,
    retry: false,
  })

  useEffect(() => {
    if (!threadId) return

    // Seed from current cache on mount / threadId change.
    const cached = queryClient.getQueryData<Item[]>(key)
    if (cached) {
      for (const item of cached) {
        if (item.turnId) knownTurnIdsRef.current.add(item.turnId)
      }
    }

    // turn.started — register turns that belong to our thread.
    const unsubTurn = onNotification('turn.started', (notif) => {
      const params = notif.params as { turn?: { id: string; threadId: string } } | undefined
      if (params?.turn?.threadId === threadId) {
        knownTurnIdsRef.current.add(params.turn.id)
      }
    })

    // item.completed — update item in cache (only for our thread's turns).
    const unsub1 = onNotification('item.completed', (notif) => {
      const params = notif.params as ItemNotificationParams | undefined
      if (!params?.item) return
      if (params.item.turnId && !knownTurnIdsRef.current.has(params.item.turnId)) return

      queryClient.setQueryData<Item[]>(key, (prev) => {
        if (!prev) return [params.item]
        const idx = prev.findIndex((i) => i.id === params.item.id)
        if (idx === -1) return [...prev, params.item]
        const next = [...prev]
        next[idx] = params.item
        return next
      })
    })

    // item.delta — accumulate streaming text into existing item's content.
    const unsub2 = onNotification('item.delta', (notif) => {
      const params = notif.params as ItemDeltaParams | undefined
      if (!params?.itemId || !params?.delta) return

      queryClient.setQueryData<Item[]>(key, (prev) => {
        if (!prev) return prev ?? []
        const idx = prev.findIndex((i) => i.id === params.itemId)
        if (idx === -1) return prev

        const next = [...prev]
        const item = { ...next[idx] }
        const content = (item.content ?? {}) as Record<string, unknown>

        if (item.type === 'agent_message' && params.delta.textDelta) {
          const text = ((content.text as string) ?? '') + params.delta.textDelta
          item.content = { ...content, text, isPartial: true } as AgentMessageContent
        } else if (item.type === 'reasoning' && params.delta.reasoningDelta) {
          const summary = ((content.summary as string) ?? '') + params.delta.reasoningDelta
          item.content = { ...content, summary } as ReasoningContent
        }

        if (item.status === 'started') {
          item.status = 'streaming'
        }
        next[idx] = item
        return next
      })
    })

    // item.started — ONLY accept items for turns we know belong to our thread.
    // This prevents cross-team pollution when the SSE stream delivers notifications
    // from all teams simultaneously.
    const unsub3 = onNotification('item.started', (notif) => {
      const params = notif.params as ItemNotificationParams | undefined
      if (!params?.item) return
      // Filter: skip items from unknown turns (they belong to other threads/teams).
      if (params.item.turnId && !knownTurnIdsRef.current.has(params.item.turnId)) return

      queryClient.setQueryData<Item[]>(key, (prev) => {
        if (!prev) return [params.item]
        // Replace optimistic items for the same turn/type.
        const filtered = prev.filter(
          (i) =>
            !(
              i.id.startsWith('optimistic-') &&
              i.turnId === params.item.turnId &&
              i.type === params.item.type
            ),
        )
        if (filtered.find((i) => i.id === params.item.id)) return filtered
        return [...filtered, params.item]
      })
    })

    return () => {
      unsubTurn()
      unsub1()
      unsub2()
      unsub3()
    }
  }, [threadId, queryClient])

  // Expose so Conversation can pre-register a turnId right after turn.create,
  // before SSE delivers item.started notifications for that turn.
  const registerTurnId = useCallback((turnId: string) => {
    knownTurnIdsRef.current.add(turnId)
  }, [])

  return { query, registerTurnId }
}
