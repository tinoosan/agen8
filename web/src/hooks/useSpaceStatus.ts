import { useQuery } from '@tanstack/react-query'
import { rpcCall } from '../lib/rpc'
import type {
  Space,
  SpaceGetManifestResult,
  SpaceGetRosterResult,
  SpaceGetStatusResult,
  SpaceMember,
  SpaceMemberListResult,
  PlanMode,
} from '../lib/types'

interface SpaceGetResult {
  space: Space
}

function memberLabel(member: SpaceMember): string {
  return member.displayName || member.memberType || member.id
}

function isCoordinator(member: SpaceMember): boolean {
  return member.memberType === 'coordinator'
}

function planMode(value: string | undefined): PlanMode | undefined {
  return value === 'autonomous' || value === 'supervised' ? value : undefined
}

function toRosterEntry(member: SpaceMember) {
  return {
    spaceId: member.spaceId,
    memberLabel: memberLabel(member),
    runId: member.currentRunId ?? member.id,
    model: member.model,
    runtimeKind: member.harnessKind,
    activeReasoningEffort: member.effort,
    workerPresent: member.lifecycleState === 'active',
    effectiveStatus: member.lifecycleState,
    runTotalTokens: 0,
    runTotalCostUSD: 0,
    lifecyclePhase: member.lifecycleState,
  }
}

async function loadSpace(spaceId: string): Promise<Space> {
  const res = await rpcCall<SpaceGetResult>('space.get', { spaceId })
  return res.space
}

async function loadMembers(spaceId: string): Promise<SpaceMember[]> {
  const res = await rpcCall<SpaceMemberListResult>('space.member.list', { spaceId })
  return (res.members ?? []).filter(member => (member.lifecycleState ?? '').toLowerCase() !== 'removed')
}

export function useSpaceStatus(spaceId: string | null) {
  return useQuery<SpaceGetStatusResult>({
    queryKey: ['space.status', spaceId],
    queryFn: async () => {
      if (!spaceId) throw new Error('spaceId is required')
      const [space, members] = await Promise.all([loadSpace(spaceId), loadMembers(spaceId)])
      const memberStatuses = members.map((member) => ({
        memberLabel: memberLabel(member),
        info: member.lifecycleState,
      }))
      const runStatuses = members
        .filter((member) => member.currentRunId)
        .map((member) => ({
          runId: member.currentRunId!,
          memberLabel: memberLabel(member),
          info: member.lifecycleState,
        }))
      const memberLabelByRunId = Object.fromEntries(
        members
          .filter((member) => member.currentRunId)
          .map((member) => [member.currentRunId!, memberLabel(member)]),
      )
      return {
        pending: members.filter((member) => member.lifecycleState === 'pending').length,
        active: members.filter((member) => member.lifecycleState === 'active').length,
        done: members.filter((member) => member.lifecycleState === 'closed').length,
        members: memberStatuses,
        runs: runStatuses,
        runIds: runStatuses.map((run) => run.runId),
        memberLabelByRunId,
        totalTokensIn: space.inputTokens ?? 0,
        totalTokensOut: space.outputTokens ?? 0,
        totalTokens: space.totalTokens ?? 0,
        totalCostUSD: space.costUSD ?? 0,
        pricingKnown: (space.costUSD ?? 0) > 0,
      }
    },
    enabled: !!spaceId,
    refetchInterval: 5000,
    retry: false,
  })
}

export function useSpaceManifest(spaceId: string | null) {
  return useQuery<SpaceGetManifestResult>({
    queryKey: ['space.manifest', spaceId],
    queryFn: async () => {
      if (!spaceId) throw new Error('spaceId is required')
      const [space, members] = await Promise.all([loadSpace(spaceId), loadMembers(spaceId)])
      const coordinator = members.find(isCoordinator) ?? members[0]
      const result: SpaceGetManifestResult = {
        spaceId,
        spaceDescription: space.title,
        spaceModel: coordinator?.model,
        planMode: planMode(space.planMode),
        supervisedBlockedTools: [],
        coordinatorMember: coordinator ? memberLabel(coordinator) : '',
        coordinatorRunId: coordinator?.currentRunId ?? '',
        members: members.map((member) => ({
          memberLabel: memberLabel(member),
          runId: member.currentRunId ?? member.id,
          runtimeKind: member.harnessKind,
          allowedTools: [],
        })),
        createdAt: space.createdAt ?? '',
      }
      return result
    },
    enabled: !!spaceId,
    refetchInterval: 15000,
    retry: false,
  })
}

export function useSpaceRoster(spaceId: string | null) {
  return useQuery<SpaceGetRosterResult>({
    queryKey: ['space.roster', spaceId],
    queryFn: async () => {
      if (!spaceId) throw new Error('spaceId is required')
      const members = await loadMembers(spaceId)
      return {
        spaceId,
        members: members.map(toRosterEntry),
      }
    },
    enabled: !!spaceId,
    refetchInterval: 5000,
    retry: false,
  })
}
