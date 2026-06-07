import { describe, it, expect, beforeEach } from 'vitest'
import { useStore } from './store'

describe('useStore', () => {
  beforeEach(() => {
    // Reset store to initial state
    useStore.setState({
      artifactsOpen: false,
      strategySearchOpen: false,
    })
  })

  it('has correct initial state', () => {
    const state = useStore.getState()
    expect(state.artifactsOpen).toBe(false)
    expect(state.strategySearchOpen).toBe(false)
  })

  it('toggles artifacts open state', () => {
    expect(useStore.getState().artifactsOpen).toBe(false)
    useStore.getState().setArtifactsOpen(true)
    expect(useStore.getState().artifactsOpen).toBe(true)
    useStore.getState().setArtifactsOpen(false)
    expect(useStore.getState().artifactsOpen).toBe(false)
  })

  it('toggles strategy search open state', () => {
    expect(useStore.getState().strategySearchOpen).toBe(false)
    useStore.getState().setStrategySearchOpen(true)
    expect(useStore.getState().strategySearchOpen).toBe(true)
    useStore.getState().setStrategySearchOpen(false)
    expect(useStore.getState().strategySearchOpen).toBe(false)
  })

  it('state changes are independent (artifacts does not affect strategy search)', () => {
    useStore.getState().setArtifactsOpen(true)
    expect(useStore.getState().strategySearchOpen).toBe(false)
  })

  it('resetEphemeral clears all panel states', () => {
    useStore.getState().setArtifactsOpen(true)
    useStore.getState().setStrategySearchOpen(true)

    useStore.getState().resetEphemeral()

    const state = useStore.getState()
    expect(state.artifactsOpen).toBe(false)
    expect(state.strategySearchOpen).toBe(false)
  })
})
