export type SpaceActionDialogKind =
  | 'disable'
  | 'enable'
  | 'delete-desired'
  | 'delete-runtime'
  | 'stop'
  | 'clear-history'
  | 'close-chat'

export interface SpaceActionDialogContent {
  title: string
  message: string
  confirmLabel: string
  tone: 'default' | 'danger'
}

export function getSpaceActionDialogContent(kind: SpaceActionDialogKind, spaceName: string): SpaceActionDialogContent {
  const name = spaceName.trim() || 'this space'
  switch (kind) {
    case 'disable':
      return {
        title: 'Turn Off Space',
        message: `Turn off "${name}"? It will stay in this project, but it will stop running until you turn it back on.`,
        confirmLabel: 'Turn off',
        tone: 'default',
      }
    case 'enable':
      return {
        title: 'Turn On Space',
        message: `Turn on "${name}"? Agen8 will bring it back online for this project.`,
        confirmLabel: 'Turn on',
        tone: 'default',
      }
    case 'delete-desired':
      return {
        title: 'Remove Space',
        message: `Remove "${name}" from this project? It will stop running and be removed from the project setup.`,
        confirmLabel: 'Remove',
        tone: 'danger',
      }
    case 'delete-runtime':
      return {
        title: 'Delete Space',
        message: `Delete "${name}"? This will permanently remove this space and its current setup.`,
        confirmLabel: 'Delete',
        tone: 'danger',
      }
    case 'stop':
      return {
        title: 'Stop Space',
        message: `Stop "${name}" now? It will stop running until you start it again.`,
        confirmLabel: 'Stop',
        tone: 'danger',
      }
    case 'clear-history':
      return {
        title: 'Clear Space History',
        message: `Clear the conversation and task history for "${name}"? This cannot be undone.`,
        confirmLabel: 'Clear history',
        tone: 'danger',
      }
    case 'close-chat':
      return {
        title: 'Close Chat',
        message: `Close this chat for "${name}"? You can reopen past conversations from history.`,
        confirmLabel: 'Close chat',
        tone: 'default',
      }
  }
}
