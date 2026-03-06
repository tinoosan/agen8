import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { Item, UserMessageContent, AgentMessageContent, ToolExecutionContent } from '../lib/types'

// Define mock functions at module level before vi.mock calls
const mockUseConversation = vi.fn()
const mockRpcCall = vi.fn()

vi.mock('../hooks/useConversation', () => ({
  useConversation: (...args: unknown[]) => mockUseConversation(...args),
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

function renderConversation(threadId: string | null = 'thread-1') {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })

  return render(
    <QueryClientProvider client={queryClient}>
      <Conversation threadId={threadId} />
    </QueryClientProvider>,
  )
}

describe('Conversation', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // Default: empty conversation
    mockUseConversation.mockReturnValue({ data: [], isLoading: false, isSuccess: true })
  })

  it('shows loading state when threadId is null', () => {
    mockUseConversation.mockReturnValue({ data: undefined, isLoading: true })
    renderConversation(null)
    expect(screen.getByText(/loading conversation/i)).toBeInTheDocument()
  })

  it('shows empty state when there are no items', () => {
    renderConversation('thread-1')
    expect(screen.getByText(/send a message to the coordinator/i)).toBeInTheDocument()
  })

  it('renders user message bubbles', () => {
    const items = [
      makeItem({
        id: 'item-1',
        type: 'user_message',
        content: { text: 'Hello coordinator' } as UserMessageContent,
      }),
    ]
    mockUseConversation.mockReturnValue({ data: items, isLoading: false })
    renderConversation()
    expect(screen.getByText('Hello coordinator')).toBeInTheDocument()
  })

  it('renders agent message bubbles with AI avatar', () => {
    const items = [
      makeItem({
        id: 'item-1',
        type: 'agent_message',
        content: { text: 'I can help you with that' } as AgentMessageContent,
      }),
    ]
    mockUseConversation.mockReturnValue({ data: items, isLoading: false })
    renderConversation()
    expect(screen.getByText('I can help you with that')).toBeInTheDocument()
    expect(screen.getByText('AI')).toBeInTheDocument()
  })

  it('renders tool execution chips', () => {
    const items = [
      makeItem({
        id: 'item-1',
        type: 'tool_execution',
        content: { toolName: 'search', ok: true } as ToolExecutionContent,
      }),
    ]
    mockUseConversation.mockReturnValue({ data: items, isLoading: false })
    renderConversation()
    expect(screen.getByText('search')).toBeInTheDocument()
    expect(screen.getByText('ok')).toBeInTheDocument()
  })

  it('renders tool execution with error status', () => {
    const items = [
      makeItem({
        id: 'item-1',
        type: 'tool_execution',
        content: { toolName: 'write_file', ok: false } as ToolExecutionContent,
      }),
    ]
    mockUseConversation.mockReturnValue({ data: items, isLoading: false })
    renderConversation()
    expect(screen.getByText('write_file')).toBeInTheDocument()
    expect(screen.getByText('err')).toBeInTheDocument()
  })

  it('renders reasoning items', () => {
    const items = [
      makeItem({
        id: 'item-1',
        type: 'reasoning',
        content: { summary: 'Analyzing the problem' },
      }),
    ]
    mockUseConversation.mockReturnValue({ data: items, isLoading: false })
    renderConversation()
    expect(screen.getByText('Analyzing the problem')).toBeInTheDocument()
  })

  it('shows streaming cursor for partial agent messages', () => {
    const items = [
      makeItem({
        id: 'item-1',
        type: 'agent_message',
        status: 'streaming',
        content: { text: 'Working on', isPartial: true } as AgentMessageContent,
      }),
    ]
    mockUseConversation.mockReturnValue({ data: items, isLoading: false })
    renderConversation()
    expect(screen.getByText('Working on')).toBeInTheDocument()
    const cursor = document.querySelector('.streaming-cursor')
    expect(cursor).toBeInTheDocument()
  })

  it('renders multiple items in order', () => {
    const items = [
      makeItem({ id: 'item-1', type: 'user_message', content: { text: 'First message' } }),
      makeItem({ id: 'item-2', type: 'agent_message', content: { text: 'Second message' } }),
      makeItem({ id: 'item-3', type: 'user_message', content: { text: 'Third message' } }),
    ]
    mockUseConversation.mockReturnValue({ data: items, isLoading: false })
    renderConversation()

    for (const msg of ['First message', 'Second message', 'Third message']) {
      expect(screen.getByText(msg)).toBeInTheDocument()
    }
  })

  it('disables send button when input is empty', () => {
    renderConversation()
    const sendButton = screen.getByRole('button')
    expect(sendButton).toBeDisabled()
  })

  it('disables textarea when threadId is null', () => {
    mockUseConversation.mockReturnValue({ data: undefined, isLoading: true })
    renderConversation(null)
    const textarea = screen.getByPlaceholderText('Message the coordinator…')
    expect(textarea).toBeDisabled()
  })

  it('enables textarea when threadId is set', () => {
    renderConversation('thread-1')
    const textarea = screen.getByPlaceholderText('Message the coordinator…')
    expect(textarea).not.toBeDisabled()
  })

  it('sends message on Enter key press', async () => {
    const user = userEvent.setup()
    mockRpcCall.mockResolvedValueOnce({ turn: { id: 'turn-new' } })
    renderConversation()

    const textarea = screen.getByPlaceholderText('Message the coordinator…')
    await user.type(textarea, 'Hello{Enter}')

    await waitFor(() => {
      expect(mockRpcCall).toHaveBeenCalledWith('turn.create', {
        threadId: 'thread-1',
        input: { text: 'Hello' },
      })
    })
  })

  it('clears input after sending', async () => {
    const user = userEvent.setup()
    mockRpcCall.mockResolvedValueOnce({ turn: { id: 'turn-new' } })
    renderConversation()

    const textarea = screen.getByPlaceholderText('Message the coordinator…') as HTMLTextAreaElement
    await user.type(textarea, 'Hello{Enter}')

    await waitFor(() => {
      expect(textarea.value).toBe('')
    })
  })

  it('restores input on send failure', async () => {
    const user = userEvent.setup()
    mockRpcCall.mockRejectedValueOnce(new Error('Network error'))
    renderConversation()

    const textarea = screen.getByPlaceholderText('Message the coordinator…') as HTMLTextAreaElement
    await user.type(textarea, 'Hello{Enter}')

    await waitFor(() => {
      expect(textarea.value).toBe('Hello')
    })
  })

  it('does not send empty messages', async () => {
    const user = userEvent.setup()
    renderConversation()

    const textarea = screen.getByPlaceholderText('Message the coordinator…')
    await user.type(textarea, '   {Enter}')

    expect(mockRpcCall).not.toHaveBeenCalled()
  })

  it('allows Shift+Enter for newline without sending', async () => {
    const user = userEvent.setup()
    renderConversation()

    const textarea = screen.getByPlaceholderText('Message the coordinator…') as HTMLTextAreaElement
    await user.type(textarea, 'Line 1{Shift>}{Enter}{/Shift}Line 2')

    expect(mockRpcCall).not.toHaveBeenCalled()
    expect(textarea.value).toContain('Line 1')
    expect(textarea.value).toContain('Line 2')
  })

  it('skips rendering tool_execution with no content', () => {
    const items = [makeItem({ id: 'item-1', type: 'tool_execution', content: undefined })]
    mockUseConversation.mockReturnValue({ data: items, isLoading: false })
    renderConversation()
    expect(screen.queryByText('ok')).not.toBeInTheDocument()
  })

  it('skips rendering reasoning with empty text', () => {
    const items = [makeItem({ id: 'item-1', type: 'reasoning', content: { summary: '' } })]
    mockUseConversation.mockReturnValue({ data: items, isLoading: false })
    renderConversation()
    const reasoningElements = document.querySelectorAll('[style*="italic"]')
    expect(reasoningElements).toHaveLength(0)
  })
})
