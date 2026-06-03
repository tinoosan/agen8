import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useMemo } from 'react'
import { onNotification, rpcCall } from '../lib/rpc'

export interface HumanInputQuestion {
  id: string
  text: string
  // multiple_choice: pick one option (radio)
  // multi_select:    pick any subset of options (checkboxes)
  // free_form:       open text answer
  type: 'multiple_choice' | 'multi_select' | 'free_form'
  options?: string[]
  allowFreeForm?: boolean
  recommendation?: string
  blocking?: boolean
}

export interface QuestionsPayload {
  title?: string
  context?: string
  questions: HumanInputQuestion[]
}

export interface HumanInputAnswer {
  questionId: string
  // selectedOption is set for multiple_choice answers.
  // selectedOptions is set for multi_select answers (one entry per
  // checked option). Both fields stay distinct on the wire so a
  // consumer that only handles single-select doesn't accidentally
  // treat a multi-select answer as the first picked option.
  selectedOption?: string
  selectedOptions?: string[]
  freeFormText?: string
}

export interface QuestionsResult {
  cancelled?: boolean
  answers?: HumanInputAnswer[]
}

export interface ApproveRejectPayload {
  title: string
  description?: string
  context?: string
}

export interface ApproveRejectResult {
  cancelled?: boolean
  decision?: 'approve' | 'reject'
  note?: string
}

export type HumanInputPayload = QuestionsPayload | ApproveRejectPayload | Record<string, unknown>
export type HumanInputResult = QuestionsResult | ApproveRejectResult

// PendingHumanInput identifies the asker (spaceId + memberId) and the
// panel delivery target (channelId) separately. The UI subscribes by
// channelId; submit/cancel use spaceId + memberId because the answer
// is logically owned by the asking member.
export interface PendingHumanInput {
  spaceId: string
  memberId: string
  channelId: string
  toolCallId: string
  toolName: string
  primitive: string
  payload: HumanInputPayload
  projectId: string
  createdAt: string
}

interface PendingHumanInputResult {
  pending: null | PendingHumanInput
}

export function useHumanInput(channelId: string | null) {
  const queryClient = useQueryClient()
  const key = useMemo(() => ['channel.human_input.pending', channelId ?? ''], [channelId])

  const query = useQuery<PendingHumanInput | null>({
    queryKey: key,
    enabled: !!channelId,
    retry: false,
    queryFn: async () => {
      const res = await rpcCall<PendingHumanInputResult>('channel.human_input.pending', { channelId })
      return res.pending ?? null
    },
  })

  useEffect(() => {
    if (!channelId) return
    return onNotification('channel.human_input.changed', (notification) => {
      const params = (notification.params ?? {}) as { channelId?: string }
      if ((params.channelId ?? '').trim() !== channelId) return
      void queryClient.invalidateQueries({ queryKey: key })
    })
  }, [key, queryClient, channelId])

  const submit = useMutation({
    mutationFn: async ({
      spaceId,
      memberId,
      toolCallId,
      result,
    }: {
      spaceId: string
      memberId: string
      toolCallId: string
      result: HumanInputResult
    }) => {
      return rpcCall<{ ok: boolean }>('channel.human_input.submit', {
        spaceId,
        memberId,
        toolCallId,
        result,
      })
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: key })
    },
  })

  const cancel = useMutation({
    mutationFn: async ({
      spaceId,
      memberId,
      toolCallId,
    }: {
      spaceId: string
      memberId: string
      toolCallId: string
    }) => {
      return rpcCall<{ ok: boolean }>('channel.human_input.cancel', {
        spaceId,
        memberId,
        toolCallId,
      })
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: key })
    },
  })

  return {
    query,
    pending: query.data ?? null,
    submit,
    cancel,
    isSubmitting: submit.isPending || cancel.isPending,
  }
}
