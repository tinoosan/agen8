import type { ReactNode } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { Router } from 'wouter'
import { memoryLocation } from 'wouter/memory-location'

/**
 * Create a fresh QueryClient with defaults suitable for testing:
 * - no retries
 * - no automatic refetching
 */
export function createTestQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        staleTime: Infinity,
        refetchInterval: false,
        refetchOnWindowFocus: false,
        refetchOnMount: false,
        refetchOnReconnect: false,
      },
    },
  })
}

/**
 * A wrapper component that provides a QueryClient context for testing hooks
 * and components that use @tanstack/react-query.
 */
export function createWrapper() {
  const queryClient = createTestQueryClient()

  function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )
  }

  return { Wrapper, queryClient }
}

/**
 * A wrapper component that provides both QueryClient and wouter Router context
 * for testing components with URL-based routing.
 */
export function createRouterWrapper(path: string = '/') {
  const queryClient = createTestQueryClient()
  const { hook } = memoryLocation({ path })

  function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>
        <Router hook={hook}>
          {children}
        </Router>
      </QueryClientProvider>
    )
  }

  return { Wrapper, queryClient }
}
