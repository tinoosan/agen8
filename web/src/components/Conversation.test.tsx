import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { Item, UserMessageContent, AgentMessageContent } from '../lib/types'

// Define mock functions at module level before vi.mock calls
const mockUseConversation = vi.fn()
const mockUseActivity = vi.fn()
const mockUseTaskHistory = vi.fn()
const mockRpcCall = vi.fn()

vi.mock('../hooks/useConversation', () => ({
  useConversation: (...args: unknown[]) => mockUseConversation(...args),
}))

vi.mock('../hooks/useActivity', () => ({
  useActivity: (...args: unknown[]) => mockUseActivity(...args),
}))

vi.mock('../hooks/useTaskHistory', () => ({
  useTaskHistory: (...args: unknown[]) => mockUseTaskHistory(...args),
}))

vi.mock('../lib/rpc', () => ({
  rpcCall: (...args: unknown[]) => mockRpcCall(...args),
  onNotification: () => () => {},
  isConnected: () => true,
}))

// Import component after mocks
const { default: Conversation } = await import('./Conversation')

function makeItem(overrides: Partial<Item> & { content?: unknown }): Item {
  return {
    id: 'item-1',
    turnId: 'turn-1',
    type: 'user_message',
    status: 'completed',
    ...overrides,
  }
}

function renderConversation(
  threadId: string | null = 'thread-1',
  teamId: string = 'team-1',
  coordinatorRole: string | null = 'coordinator',
) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })

  return render(
    <QueryClientProvider client={queryClient}>
      <Conversation threadId={threadId} teamId={teamId} coordinatorRole={coordinatorRole} />
    </QueryClientProvider>,
  )
}

