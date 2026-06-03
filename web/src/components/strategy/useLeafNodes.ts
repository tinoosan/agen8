import { useMemo } from 'react'
import type { Node, Edge } from '@xyflow/react'
import { useProjectSpaces } from '../../hooks/useProjectSpaces'
import { useProjectTasks } from '../../hooks/useProjectTasks'
import { useRecentDecisions } from '../../hooks/useDecisions'
import { useStrategyMapOpActions } from '../../hooks/useOpActions'
import { useAllEscalations } from '../../hooks/useEscalations'
import type { DecisionNodeData } from './DecisionNode'
import type { TaskNodeData } from './TaskNode'
import type { OANodeData } from './OANode'
import type { EscalationNodeData } from './EscalationNode'

/**
 * Fetches leaf-level graph nodes: Decisions, Tasks, OAs, and Escalations.
 *
 * Edge routing:
 *   Decision   → KR (keyResultRef) OR Mission (missionRef)  — denormalised at write time
 *   Task       → KR (keyResultRef)                          — only tasks with a KR ref shown
 *   OA         → KR (keyResultRef)                          — denormalised at write time
 *   Escalation → KR (keyResultRef)                          — denormalised at write time
 *
 * All four entity types now have keyResultRef set directly by the backend
 * (daemon_callback_wiring.go resolves taskRef → task.KeyResultRef on save).
 * Nodes without a valid parent edge are not shown to keep the graph clean.
 */
