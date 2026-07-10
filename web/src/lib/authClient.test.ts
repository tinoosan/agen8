import { afterEach, describe, expect, it, vi } from 'vitest'
import { getAuthStatus, listAPIKeys } from './authClient'

function rpcResponse(result: unknown) {
  return new Response(JSON.stringify({
    jsonrpc: '2.0',
    id: '1',
    result,
  }), {
    headers: { 'Content-Type': 'application/json' },
    status: 200,
  })
}

describe('authClient', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    localStorage.clear()
  })

  it('reports setup availability without receiving a setup token URL', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch')
      .mockResolvedValueOnce(rpcResponse({ authenticated: false }))
      .mockResolvedValueOnce(rpcResponse({ setupOpen: true }))

    await expect(getAuthStatus()).resolves.toMatchObject({
      authenticated: false,
      setupOpen: true,
      setupUrl: undefined,
    })

    const secondBody = JSON.parse(String(fetchMock.mock.calls[1]?.[1]?.body))
    expect(secondBody.method).toBe('auth.setupStatus')
  })

  it('maps API key list responses', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(rpcResponse({
      keys: [{
        id: 'key-1',
        name: 'Agen8 MCP key',
        prefix: 'ak_test',
        createdAt: '2026-06-01T12:00:00Z',
        revokedAt: '2026-06-02T12:00:00Z',
        active: false,
      }],
    }))

    await expect(listAPIKeys()).resolves.toEqual([{
      id: 'key-1',
      name: 'Agen8 MCP key',
      prefix: 'ak_test',
      createdAt: '2026-06-01T12:00:00Z',
      revokedAt: '2026-06-02T12:00:00Z',
      active: false,
    }])
  })
})