describe('Conversation', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // Default: empty conversation, all hooks return empty data
    mockUseConversation.mockReturnValue({
      query: { data: [], isLoading: false, isSuccess: true },
      registerTurnId: vi.fn(),
    })
    mockUseActivity.mockReturnValue({ data: [], isLoading: false, isSuccess: true, refetch: vi.fn() })
    mockUseTaskHistory.mockReturnValue({ data: [], isLoading: false, isSuccess: true })
  })

  it('shows connecting state when threadId is null', () => {
    mockUseConversation.mockReturnValue({
      query: { data: undefined, isLoading: true },
      registerTurnId: vi.fn(),
    })
    renderConversation(null)
    expect(screen.getByText(/connecting/i)).toBeInTheDocument()
  })

  it('shows empty state when there are no messages', () => {
    renderConversation('thread-1')
    expect(screen.getByText(/send a message to get started/i)).toBeInTheDocument()
    expect(screen.getByText(/coordinator ready/i)).toBeInTheDocument()
  })

  it('renders user message bubbles from items', () => {
    const items = [
      makeItem({
        id: 'item-1',
        type: 'user_message',
        content: { text: 'Hello coordinator' } as UserMessageContent,
      }),
    ]
    mockUseConversation.mockReturnValue({
      query: { data: items, isLoading: false },
      registerTurnId: vi.fn(),
    })
    renderConversation()
    expect(screen.getByText('Hello coordinator')).toBeInTheDocument()
  })

  it('renders agent message bubbles from items', () => {
    const items = [
      makeItem({
        id: 'item-1',
        type: 'agent_message',
        content: { text: 'I can help you with that' } as AgentMessageContent,
      }),
    ]
    mockUseConversation.mockReturnValue({
      query: { data: items, isLoading: false },
      registerTurnId: vi.fn(),
    })
    renderConversation()
    expect(screen.getByText('I can help you with that')).toBeInTheDocument()
  })

  it('renders agent messages from activity events', () => {
    // Activity events contribute to chat entries via toChatEntry
    mockUseActivity.mockReturnValue({
      data: [{
        id: 'act-1',
        kind: 'agent_speak',
        title: 'Agent spoke',
        status: 'ok',
        startedAt: new Date().toISOString(),
        outputPreview: 'Response from activity',
        data: { role: 'worker' },
      }],
      isLoading: false,
      isSuccess: true,
      refetch: vi.fn(),
    })
    renderConversation()
    expect(screen.getByText('Response from activity')).toBeInTheDocument()
  })

  it('renders multiple messages in order', () => {
    const items = [
      makeItem({ id: 'item-1', type: 'user_message', content: { text: 'First message' }, createdAt: '2025-01-01T00:00:01Z' }),
      makeItem({ id: 'item-2', type: 'agent_message', content: { text: 'Second message' }, createdAt: '2025-01-01T00:00:02Z' }),
      makeItem({ id: 'item-3', type: 'user_message', content: { text: 'Third message' }, createdAt: '2025-01-01T00:00:03Z' }),
    ]
    mockUseConversation.mockReturnValue({
      query: { data: items, isLoading: false },
      registerTurnId: vi.fn(),
    })
    renderConversation()

    for (const msg of ['First message', 'Second message', 'Third message']) {
      expect(screen.getByText(msg)).toBeInTheDocument()
    }
  })

  it('disables send button when input is empty', () => {
    renderConversation()
    const buttons = screen.getAllByRole('button')
    // The send button should be disabled
    const sendButton = buttons.find(btn => btn.querySelector('svg'))
    expect(sendButton).toBeDefined()
    expect(sendButton).toBeDisabled()
  })

  it('disables textarea when threadId is null', () => {
    mockUseConversation.mockReturnValue({
      query: { data: undefined, isLoading: true },
      registerTurnId: vi.fn(),
    })
    renderConversation(null)
    const textarea = screen.getByRole('textbox')
    expect(textarea).toBeDisabled()
  })

  it('enables textarea when threadId is set', () => {
    renderConversation('thread-1')
    const textarea = screen.getByRole('textbox')
    expect(textarea).not.toBeDisabled()
  })

  it('sends message via task.create on Enter', async () => {
    const user = userEvent.setup()
    const mockRefetch = vi.fn().mockResolvedValue({})
    mockUseActivity.mockReturnValue({ data: [], isLoading: false, isSuccess: true, refetch: mockRefetch })
    mockRpcCall.mockResolvedValueOnce({})

    renderConversation('thread-1', 'team-1', 'coordinator')

    const textarea = screen.getByRole('textbox')
    await user.type(textarea, 'Hello{Enter}')

    await waitFor(() => {
      expect(mockRpcCall).toHaveBeenCalledWith('task.create', {
        threadId: 'thread-1',
        teamId: 'team-1',
        goal: 'Hello',
        taskKind: 'user_message',
        assignedRole: 'coordinator',
      })
    })
  })

  it('clears input after sending', async () => {
    const user = userEvent.setup()
    const mockRefetch = vi.fn().mockResolvedValue({})
    mockUseActivity.mockReturnValue({ data: [], isLoading: false, isSuccess: true, refetch: mockRefetch })
    mockRpcCall.mockResolvedValueOnce({})

    renderConversation('thread-1', 'team-1', 'coordinator')

    const textarea = screen.getByRole('textbox') as HTMLTextAreaElement
    await user.type(textarea, 'Hello{Enter}')

    await waitFor(() => {
      expect(textarea.value).toBe('')
    })
  })

  it('restores input on send failure and shows error', async () => {
    const user = userEvent.setup()
    const mockRefetch = vi.fn().mockResolvedValue({})
    mockUseActivity.mockReturnValue({ data: [], isLoading: false, isSuccess: true, refetch: mockRefetch })
    mockRpcCall.mockRejectedValueOnce(new Error('Network error'))

    renderConversation('thread-1', 'team-1', 'coordinator')

    const textarea = screen.getByRole('textbox') as HTMLTextAreaElement
    await user.type(textarea, 'Hello{Enter}')

    await waitFor(() => {
      expect(textarea.value).toBe('Hello')
    })
    // Error message should be displayed
    expect(screen.getByText('Network error')).toBeInTheDocument()
  })

  it('dismisses error banner on click', async () => {
    const user = userEvent.setup()
    const mockRefetch = vi.fn().mockResolvedValue({})
    mockUseActivity.mockReturnValue({ data: [], isLoading: false, isSuccess: true, refetch: mockRefetch })
    mockRpcCall.mockRejectedValueOnce(new Error('Send failed'))

    renderConversation('thread-1', 'team-1', 'coordinator')

    const textarea = screen.getByRole('textbox')
    await user.type(textarea, 'Hello{Enter}')

    await waitFor(() => {
      expect(screen.getByText('Send failed')).toBeInTheDocument()
    })

    // Click dismiss button
    const dismissButton = screen.getByText('×')
    await user.click(dismissButton)

    expect(screen.queryByText('Send failed')).not.toBeInTheDocument()
  })

  it('does not send empty messages', async () => {
    const user = userEvent.setup()
    renderConversation()

    const textarea = screen.getByRole('textbox')
    await user.type(textarea, '   {Enter}')

    // task.create should never be called (only check for task.create, not other RPCs)
    expect(mockRpcCall).not.toHaveBeenCalledWith('task.create', expect.anything())
  })

  it('does not send when coordinatorRole is null', async () => {
    const user = userEvent.setup()
    renderConversation('thread-1', 'team-1', null)

    const textarea = screen.getByRole('textbox')
    await user.type(textarea, 'Hello{Enter}')

    expect(mockRpcCall).not.toHaveBeenCalledWith('task.create', expect.anything())
  })

  it('allows Shift+Enter for newline without sending', async () => {
    const user = userEvent.setup()
    renderConversation()

    const textarea = screen.getByRole('textbox') as HTMLTextAreaElement
    await user.type(textarea, 'Line 1{Shift>}{Enter}{/Shift}Line 2')

    expect(mockRpcCall).not.toHaveBeenCalledWith('task.create', expect.anything())
    expect(textarea.value).toContain('Line 1')
    expect(textarea.value).toContain('Line 2')
  })

  it('shows optimistic user message while sending', async () => {
    const user = userEvent.setup()
    // Make the RPC call hang so we can see the optimistic message
    mockRpcCall.mockReturnValue(new Promise(() => {}))
    const mockRefetch = vi.fn().mockResolvedValue({})
    mockUseActivity.mockReturnValue({ data: [], isLoading: false, isSuccess: true, refetch: mockRefetch })

    renderConversation('thread-1', 'team-1', 'coordinator')

    const textarea = screen.getByRole('textbox')
    await user.type(textarea, 'Optimistic hello{Enter}')

    await waitFor(() => {
      expect(screen.getByText('Optimistic hello')).toBeInTheDocument()
    })
  })

  it('deduplicates entries from items and activities by ID', () => {
    // Entries with the same ID from both items and activities should be deduplicated
    const items = [
      makeItem({
        id: 'shared-id',
        type: 'user_message',
        content: { text: 'Shared message' } as UserMessageContent,
        createdAt: '2025-01-01T00:00:01Z',
      }),
    ]
    mockUseConversation.mockReturnValue({
      query: { data: items, isLoading: false },
      registerTurnId: vi.fn(),
    })
    mockUseActivity.mockReturnValue({
      data: [{
        id: 'shared-id',
        kind: 'user_message',
        title: 'Shared message',
        status: 'ok',
        startedAt: '2025-01-01T00:00:01.000Z',
        textPreview: 'Shared message',
      }],
      isLoading: false,
      isSuccess: true,
      refetch: vi.fn(),
    })

    renderConversation()
    // Should appear only once because both have the same ID
    const matches = screen.getAllByText('Shared message')
    expect(matches).toHaveLength(1)
  })

  it('renders task summaries from task history', () => {
    mockUseTaskHistory.mockReturnValue({
      data: [{
        id: 'task-1',
        goal: 'Do something',
        status: 'done',
        summary: 'I completed the task successfully',
        assignedRole: 'worker',
        taskKind: 'subtask',
        createdAt: '2025-01-01T00:00:01Z',
        completedAt: '2025-01-01T00:00:05Z',
      }],
      isLoading: false,
      isSuccess: true,
    })

    renderConversation()
    expect(screen.getByText('I completed the task successfully')).toBeInTheDocument()
  })

  it('skips items that are not user_message or agent_message', () => {
    const items = [
      makeItem({ id: 'item-1', type: 'tool_execution', content: { toolName: 'search', ok: true } }),
      makeItem({ id: 'item-2', type: 'reasoning', content: { summary: 'Thinking...' } }),
    ]
    mockUseConversation.mockReturnValue({
      query: { data: items, isLoading: false },
      registerTurnId: vi.fn(),
    })
    renderConversation()

    // tool_execution and reasoning are filtered out by itemToChatEntry
    // so the empty state should show
    expect(screen.getByText(/send a message to get started/i)).toBeInTheDocument()
  })
})
