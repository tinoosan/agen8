/**
 * useCreateSpace — shared hook for space creation logic used by both
 * the section header + button and the SpaceList empty state. Eliminates
 * the duplicated RPC + invalidation + navigation + error handling code.
 */
import { useState, useCallback } from 'react'
import { useLocation } from 'wouter'
import { useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { useNavigation } from '../lib/routing'
import { rpcCall } from '../lib/rpc'
import { isCanonicalSpaceId } from '../lib/spaceHelpers'

interface CreateSpaceOptions {
  /** Called with the new space ID after successful creation. */
  onCreated?: (spaceId: string) => void
}

export function useCreateSpace(options?: CreateSpaceOptions) {
  const { projectId, setFocusedSpaceId } = useNavigation()
  const [, navigate] = useLocation()
  const queryClient = useQueryClient()
  const [creating, setCreating] = useState(false)

  const createSpace = useCallback(async () => {
    if (creating || !projectId) {
      if (!projectId) toast.error('Select a project before creating a space')
      return
    }
    setCreating(true)
    try {
      const result = await rpcCall<{ space?: { id?: string } }>('space.create', { projectId })
      const newSpaceId = (result.space?.id ?? '').trim()
      if (!newSpaceId) throw new Error('space.create returned no space id')
      if (!isCanonicalSpaceId(newSpaceId)) throw new Error(`space.create returned invalid space id: ${newSpaceId}`)

      setFocusedSpaceId(newSpaceId)
      await queryClient.invalidateQueries({ queryKey: ['space.list'] })
      navigate(`/project/${encodeURIComponent(projectId)}/space/${encodeURIComponent(newSpaceId)}`)
      toast.success('Space created')
      options?.onCreated?.(newSpaceId)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to create space')
    } finally {
      setCreating(false)
    }
  }, [creating, projectId, setFocusedSpaceId, queryClient, navigate, options])

  return { createSpace, creating }
}
