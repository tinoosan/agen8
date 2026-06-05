import type { Task } from './types'
import { memberDisplayName } from './memberDisplay'

type TaskWithMemberAliases = Task & {
  assignedToMemberId?: string
}

export function normalizeTaskMembers(task: TaskWithMemberAliases): Task {
  return {
    ...task,
    assignedTo: task.assignedTo ?? task.assignedToMemberId,
  }
}

export function taskAssignedMemberLabel(task: Task): string | undefined {
  return memberDisplayName(task.assignedToLabel, task.assignedTo)
}

export function taskClaimedMemberLabel(task: Task): string | undefined {
  return memberDisplayName(task.claimedByMemberLabel, task.claimedByMemberId)
}

export function taskCreatedMemberLabel(task: Task): string | undefined {
  return memberDisplayName(task.createdByLabel, task.createdBy)
}
