import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect } from 'react'
import { rpcCall, onNotification } from '../lib/rpc'
import type { Item } from '../lib/types'

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

    const unsub1 = onNotification('item.completed', () => {
      queryClient.invalidateQueries({ queryKey: key })
    })

    const unsub2 = onNotification('item.delta', (notif) => {
      const params = notif.params as { item?: Item } | undefined
      if (!params?.item) return
      queryClient.setQueryData<Item[]>(key, (prev) => {
        if (!prev) return [params.item!]
        const idx = prev.findIndex(i => i.id === params.item!.id)
        if (idx === -1) return [...prev, params.item!]
        const next = [...prev]
        next[idx] = params.item!
        return next
      })
    })

    const unsub3 = onNotification('item.started', (notif) => {
      const params = notif.params as { item?: Item } | undefined
      if (!params?.item) return
      queryClient.setQueryData<Item[]>(key, (prev) => {
        if (!prev) return [params.item!]
        if (prev.find(i => i.id === params.item!.id)) return prev
        return [...prev, params.item!]
      })
    })

    return () => {
      unsub1()
      unsub2()
      unsub3()
    }
  }, [threadId, queryClient])

  return query
}
