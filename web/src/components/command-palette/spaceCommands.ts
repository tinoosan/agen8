import { rpcCall } from '../../lib/rpc'
import type { QueryClient } from '@tanstack/react-query'

interface CommandContext {
  queryClient: QueryClient
  setPaletteOpen: (open: boolean) => void
}

export async function deleteSpace(
  spaceId: string,
  projectRoot: string | null | undefined,
  { queryClient, setPaletteOpen }: CommandContext,
) {
  await rpcCall('space.delete', { spaceId, projectRoot: projectRoot ?? undefined })
  queryClient.invalidateQueries({ queryKey: ['project.space.list'] })
  setPaletteOpen(false)
}

export async function stopSpace(spaceId: string, { queryClient, setPaletteOpen }: CommandContext) {
  if (!spaceId) {
    throw new Error('spaceId is required')
  }
  await rpcCall('space.stop', { spaceId })
  queryClient.invalidateQueries({ queryKey: ['space.status'] })
  queryClient.invalidateQueries({ queryKey: ['space.roster'] })
  queryClient.invalidateQueries({ queryKey: ['logs.query'] })
  setPaletteOpen(false)
}

export async function clearHistory(spaceId: string, { queryClient, setPaletteOpen }: CommandContext) {
  await rpcCall('space.clearHistory', { spaceId })
  queryClient.invalidateQueries({ queryKey: ['space.status'] })
  queryClient.invalidateQueries({ queryKey: ['space.roster'] })
  queryClient.invalidateQueries({ queryKey: ['logs.query'] })
  queryClient.invalidateQueries({ queryKey: ['logs.query'] })
  queryClient.invalidateQueries({ queryKey: ['space.detail', spaceId] })
  setPaletteOpen(false)
}
