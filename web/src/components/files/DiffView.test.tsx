import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import DiffView from './DiffView'

describe('DiffView', () => {
  it('marks added and removed lines and counts them in the header', () => {
    render(
      <DiffView
        baseline={'keep\nold line\nkeep too\n'}
        current={'keep\nnew line\nkeep too\nappended\n'}
      />,
    )
    expect(screen.getByTestId('diff-view')).toBeInTheDocument()
    expect(screen.getByText('old line')).toBeInTheDocument()
    expect(screen.getByText('new line')).toBeInTheDocument()
    expect(screen.getByText('appended')).toBeInTheDocument()
    expect(screen.getByText('+2')).toBeInTheDocument()
    expect(screen.getByText('−1')).toBeInTheDocument()
  })

  it('shows an explicit no-changes state for identical content', () => {
    render(<DiffView baseline={'same\ncontent\n'} current={'same\ncontent\n'} />)
    expect(screen.getByText(/No uncommitted changes/)).toBeInTheDocument()
    expect(screen.queryByTestId('diff-view')).not.toBeInTheDocument()
  })
})
