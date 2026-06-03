import { describe, it, expect, vi, beforeEach } from 'vitest'
import { createElement } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { renderHook, waitFor } from '@testing-library/react'
import { useAuth } from './useAuth'

vi.mock('../lib/authClient', () => ({
  getAuthStatus: vi.fn(),
  login: vi.fn(),
  logout: vi.fn(),
}))

import { getAuthStatus } from '../lib/authClient'

const mockGetAuthStatus = vi.mocked(getAuthStatus)

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return ({ children }: { children: React.ReactNode }) =>
    createElement(QueryClientProvider, { client: queryClient }, children)
}

describe('useAuth', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('derives hosted mode from hostedMode flag, not enabled', async () => {
    mockGetAuthStatus.mockResolvedValueOnce({
      enabled: true,
      hostedMode: false,
      authenticated: false,
      user: null,
      bridge: null,
    })

    const { result } = renderHook(() => useAuth(), { wrapper: createWrapper() })

    await waitFor(() => expect(result.current.status).toBeDefined())

    expect(result.current.isHosted).toBe(false)
    expect(result.current.isAuthenticated).toBe(false)
  })

  it('reports hosted mode when hostedMode is true', async () => {
    mockGetAuthStatus.mockResolvedValueOnce({
      enabled: true,
      hostedMode: true,
      authenticated: true,
      user: {
        id: 'user-1',
        email: 'user@example.com',
        name: 'User',
        createdAt: '2026-01-01T00:00:00Z',
      },
      bridge: { connected: false },
    })

    const { result } = renderHook(() => useAuth(), { wrapper: createWrapper() })

    await waitFor(() => expect(result.current.status).toBeDefined())

    expect(result.current.isHosted).toBe(true)
    expect(result.current.isAuthenticated).toBe(true)
  })
})
