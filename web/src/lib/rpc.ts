// JSON-RPC 2.0 client over HTTP (requests) + SSE (notifications).
// - POST /rpc for individual request/response calls
// - GET /events (EventSource) for server-pushed notifications

type RpcRequest = {
  jsonrpc: '2.0'
  id: string
  method: string
  params: unknown
}

type RpcResponse = {
  jsonrpc: '2.0'
  id: string
  result?: unknown
  error?: { code: number; message: string }
}

type RpcNotification = {
  jsonrpc: '2.0'
  method: string
  params?: unknown
}

type NotificationHandler = (notification: RpcNotification) => void

let requestSeq = 0
const handlers = new Map<string, NotificationHandler[]>()
let eventSource: EventSource | null = null
let reconnectTimer: ReturnType<typeof setTimeout> | null = null
export const AUTH_TOKEN_STORAGE_KEY = 'agen8.sessionToken'

export function getStoredSessionToken(): string {
  if (typeof window === 'undefined') return ''
  try {
    return window.localStorage.getItem(AUTH_TOKEN_STORAGE_KEY)?.trim() ?? ''
  } catch {
    return ''
  }
}

export function setStoredSessionToken(token: string) {
  if (typeof window === 'undefined') return
  const trimmed = token.trim()
  try {
    if (trimmed) {
      window.localStorage.setItem(AUTH_TOKEN_STORAGE_KEY, trimmed)
      return
    }
    window.localStorage.removeItem(AUTH_TOKEN_STORAGE_KEY)
  } catch {
    // Blocked storage means the browser cannot persist the session token.
  }
}

export function clearStoredSessionToken() {
  if (typeof window === 'undefined') return
  try {
    window.localStorage.removeItem(AUTH_TOKEN_STORAGE_KEY)
  } catch {
    // Ignore blocked storage while removing the legacy bearer-token cache.
  }
}

// ---- Request / Response ------------------------------------------------

export async function rpcCall<T = unknown>(method: string, params: unknown = {}): Promise<T> {
  const id = String(++requestSeq)
  const body: RpcRequest = { jsonrpc: '2.0', id, method, params }
  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  const token = getStoredSessionToken()
  if (token) {
    headers.Authorization = `Bearer ${token}`
  }
  const res = await fetch('/rpc', {
    method: 'POST',
    headers,
    body: JSON.stringify(body),
  })
  if (!res.ok) {
    throw new Error(`HTTP ${res.status}: ${await res.text()}`)
  }
  const msg: RpcResponse = await res.json()
  if (msg.error) {
    throw Object.assign(new Error(msg.error.message), { code: msg.error.code })
  }
  return msg.result as T
}

/**
 * Calls an RPC method and returns a single named field from the result
 * envelope. Collapses the `const res = await rpcCall<{ x: T }>(...); return res.x`
 * idiom for single-object results. The cast is contained here so call sites
 * read as `rpcUnwrap<KeyResultView>('mission.kr.get', params, 'keyResult')`.
 */
export async function rpcUnwrap<T>(method: string, params: unknown, field: string): Promise<T> {
  const res = await rpcCall<Record<string, unknown>>(method, params)
  return res[field] as T
}

/**
 * Calls an RPC method and returns a named array field, defaulting to [] when
 * the server omits it. Collapses the `const res = await rpcCall<{ x: T[] }>(...);
 * return res.x ?? []` idiom that recurs across the list query hooks.
 */
export async function rpcUnwrapList<T>(method: string, params: unknown, field: string): Promise<T[]> {
  const res = await rpcCall<Record<string, T[] | undefined>>(method, params)
  return res[field] ?? []
}

// ---- Notifications (SSE) -----------------------------------------------

export function onNotification(method: string, handler: NotificationHandler): () => void {
  if (!handlers.has(method)) handlers.set(method, [])
  handlers.get(method)!.push(handler)
  ensureEventSource()
  return () => {
    const list = handlers.get(method)
    if (!list) return
    const idx = list.indexOf(handler)
    if (idx !== -1) list.splice(idx, 1)
  }
}

function dispatch(notification: RpcNotification) {
  const list = handlers.get(notification.method)
  if (list) list.forEach(h => h(notification))
  // Also dispatch to wildcard handlers
  const all = handlers.get('*')
  if (all) all.forEach(h => h(notification))
}

function ensureEventSource() {
  if (eventSource && eventSource.readyState !== EventSource.CLOSED) return
  connect()
}

function connect() {
  if (reconnectTimer) clearTimeout(reconnectTimer)
  const es = new EventSource('/events')
  eventSource = es

  es.onmessage = (e) => {
    try {
      const msg: RpcNotification = JSON.parse(e.data)
      if (msg.method) dispatch(msg)
    } catch {
      // ignore malformed messages
    }
  }

  es.onerror = () => {
    es.close()
    eventSource = null
    // Reconnect after 2s
    reconnectTimer = setTimeout(connect, 2000)
  }
}

export function isConnected(): boolean {
  return eventSource?.readyState === EventSource.OPEN
}
