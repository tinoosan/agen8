import { describe, it, expect, beforeEach } from 'vitest'
import { useStore } from './store'

describe('useStore', () => {
  beforeEach(() => {
    // Reset store to initial state
    useStore.setState({
      artifactsOpen: false,
      paletteOpen: false,
    })
  })

  it('has correct initial state', () => {
    const state = useStore.getState()
    expect(state.artifactsOpen).toBe(false)
    expect(state.paletteOpen).toBe(false)
  })

  it('toggles artifacts open state', () => {
    expect(useStore.getState().artifactsOpen).toBe(false)
    useStore.getState().setArtifactsOpen(true)
    expect(useStore.getState().artifactsOpen).toBe(true)
    useStore.getState().setArtifactsOpen(false)
    expect(useStore.getState().artifactsOpen).toBe(false)
  })

  it('toggles palette open state', () => {
    expect(useStore.getState().paletteOpen).toBe(false)
    useStore.getState().setPaletteOpen(true)
    expect(useStore.getState().paletteOpen).toBe(true)
    useStore.getState().setPaletteOpen(false)
    expect(useStore.getState().paletteOpen).toBe(false)
  })

  it('state changes are independent (artifacts does not affect palette)', () => {
    useStore.getState().setArtifactsOpen(true)
    expect(useStore.getState().paletteOpen).toBe(false)
  })

  it('focusedSpaceId starts null and can be set and cleared', () => {
    expect(useStore.getState().focusedSpaceId).toBeNull()
    useStore.getState().setFocusedSpaceId('space-abc123')
    expect(useStore.getState().focusedSpaceId).toBe('space-abc123')
    useStore.getState().setFocusedSpaceId(null)
    expect(useStore.getState().focusedSpaceId).toBeNull()
  })

  it('resetEphemeral clears all panel states', () => {
    useStore.getState().setArtifactsOpen(true)

    useStore.getState().resetEphemeral()

    const state = useStore.getState()
    expect(state.artifactsOpen).toBe(false)
  })
})
