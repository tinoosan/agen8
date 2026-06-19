import { ReactFlowProvider, type NodeProps } from '@xyflow/react'
import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { TaskNode, type TaskNodeData } from './TaskNode'

function renderTaskNode(data: TaskNodeData) {
  const props = {
    id: `task:${data.task.id}`,
    data,
    selected: false,
  } as NodeProps

  return render(
    <ReactFlowProvider>
      <TaskNode {...props} />
    </ReactFlowProvider>,
  )
}

describe('TaskNode mission-direct marker', () => {
  it('marks a task linked directly to a mission', () => {
    renderTaskNode({
      task: {
        id: 'task-m',
        description: 'Mission task',
        title: 'Mission task',
        status: 'pending',
        missionRef: 'mission-1',
      },
      isMissionDirect: true,
      clusterColor: 'rgb(50, 120, 220)',
    })

    expect(screen.getByLabelText('Linked directly to mission')).toBeInTheDocument()
    expect(screen.getByTestId('mission-direct-task-marker')).toBeInTheDocument()
  })

  it('leaves KR-linked tasks visually unchanged', () => {
    renderTaskNode({
      task: {
        id: 'task-k',
        description: 'KR task',
        title: 'KR task',
        status: 'pending',
        keyResultRef: 'kr-1',
      },
      isMissionDirect: false,
      clusterColor: 'rgb(50, 120, 220)',
    })

    expect(screen.queryByLabelText('Linked directly to mission')).not.toBeInTheDocument()
    expect(screen.queryByTestId('mission-direct-task-marker')).not.toBeInTheDocument()
  })
})
