import { useState, useCallback } from 'react'

function storageKey(projectId: string): string {
  return `oa-pinned-missions:${projectId}`
}

function readFromStorage(projectId: string): Set<string> {
  try {
    const raw = localStorage.getItem(storageKey(projectId))
    if (!raw) return new Set()
    const parsed = JSON.parse(raw)
    if (!Array.isArray(parsed)) return new Set()
    return new Set(parsed as string[])
  } catch {
    return new Set()
  }
}

function writeToStorage(projectId: string, ids: Set<string>): void {
  const key = storageKey(projectId)
  try {
    localStorage.setItem(key, JSON.stringify([...ids]))
  } catch {
    // Nuclear sweep on quota exhaustion — same pattern as useBatchExpandedState
    try {
      for (let i = localStorage.length - 1; i >= 0; i--) {
        const k = localStorage.key(i)
        if (k?.startsWith('oa-pinned-missions:')) localStorage.removeItem(k)
      }
      localStorage.setItem(key, JSON.stringify([...ids]))
    } catch { /* noop */ }
  }
}

export interface UsePinnedMissionsResult {
  pinnedIds: Set<string>
  isPinned: (missionId: string) => boolean
  togglePin: (missionId: string) => void
}

/**
 * Persists pinned mission IDs in localStorage per project.
 * Key: oa-pinned-missions:{projectId}
 * Usable across any part of the UI that imports this hook.
 */
export function usePinnedMissions(projectId: string | null): UsePinnedMissionsResult {
  const [pinnedIds, setPinnedIds] = useState<Set<string>>(() =>
    projectId ? readFromStorage(projectId) : new Set()
  )

  const isPinned = useCallback(
    (missionId: string) => pinnedIds.has(missionId),
    [pinnedIds],
  )

  const togglePin = useCallback(
    (missionId: string) => {
      if (!projectId) return
      setPinnedIds(prev => {
        const next = new Set(prev)
        if (next.has(missionId)) {
          next.delete(missionId)
        } else {
          next.add(missionId)
        }
        writeToStorage(projectId, next)
        return next
      })
    },
    [projectId],
  )

  return { pinnedIds, isPinned, togglePin }
}
