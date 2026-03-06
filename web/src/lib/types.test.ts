import { describe, it, expect } from 'vitest'
import {
  getItemText,
  type Item,
  type UserMessageContent,
  type AgentMessageContent,
  type ToolExecutionContent,
  type ReasoningContent,
} from './types'

function makeItem(overrides: Partial<Item> & { content?: unknown }): Item {
  return {
    id: 'item-1',
    turnId: 'turn-1',
    type: 'user_message',
    status: 'completed',
    ...overrides,
  }
}

describe('getItemText', () => {
  it('returns empty string when content is undefined', () => {
    const item = makeItem({ content: undefined })
    expect(getItemText(item)).toBe('')
  })

  it('extracts text from user_message content', () => {
    const content: UserMessageContent = { text: 'Hello world' }
    const item = makeItem({ type: 'user_message', content })
    expect(getItemText(item)).toBe('Hello world')
  })

  it('extracts text from agent_message content', () => {
    const content: AgentMessageContent = { text: 'Here is my response' }
    const item = makeItem({ type: 'agent_message', content })
    expect(getItemText(item)).toBe('Here is my response')
  })

  it('extracts summary from reasoning content', () => {
    const content: ReasoningContent = { summary: 'Thinking about the problem...', step: 1 }
    const item = makeItem({ type: 'reasoning', content })
    expect(getItemText(item)).toBe('Thinking about the problem...')
  })

  it('extracts toolName from tool_execution content', () => {
    const content: ToolExecutionContent = { toolName: 'search', input: { q: 'test' }, ok: true }
    const item = makeItem({ type: 'tool_execution', content })
    expect(getItemText(item)).toBe('search')
  })

  it('extracts toolName + truncated output from tool_execution content', () => {
    const content: ToolExecutionContent = {
      toolName: 'search',
      input: { q: 'test' },
      output: 'Found 3 results',
      ok: true,
    }
    const item = makeItem({ type: 'tool_execution', content })
    expect(getItemText(item)).toBe('search: Found 3 results')
  })

  it('truncates long tool output to 200 chars', () => {
    const longOutput = 'x'.repeat(300)
    const content: ToolExecutionContent = {
      toolName: 'read_file',
      output: longOutput,
      ok: true,
    }
    const item = makeItem({ type: 'tool_execution', content })
    const result = getItemText(item)
    // "read_file: " + 200 chars
    expect(result.length).toBeLessThanOrEqual('read_file: '.length + 200)
  })

  it('returns empty string for null content', () => {
    const item = makeItem({ content: null })
    expect(getItemText(item)).toBe('')
  })

  it('returns empty string when content has no recognized fields', () => {
    const item = makeItem({ content: { randomField: 'something' } })
    expect(getItemText(item)).toBe('')
  })

  it('handles agent_message with isPartial flag', () => {
    const content: AgentMessageContent = { text: 'Partial resp', isPartial: true }
    const item = makeItem({ type: 'agent_message', content, status: 'streaming' })
    expect(getItemText(item)).toBe('Partial resp')
  })
})
