import { describe, expect, it } from 'vitest'
import {
  spaceDetailEntryToActivity,
  spaceDetailEntryToChatEntry,
  spaceDetailEntryToInspectorEvent,
} from './useSpaceDetail'
import type { SpaceDetailEntry } from '../lib/types'

describe('space detail projection adapters', () => {
  it('maps user and agent detail entries without activity fallback semantics', () => {
    const user = spaceDetailEntryToChatEntry({
      id: 'msg-1',
      kind: 'user_message',
      runId: 'run-1',
      messageId: 'msg-1',
      text: 'hello',
      createdAt: '2026-05-02T12:00:00Z',
    })
    const agent = spaceDetailEntryToChatEntry({
      id: 'agent-1',
      kind: 'agent_message',
      runId: 'run-1',
      turnId: 'turn-1',
      text: 'hi there',
      status: 'streaming',
      live: true,
      createdAt: '2026-05-02T12:00:01Z',
    })

    expect(user).toMatchObject({
      kind: 'user',
      text: 'hello',
      source: 'space-detail',
    })
    expect(agent).toMatchObject({
      kind: 'agent',
      text: 'hi there',
      live: true,
      source: 'space-detail',
    })
  })

  it('maps tool detail entries into the existing semantic card payload shape', () => {
    const entry: SpaceDetailEntry = {
      id: 'tool:run-1:call-1',
      kind: 'tool_call',
      runId: 'run-1',
      turnId: 'turn-1',
      toolCallId: 'call-1',
      title: 'bash',
      status: 'completed',
      createdAt: '2026-05-02T12:00:01Z',
      completedAt: '2026-05-02T12:00:02Z',
      data: {
        op: 'bash',
        command: 'pwd',
        outputPreview: '/repo',
      },
    }

    expect(spaceDetailEntryToActivity(entry)).toMatchObject({
      id: 'tool:run-1:call-1',
      kind: 'agent.tool.call',
      status: 'ok',
      title: 'bash',
      outputPreview: '/repo',
      data: {
        op: 'bash',
        command: 'pwd',
        runId: 'run-1',
        turnId: 'turn-1',
        opId: 'call-1',
      },
    })
  })

  it('maps thinking detail entries into thought transcript entries only', () => {
    const entry: SpaceDetailEntry = {
      id: 'thinking:session-1:turn-1:reason-1',
      kind: 'thinking',
      runId: 'session-1',
      turnId: 'turn-1',
      toolCallId: 'thinking-reason-1',
      member: 'Claude',
      text: 'Checking the plan',
      status: 'completed',
      createdAt: '2026-05-02T12:00:01Z',
      completedAt: '2026-05-02T12:00:02Z',
      data: {
        channelId: 'channel:space-1:member:member-1',
        itemId: 'reason-1',
        kind: 'reasoning',
      },
    }

    expect(spaceDetailEntryToChatEntry(entry)).toMatchObject({
      id: 'thinking:session-1:turn-1:reason-1',
      kind: 'thought',
      text: 'Checking the plan',
      role: 'Claude',
      channelId: 'channel:space-1:member:member-1',
      runId: 'session-1',
      turnId: 'turn-1',
      source: 'space-detail',
    })
    expect(spaceDetailEntryToActivity(entry)).toBeNull()
    expect(spaceDetailEntryToInspectorEvent(entry)).toMatchObject({
      id: 'thinking:session-1:turn-1:reason-1',
      kind: 'model.thinking.completed',
      outputPreview: 'Checking the plan',
      data: {
        itemId: 'reason-1',
        kind: 'reasoning',
      },
    })
  })

  it('preserves internal harness lifecycle rows for run-state tracking', () => {
    expect(
      spaceDetailEntryToActivity({
        id: 'run-started',
        kind: 'tool_call',
        runId: 'run-1',
        turnId: 'turn-1',
        toolCallId: 'run-1:harness.run.started',
        title: 'harness.run.started',
        status: 'pending',
        createdAt: '2026-05-02T12:00:01Z',
        data: {
          toolName: 'harness.run.started',
          kind: 'harness.run.started',
        },
      })?.kind,
    ).toBe('harness.run.started')
  })

  it('maps received member messages into activity and inspector events', () => {
    const entry: SpaceDetailEntry = {
      id: 'agent_message_received_msg-1',
      kind: 'agent_message_received',
      runId: 'session-1',
      turnId: 'turn-1',
      toolCallId: 'msg-1',
      title: 'Message from Sarah',
      status: 'completed',
      text: 'Please check whether Fred received this.',
      createdAt: '2026-05-02T12:00:01Z',
      completedAt: '2026-05-02T12:00:02Z',
      data: {
        sourceMemberLabel: 'Sarah',
        sourceMemberId: 'member-sarah',
        kind: 'query',
        correlationId: 'corr-1',
        error:
          'This is a stale non-error annotation and should not render as event.error.',
      },
    }

    const expected = {
      id: 'agent_message_received_msg-1',
      kind: 'agent_message_received',
      title: 'Message from Sarah',
      status: 'ok',
      textPreview: 'Please check whether Fred received this.',
      data: {
        runId: 'session-1',
        turnId: 'turn-1',
        opId: 'msg-1',
        sourceMemberLabel: 'Sarah',
      },
    }

    expect(spaceDetailEntryToActivity(entry)).toMatchObject(expected)
    expect(spaceDetailEntryToActivity(entry)?.error).toBeUndefined()
    expect(spaceDetailEntryToInspectorEvent(entry)).toMatchObject(expected)
  })

  it('does not turn context/system detail into transcript entries', () => {
    expect(
      spaceDetailEntryToChatEntry({
        id: 'ctx-1',
        kind: 'context',
        text: 'Context updated',
        createdAt: '2026-05-02T12:00:00Z',
      }),
    ).toBeNull()
    expect(
      spaceDetailEntryToActivity({
        id: 'ctx-1',
        kind: 'context',
        text: 'Context updated',
        createdAt: '2026-05-02T12:00:00Z',
      }),
    ).toBeNull()
    expect(
      spaceDetailEntryToInspectorEvent({
        id: 'ctx-1',
        kind: 'context',
        text: 'Context updated',
        createdAt: '2026-05-02T12:00:00Z',
      }),
    ).toBeNull()
  })

  it('maps transcript and error detail entries into inspector events', () => {
    const user = spaceDetailEntryToInspectorEvent({
      id: 'msg-1',
      kind: 'user_message',
      text: 'hello',
      member: 'user',
      runId: 'run-1',
      createdAt: '2026-05-01T10:00:00Z',
      status: 'completed',
    })
    const error = spaceDetailEntryToInspectorEvent({
      id: 'err-1',
      kind: 'error',
      title: 'LLM request failed',
      text: 'context canceled',
      member: 'cto',
      runId: 'run-1',
      createdAt: '2026-05-01T10:00:01Z',
      status: 'failed',
    })

    expect(user).toMatchObject({
      id: 'msg-1',
      kind: 'user_message',
      textPreview: 'hello',
      data: { member: 'user', runId: 'run-1' },
    })
    expect(error).toMatchObject({
      id: 'err-1',
      kind: 'llm.error',
      title: 'LLM request failed',
      status: 'error',
      error: 'context canceled',
      data: { member: 'cto', runId: 'run-1' },
    })
  })
})
