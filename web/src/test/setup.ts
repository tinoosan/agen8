import '@testing-library/jest-dom/vitest'
import { beforeEach } from 'vitest'

// Keep date/time formatting deterministic across developer machines and CI.
process.env.TZ = 'UTC'

// jsdom's localStorage may be non-functional when --localstorage-file is passed
// without a valid path. Provide a reliable in-memory implementation.
const _localStorageData: Record<string, string> = {}
const localStorageMock: Storage = {
  getItem: (key) => _localStorageData[key] ?? null,
  setItem: (key, value) => { _localStorageData[key] = String(value) },
  removeItem: (key) => { delete _localStorageData[key] },
  clear: () => { Object.keys(_localStorageData).forEach((k) => delete _localStorageData[k]) },
  key: (index) => Object.keys(_localStorageData)[index] ?? null,
  get length() { return Object.keys(_localStorageData).length },
}
Object.defineProperty(window, 'localStorage', { writable: true, value: localStorageMock })
beforeEach(() => { localStorageMock.clear() })

// Node 24+ / vitest may expose a broken global localStorage; provide a minimal in-memory stub.
{
  let store: Record<string, string> = {}
  const stub = {
    getItem: (key: string) => (key in store ? store[key] : null),
    setItem: (key: string, value: string) => { store[key] = String(value) },
    removeItem: (key: string) => { delete store[key] },
    clear: () => { store = {} },
    get length() { return Object.keys(store).length },
    key: (i: number) => Object.keys(store)[i] ?? null,
  }
  Object.defineProperty(globalThis, 'localStorage', { value: stub, configurable: true })
}

// jsdom doesn't implement ResizeObserver (required by cmdk)
class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
window.ResizeObserver = ResizeObserverStub as unknown as typeof ResizeObserver

// jsdom doesn't implement EventSource (required by rpc.ts onNotification)
class EventSourceStub {
  static readonly CONNECTING = 0
  static readonly OPEN = 1
  static readonly CLOSED = 2
  readonly CONNECTING = 0
  readonly OPEN = 1
  readonly CLOSED = 2
  readyState = 2 // CLOSED — prevent reconnection attempts
  onopen: (() => void) | null = null
  onmessage: ((event: MessageEvent) => void) | null = null
  onerror: (() => void) | null = null
  url: string
  withCredentials = false
  constructor(url: string | URL) { this.url = String(url) }
  addEventListener() {}
  removeEventListener() {}
  dispatchEvent() { return false }
  close() {}
}
window.EventSource = EventSourceStub as unknown as typeof EventSource

// jsdom doesn't implement scrollIntoView
Element.prototype.scrollIntoView = () => {}

// jsdom doesn't implement pointer capture (required by Radix UI primitives)
Element.prototype.hasPointerCapture = () => false
Element.prototype.setPointerCapture = () => {}
Element.prototype.releasePointerCapture = () => {}

// jsdom doesn't fully implement matchMedia (needed by shadcn sidebar / useIsMobile)
Object.defineProperty(window, 'matchMedia', {
  writable: true,
  value: (query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: () => {},
    removeListener: () => {},
    addEventListener: () => {},
    removeEventListener: () => {},
    dispatchEvent: () => false,
  }),
})
