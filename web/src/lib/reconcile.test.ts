import { describe, expect, it } from 'vitest'

import { getProductReconcileBadge, getProductReconcileDetail, getProductReconcileReason } from './reconcile'
import type { ProjectSpaceSummary } from './types'

describe('reconcile helpers', () => {
  it('ignores stale warning diagnostics once a space is ready', () => {
    const space: ProjectSpaceSummary = {
      spaceId: 'space-market-stock',
      reconcileStatus: 'converged',
      reconcileReason: 'This space matches the current project configuration.',
      lifecyclePhase: 'ready',
      lifecycleReason: 'Ready',
      lifecycleMessage: 'This space matches the current project configuration.',
      diagnostic: {
        severity: 'warning',
        reasonCode: 'CoordinatorPointerDrift',
        summary: 'Refreshing coordinator routing',
        detail: 'Agen8 has newer coordinator members available, but the space has not switched routing over cleanly yet.',
      },
    }

    expect(getProductReconcileBadge(space)).toBeNull()
    expect(getProductReconcileReason(space)).toBe('This space matches the current project configuration.')
    expect(getProductReconcileDetail(space)).toBe('')
  })

  it('ignores stale error diagnostics once a space is stopped', () => {
    const space: ProjectSpaceSummary = {
      spaceId: 'space-market-stock',
      reconcileStatus: 'drifting',
      reconcileReason: 'This space is turned off in the project configuration.',
      lifecyclePhase: 'stopped',
      lifecycleReason: 'SpaceDisabled',
      lifecycleMessage: 'This space is turned off in the project configuration.',
      diagnostic: {
        severity: 'error',
        reasonCode: 'RecoveryAttemptFailed',
        summary: 'Recovery failed',
        detail: 'Agen8 could not recover this space after a runtime failure.',
      },
    }

    expect(getProductReconcileBadge(space)?.label).toBe('Stopped')
    expect(getProductReconcileReason(space)).toBe('This space is turned off in the project configuration.')
    expect(getProductReconcileDetail(space)).toBe('This space is turned off in the project configuration.')
  })

  it('keeps active diagnostics when the space is still progressing', () => {
    const space: ProjectSpaceSummary = {
      spaceId: 'space-market-stock',
      reconcileStatus: 'reconciling',
      reconcileReason: 'Agen8 is applying the latest project configuration.',
      lifecyclePhase: 'progressing',
      lifecycleReason: 'DefinitionChanged',
      lifecycleMessage: 'Agen8 is applying the latest project configuration.',
      diagnostic: {
        severity: 'warning',
        reasonCode: 'CoordinatorPointerDrift',
        summary: 'Refreshing coordinator routing',
        detail: 'Agen8 has newer coordinator members available, but the space has not switched routing over cleanly yet.',
      },
    }

    expect(getProductReconcileBadge(space)?.label).toBe('Switching over')
    expect(getProductReconcileReason(space)).toBe('Refreshing coordinator routing')
    expect(getProductReconcileDetail(space)).toBe(
      'Agen8 has newer coordinator members available, but the space has not switched routing over cleanly yet.',
    )
  })

  it('does not show syncing when a converged space only has low-severity role churn', () => {
    const space: ProjectSpaceSummary = {
      spaceId: 'space-engineering',
      reconcileStatus: 'converged',
      reconcileReason: 'This space matches the current project configuration.',
      lifecyclePhase: 'ready',
      lifecycleReason: 'Ready',
      lifecycleMessage: 'This space matches the current project configuration.',
      roles: [
        {
          role: 'cto',
          reconcileStatus: 'converged',
          lifecyclePhase: 'ready',
        },
        {
          role: 'backend-engineer',
          reconcileStatus: 'reconciling',
          lifecyclePhase: 'progressing',
          reconcileReason: 'role is unavailable',
        },
      ],
    }

    expect(getProductReconcileBadge(space)).toBeNull()
    expect(getProductReconcileReason(space)).toBe('This space matches the current project configuration.')
    expect(getProductReconcileDetail(space)).toBe('')
  })

  it('renders space-missing lifecycle state without requiring diagnostics', () => {
    const space: ProjectSpaceSummary = {
      spaceId: 'space-exec',
      reconcileStatus: 'reconciling',
      reconcileReason: 'desired space is missing in runtime',
      lifecyclePhase: 'progressing',
      lifecycleReason: 'SpaceMissing',
      lifecycleMessage: '',
    }

    expect(getProductReconcileBadge(space)?.label).toBe('Syncing')
    expect(getProductReconcileReason(space)).toBe(
      'This space is configured for the project, but it has not been created yet.',
    )
    expect(getProductReconcileDetail(space)).toBe(
      'This space is configured for the project, but it has not been created yet.',
    )
  })
})
