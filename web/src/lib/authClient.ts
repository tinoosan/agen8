import { clearStoredSessionToken, getStoredSessionToken, rpcCall, setStoredSessionToken } from './rpc'
import type { AuthAPIKey, AuthStatus, AuthUser } from './types'

export interface LoginInput {
  email: string
  password: string
}

export interface AuthResult {
  user: AuthUser
  token?: string
}

export interface UpdateProfileInput {
  email: string
  name: string
}

export interface CreateAPIKeyResult {
  key: AuthAPIKey
  secret: string
}

interface AuthStatusResult {
  authenticated: boolean
  userId?: string
  role?: string
  user?: {
    id: string
    role?: string
  }
}

interface UserStatusResult {
  setupOpen: boolean
  setupUrl?: string
  user?: AuthUser | null
}

interface LoginResult {
  userId: string
  role: string
  token: string
  expiresAt: string
}

interface UserResult {
  user: AuthUser
}

interface CreateAPIKeyRPCResult {
  id: string
  name: string
  prefix: string
  token: string
  expiresAt?: string
}

interface APIKeyRPCView {
  id: string
  name: string
  prefix: string
  createdAt: string
  expiresAt?: string
  revokedAt?: string
  active: boolean
}

interface ListAPIKeysRPCResult {
  keys: APIKeyRPCView[]
}

export async function getAuthStatus(): Promise<AuthStatus> {
  let auth: AuthStatusResult
  try {
    auth = await rpcCall<AuthStatusResult>('auth.status', {})
  } catch (err) {
    if (err instanceof Error && err.message.includes('HTTP 401')) {
      clearStoredSessionToken()
      auth = await rpcCall<AuthStatusResult>('auth.status', {})
    } else {
      throw err
    }
  }
  if (!auth.authenticated) {
    const setup = await getSetupStatus()
    return {
      enabled: true,
      hostedMode: true,
      authenticated: false,
      setupOpen: setup.setupOpen,
      setupUrl: setup.setupUrl,
      user: null,
    }
  }

  const userStatus = await rpcCall<UserStatusResult>('user.status', {})
  return {
    enabled: true,
    hostedMode: true,
    authenticated: true,
    setupOpen: userStatus.setupOpen,
    setupUrl: userStatus.setupUrl,
    user: userStatus.user ?? {
      id: auth.user?.id ?? auth.userId ?? '',
      email: '',
      name: '',
      role: auth.role ?? auth.user?.role,
      createdAt: '',
    },
  }
}

async function getSetupStatus(): Promise<UserStatusResult> {
  try {
    return await rpcCall<UserStatusResult>('auth.setupStatus', {})
  } catch {
    return { setupOpen: false }
  }
}

export async function login(input: LoginInput): Promise<AuthResult> {
  const result = await rpcCall<LoginResult>('auth.login', input)
  setStoredSessionToken(result.token)
  return {
    token: result.token,
    user: {
      id: result.userId,
      email: input.email,
      name: '',
      role: result.role,
      createdAt: '',
    },
  }
}

export async function logout(): Promise<void> {
  const token = getStoredSessionToken()
  try {
    if (token) {
      await rpcCall('auth.logout', { token })
    }
  } finally {
    clearStoredSessionToken()
  }
}

export async function updateProfile(input: UpdateProfileInput): Promise<AuthResult> {
  const result = await rpcCall<UserResult>('user.updateProfile', input)
  return { user: result.user }
}

export async function createAPIKey(name: string): Promise<CreateAPIKeyResult> {
  const result = await rpcCall<CreateAPIKeyRPCResult>('auth.apiKey.create', { name })
  return {
    secret: result.token,
    key: {
      id: result.id,
      name: result.name,
      prefix: result.prefix,
      createdAt: '',
    },
  }
}

export async function listAPIKeys(): Promise<AuthAPIKey[]> {
  const result = await rpcCall<ListAPIKeysRPCResult>('auth.apiKey.list', {})
  return result.keys.map((key) => ({
    id: key.id,
    name: key.name,
    prefix: key.prefix,
    createdAt: key.createdAt,
    expiresAt: key.expiresAt,
    revokedAt: key.revokedAt,
    active: key.active,
  }))
}

export async function revokeAPIKey(keyId: string): Promise<void> {
  await rpcCall('auth.apiKey.revoke', { id: keyId })
}
