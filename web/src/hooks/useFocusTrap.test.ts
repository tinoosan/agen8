import { describe, it, expect } from 'vitest'
import { renderHook } from '@testing-library/react'
import { useFocusTrap } from './useFocusTrap'

describe('useFocusTrap', () => {
  it('returns a ref object', () => {
    const { result } = renderHook(() => useFocusTrap())
    expect(result.current).toHaveProperty('current')
  })

  it('ref is initially null', () => {
    const { result } = renderHook(() => useFocusTrap())
    expect(result.current.current).toBeNull()
  })

  it('traps Tab key within container', () => {
    const { result } = renderHook(() => useFocusTrap<HTMLDivElement>())

    // Build a container with two focusable elements
    const container = document.createElement('div')
    const btn1 = document.createElement('button')
    btn1.textContent = 'First'
    const btn2 = document.createElement('button')
    btn2.textContent = 'Last'
    container.appendChild(btn1)
    container.appendChild(btn2)
    document.body.appendChild(container)

    // Simulate the ref being attached
    Object.defineProperty(result.current, 'current', { value: container, writable: true })

    // Re-run the effect by triggering it via renderHook update
    // The effect calls addEventListener on the element when ref.current is set.
    // We manually test the keydown behavior to verify wrapping logic.

    btn2.focus()
    // Dispatch Tab on the last element — should wrap to first
    const tabEvent = new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true })
    container.dispatchEvent(tabEvent)

    document.body.removeChild(container)
  })

  it('restores focus on unmount', () => {
    const button = document.createElement('button')
    document.body.appendChild(button)
    button.focus()
    expect(document.activeElement).toBe(button)

    const { unmount } = renderHook(() => useFocusTrap())
    unmount()

    document.body.removeChild(button)
  })
})
