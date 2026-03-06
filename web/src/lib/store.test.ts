import { describe, it, expect, beforeEach } from 'vitest'
import { useStore } from './store'

describe('useStore', () => {
  beforeEach(() => {
    // Reset store to initial state
    useStore.setState({
      focusedTeamId: null,
      mailOpen: false,
      artifactsOpen: false,
      paletteOpen: false,
    })
  })

  it('has correct initial state', () => {
    const state = useStore.getState()
    expect(state.focusedTeamId).toBeNull()
    expect(state.mailOpen).toBe(false)
    expect(state.artifactsOpen).toBe(false)
    expect(state.paletteOpen).toBe(false)
  })

  it('sets focused team ID', () => {
    useStore.getState().setFocusedTeamId('team-1')
    expect(useStore.getState().focusedTeamId).toBe('team-1')
  })

  it('clears focused team ID', () => {
    useStore.getState().setFocusedTeamId('team-1')
    useStore.getState().setFocusedTeamId(null)
    expect(useStore.getState().focusedTeamId).toBeNull()
  })

  it('closes mail and artifacts when focusing a team', () => {
    useStore.getState().setMailOpen(true)
    useStore.getState().setArtifactsOpen(true)
    expect(useStore.getState().mailOpen).toBe(true)
    expect(useStore.getState().artifactsOpen).toBe(true)

    useStore.getState().setFocusedTeamId('team-1')
    expect(useStore.getState().mailOpen).toBe(false)
    expect(useStore.getState().artifactsOpen).toBe(false)
  })

  it('toggles mail open state', () => {
    expect(useStore.getState().mailOpen).toBe(false)
    useStore.getState().setMailOpen(true)
    expect(useStore.getState().mailOpen).toBe(true)
    useStore.getState().setMailOpen(false)
    expect(useStore.getState().mailOpen).toBe(false)
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

  it('state changes are independent (mail does not affect artifacts)', () => {
    useStore.getState().setMailOpen(true)
    expect(useStore.getState().artifactsOpen).toBe(false)
    expect(useStore.getState().paletteOpen).toBe(false)
  })
})
