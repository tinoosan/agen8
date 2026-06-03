import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import ApproveRejectPanel from './ApproveRejectPanel'

describe('ApproveRejectPanel', () => {
  it('submits an approval decision', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()

    render(
      <ApproveRejectPanel
        payload={{ title: 'Approve plan', description: 'Review the submitted plan.' }}
        busy={false}
        onSubmit={onSubmit}
        onCancel={vi.fn()}
      />,
    )

    await user.click(screen.getByRole('button', { name: /^approve$/i }))
    await user.click(screen.getByRole('button', { name: /^submit$/i }))

    expect(onSubmit).toHaveBeenCalledWith({ decision: 'approve' })
  })

  it('submits a rejection with an optional note', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()

    render(
      <ApproveRejectPanel
        payload={{ title: 'Approve plan', context: 'The plan needs sign-off before task delegation.' }}
        busy={false}
        onSubmit={onSubmit}
        onCancel={vi.fn()}
      />,
    )

    await user.click(screen.getByRole('button', { name: /^reject$/i }))
    await user.type(screen.getByRole('textbox'), 'Split this into smaller phases first.')
    await user.click(screen.getByRole('button', { name: /^submit$/i }))

    expect(onSubmit).toHaveBeenCalledWith({
      decision: 'reject',
      note: 'Split this into smaller phases first.',
    })
  })

  it('dismisses through the secondary action', async () => {
    const user = userEvent.setup()
    const onCancel = vi.fn()

    render(
      <ApproveRejectPanel
        payload={{ title: 'Approve plan' }}
        busy={false}
        onSubmit={vi.fn()}
        onCancel={onCancel}
      />,
    )

    await user.click(screen.getByRole('button', { name: /dismiss/i }))
    expect(onCancel).toHaveBeenCalledTimes(1)
  })
})
