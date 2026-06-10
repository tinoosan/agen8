import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { Task } from '../../lib/types'
import { TaskArtifactsSection, fileArtifactVPath } from './TaskArtifactsSection'

function task(artifacts: string[]): Task {
  return { id: 'task-1', description: 'desc', status: 'in_review', artifacts } as Task
}

function renderSection(artifacts: string[], onOpenArtifact = vi.fn()) {
  render(<TaskArtifactsSection task={task(artifacts)} onOpenArtifact={onOpenArtifact} />)
  return onOpenArtifact
}

async function expandArtifacts() {
  await userEvent.click(screen.getByRole('button', { name: /artifacts/i }))
}

beforeEach(() => {
  localStorage.clear()
})

describe('fileArtifactVPath', () => {
  it('extracts the vpath from file: refs and rejects everything else', () => {
    expect(fileArtifactVPath('file:/project/.agen8/attachments/task-1/shot.png'))
      .toBe('/project/.agen8/attachments/task-1/shot.png')
    expect(fileArtifactVPath('commit:abc123')).toBeNull()
    expect(fileArtifactVPath('plain prose artifact')).toBeNull()
    expect(fileArtifactVPath('file:')).toBeNull()
    expect(fileArtifactVPath('file:   ')).toBeNull()
  })
})

describe('TaskArtifactsSection', () => {
  it('renders file: refs as buttons and other refs as plain text', async () => {
    renderSection([
      'file:/project/.agen8/attachments/task-1/shot.png',
      'commit:abc123',
      'shipped the thing',
    ])
    await expandArtifacts()

    expect(screen.getByRole('button', { name: 'View shot.png' })).toBeInTheDocument()
    // Non-file refs keep their plain rendering: visible, but not interactive.
    expect(screen.getByText('commit:abc123')).toBeInTheDocument()
    expect(screen.getByText('shipped the thing')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /commit:abc123/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /shipped the thing/ })).not.toBeInTheDocument()
  })

  it('renders nothing for a task without artifacts', () => {
    const { container } = render(<TaskArtifactsSection task={task([])} onOpenArtifact={vi.fn()} />)
    expect(container).toBeEmptyDOMElement()
  })

  it('reports the clicked vpath to the parent viewer host', async () => {
    const onOpen = renderSection(['file:/project/web/src/App.tsx'])
    await expandArtifacts()
    await userEvent.click(screen.getByRole('button', { name: 'View App.tsx' }))
    expect(onOpen).toHaveBeenCalledWith('/project/web/src/App.tsx')
  })
})
