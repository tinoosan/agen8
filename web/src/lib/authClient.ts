import { clearLegacySessionToken, resumeSessionAfterAuthentication, rpcCall, suspendSessionAfterLogout } from './rpc'
import type { AuthAPIKey, AuthStatus, AuthUser } from './types'
import type { UserPreferences } from './store'

export interface LoginInput {
  email: string
  password: string
}

export interface AuthResult {
  user: AuthUser
}

export interface UpdateProfileInput {
  email?: string
  name?: string
  preferences?: UserPreferences
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
  const auth = await rpcCall<AuthStatusResult>('auth.status', {})
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
  clearLegacySessionToken()
  resumeSessionAfterAuthentication()
  return {
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
  try {
    await rpcCall('auth.logout', {})
  } finally {
    clearLegacySessionToken()
    suspendSessionAfterLogout()
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
