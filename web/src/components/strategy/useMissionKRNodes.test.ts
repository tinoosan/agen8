import { beforeEach, describe, expect, it, vi } from 'vitest'
import { renderHook } from '@testing-library/react'
import type { MissionView, KeyResultView } from '../../lib/types'
import { useMissionKRNodes } from './useMissionKRNodes'

vi.mock('../../hooks/useMissions', () => ({ useMissions: vi.fn() }))
vi.mock('@tanstack/react-query', async (importOriginal) => {
  const mod = await importOriginal<typeof import('@tanstack/react-query')>()
  return { ...mod, useQuery: vi.fn() }
})

import { useMissions } from '../../hooks/useMissions'
import { useQuery } from '@tanstack/react-query'

const mockUseMissions = vi.mocked(useMissions)
const mockUseQuery = vi.mocked(useQuery)

function mission(overrides: Partial<MissionView>): MissionView {
  return {
    id: 'mis-1',
    projectId: 'playground',
    title: 'Mission',
    status: 'active',
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function kr(overrides: Partial<KeyResultView>): KeyResultView {
  return {
    id: 'kr-1',
    missionId: 'mis-1',
    title: 'KR',
    measurementType: 'numeric',
    direction: 'increase',
    targetValue: 100,
    currentValue: 20,
    progressPercent: 20,
    status: 'open',
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

beforeEach(() => {
  mockUseMissions.mockReset()
  mockUseQuery.mockReset()
})

describe('useMissionKRNodes', () => {
  it('excludes archived missions from strategy-map mission and KR topology', () => {
    mockUseMissions.mockReturnValue({
      data: [
        mission({ id: 'mis-active', status: 'active' }),
        mission({ id: 'mis-archived', status: 'archived' }),
      ],
      isLoading: false,
    } as never)

    mockUseQuery.mockReturnValue({
      data: {
        'mis-active': [kr({ id: 'kr-active', missionId: 'mis-active' })],
        'mis-archived': [kr({ id: 'kr-archived', missionId: 'mis-archived' })],
      },
      isLoading: false,
    } as never)

    const { result } = renderHook(() => useMissionKRNodes('playground', '/tmp/project'))

    const nodeIds = result.current.nodes.map((node) => node.id)
    expect(nodeIds).toContain('mis-active')
    expect(nodeIds).toContain('kr-active')

    expect(nodeIds).not.toContain('mis-archived')
    expect(nodeIds).not.toContain('kr-archived')
    expect(result.current.edges.some((edge) => edge.id.includes('mis-archived'))).toBe(false)
  })

  it('includes archived missions and KRs when archived visibility is enabled', () => {
    mockUseMissions.mockReturnValue({
      data: [
        mission({ id: 'mis-active', status: 'active' }),
        mission({ id: 'mis-archived', status: 'archived' }),
      ],
      isLoading: false,
    } as never)

    mockUseQuery.mockReturnValue({
      data: {
        'mis-active': [kr({ id: 'kr-active', missionId: 'mis-active' })],
        'mis-archived': [kr({ id: 'kr-archived', missionId: 'mis-archived' })],
      },
      isLoading: false,
    } as never)

    const { result } = renderHook(() => useMissionKRNodes('playground', '/tmp/project', { showArchived: true }))

    const nodeIds = result.current.nodes.map((node) => node.id)
    expect(nodeIds).toContain('mis-active')
    expect(nodeIds).toContain('kr-active')
    expect(nodeIds).toContain('mis-archived')
    expect(nodeIds).toContain('kr-archived')
  })
})
