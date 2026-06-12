import { useCallback } from 'react'
import { useLocation, useSearch } from 'wouter'

/**
 * usePageParam — the pager's page number as URL state (?page=), so a paged
 * position survives navigation and can be shared. Page 1 is encoded as the
 * absence of the param, matching how the tasks panel's status filter treats
 * its default. Other query params (panel, status) are preserved.
 */
export function usePageParam(): [number, (page: number) => void] {
  const rawSearch = useSearch()
  const [location, navigate] = useLocation()

  const parsed = Number(new URLSearchParams(rawSearch).get('page'))
  const page = Number.isInteger(parsed) && parsed > 1 ? parsed : 1

  const setPage = useCallback(
    (next: number) => {
      const params = new URLSearchParams(rawSearch)
      if (next > 1) params.set('page', String(next))
      else params.delete('page')
      const qs = params.toString()
      navigate(`${location}${qs ? `?${qs}` : ''}`)
    },
    [rawSearch, location, navigate],
  )

  return [page, setPage]
}
