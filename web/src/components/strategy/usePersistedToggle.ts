import { useState } from 'react'

export function usePersistedToggle(key: string, defaultOpen: boolean): [boolean, () => void] {
  const [open, setOpen] = useState(() => {
    const s = localStorage.getItem(key)
    return s !== null ? s === 'true' : defaultOpen
  })
  const toggle = () => {
    setOpen(v => {
      const next = !v
      localStorage.setItem(key, String(next))
      return next
    })
  }
  return [open, toggle]
}

