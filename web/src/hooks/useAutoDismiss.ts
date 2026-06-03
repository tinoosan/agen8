import { useState, useEffect } from 'react'
import type { BannerState } from '../components/fields'

/**
 * useState + auto-dismiss for temporary banners.
 * Errors stay visible for 8 s, other kinds for 5 s.
 */
export function useAutoDismissBanner() {
  const [banner, setBanner] = useState<BannerState | null>(null)

  useEffect(() => {
    if (!banner) return
    const ms = banner.kind === 'error' ? 8000 : 5000
    const t = setTimeout(() => setBanner(null), ms)
    return () => clearTimeout(t)
  }, [banner])

  return [banner, setBanner] as const
}
