import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { rpcCall } from '../lib/rpc'
import type { Channel } from '../lib/types'

export function useChannelList(spaceId: string | null) {
  return useQuery<Channel[]>({
    queryKey: ['channel.list', spaceId ?? ''],
    queryFn: async () => {
      const res = await rpcCall<{ channels: Channel[] }>('channel.list', { spaceId })
      return res.channels ?? []
    },
    enabled: !!spaceId,
    refetchInterval: 5000,
    retry: false,
  })
}

export function useChannelGet(channelId: string | null) {
  return useQuery<Channel | null>({
    queryKey: ['channel.get', channelId ?? ''],
    queryFn: async () => {
      const res = await rpcCall<{ channel: Channel }>('channel.get', { channelId })
      return res.channel ?? null
    },
    enabled: !!channelId,
    placeholderData: keepPreviousData,
    retry: false,
  })
}

interface ChannelMarkReadResult {
  channelId: string
  lastSeenAt: string
}

/**
 * Mark a channel as read up to "now" for the current user. Backed by
 * the channel.markRead RPC which writes the per-user last_seen_at
 * marker server-side. Replaces the prior client-side localStorage
 * approach that lost state on refresh and over-fired on noisy
 * channel.updated_at touches.
 *
 * On success, invalidates channel.list so any sidebar entries
 * showing the unread dot for this channel refetch and clear it.
 */
export function useChannelMarkRead() {
  const queryClient = useQueryClient()
  return useMutation<ChannelMarkReadResult, Error, { channelId: string; spaceId?: string | null }>({
    mutationFn: async ({ channelId }) => {
      return await rpcCall<ChannelMarkReadResult>('channel.markRead', { channelId })
    },
    onSuccess: (_result, variables) => {
      // Optimistic local update first so the dot disappears
      // immediately, then invalidate so a refetch confirms the
      // server state. Falls back to a full refetch if the
      // optimistic patch can't find the entry.
      queryClient.setQueriesData<Channel[]>({ queryKey: ['channel.list'] }, old => {
        if (!Array.isArray(old)) return old
        return old.map(ch =>
          (ch.id === variables.channelId && ch.unread)
            ? { ...ch, unread: false }
            : ch,
        )
      })
      void queryClient.invalidateQueries({ queryKey: ['channel.list'] })
    },
  })
}
