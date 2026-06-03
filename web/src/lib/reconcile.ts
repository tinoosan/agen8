import type { ProjectMemberLifecycleSummary, ProjectSpaceSummary, SpaceRecoveryDiagnostic } from './types'

type ReconcileTarget = Pick<
  ProjectSpaceSummary,
  'reconcileStatus' | 'reconcileReason' | 'lifecyclePhase' | 'lifecycleReason' | 'lifecycleMessage'
> & {
  member?: string
}

function normalizeText(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

function toMemberEntry(member: string, value: unknown): ProjectMemberLifecycleSummary | null {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return null
  const record = value as Record<string, unknown>
  const resolvedMember = normalizeText(record.member) || member
  if (!resolvedMember) return null
  return {
    member: resolvedMember,
    spaceId: normalizeText(record.spaceId) || undefined,
    status: normalizeText(record.status) || undefined,
    reconcileStatus: normalizeText(record.reconcileStatus) || undefined,
    reconcileReason: normalizeText(record.reconcileReason) || undefined,
    lifecyclePhase: normalizeText(record.lifecyclePhase) || undefined,
    lifecycleReason: normalizeText(record.lifecycleReason) || undefined,
    lifecycleMessage: normalizeText(record.lifecycleMessage) || undefined,
    managedBy: normalizeText(record.managedBy) || undefined,
    updatedAt: normalizeText(record.updatedAt) || undefined,
  }
}

function appendMemberEntries(entries: ProjectMemberLifecycleSummary[], source: unknown): void {
  if (!source) return
  if (Array.isArray(source)) {
    for (const entry of source) {
      const memberEntry = toMemberEntry('', entry)
      if (memberEntry) entries.push(memberEntry)
    }
    return
  }
  if (typeof source !== 'object') return
  for (const [member, value] of Object.entries(source as Record<string, unknown>)) {
    const memberEntry = toMemberEntry(member, value)
    if (memberEntry) entries.push(memberEntry)
  }
}

function getMemberLifecycleEntries(space: ProjectSpaceSummary): ProjectMemberLifecycleSummary[] {
  const entries: ProjectMemberLifecycleSummary[] = []
  appendMemberEntries(entries, space.members)
  appendMemberEntries(entries, space.memberLifecycles)
  appendMemberEntries(entries, space.memberLifecycle)
  appendMemberEntries(entries, space.lifecycleByMember)
  appendMemberEntries(entries, space.reconcileByMember)
  appendMemberEntries(entries, space.metadata?.members)
  appendMemberEntries(entries, space.metadata?.memberLifecycles)
  appendMemberEntries(entries, space.metadata?.memberLifecycle)
  appendMemberEntries(entries, space.metadata?.lifecycleByMember)
  appendMemberEntries(entries, space.metadata?.reconcileByMember)
  return entries
}

function effectiveState(target: ReconcileTarget): string {
  return normalizeText(target.lifecyclePhase).toLowerCase() || normalizeText(target.reconcileStatus).toLowerCase()
}

function stateSeverity(target: ReconcileTarget): number {
  switch (effectiveState(target)) {
    case 'failed':
      return 60
    case 'degraded':
    case 'drifting':
      return 50
    case 'deleting':
      return 40
    case 'progressing':
    case 'reconciling':
      return 30
    case 'stopped':
      return 20
    case 'ready':
    case 'converged':
      return 10
    default:
      return 0
  }
}

function hasSpecificMessage(target: ReconcileTarget): boolean {
  return Boolean(
    normalizeText(target.lifecycleMessage) ||
    normalizeText(target.lifecycleReason) ||
    normalizeText(target.reconcileReason),
  )
}

function pickMemberDerivedTarget(space: ProjectSpaceSummary): ReconcileTarget | null {
  const memberEntries = getMemberLifecycleEntries(space)
  if (memberEntries.length === 0) return null

  let best: ProjectMemberLifecycleSummary | null = null
  let bestSeverity = -1
  for (const entry of memberEntries) {
    const severity = stateSeverity(entry)
    if (severity > bestSeverity) {
      best = entry
      bestSeverity = severity
      continue
    }
    if (severity === bestSeverity && !best?.lifecycleMessage && entry.lifecycleMessage) {
      best = entry
    }
  }
  if (!best) return null

  const spaceSeverity = stateSeverity(space)
  if (bestSeverity === 0) return null
  // If the space is already converged/ready, ignore low-severity member churn
  // (progressing/reconciling) to avoid stale "Syncing" badges.
  if (spaceSeverity <= 10 && bestSeverity < 50) return null
  if (bestSeverity > spaceSeverity) return best
  if (bestSeverity === spaceSeverity && hasSpecificMessage(best) && !hasSpecificMessage(space)) return best
  if (spaceSeverity === 0) return best
  return null
}

function getEffectiveTarget(space: ProjectSpaceSummary): ReconcileTarget {
  return pickMemberDerivedTarget(space) ?? space
}

function getDiagnostic(space: ProjectSpaceSummary): SpaceRecoveryDiagnostic | null {
  return space.diagnostic ?? null
}

function preferredDiagnostic(space: ProjectSpaceSummary): SpaceRecoveryDiagnostic | null {
  const diagnostic = getDiagnostic(space)
  if (!diagnostic) return null
  const effective = effectiveState(getEffectiveTarget(space))
  if (effective === 'ready' || effective === 'converged' || effective === 'stopped') {
    return null
  }
  return diagnostic
}

function formatReason(target: ReconcileTarget): string {
  const phase = normalizeText(target.lifecyclePhase).toLowerCase()
  const reasonCode = normalizeText(target.lifecycleReason)
  const lifecycleMessage = normalizeText(target.lifecycleMessage)
  if (lifecycleMessage) return lifecycleMessage

  const status = normalizeText(target.reconcileStatus).toLowerCase()
  const reason = normalizeText(target.reconcileReason).toLowerCase()

  if (reasonCode === 'SpaceMissing') return 'This space is configured for the project, but it has not been created yet.'
  if (reasonCode === 'SpaceDisabled') return phase === 'progressing'
    ? 'Agen8 is stopping this space because it is turned off in the project configuration.'
    : 'This space is turned off in the project configuration.'
  if (reasonCode === 'RuntimeUnavailable') return 'Some members are unavailable. Agen8 is recovering the space while keeping its identity and history.'
  if (reasonCode === 'DefinitionChanged') return 'This space definition changed. Agen8 is refreshing the space while preserving the same space identity.'
  if (reasonCode === 'MarkedForDeletion') return 'This space is no longer in the project configuration and will be deleted.'
  if (reasonCode === 'DeleteFailed') return 'Agen8 could not finish deleting this space.'
  if (reasonCode === 'RefreshFailed') return 'Agen8 could not finish refreshing this space.'
  if (reasonCode === 'RecoveryFailed') return 'Agen8 could not recover this space after a runtime failure.'

  if (reason.includes('desired space has stale runtime state')) {
    return 'Some members did not reconnect cleanly after restart. Agen8 is trying to bring the space back to a healthy state.'
  }
  if (reason.includes('desired space is not running')) {
    return 'This space is configured to be available, but it is not currently running.'
  }
  if (reason.includes('desired space is paused')) {
    return 'This space is paused, so its members are not currently available.'
  }
  if (reason.includes('desired space is missing')) {
    return 'This space is configured for the project, but it has not been started yet.'
  }
  if (reason.includes('desired space definition changed')) {
    return 'This space definition changed, so Agen8 is refreshing the space to apply the update.'
  }
  if (reason.includes('managed space is absent from desired state')) {
    return 'This space is no longer part of the project setup and will be removed.'
  }
  if (reason.includes('managed space is disabled in desired state')) {
    return 'This space is currently turned off in the project configuration.'
  }

  if (status === 'converged') return 'This space matches the current project configuration.'
  if (status === 'reconciling') return 'Agen8 is applying the latest project configuration.'
  if (status === 'failed') return 'Agen8 could not finish updating this space. Check the daemon logs for the underlying error.'
  if (status === 'drifting') return 'This space no longer matches the current project configuration.'
  if (phase === 'stopped') return 'This space is stopped.'
  if (phase === 'deleting') return 'This space is being deleted.'
  if (phase === 'progressing') return 'Agen8 is applying the latest project configuration.'
  if (phase === 'degraded') return 'This space needs attention.'
  return normalizeText(target.reconcileReason)
}

function withMemberPrefix(target: ReconcileTarget, reason: string): string {
  const member = normalizeText(target.member)
  if (!member || !reason) return reason
  return `${member}: ${reason}`
}

export function getProductReconcileBadge(space: ProjectSpaceSummary): { label: string; color: string; bg: string; border: string; tooltip: string } | null {
  const diagnostic = preferredDiagnostic(space)
  if (diagnostic?.severity === 'error') {
    return { label: 'Needs attention', color: 'var(--red)', bg: 'rgba(239,68,68,0.12)', border: 'rgba(239,68,68,0.26)', tooltip: 'This space has an issue that may need manual intervention.' }
  }
  if (diagnostic?.severity === 'warning') {
    const label = normalizeText(diagnostic.reasonCode) === 'CoordinatorPointerDrift'
      ? 'Switching over'
      : 'Recovering'
    const tooltip = label === 'Switching over'
      ? 'The space coordinator is being moved to a new member.'
      : 'The space is recovering from an unexpected issue.'
    return { label, color: 'var(--amber)', bg: 'rgba(245,158,11,0.12)', border: 'rgba(245,158,11,0.26)', tooltip }
  }
  const effective = effectiveState(getEffectiveTarget(space))
  if (!effective) return null
  if (effective === 'ready' || effective === 'converged') {
    // Hide badge when converged – no action needed from the user
    return null
  }
  if (effective === 'stopped') {
    return { label: 'Stopped', color: 'var(--text-2)', bg: 'rgba(148,163,184,0.12)', border: 'rgba(148,163,184,0.24)', tooltip: 'This space is stopped and not processing any work.' }
  }
  if (effective === 'progressing' || effective === 'reconciling') {
    return { label: 'Syncing', color: 'var(--amber)', bg: 'rgba(245,158,11,0.12)', border: 'rgba(245,158,11,0.26)', tooltip: 'Applying the latest project configuration to this space.' }
  }
  if (effective === 'deleting') {
    return { label: 'Deleting', color: 'var(--red)', bg: 'rgba(239,68,68,0.12)', border: 'rgba(239,68,68,0.26)', tooltip: 'This space is being removed from the project.' }
  }
  if (effective === 'failed') {
    return { label: 'Needs setup', color: 'var(--red)', bg: 'rgba(239,68,68,0.12)', border: 'rgba(239,68,68,0.26)', tooltip: 'This space could not be set up. Check configuration and logs.' }
  }
  if (effective === 'degraded' || effective === 'drifting') {
    return { label: 'Out of sync', color: 'var(--red)', bg: 'rgba(239,68,68,0.12)', border: 'rgba(239,68,68,0.26)', tooltip: 'This space no longer matches the project configuration and needs to be synced.' }
  }
  return null
}

export function getProductReconcileReason(space: ProjectSpaceSummary): string {
  const diagnostic = preferredDiagnostic(space)
  if (diagnostic) {
    return normalizeText(diagnostic.summary) || normalizeText(diagnostic.detail)
  }
  const effectiveTarget = getEffectiveTarget(space)
  return withMemberPrefix(effectiveTarget, formatReason(effectiveTarget))
}

export function getProductReconcileDetail(space: ProjectSpaceSummary): string {
  const diagnostic = preferredDiagnostic(space)
  if (diagnostic) {
    return normalizeText(diagnostic.detail) || normalizeText(diagnostic.summary)
  }
  const effectiveTarget = getEffectiveTarget(space)
  if (effectiveState(effectiveTarget) === 'ready' || effectiveState(effectiveTarget) === 'converged') {
    return ''
  }
  return withMemberPrefix(effectiveTarget, formatReason(effectiveTarget))
}
