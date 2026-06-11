import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

// The rpc module keeps connection state (eventSource, backoff counters, auth
// block) in module scope, so each test imports a fresh copy via resetModules.

class FakeEventSource {
  static instances: FakeEventSource[] = []
  static OPEN = 1
  static CLOSED = 2
  readyState = 0
  url: string
  onopen: (() => void) | null = null
  onmessage: ((e: MessageEvent) => void) | null = null
  onerror: (() => void) | null = null

  constructor(url: string) {
    this.url = url
    FakeEventSource.instances.push(this)
  }

  close() {
    this.readyState = FakeEventSource.CLOSED
  }

  emitOpen() {
    this.readyState = FakeEventSource.OPEN
    this.onopen?.()
  }

  emitError() {
    this.onerror?.()
  }
}

function mockEventsProbe(status: number) {
  return vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => {
    if (String(input) === '/events') {
      return new Response(null, { status })
    }
    throw new Error(`unexpected fetch: ${String(input)}`)
  })
}

async function importRpc() {
  vi.resetModules()
  return import('./rpc')
}

// Flushes the async probe inside scheduleReconnect (fetch + finally).
async function flushProbe() {
  await vi.advanceTimersByTimeAsync(0)
}

describe('rpc SSE reconnect policy', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    FakeEventSource.instances = []
    vi.stubGlobal('EventSource', FakeEventSource)
    window.localStorage.clear()
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
    vi.useRealTimers()
  })

  it('backs off exponentially on transient errors and resets after open', async () => {
    mockEventsProbe(500)
    const rpc = await importRpc()
    rpc.onNotification('event.test', () => {})
    expect(FakeEventSource.instances).toHaveLength(1)

    // First error: 1s delay.
    FakeEventSource.instances[0].emitError()
    await flushProbe()
    await vi.advanceTimersByTimeAsync(999)
    expect(FakeEventSource.instances).toHaveLength(1)
    await vi.advanceTimersByTimeAsync(1)
    expect(FakeEventSource.instances).toHaveLength(2)

    // Second error: 2s delay.
    FakeEventSource.instances[1].emitError()
    await flushProbe()
    await vi.advanceTimersByTimeAsync(1999)
    expect(FakeEventSource.instances).toHaveLength(2)
    await vi.advanceTimersByTimeAsync(1)
    expect(FakeEventSource.instances).toHaveLength(3)

    // Open resets the schedule: the next failure waits 1s again.
    FakeEventSource.instances[2].emitOpen()
    FakeEventSource.instances[2].emitError()
    await flushProbe()
    await vi.advanceTimersByTimeAsync(1000)
    expect(FakeEventSource.instances).toHaveLength(4)
  })

  it('caps the backoff delay at 30s', async () => {
    mockEventsProbe(500)
    const rpc = await importRpc()
    rpc.onNotification('event.test', () => {})

    // Walk the schedule well past the cap (2^10s uncapped = 1024s).
    for (let i = 0; i < 10; i++) {
      FakeEventSource.instances[FakeEventSource.instances.length - 1].emitError()
      await flushProbe()
      await vi.advanceTimersByTimeAsync(30_000)
    }
    const countAtCap = FakeEventSource.instances.length
    FakeEventSource.instances[countAtCap - 1].emitError()
    await flushProbe()
    await vi.advanceTimersByTimeAsync(30_000)
    expect(FakeEventSource.instances).toHaveLength(countAtCap + 1)
  })

  it('re-subscribing does not bypass a pending backoff timer', async () => {
    mockEventsProbe(500)
    const rpc = await importRpc()
    rpc.onNotification('event.test', () => {})
    FakeEventSource.instances[0].emitError()
    await flushProbe()

    // A new subscriber arrives while the 1s timer is pending — the old code
    // reconnected immediately here, producing the storm.
    rpc.onNotification('event.other', () => {})
    expect(FakeEventSource.instances).toHaveLength(1)
    await vi.advanceTimersByTimeAsync(1000)
    expect(FakeEventSource.instances).toHaveLength(2)
  })

  it('halts retries entirely on 403 until the token changes', async () => {
    mockEventsProbe(403)
    const rpc = await importRpc()
    rpc.onNotification('event.test', () => {})
    FakeEventSource.instances[0].emitError()
    await flushProbe()

    // No timer is scheduled and re-subscribing does not reconnect.
    await vi.advanceTimersByTimeAsync(120_000)
    rpc.onNotification('event.other', () => {})
    await vi.advanceTimersByTimeAsync(120_000)
    expect(FakeEventSource.instances).toHaveLength(1)

    // A new token lifts the block immediately.
    rpc.setStoredSessionToken('fresh-token')
    expect(FakeEventSource.instances).toHaveLength(2)
  })

  it('stays halted when the stored token has not changed', async () => {
    mockEventsProbe(403)
    const rpc = await importRpc()
    rpc.setStoredSessionToken('stale-token')
    rpc.onNotification('event.test', () => {})
    const after = FakeEventSource.instances.length
    FakeEventSource.instances[after - 1].emitError()
    await flushProbe()

    // Re-setting the SAME token must not lift the block.
    rpc.setStoredSessionToken('stale-token')
    await vi.advanceTimersByTimeAsync(120_000)
    expect(FakeEventSource.instances).toHaveLength(after)
  })
})
