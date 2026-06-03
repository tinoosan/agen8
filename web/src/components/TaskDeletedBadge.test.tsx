import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { TaskDeletedBadge } from './TaskDeletedBadge'
import { taskRefDisplay } from '../lib/taskRef'

describe('TaskDeletedBadge', () => {
  it('renders "[Task deleted]" text', () => {
    render(<TaskDeletedBadge />)
    expect(screen.getByText('[Task deleted]')).toBeInTheDocument()
  })

  it('applies italic styling', () => {
    render(<TaskDeletedBadge />)
    const el = screen.getByText('[Task deleted]')
    expect(el).toHaveClass('italic')
  })
})

describe('taskRefDisplay', () => {
  it('returns the task title when task exists', () => {
    const result = taskRefDisplay('task-123', { id: 'task-123', title: 'Build widget' })
    expect(result).toEqual({ text: 'Build widget', isDeleted: false })
  })

  it('returns deleted indicator when task is null', () => {
    const result = taskRefDisplay('task-123', null)
    expect(result).toEqual({ text: '[Task deleted]', isDeleted: true })
  })

  it('returns deleted indicator when task is undefined', () => {
    const result = taskRefDisplay('task-123', undefined)
    expect(result).toEqual({ text: '[Task deleted]', isDeleted: true })
  })

  it('returns null when taskRef is null (no task reference)', () => {
    const result = taskRefDisplay(null, undefined)
    expect(result).toBeNull()
  })

  it('returns null when taskRef is undefined', () => {
    const result = taskRefDisplay(undefined, undefined)
    expect(result).toBeNull()
  })

  it('returns deleted indicator when taskRef exists but task has no title', () => {
    const result = taskRefDisplay('task-123', { id: 'task-123' })
    expect(result).toEqual({ text: 'task-123', isDeleted: false })
  })
})
