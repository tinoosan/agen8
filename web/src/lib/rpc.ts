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
// Reconnect backoff state. EventSource gives onerror no HTTP status, so a
// fetch probe classifies the failure: transient errors retry on an
// exponential schedule; 401/403 halts retries until the token changes.
let reconnectAttempts = 0
let authBlockedToken: string | null = null
let probeInFlight = false
const RECONNECT_BASE_DELAY_MS = 1000
const RECONNECT_MAX_DELAY_MS = 30_000
export const AUTH_TOKEN_STORAGE_KEY = 'agen8.sessionToken'
const AUTH_TOKEN_COOKIE_NAME = 'agen8.sessionToken'

export function getStoredSessionToken(): string {
  if (typeof window === 'undefined') return ''
  try {
    const token = window.localStorage.getItem(AUTH_TOKEN_STORAGE_KEY)?.trim() ?? ''
    if (token) writeSessionCookie(token)
    return token
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
      writeSessionCookie(trimmed)
      resumeEventSourceAfterAuthChange(trimmed)
      return
    }
    window.localStorage.removeItem(AUTH_TOKEN_STORAGE_KEY)
    clearSessionCookie()
  } catch {
    // Blocked storage means the browser cannot persist the session token.
  }
}

// A fresh token invalidates an auth block: if /events was halted on 401/403
// under a different token, reconnect now that credentials changed.
function resumeEventSourceAfterAuthChange(token: string) {
  if (authBlockedToken === null || authBlockedToken === token) return
  authBlockedToken = null
  reconnectAttempts = 0
  if (handlers.size > 0) ensureEventSource()
}

export function clearStoredSessionToken() {
  if (typeof window === 'undefined') return
  try {
    window.localStorage.removeItem(AUTH_TOKEN_STORAGE_KEY)
    clearSessionCookie()
  } catch {
    // Ignore blocked storage while removing the legacy bearer-token cache.
  }
}

function writeSessionCookie(token: string) {
  if (typeof document === 'undefined') return
  document.cookie = `${AUTH_TOKEN_COOKIE_NAME}=${encodeURIComponent(token)}; path=/; SameSite=Lax`
}

function clearSessionCookie() {
  if (typeof document === 'undefined') return
  document.cookie = `${AUTH_TOKEN_COOKIE_NAME}=; path=/; max-age=0; SameSite=Lax`
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
  // A pending backoff timer or in-flight probe owns the next attempt; a halted
  // auth block waits for a token change. Re-subscribing must not bypass either —
  // that immediate-reconnect path is what turned a persistent 403 into a storm.
  if (reconnectTimer || probeInFlight) return
  if (authBlockedToken !== null && authBlockedToken === getStoredSessionToken()) return
  connect()
}

function connect() {
  if (reconnectTimer) {
    clearTimeout(reconnectTimer)
    reconnectTimer = null
  }
  const es = new EventSource('/events')
  eventSource = es

  es.onopen = () => {
    reconnectAttempts = 0
    authBlockedToken = null
  }

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
    void scheduleReconnect()
  }
}

// Classifies the failure and schedules (or halts) the next attempt.
// EventSource hides the response status, so probe /events with an aborted
// fetch: 401/403 halts retries until the token changes; anything else retries
// on an exponential schedule capped at RECONNECT_MAX_DELAY_MS.
async function scheduleReconnect() {
  if (probeInFlight) return
  probeInFlight = true
  let authFailure = false
  try {
    const controller = new AbortController()
    const res = await fetch('/events', {
      headers: { Accept: 'text/event-stream' },
      signal: controller.signal,
    })
    // Headers are enough to classify; abort before the stream body streams.
    controller.abort()
    authFailure = res.status === 401 || res.status === 403
  } catch {
    // Network-level failure: treat as transient and keep backing off.
  } finally {
    probeInFlight = false
  }

  if (authFailure) {
    authBlockedToken = getStoredSessionToken()
    return
  }

  const delay = Math.min(RECONNECT_BASE_DELAY_MS * 2 ** reconnectAttempts, RECONNECT_MAX_DELAY_MS)
  reconnectAttempts += 1
  reconnectTimer = setTimeout(() => {
    reconnectTimer = null
    connect()
  }, delay)
}

export function isConnected(): boolean {
  return eventSource?.readyState === EventSource.OPEN
}