export function useLeafNodes(projectId: string | null, _projectRoot: string | null): {
  nodes: Node[]
  edges: Edge[]
  isLoading: boolean
} {
  const spacesQuery = useProjectSpaces(projectId, { refetchInterval: 30_000, includeDeleted: true })
  const spaces = useMemo(() => spacesQuery.data ?? [], [spacesQuery.data])

  const tasksQuery = useProjectTasks(spaces)
  const tasks = useMemo(() => tasksQuery.data ?? [], [tasksQuery.data])


  // Index tasks by ID so decisions/OAs/escalations can find their parent task.
  // Only tasks with a keyResultRef are in the graph — the deepest-path rule
  // checks membership here before preferring the task node as parent.
  const taskMap = useMemo(() => {
    const m = new Map<string, (typeof tasks)[number]>()
    for (const t of tasks) m.set(t.id, t)
    return m
  }, [tasks])

  const decisionsQuery = useRecentDecisions(projectId, undefined, { refetchInterval: 30_000 })
  const decisions = useMemo(() => decisionsQuery.data ?? [], [decisionsQuery.data])

  const oaQuery = useStrategyMapOpActions(projectId, { refetchInterval: 30_000 })
  const oas = useMemo(() => oaQuery.data ?? [], [oaQuery.data])

  const escalationsQuery = useAllEscalations(projectId, { refetchInterval: 30_000 })
  const escalations = useMemo(() => escalationsQuery.data ?? [], [escalationsQuery.data])

  const isLoading =
    (spacesQuery.isLoading && !spacesQuery.data) ||
    (tasksQuery.isLoading && !tasksQuery.data) ||
    (decisionsQuery.isLoading && !decisionsQuery.data) ||
    (oaQuery.isLoading && !oaQuery.data) ||
    (escalationsQuery.isLoading && !escalationsQuery.data)


  const { nodes, edges } = useMemo(() => {
    const nodes: Node[] = []
    const edges: Edge[] = []
    const addedTaskIds = new Set<string>()

    // Adds a task to the graph on-demand. Idempotent — safe to call multiple
    // times for the same task. Returns the React Flow node ID, or undefined
    // if the task isn't in the tasks list at all.
    //
    // Tasks without a keyResultRef are still added as orphan nodes (no edge
    // to a KR parent). This is what lets agent-created escalations/decisions/
    // OAs anchor to their spawning task even when the task itself lacks the
    // denormalised keyResultRef. Without this, the whole work lineage is
    // invisible — and every write path that skips the denormalisation becomes
    // a silent "missing node" bug (see PRs #1548, #1557, #1578, #1594).
    const ensureTaskNode = (taskId: string): string | undefined => {
      const nodeId = `task:${taskId}`
      if (addedTaskIds.has(taskId)) return nodeId
      const task = taskMap.get(taskId)
      if (!task) return undefined
      const taskData: TaskNodeData = { task }
      nodes.push({
        id: nodeId,
        type: 'task',
        position: { x: 0, y: 0 },
        data: taskData,
      })
      addedTaskIds.add(taskId)
      if (task.keyResultRef) {
        edges.push({
          id: `${nodeId}-->${task.keyResultRef}`,
          source: task.keyResultRef,
          target: nodeId,
          type: 'statusEdge',
          animated: false,
          data: { status: 'open' },
        })
      }
      return nodeId
    }

    // ── Tasks ───────────────────────────────────────────────────────────────
    // Eagerly add tasks that have a keyResultRef. Tasks without one are added
    // on-demand via ensureTaskNode when referenced by a decision/OA/escalation.
    for (const task of tasks) {
      if (!task.keyResultRef) continue
      ensureTaskNode(task.id)
    }

    // Resolves a leaf's structural parent for graph attachment. Preference
    // order is chosen so the parent is guaranteed reachable from a mission
    // by the BFS cluster-meta walk — otherwise the leaf never gets a
    // clusterColor and its selection outline/edge highlighting falls back
    // to neutral gray:
    //
    //   1. Task with a keyResultRef  — deepest path, decision visually sits
    //                                  near its task, cluster via task→kr→mission
    //   2. Direct keyResultRef       — cluster via kr→mission
    //   3. Direct missionRef         — cluster directly from mission
    //   4. Orphan task (no kr/mission) — last resort, leaf will be colourless
    //
    // If the leaf has a taskRef pointing to an *orphan* task, we skip the
    // task in favour of any direct KR/mission reference. This keeps the
    // leaf in the BFS tree while still letting ensureTaskNode render the
    // orphan task visually (it just won't be the structural parent).
    const resolveLeafParent = (
      taskRef: string | undefined,
      keyResultRef: string | undefined,
      missionRef?: string,
    ): string | undefined => {
      // Prefer the task only if it has a KR — otherwise the task itself is
      // orphan and decision attached to it loses its cluster identity.
      if (taskRef) {
        const linkedTask = taskMap.get(taskRef)
        if (linkedTask?.keyResultRef) {
          return ensureTaskNode(taskRef)
        }
      }
      if (keyResultRef) return keyResultRef
      if (missionRef) return missionRef
      // Last resort: render against the orphan task (if any) so the leaf
      // isn't dropped entirely, even though it won't get a cluster colour.
      if (taskRef) return ensureTaskNode(taskRef)
      return undefined
    }

    // ── Decisions ──────────────────────────────────────────────────────────
    for (const decision of decisions) {
      const targetId = resolveLeafParent(
        decision.taskRef,
        decision.keyResultRef,
        decision.missionRef,
      )
      if (!targetId) continue

      const nodeData: DecisionNodeData = { decision }
      nodes.push({
        id: `decision:${decision.id}`,
        type: 'decision',
        position: { x: 0, y: 0 },
        data: nodeData,
      })
      edges.push({
        id: `decision:${decision.id}-->${targetId}`,
        source: targetId,
        target: `decision:${decision.id}`,
        type: 'statusEdge',
        animated: false,
        data: { status: 'open' },
      })
    }

    // ── Operator Actions ────────────────────────────────────────────────────
    // OAs are ALWAYS rendered even without a resolvable parent — the same
    // pattern escalations use (see the block below). Backend write paths
    // that forget to denormalise keyResultRef onto the OA would otherwise
    // silently drop the node from the map, producing the exact "missing OA"
    // bug the escalation-visibility comment warned about for that entity type.
    // Orphaned OAs float as amber pills so operators can still see and
    // investigate them.
    for (const oa of oas) {
      const targetId = resolveLeafParent(oa.taskRef, oa.keyResultRef)

      const nodeData: OANodeData = { oa }
      nodes.push({
        id: `oa:${oa.id}`,
        type: 'operatorAction',
        position: { x: 0, y: 0 },
        data: nodeData,
      })

      if (targetId) {
        edges.push({
          id: `oa:${oa.id}-->${targetId}`,
          source: targetId,
          target: `oa:${oa.id}`,
          type: 'statusEdge',
          animated: false,
          data: { status: oa.urgency === 'critical' ? 'at_risk' : 'open' },
        })
      }
    }

    // ── Escalations ─────────────────────────────────────────────────────────
    // Unlike the other leaf types, escalation nodes are ALWAYS rendered
    // even without a resolvable parent — orphaned escalations float as
    // amber nodes so operators can see and investigate them. Hiding them
    // has historically caused "missing escalation" bugs every time a new
    // write path failed to denormalise keyResultRef (see PRs #1548, #1557,
    // #1578, #1594). Treating the frontend filter as the single source of
    // truth for visibility eliminates that whole class.
    for (const escalation of escalations) {
      const targetId = resolveLeafParent(escalation.taskRef, escalation.keyResultRef)

      const nodeData: EscalationNodeData = { escalation }
      nodes.push({
        id: `escalation:${escalation.id}`,
        type: 'escalation',
        position: { x: 0, y: 0 },
        data: nodeData,
      })

      if (targetId) {
        edges.push({
          id: `escalation:${escalation.id}-->${targetId}`,
          source: targetId,
          target: `escalation:${escalation.id}`,
          type: 'statusEdge',
          animated: false,
          data: { status: escalation.urgency === 'critical' ? 'at_risk' : 'open' },
        })
      }
    }

    return { nodes, edges }
  }, [decisions, tasks, taskMap, oas, escalations])


  return { nodes, edges, isLoading }
}
