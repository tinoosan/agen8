import { describe, expect, it, vi } from 'vitest'
import {
  AUTH_TOKEN_STORAGE_KEY,
  clearStoredSessionToken,
  getStoredSessionToken,
  rpcCall,
  setStoredSessionToken,
} from './rpc'

describe('rpc auth transport', () => {
  it('persists session tokens in localStorage', () => {
    window.localStorage.setItem(AUTH_TOKEN_STORAGE_KEY, 'legacy-token')

    setStoredSessionToken('new-token')

    expect(getStoredSessionToken()).toBe('new-token')
    expect(window.localStorage.getItem(AUTH_TOKEN_STORAGE_KEY)).toBe('new-token')
  })

  it('clears legacy session tokens', () => {
    window.localStorage.setItem(AUTH_TOKEN_STORAGE_KEY, 'legacy-token')

    clearStoredSessionToken()

    expect(window.localStorage.getItem(AUTH_TOKEN_STORAGE_KEY)).toBeNull()
  })

  it('sends stored bearer tokens on rpc calls', async () => {
    window.localStorage.setItem(AUTH_TOKEN_STORAGE_KEY, 'test-token')
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({
      jsonrpc: '2.0',
      id: '1',
      result: { ok: true },
    }), {
      headers: { 'Content-Type': 'application/json' },
      status: 200,
    }))

    await expect(rpcCall('auth.status', {})).resolves.toEqual({ ok: true })

    const init = fetchMock.mock.calls[0]?.[1] as RequestInit
    expect((init.headers as Record<string, string>).Authorization).toBe('Bearer test-token')
    fetchMock.mockRestore()
  })
})
