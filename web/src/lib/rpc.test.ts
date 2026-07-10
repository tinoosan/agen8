import { describe, expect, it, vi } from 'vitest'
import {
  clearLegacySessionToken,
  LEGACY_AUTH_TOKEN_STORAGE_KEY,
  rpcCall,
} from './rpc'

describe('rpc auth transport', () => {
  it('removes legacy browser-readable session tokens', () => {
    window.localStorage.setItem(LEGACY_AUTH_TOKEN_STORAGE_KEY, 'legacy-token')

    clearLegacySessionToken()

    expect(window.localStorage.getItem(LEGACY_AUTH_TOKEN_STORAGE_KEY)).toBeNull()
  })

  it('uses same-origin cookies without constructing an authorization header', async () => {
    window.localStorage.setItem(LEGACY_AUTH_TOKEN_STORAGE_KEY, 'legacy-token')
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
    expect(init.credentials).toBe('same-origin')
    expect((init.headers as Record<string, string>).Authorization).toBeUndefined()
    expect(window.localStorage.getItem(LEGACY_AUTH_TOKEN_STORAGE_KEY)).toBe('legacy-token')
    fetchMock.mockRestore()
  })
})
