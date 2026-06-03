export interface AuthStatus {
  provider: string
  loggedIn: boolean
  expiresAt?: string
  accountId?: string
  source?: string
}

async function readJSON<T>(res: Response): Promise<T> {
  if (!res.ok) {
    let msg = `HTTP ${res.status}`
    try {
      const body = await res.json() as { error?: string }
      if (body?.error) msg = body.error
    } catch {
      try {
        msg = await res.text()
      } catch {
        // Ignore response body parsing failures and fall back to HTTP status.
      }
    }
    throw new Error(msg)
  }
  return res.json() as Promise<T>
}

export async function logoutAuth(): Promise<AuthStatus> {
  const res = await fetch('/api/auth/logout', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
  })
  return readJSON<AuthStatus>(res)
}
