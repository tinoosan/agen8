import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import QuestionsPanel from './QuestionsPanel'
import type { QuestionsPayload } from '../../hooks/useHumanInput'

describe('QuestionsPanel', () => {
  it('renders one question at a time and submits collected answers', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    const payload: QuestionsPayload = {
      title: 'Operator UI review',
      context: 'Review the ask_user wizard.',
      questions: [
        {
          id: 'layout',
          text: 'Which layout should this use?',
          type: 'multiple_choice',
          options: ['Compact panel', 'Full-width card'],
          recommendation: 'Compact panel',
        },
        {
          id: 'notes',
          text: 'What should change?',
          type: 'free_form',
        },
      ],
    }

    render(<QuestionsPanel payload={payload} busy={false} onSubmit={onSubmit} onCancel={vi.fn()} />)

    expect(screen.getByText('1 / 2')).toBeInTheDocument()
    expect(screen.getByText('Which layout should this use?')).toBeInTheDocument()
    expect(screen.queryByText('What should change?')).not.toBeInTheDocument()
    expect(screen.getByText('Recommended')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /compact panel/i }))
    await user.click(screen.getByRole('button', { name: /^next$/i }))

    expect(screen.getByText('2 / 2')).toBeInTheDocument()
    expect(screen.getByText('What should change?')).toBeInTheDocument()

    await user.type(screen.getByRole('textbox'), 'Keep it compact and above the composer.')
    await user.click(screen.getByRole('button', { name: /^submit$/i }))

    expect(onSubmit).toHaveBeenCalledWith([
      { questionId: 'layout', selectedOption: 'Compact panel' },
      { questionId: 'notes', freeFormText: 'Keep it compact and above the composer.' },
    ])
  })

  it('dismisses the panel through the secondary action', async () => {
    const user = userEvent.setup()
    const onCancel = vi.fn()
    render(
      <QuestionsPanel
        payload={{
          questions: [{ id: 'q1', text: 'Which option?', type: 'multiple_choice', options: ['A', 'B'] }],
        }}
        busy={false}
        onSubmit={vi.fn()}
        onCancel={onCancel}
      />,
    )

    await user.click(screen.getByRole('button', { name: /dismiss/i }))
    expect(onCancel).toHaveBeenCalledTimes(1)
  })

  it('accepts a custom free-form answer for multiple-choice questions without selecting an option', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    render(
      <QuestionsPanel
        payload={{
          questions: [{ id: 'q1', text: 'Which option?', type: 'multiple_choice', options: ['A', 'B'] }],
        }}
        busy={false}
        onSubmit={onSubmit}
        onCancel={vi.fn()}
      />,
    )

    expect(screen.getByText('Or enter a different answer')).toBeInTheDocument()
    await user.type(screen.getByRole('textbox'), 'Option C')
    await user.click(screen.getByRole('button', { name: /^submit$/i }))

    expect(onSubmit).toHaveBeenCalledWith([
      { questionId: 'q1', freeFormText: 'Option C' },
    ])
  })

  it('highlights blocking questions in the composer panel', () => {
    render(
      <QuestionsPanel
        payload={{
          title: 'Execution gate',
          questions: [{
            id: 'q1',
            text: 'Can dependent workstreams start?',
            type: 'multiple_choice',
            options: ['Yes', 'No'],
            blocking: true,
          }],
        }}
        busy={false}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
      />,
    )

    expect(screen.getByText('Blocking')).toBeInTheDocument()
    expect(screen.getByText(/Answer required before dependent work can continue/i)).toBeInTheDocument()
  })
})
