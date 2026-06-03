const DASHBOARD_CONTEXT_COLLAPSED_KEY = 'dashboard.context-panel-collapsed'

export function readStoredDashboardContextCollapsed(): boolean {
  try {
    return localStorage.getItem(DASHBOARD_CONTEXT_COLLAPSED_KEY) === 'true'
  } catch {
    return false
  }
}

export function writeStoredDashboardContextCollapsed(value: boolean): void {
  try {
    localStorage.setItem(DASHBOARD_CONTEXT_COLLAPSED_KEY, String(value))
  } catch {
    // ignore storage failures
  }
}
