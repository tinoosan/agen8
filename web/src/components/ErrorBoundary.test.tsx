import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { ErrorBoundary, SectionErrorBoundary, PageErrorBoundary } from './ErrorBoundary'

// A component that always throws
function Boom(): JSX.Element {
  throw new Error('Test explosion')
}

// Suppress React error boundary console output during tests
const originalError = console.error
beforeEach(() => {
  console.error = vi.fn()
  window.sessionStorage.clear()
})
afterEach(() => {
  console.error = originalError
})

describe('ErrorBoundary', () => {
  it('renders children when no error', () => {
    render(
      <ErrorBoundary>
        <div>Safe content</div>
      </ErrorBoundary>,
    )
    expect(screen.getByText('Safe content')).toBeInTheDocument()
  })

  it('catches render errors and shows fallback UI', () => {
    render(
      <ErrorBoundary>
        <Boom />
      </ErrorBoundary>,
    )
    expect(screen.getByText('Something went wrong')).toBeInTheDocument()
    expect(screen.getByText('Test explosion')).toBeInTheDocument()
  })

  it('shows "Try again" recovery button', () => {
    render(
      <ErrorBoundary>
        <Boom />
      </ErrorBoundary>,
    )
    expect(screen.getByRole('button', { name: 'Try again' })).toBeInTheDocument()
  })

  it('resets state when "Try again" is clicked', () => {
    let shouldThrow = true
    function MaybeThrow() {
      if (shouldThrow) throw new Error('Oops')
      return <div>Recovered</div>
    }

    render(
      <ErrorBoundary>
        <MaybeThrow />
      </ErrorBoundary>,
    )

    expect(screen.getByText('Something went wrong')).toBeInTheDocument()

    shouldThrow = false
    fireEvent.click(screen.getByRole('button', { name: 'Try again' }))

    expect(screen.getByText('Recovered')).toBeInTheDocument()
  })

  it('calls onError callback when error is caught', () => {
    const onError = vi.fn()
    render(
      <ErrorBoundary onError={onError}>
        <Boom />
      </ErrorBoundary>,
    )
    expect(onError).toHaveBeenCalledTimes(1)
    expect(onError.mock.calls[0][0]).toBeInstanceOf(Error)
    expect(onError.mock.calls[0][0].message).toBe('Test explosion')
  })

  it('uses custom fallback when provided', () => {
    render(
      <ErrorBoundary fallback={<div>Custom fallback</div>}>
        <Boom />
      </ErrorBoundary>,
    )
    expect(screen.getByText('Custom fallback')).toBeInTheDocument()
  })

  it('reloads once for retryable dynamic import errors', () => {
    const onError = vi.fn()
    const reloadKey = `agen8:lazy-retry:error-boundary:${window.location.pathname}`

    function DynamicImportBoom(): JSX.Element {
      throw new Error('Failed to fetch dynamically imported module: http://localhost:5173/src/pages/StrategyMap.tsx')
    }

    render(
      <ErrorBoundary onError={onError}>
        <DynamicImportBoom />
      </ErrorBoundary>,
    )

    expect(window.sessionStorage.getItem(reloadKey)).toBe('1')
    expect(onError).not.toHaveBeenCalled()
  })
})

describe('SectionErrorBoundary', () => {
  it('renders children when no error', () => {
    render(
      <SectionErrorBoundary>
        <div>Section content</div>
      </SectionErrorBoundary>,
    )
    expect(screen.getByText('Section content')).toBeInTheDocument()
  })

  it('shows compact error message on failure', () => {
    render(
      <SectionErrorBoundary>
        <Boom />
      </SectionErrorBoundary>,
    )
    expect(screen.getByText('This section failed to load.')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Try again' })).toBeInTheDocument()
  })
})

describe('PageErrorBoundary', () => {
  it('renders children when no error', () => {
    render(
      <PageErrorBoundary>
        <div>Page content</div>
      </PageErrorBoundary>,
    )
    expect(screen.getByText('Page content')).toBeInTheDocument()
  })

  it('shows page-level crash message on failure', () => {
    render(
      <PageErrorBoundary>
        <Boom />
      </PageErrorBoundary>,
    )
    expect(screen.getByText('This page crashed')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Try again' })).toBeInTheDocument()
  })
})
