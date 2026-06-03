import { describe, expect, it } from 'vitest'
import type { AgentEvent } from '../../lib/types'
import { resolveInspectorErrorText } from './spaceInspectorError'
import { resolveGraphQueryRender } from './spaceInspectorGraphQuery'
import { resolveSpaceMessageRender } from './spaceInspectorMessage'
import { materializeNestedJSONStrings } from './spaceInspectorPrettyPayload'
import { resolveEventTitle } from './spaceInspectorTitle'

describe('SpaceInspectorTab title resolution', () => {
  it('does not treat null-ish error titles as display text', () => {
    const event = {
      id: 'event-error-title',
      kind: 'task',
      title: 'null',
      startedAt: '2026-05-03T00:00:00.000Z',
      status: 'error',
      data: {
        action: 'submit',
      },
    } as AgentEvent

    expect(resolveEventTitle(event)).toBe('Submit Task')
    expect(resolveInspectorErrorText(event)).toBe('Error details were not captured for this event.')
  })

  it('extracts visible error text from nested tool result payloads', () => {
    const payload = JSON.stringify({
      ok: false,
      op: 'task',
      text: JSON.stringify({
        error: 'task lifecycle publisher not configured',
        task: {
          title: 'Submit',
        },
      }),
    })
    const event = {
      id: 'event-error-payload',
      kind: 'task',
      title: 'Submit',
      startedAt: '2026-05-03T00:00:00.000Z',
      status: 'error',
      data: {
        action: 'submit',
        result: payload,
      },
    } as AgentEvent

    expect(resolveInspectorErrorText(event)).toBe('task lifecycle publisher not configured')
  })

  it('uses task payload titles instead of raw serialized event titles', () => {
    const payload = JSON.stringify({
      ok: true,
      op: 'task',
      text: JSON.stringify({
        task: {
          id: 'task-499d8fdcbdaa0b8a',
          title: 'Validate final answer control',
          status: 'succeeded',
        },
      }),
    })

    const event = {
      id: 'event-1',
      kind: 'task',
      title: payload,
      startedAt: '2026-05-03T00:00:00.000Z',
      status: 'success',
      data: {
        action: 'complete',
        result: payload,
      },
    } as AgentEvent

    expect(resolveEventTitle(event)).toBe('Validate final answer control')
  })

  it('uses plan payload fields instead of raw serialized event titles', () => {
    const payload = JSON.stringify({
      ok: true,
      op: 'plan',
      text: JSON.stringify({
        guidance: 'All phases are complete. Submit the plan.',
        phase: {
          id: '52e23b75-87d7-4e4c-92bc-29ccc0c3b023',
          title: 'Phase 4: Verify observability, authenticated access, and closeout',
          status: 'completed',
        },
      }),
    })

    const event = {
      id: 'event-2',
      kind: 'plan',
      title: payload,
      startedAt: '2026-05-03T00:00:00.000Z',
      status: 'success',
      data: {
        action: 'complete_phase',
        result: payload,
      },
    } as AgentEvent

    expect(resolveEventTitle(event)).toBe('Phase 4: Verify observability, authenticated access, and closeout')
  })

  it('uses metrics payload scope instead of raw serialized event titles', () => {
    const payload = JSON.stringify({
      ok: true,
      op: 'metrics',
      text: JSON.stringify({
        Role: 'ceo',
        Tokens: {
          InputTokens: 0,
          OutputTokens: 0,
          CostUSD: 0,
        },
      }),
    })

    const event = {
      id: 'event-3',
      kind: 'metrics',
      title: payload,
      startedAt: '2026-05-03T00:00:00.000Z',
      status: 'success',
      data: {
        action: 'agent',
        result: payload,
      },
    } as AgentEvent

    expect(resolveEventTitle(event)).toBe('Agent metrics · ceo')
  })

  it('uses schedule entry names instead of raw serialized event titles', () => {
    const payload = JSON.stringify({
      ok: true,
      op: 'schedule',
      text: JSON.stringify({
        action: 'list',
        message: 'Managed schedule schedules',
        entries: [{
          EntryID: 'entry-588e2d9d-72b2-469e-a692-ed01cd3d0f8e',
          RoleID: 'ceo',
          Name: 'Validate schedule management KR',
          Goal: 'Validate schedule management tool path',
          Status: 'active',
        }],
      }),
    })

    const event = {
      id: 'event-4',
      kind: 'schedule',
      title: payload,
      startedAt: '2026-05-03T00:00:00.000Z',
      status: 'success',
      data: {
        action: 'list',
        result: payload,
      },
    } as AgentEvent

    expect(resolveEventTitle(event)).toBe('Schedule List · Validate schedule management KR')
  })

  it('formats space message result bodies instead of rendering raw JSON envelopes', () => {
    const payload = JSON.stringify({
      body: {
        broadcast: false,
        correlationId: '40c25245-ebf5-4838-8790-1723712f34aa',
        destinationChannel: 'channel:space-a87ddb84-fe08-4502-86e3-55c58e024ac7:role:run-ceo',
        destinationSpaceRef: 'engineering',
        destinationSpaceId: 'space-a87ddb84-fe08-4502-86e3-55c58e024ac7',
        kind: 'inform',
        subject: 'Space messaging validation for Agen8 tool-test mission',
        body: 'Validation message from executive/ceo for mission mis-b2b1a22a.',
      },
      ok: true,
      op: 'space',
      text: 'Sent inform to space engineering: Space messaging validation for Agen8 tool-test mission.',
    })

    const event = {
      id: 'event-5',
      kind: 'space_message',
      title: 'ceo',
      startedAt: '2026-05-03T00:00:00.000Z',
      status: 'success',
      data: {
        action: 'message',
        result: payload,
      },
    } as AgentEvent

    const spec = resolveSpaceMessageRender(event)
    expect(spec?.label).toBe('Message')
    expect(spec?.body).toContain('Validation message from executive/ceo')
    expect(spec?.body).toContain('Space messaging validation for Agen8 tool-test mission')
    expect(spec?.body).not.toContain('{"body"')
    expect(spec?.metadata).toEqual(expect.arrayContaining([
      { label: 'Destination', value: 'engineering', kind: 'code' },
      { label: 'Kind', value: 'Inform' },
      { label: 'Broadcast', value: 'No' },
    ]))
  })

  it('formats graph_query node results instead of rendering raw JSON envelopes', () => {
    const payload = JSON.stringify({
      body: JSON.stringify({
        action: 'node',
        node_type: 'decision',
        node_id: 'dec-6dc749e4-3bc0-4f01-9295-130e167c6f37',
        node: {
          id: 'dec-6dc749e4-3bc0-4f01-9295-130e167c6f37',
          type: 'decision',
          title: 'Use evidence-first KR completion for decision logging validation',
          status: 'log',
          spaceName: 'research-space',
          fields: {
            alternativesRejected: 'Marking the KR complete based only on prior decision logs.',
            confidence: 0.95,
          },
          neighbours: [{
            id: 'kr-38895649-9123-47ce-9384-7134ba736965',
            type: 'key_result',
            title: 'Validate graph visibility',
            status: 'completed',
          }],
        },
      }),
    })

    const event = {
      id: 'event-6',
      kind: 'graph_query',
      title: 'Graph query',
      startedAt: '2026-05-03T00:00:00.000Z',
      status: 'success',
      data: {
        action: 'node',
        result: payload,
      },
    } as AgentEvent

    const spec = resolveGraphQueryRender(event)
    expect(spec?.label).toBe('Graph node')
    expect(spec?.body).toContain('Use evidence-first KR completion')
    expect(spec?.body).toContain('Marking the KR complete')
    expect(spec?.body).toContain('Validate graph visibility')
    expect(spec?.body).not.toContain('{"body"')
    expect(spec?.metadata).toEqual(expect.arrayContaining([
      { label: 'Action', value: 'Node' },
      { label: 'Type', value: 'Decision' },
      { label: 'Status', value: 'Log' },
    ]))
    expect(spec?.confidence).toBe(0.95)
  })

  it('materializes nested JSON strings for inspector pretty mode', () => {
    const payload = {
      ok: true,
      op: 'schedule',
      text: JSON.stringify({
        action: 'list',
        message: 'Managed schedule schedules',
        entries: [{
          EntryID: 'entry-588e2d9d-72b2-469e-a692-ed01cd3d0f8e',
          Name: 'Validate schedule management KR',
          Context: JSON.stringify({
            relatedTaskId: 'task-4467336dd4bf5587',
            successCriteria: 'Schedule create and list paths return successfully.',
          }),
        }],
      }),
    }

    const pretty = materializeNestedJSONStrings(payload) as {
      text: {
        entries: Array<{
          Context: {
            relatedTaskId: string
            successCriteria: string
          }
        }>
      }
    }

    expect(pretty.text.entries[0].Context.relatedTaskId).toBe('task-4467336dd4bf5587')
    expect(pretty.text.entries[0].Context.successCriteria).toContain('Schedule create')
  })
})
