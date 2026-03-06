import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect } from 'react'
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

  const query = useQuery<Item[]>({
    queryKey: key,
    queryFn: async () => {
      const res = await rpcCall<ItemListResult>('item.list', {
        threadId,
        limit: 200,
      })
      return res.items ?? []
    },
    enabled: !!threadId,
    staleTime: Infinity,
    retry: false,
  })

  useEffect(() => {
    if (!threadId) return

    // Track which turnIds belong to our thread so we can filter notifications.
    const knownTurnIds = new Set<string>()

    // Seed from current cache.
    const cached = queryClient.getQueryData<Item[]>(key)
    if (cached) {
      for (const item of cached) {
        if (item.turnId) knownTurnIds.add(item.turnId)
      }
    }

    const unsubTurn = onNotification('turn.started', (notif) => {
      const params = notif.params as { turn?: { id: string; threadId: string } } | undefined
      if (params?.turn?.threadId === threadId) {
        knownTurnIds.add(params.turn.id)
      }
    })

    // item.completed — optimistically update item in cache.
    const unsub1 = onNotification('item.completed', (notif) => {
      const params = notif.params as ItemNotificationParams | undefined
      if (!params?.item) return
      if (params.item.turnId && !knownTurnIds.has(params.item.turnId)) return

      queryClient.setQueryData<Item[]>(key, (prev) => {
        if (!prev) return [params.item]
        const idx = prev.findIndex((i) => i.id === params.item.id)
        if (idx === -1) return [...prev, params.item]
        const next = [...prev]
        next[idx] = params.item
        return next
      })
    })

    // item.delta — accumulate text into existing item's content.
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

    // item.started — add new item to cache.
    const unsub3 = onNotification('item.started', (notif) => {
      const params = notif.params as ItemNotificationParams | undefined
      if (!params?.item) return

      // Track this turn as belonging to our thread.
      if (params.item.turnId) knownTurnIds.add(params.item.turnId)

      queryClient.setQueryData<Item[]>(key, (prev) => {
        if (!prev) return [params.item]
        // Replace optimistic items for the same turn.
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

  return query
}
