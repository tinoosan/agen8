import { useEffect, useCallback } from 'react'

interface ShortcutActions {
  onNext?: () => void        // J
  onPrev?: () => void        // K
  onOpen?: () => void        // Enter
  onClose?: () => void       // Escape
  onApprove?: () => void     // A
  onReject?: () => void      // R
  onToggleFocus?: () => void // F
  onComplete?: () => void    // C
  onAddNote?: () => void     // N
}

export function useKeyboardShortcuts(actions: ShortcutActions, enabled = true) {
  const handler = useCallback((e: KeyboardEvent) => {
    // Don't capture when typing in inputs
    const target = e.target as HTMLElement
    if (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.tagName === 'SELECT' || target.isContentEditable) return

    switch (e.key.toLowerCase()) {
      case 'j': actions.onNext?.(); break
      case 'k': actions.onPrev?.(); break
      case 'enter': actions.onOpen?.(); break
      case 'escape': actions.onClose?.(); break
      case 'a': actions.onApprove?.(); break
      case 'r': actions.onReject?.(); break
      case 'f': actions.onToggleFocus?.(); break
      case 'c': actions.onComplete?.(); break
      case 'n': actions.onAddNote?.(); break
    }
  }, [actions])

  useEffect(() => {
    if (!enabled) return
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [handler, enabled])
}
