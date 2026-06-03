import { describe, expect, it } from 'vitest'
import { buildMemberLabelMap, resolveMemberLabel } from './memberLabels'
import type { ProjectSpaceSummary } from './types'

describe('memberLabels', () => {
  it('builds labels from project space member summaries', () => {
    const labels = buildMemberLabelMap([
      {
        projectId: 'project-1',
        spaceId: 'space-1',
        status: 'open',
        sortOrder: 0,
        pinned: false,
        spaceOpen: true,
        members: [
          { member: 'member-fred', memberId: 'member-fred', label: 'Fred' },
        ],
      },
    ] satisfies ProjectSpaceSummary[])

    expect(resolveMemberLabel('member-fred', labels)).toBe('Fred')
    expect(resolveMemberLabel('member-missing', labels)).toBe('member-missing')
  })
})
