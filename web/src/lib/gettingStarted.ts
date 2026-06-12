/**
 * gettingStarted — step derivation for the fresh-project onboarding checklist.
 *
 * Steps tick from queryable project state only; there is no dedicated
 * onboarding state on the backend. The first member registration is the load-
 * bearing signal: it proves the harness connected AND the skill ran (an agent
 * can only register through a connected MCP client), so the connect and skill
 * steps tick implicitly with it. There is no "token was used" signal on the
 * server (API keys are hashed, last-use is not stamped), which is why
 * registration stands in for connection.
 */

export type GettingStartedStepId = 'connect' | 'skill' | 'agent' | 'work'

export interface GettingStartedInput {
  memberCount: number
  missionCount: number
  taskCount: number
}

export interface GettingStartedState {
  done: Record<GettingStartedStepId, boolean>
  /** All steps ticked — the card hides itself. */
  complete: boolean
}

export function deriveGettingStarted(input: GettingStartedInput): GettingStartedState {
  const agent = input.memberCount > 0
  const work = input.missionCount > 0 || input.taskCount > 0
  const done: Record<GettingStartedStepId, boolean> = {
    connect: agent,
    skill: agent,
    agent,
    work,
  }
  return { done, complete: agent && work }
}

/* Dismissal is per project: a user may dismiss the card on one project and
 * still want it on the next fresh one. */
function dismissKey(projectId: string): string {
  return `dashboard.gettingStarted.dismissed:${projectId}`
}

export function readGettingStartedDismissed(projectId: string): boolean {
  try {
    return localStorage.getItem(dismissKey(projectId)) === 'true'
  } catch {
    return false
  }
}

export function writeGettingStartedDismissed(projectId: string): void {
  try {
    localStorage.setItem(dismissKey(projectId), 'true')
  } catch {
    /* noop */
  }
}
