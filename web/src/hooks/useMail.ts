import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { rpcCall } from '../lib/rpc'
import type { MailMessage } from '../lib/types'

interface MessageListParams {
  teamId?: string
  view?: 'inbox' | 'outbox'
  limit?: number
}

interface MessageListResult {
  messages: MailMessage[]
}

export function useMail(teamId: string | null) {
  const queryClient = useQueryClient()
  const key = ['message.list', teamId]

  const query = useQuery<MailMessage[]>({
    queryKey: key,
    queryFn: async () => {
      const [inboxRes, outboxRes] = await Promise.all([
        rpcCall<MessageListResult>('message.list', {
          teamId: teamId ?? undefined,
          view: 'inbox',
          limit: 100,
        } satisfies MessageListParams),
        rpcCall<MessageListResult>('message.list', {
          teamId: teamId ?? undefined,
          view: 'outbox',
          limit: 100,
        } satisfies MessageListParams),
      ])
      return [...(inboxRes.messages ?? []), ...(outboxRes.messages ?? [])]
    },
    enabled: !!teamId,
    refetchInterval: 3000,
    staleTime: 2000,
  })

  const claimMutation = useMutation({
    mutationFn: (taskId: string) =>
      rpcCall('task.claim', { taskId }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: key }),
  })

  const completeMutation = useMutation({
    mutationFn: ({ taskId, summary }: { taskId: string; summary?: string }) =>
      rpcCall('task.complete', { taskId, summary }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: key }),
  })

  const messages = query.data ?? []
  const inbox = messages.filter(m => !isOutboxMessage(m))
  const outbox = messages.filter(m => isOutboxMessage(m))
  const badgeCount = inbox.length

  return {
    query,
    inbox,
    outbox,
    badgeCount,
    claim: claimMutation.mutate,
    complete: completeMutation.mutate,
  }
}

function isOutboxMessage(message: MailMessage): boolean {
  const taskStatus = message.taskStatus ?? message.task?.status
  if (taskStatus) {
    return taskStatus === 'review_pending' || taskStatus === 'succeeded' || taskStatus === 'failed' || taskStatus === 'canceled'
  }
  return message.channel === 'outbox' || message.status === 'acked' || message.status === 'deadletter'
}
