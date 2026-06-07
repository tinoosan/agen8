import { useQuery, useQueryClient } from '@tanstack/react-query'
import { qk } from '../lib/queryKeys'
import {
  getAuthStatus,
  login as loginRequest,
  logout as logoutRequest,
  updateProfile as updateProfileRequest,
  type LoginInput,
  type UpdateProfileInput,
} from '../lib/authClient'

export function useAuth() {
  const queryClient = useQueryClient()
  const statusQuery = useQuery({
    queryKey: qk.authStatus,
    queryFn: getAuthStatus,
    staleTime: 15_000,
    retry: false,
    refetchOnWindowFocus: true,
  })

  const refresh = async () => queryClient.fetchQuery({
    queryKey: qk.authStatus,
    queryFn: getAuthStatus,
  })

  return {
    ...statusQuery,
    status: statusQuery.data,
    user: statusQuery.data?.user ?? null,
    isHosted: !!statusQuery.data?.hostedMode,
    isAuthenticated: !!statusQuery.data?.authenticated,
    async login(input: LoginInput) {
      await loginRequest(input)
      return refresh()
    },
    async logout() {
      await logoutRequest()
      return refresh()
    },
    async updateProfile(input: UpdateProfileInput) {
      await updateProfileRequest(input)
      return refresh()
    },
  }
}
