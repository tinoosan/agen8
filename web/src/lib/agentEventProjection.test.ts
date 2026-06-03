import { describe, expect, it } from 'vitest'
import type { AgentEvent } from './types'
import { projectAgentEvent } from './agentEventProjection'
import '../components/conversation/mcp/descriptors'

function baseEvent(overrides: Partial<AgentEvent> = {}): AgentEvent {
  return {
    id: 'evt-1',
    kind: 'agent.tool.call',
    title: 'tool call',
    status: 'ok',
    startedAt: '2026-04-22T10:00:00Z',
    data: {
      toolName: 'task',
      sourceType: 'builtin',
      sourceId: 'builtin',
      status: 'success',
      op: 'task',
      action: 'list',
    },
    ...overrides,
  }
}

describe('projectAgentEvent', () => {
  it('projects semantic tool calls to task kind', () => {
    const projected = projectAgentEvent(baseEvent())
    expect(projected.kind).toBe('task')
    expect(projected.data?.op).toBe('task')
    expect(projected.data?.action).toBe('list')
  })

  it('projects graph_query tool calls to graph_query kind', () => {
    const projected = projectAgentEvent(baseEvent({
      data: {
        toolName: 'graph_query',
        sourceType: 'mcp',
        sourceId: 'agen8',
        status: 'success',
        op: 'graph_query',
        input: JSON.stringify({
          action: 'search',
          node_type: 'all',
          query: 'pricing',
        }),
      },
    }))
    expect(projected.kind).toBe('graph_query')
    expect(projected.data?.op).toBe('graph_query')
    expect(projected.data?.action).toBe('search')
    expect(projected.data?.node_type).toBe('all')
    expect(projected.data?.query).toBe('pricing')
  })

  it('infers semantic operation from toolName when op is missing', () => {
    const projected = projectAgentEvent(baseEvent({
      data: {
        toolName: 'agen8/mission',
        sourceType: 'mcp',
        sourceId: 'agen8',
        status: 'success',
      },
    }))
    expect(projected.kind).toBe('mission')
    expect(projected.data?.op).toBe('mission')
  })

  it('infers semantic operation from namespaced MCP tool names', () => {
    const projected = projectAgentEvent(baseEvent({
      data: {
        toolName: 'mcp__agen8__plan',
        sourceType: 'mcp',
        sourceId: 'agen8',
        status: 'success',
      },
    }))
    expect(projected.kind).toBe('plan')
    expect(projected.data?.op).toBe('plan')
  })

  it('normalizes namespaced data.op values (e.g. agen8/decision_log)', () => {
    const projected = projectAgentEvent(baseEvent({
      data: {
        toolName: 'decision_log',
        sourceType: 'builtin',
        sourceId: 'builtin',
        status: 'success',
        op: 'agen8/decision_log',
        action: 'log',
      },
    }))
    expect(projected.kind).toBe('decision')
    expect(projected.data?.op).toBe('decision')
  })

  it('keeps unknown tools as generic agent.tool.call', () => {
    const projected = projectAgentEvent(baseEvent({
      data: {
        toolName: 'docs/search',
        sourceType: 'mcp',
        sourceId: 'docs',
        status: 'success',
      },
    }))
    expect(projected.kind).toBe('agent.tool.call')
    expect(projected.data?.op).toBe('search')
  })

  it('derives action from input payload when explicit action is missing', () => {
    const projected = projectAgentEvent(baseEvent({
      data: {
        toolName: 'task',
        sourceType: 'builtin',
        sourceId: 'builtin',
        status: 'pending',
        op: 'task',
        input: JSON.stringify({ action: 'create', title: 'Ship fix' }),
      },
    }))
    expect(projected.kind).toBe('task')
    expect(projected.data?.action).toBe('create')
  })

  it('spreads domain input fields onto data for semantic tools', () => {
    const projected = projectAgentEvent(baseEvent({
      data: {
        toolName: 'task',
        sourceType: 'mcp',
        sourceId: 'agen8',
        status: 'pending',
        op: 'task',
        input: JSON.stringify({
          action: 'create',
          title: 'Fix the auth bug',
          goal: 'Make login work',
          assignedRole: 'coder',
        }),
      },
    }))
    expect(projected.kind).toBe('task')
    expect(projected.data?.action).toBe('create')
    expect(projected.data?.title).toBe('Fix the auth bug')
    expect(projected.data?.goal).toBe('Make login work')
    expect(projected.data?.assignedRole).toBe('coder')
  })

  it('does not overwrite existing event metadata when spreading input fields', () => {
    const projected = projectAgentEvent(baseEvent({
      data: {
        toolName: 'task',
        sourceType: 'mcp',
        sourceId: 'agen8',
        status: 'success',
        op: 'task',
        input: JSON.stringify({
          action: 'create',
          // These collide with existing event metadata — must NOT overwrite.
          status: 'pending',
          sourceType: 'fake',
          toolName: 'fake',
        }),
      },
    }))
    expect(projected.data?.status).toBe('success')
    expect(projected.data?.sourceType).toBe('mcp')
    expect(projected.data?.toolName).toBe('task')
  })

  it('JSON-stringifies non-string input values', () => {
    const projected = projectAgentEvent(baseEvent({
      data: {
        toolName: 'decision',
        sourceType: 'mcp',
        sourceId: 'agen8',
        status: 'pending',
        input: JSON.stringify({
          action: 'ask_user',
          questions: [{ id: 'q1', text: 'Proceed?' }],
          confidence: 0.85,
        }),
      },
    }))
    expect(projected.kind).toBe('decision')
    expect(projected.data?.questions).toBe('[{"id":"q1","text":"Proceed?"}]')
    expect(projected.data?.confidence).toBe('0.85')
  })

  it('does not spread input fields for non-semantic tool ops', () => {
    const projected = projectAgentEvent(baseEvent({
      data: {
        toolName: 'docs/search',
        sourceType: 'mcp',
        sourceId: 'docs',
        status: 'success',
        input: JSON.stringify({ query: 'auth flow' }),
      },
    }))
    // 'search' is not in SEMANTIC_TOOL_OPS — no spreading.
    expect(projected.kind).toBe('agent.tool.call')
    expect(projected.data?.query).toBeUndefined()
  })

  it('extracts shell command and background flag from structured input payload', () => {
    const projected = projectAgentEvent(baseEvent({
      kind: 'agent.tool.call',
      data: {
        toolName: 'shell_exec',
        sourceType: 'cli',
        sourceId: 'codex',
        status: 'pending',
        op: 'shell_exec',
        input: JSON.stringify({ command: 'npm run dev &', background: true }),
      },
    }))
    expect(projected.kind).toBe('bash')
    expect(projected.data?.op).toBe('bash')
    expect(projected.data?.command).toBe('npm run dev &')
    expect(projected.data?.background).toBe('true')
  })

  it('normalizes dotted bash op names from external harnesses', () => {
    const projected = projectAgentEvent(baseEvent({
      data: {
        toolName: 'bash.run',
        sourceType: 'cli',
        sourceId: 'bash',
        status: 'success',
        op: 'bash.run',
        command: "bash -lc 'pwd'",
      },
    }))
    expect(projected.kind).toBe('bash')
    expect(projected.data?.op).toBe('bash')
  })

  it('normalizes codex native file tool names to semantic operations', () => {
    const writeProjected = projectAgentEvent(baseEvent({
      data: {
        toolName: 'Write',
        sourceType: 'cli',
        sourceId: 'codex',
        status: 'success',
      },
    }))
    expect(writeProjected.kind).toBe('write_file')
    expect(writeProjected.data?.op).toBe('write_file')

    const editProjected = projectAgentEvent(baseEvent({
      data: {
        toolName: 'MultiEdit',
        sourceType: 'cli',
        sourceId: 'codex',
        status: 'success',
      },
    }))
    expect(editProjected.kind).toBe('edit_file')
    expect(editProjected.data?.op).toBe('edit_file')

    const readProjected = projectAgentEvent(baseEvent({
      data: {
        toolName: 'Read',
        sourceType: 'cli',
        sourceId: 'codex',
        status: 'success',
      },
    }))
    expect(readProjected.kind).toBe('read_file')
    expect(readProjected.data?.op).toBe('read_file')

    const globProjected = projectAgentEvent(baseEvent({
      data: {
        toolName: 'Glob',
        sourceType: 'cli',
        sourceId: 'codex',
        status: 'success',
      },
    }))
    expect(globProjected.kind).toBe('search_files')
    expect(globProjected.data?.op).toBe('search_files')
  })

  it('projects Claude Code Read file_path to canonical file read path', () => {
    const projected = projectAgentEvent(baseEvent({
      data: {
        toolName: 'Read',
        sourceType: 'native',
        sourceId: 'claude-cli',
        status: 'success',
        input: JSON.stringify({ file_path: '/home/tinoosan/homelab/kustomization.yaml' }),
        outputPreview: 'apiVersion: kustomize.config.k8s.io/v1beta1',
      },
    }))

    expect(projected.kind).toBe('read_file')
    expect(projected.data?.op).toBe('read_file')
    expect(projected.data?.path).toBe('/home/tinoosan/homelab/kustomization.yaml')
    expect(projected.data?.file_path).toBe('/home/tinoosan/homelab/kustomization.yaml')
  })

  it('normalizes codex web.run tool calls to web search', () => {
    const projected = projectAgentEvent(baseEvent({
      data: {
        toolName: 'web.run',
        sourceType: 'builtin',
        sourceId: 'codex',
        status: 'success',
        input: JSON.stringify({
          search_query: [{ q: 'example domain' }],
          response_length: 'short',
        }),
      },
    }))

    expect(projected.kind).toBe('web_search')
    expect(projected.data?.op).toBe('web_search')
    expect(projected.data?.search_query).toBe('[{"q":"example domain"}]')
    expect(projected.data?.response_length).toBe('short')
  })

  it('normalizes codex image_gen tool calls to image generation', () => {
    const projected = projectAgentEvent(baseEvent({
      data: {
        toolName: 'image_gen',
        sourceType: 'native',
        sourceId: 'codex',
        status: 'success',
        input: JSON.stringify({ prompt: 'draw a small red square' }),
      },
    }))

    expect(projected.kind).toBe('image_generation')
    expect(projected.data?.op).toBe('image_generation')
    expect(projected.data?.prompt).toBe('draw a small red square')
  })

  it('treats image generation payloads as completed even when runtime status is still generating', () => {
    const projected = projectAgentEvent(baseEvent({
      status: 'pending',
      completedAt: undefined,
      data: {
        toolName: 'image_generation',
        sourceType: 'native',
        sourceId: 'codex',
        status: 'generating',
        op: 'image_generation',
        prompt: 'draw a small red square',
        imageB64: 'iVBORw0KGgo=',
      },
    }))

    expect(projected.kind).toBe('image_generation')
    expect(projected.status).toBe('ok')
    expect(projected.completedAt).toBe('2026-04-22T10:00:00Z')
    expect(projected.data?.status).toBe('completed')
  })

  it('normalizes codex-specific tool names to semantic cards', () => {
    const toolSearch = projectAgentEvent(baseEvent({
      data: {
        toolName: 'ToolSearch',
        sourceType: 'native',
        sourceId: 'codex',
        status: 'success',
        input: JSON.stringify({ query: 'select:mcp__agen8__task' }),
      },
    }))
    expect(toolSearch.kind).toBe('tool_search')
    expect(toolSearch.data?.op).toBe('tool_search')
    expect(toolSearch.data?.query).toBe('select:mcp__agen8__task')

    const execCommand = projectAgentEvent(baseEvent({
      data: {
        toolName: 'functions.exec_command',
        sourceType: 'native',
        sourceId: 'codex',
        status: 'success',
        input: JSON.stringify({ command: 'pwd' }),
      },
    }))
    expect(execCommand.kind).toBe('bash')
    expect(execCommand.data?.op).toBe('bash')
    expect(execCommand.data?.command).toBe('pwd')

    const applyPatch = projectAgentEvent(baseEvent({
      data: {
        toolName: 'functions.apply_patch',
        sourceType: 'native',
        sourceId: 'codex',
        status: 'success',
      },
    }))
    expect(applyPatch.kind).toBe('edit_file')
    expect(applyPatch.data?.op).toBe('edit_file')
  })

  it('normalizes codex app-server file_change events to semantic file change operations', () => {
    const projected = projectAgentEvent(baseEvent({
      data: {
        toolName: 'file_change',
        sourceType: 'cli',
        sourceId: 'codex',
        status: 'success',
        op: 'file_change',
        path: 'README.md',
        patchPreview: [
          '--- a/README.md',
          '+++ b/README.md',
          '@@',
          '-old',
          '+new',
        ].join('\n'),
      },
    }))

    expect(projected.kind).toBe('file_change')
    expect(projected.data?.op).toBe('file_change')
    expect(projected.data?.path).toBe('README.md')
    expect(projected.data?.patchPreview).toContain('+new')
  })

  it('normalizes shell_command aliases to bash semantic operation', () => {
    const projected = projectAgentEvent(baseEvent({
      data: {
        toolName: 'shell_command',
        sourceType: 'cli',
        sourceId: 'codex',
        status: 'success',
      },
    }))
    expect(projected.kind).toBe('bash')
    expect(projected.data?.op).toBe('bash')
  })

  it('synthesizes edit patch previews from codex/claude edit input payloads', () => {
    const projected = projectAgentEvent(baseEvent({
      data: {
        toolName: 'Edit',
        sourceType: 'cli',
        sourceId: 'claude-cli',
        status: 'success',
        input: JSON.stringify({
          file_path: '/workspace/demo.txt',
          old_string: 'line-old',
          new_string: 'line-new',
        }),
      },
    }))
    expect(projected.kind).toBe('edit_file')
    expect(projected.data?.path).toBe('/workspace/demo.txt')
    expect(projected.data?.patchPreview).toContain('--- a/workspace/demo.txt')
    expect(projected.data?.patchPreview).toContain('+++ b/workspace/demo.txt')
    expect(projected.data?.patchPreview).toContain('-line-old')
    expect(projected.data?.patchPreview).toContain('+line-new')
  })

  it('synthesizes write patch previews from codex/claude write input payloads', () => {
    const projected = projectAgentEvent(baseEvent({
      data: {
        toolName: 'Write',
        sourceType: 'cli',
        sourceId: 'claude-cli',
        status: 'success',
        input: JSON.stringify({
          path: '/workspace/new.txt',
          content: 'hello world',
        }),
      },
    }))
    expect(projected.kind).toBe('write_file')
    expect(projected.data?.path).toBe('/workspace/new.txt')
    expect(projected.data?.writeMode).toBe('created')
    expect(projected.data?.patchPreview).toContain('--- /dev/null')
    expect(projected.data?.patchPreview).toContain('+++ b/workspace/new.txt')
    expect(projected.data?.patchPreview).toContain('+hello world')
  })

  it('normalizes wrapped agen8 tool bridge calls into native coordination ops', () => {
    const projected = projectAgentEvent(baseEvent({
      data: {
        toolName: 'tool',
        sourceType: 'mcp',
        sourceId: 'agen8',
        status: 'success',
        input: JSON.stringify({
          server: 'agen8',
          tool: 'decision',
          arguments: {
            action: 'log',
            title: 'Decision title',
            rationale: 'Because',
          },
        }),
      },
    }))
    expect(projected.kind).toBe('decision')
    expect(projected.data?.op).toBe('decision')
    expect(projected.data?.action).toBe('log')
    expect(projected.data?.title).toBe('Decision title')
    expect(projected.data?.rationale).toBe('Because')
  })

  it('projects agen8 message sends into readable space message events', () => {
    const projected = projectAgentEvent(baseEvent({
      title: 'agen8/message',
      data: {
        sourceType: 'mcp',
        sourceId: 'agen8',
        status: 'success',
        channelId: 'channel:fred',
        input: JSON.stringify({
          action: 'send',
          destination_member_id: 'member-sarah',
          kind: 'query',
          subject: 'Back-and-forth conversation test',
          body: 'Hi Sarah, please confirm receipt.',
        }),
        outputPreview: JSON.stringify({
          action: 'send',
          channelId: 'channel:sarah',
          messageId: 'msg-1',
          destinationMemberId: 'member-sarah',
          destinationMemberLabel: 'Sarah',
          sourceMemberLabel: 'Fred',
          kind: 'query',
          subject: 'Back-and-forth conversation test',
          correlationId: 'corr-1',
          status: 'queued',
        }),
      },
    }))

    expect(projected.kind).toBe('space_message')
    expect(projected.data?.op).toBe('space_message')
    expect(projected.data?.channelId).toBe('channel:fred')
    expect(projected.data?.deliveryChannelId).toBe('channel:sarah')
    expect(projected.data?.destinationMemberLabel).toBe('Sarah')
    expect(projected.data?.sourceMemberLabel).toBe('Fred')
    expect(projected.data?.body).toBe('Hi Sarah, please confirm receipt.')
    expect(projected.data?.correlationId).toBe('corr-1')
    expect(projected.data?.messageId).toBe('msg-1')
  })

  it('projects Codex MCP resource listing calls into visible tool-list events', () => {
    const projected = projectAgentEvent(baseEvent({
      title: 'codex/list_mcp_resources',
      data: {
        toolName: 'codex/list_mcp_resources',
        sourceType: 'mcp',
        server: 'codex',
        status: 'success',
        op: 'list_mcp_resources',
        input: '{}',
      },
    }))

    expect(projected.kind).toBe('tool')
    expect(projected.data?.op).toBe('tool')
    expect(projected.data?.action).toBe('list')
  })

  it('keeps Codex app Notion calls generic without an app descriptor registry', () => {
    const projected = projectAgentEvent(baseEvent({
      title: 'codex_apps/notion_notion-get-users',
      data: {
        toolName: 'codex_apps/notion_notion-get-users',
        sourceType: 'mcp',
        server: 'codex_apps',
        status: 'success',
        op: 'notion_notion_get_users',
        input: '{"user_id":"self"}',
      },
    }))

    expect(projected.kind).toBe('agent.tool.call')
    expect(projected.data?.mcpServerId).toBeUndefined()
    expect(projected.data?.mcpOp).toBeUndefined()
  })

  it('keeps Codex app Google Calendar calls generic without an app descriptor registry', () => {
    const projected = projectAgentEvent(baseEvent({
      title: 'codex_apps/google calendar_get_colors',
      data: {
        toolName: 'codex_apps/google calendar_get_colors',
        sourceType: 'mcp',
        server: 'codex_apps',
        status: 'success',
        op: 'google_calendar_get_colors',
        input: '{}',
      },
    }))

    expect(projected.kind).toBe('agent.tool.call')
    expect(projected.data?.mcpServerId).toBeUndefined()
    expect(projected.data?.mcpOp).toBeUndefined()
  })
})
