import { describe, expect, it } from 'vitest'
import type { Task } from './types'
import {
  normalizeTaskMembers,
  taskAssignedMemberLabel,
  taskClaimedMemberLabel,
  taskCreatedMemberLabel,
} from './taskMembers'

describe('task member display helpers', () => {
  it('normalizes MCP task member aliases into the frontend task shape', () => {
    const task = normalizeTaskMembers({
      id: 'task-1',
      description: 'Ship it',
      status: 'active',
      assignedToMemberId: 'member-worker',
      assignedToLabel: 'Backend engineer',
    } as Task & { assignedToMemberId: string })

    expect(task.assignedTo).toBe('member-worker')
    expect(taskAssignedMemberLabel(task)).toBe('Backend engineer')
  })

  it('keeps readable labels for assigned, claimed, and created actors', () => {
    const task = {
      id: 'task-1',
      description: 'Ship it',
      status: 'active',
      assignedTo: 'member-worker',
      assignedToLabel: 'Backend engineer',
      claimedByMemberId: 'member-reviewer',
      claimedByMemberLabel: 'Reviewer',
      createdBy: 'member-coordinator',
      createdByLabel: 'Coordinator',
    } satisfies Task

    expect(taskAssignedMemberLabel(task)).toBe('Backend engineer')
    expect(taskClaimedMemberLabel(task)).toBe('Reviewer')
    expect(taskCreatedMemberLabel(task)).toBe('Coordinator')
  })
})
