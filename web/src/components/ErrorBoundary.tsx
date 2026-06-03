import { Component, type ErrorInfo, type ReactNode } from 'react'
import { AlertTriangle } from 'lucide-react'
import { shouldReloadAfterImportError } from '../lib/lazyWithRetry'

/* ── Core ErrorBoundary (class component) ────────── */

interface ErrorBoundaryProps {
  children: ReactNode
  fallback?: ReactNode
  title?: string
  onError?: (error: Error, errorInfo: ErrorInfo) => void
}

interface ErrorBoundaryState {
  hasError: boolean
  error: Error | null
}

export class ErrorBoundary extends Component<ErrorBoundaryProps, ErrorBoundaryState> {
  state: ErrorBoundaryState = { hasError: false, error: null }

  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return { hasError: true, error }
  }

  componentDidCatch(error: Error, errorInfo: ErrorInfo) {
    const path = typeof window !== 'undefined' ? window.location.pathname : ''
    const reloadKey = `error-boundary:${path || 'unknown'}`
    if (shouldReloadAfterImportError(reloadKey, error)) {
      window.location.reload()
      return
    }
    if (typeof console !== 'undefined') {
      console.error('Agen8 render error', error, errorInfo)
    }
    this.props.onError?.(error, errorInfo)
  }

  reset = () => {
    this.setState({ hasError: false, error: null })
  }

  render() {
    if (this.state.hasError) {
      if (this.props.fallback) return this.props.fallback
      return (
        <div className="flex flex-col items-center justify-center gap-3 p-8 text-center">
          <div className="w-10 h-10 rounded-[var(--r-lg)] bg-[color-mix(in_srgb,var(--red)_10%,transparent)] border border-[color-mix(in_srgb,var(--red)_20%,transparent)] flex items-center justify-center">
            <AlertTriangle size={18} className="text-[var(--red)]" />
          </div>
          <div>
            <div className="text-sm font-semibold text-[var(--text-1)] mb-1">
              {this.props.title || 'Something went wrong'}
            </div>
            <div className="text-xs text-[var(--text-3)] max-w-[320px]">
              {this.state.error?.message || 'An unexpected error occurred'}
            </div>
          </div>
          <button
            onClick={this.reset}
            className="mt-1 px-3 py-1.5 text-xs font-medium rounded-[var(--r-md)] bg-[var(--bg-elevated)] border border-[var(--border)] text-[var(--text-2)] cursor-pointer hover:bg-[var(--bg-active)] transition-colors font-[inherit]"
          >
            Try again
          </button>
        </div>
      )
    }
    return this.props.children
  }
}

/* ── PageErrorBoundary ───────────────────────────── */

export function PageErrorBoundary({ children }: { children: ReactNode }) {
  return (
    <ErrorBoundary title="This page crashed">
      {children}
    </ErrorBoundary>
  )
}

/* ── SectionErrorBoundary ────────────────────────── */

export function SectionErrorBoundary({ children }: { children: ReactNode }) {
  return (
    <ErrorBoundary
      fallback={
        <div className="flex items-center gap-2.5 px-4 py-3 bg-[color-mix(in_srgb,var(--red)_6%,transparent)] border border-[color-mix(in_srgb,var(--red)_15%,transparent)] rounded-[var(--r-lg)] text-xs">
          <AlertTriangle size={14} className="text-[var(--red)] shrink-0" />
          <span className="text-[var(--text-2)]">This section failed to load.</span>
          <button
            onClick={() => window.location.reload()}
            className="ml-auto px-2 py-1 text-[11px] font-medium rounded-[var(--r-sm)] bg-[var(--bg-elevated)] border border-[var(--border)] text-[var(--text-2)] cursor-pointer hover:bg-[var(--bg-active)] transition-colors font-[inherit]"
          >
            Try again
          </button>
        </div>
      }
    >
      {children}
    </ErrorBoundary>
  )
}

export default ErrorBoundary
