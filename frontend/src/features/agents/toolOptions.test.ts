import { describe, expect, it } from 'vitest'
import { buildToolOptions } from './toolOptions'
import type { Tool } from '@/api/tools'

const base: Tool = {
  id: 1, name: 'X', title: '', description: '', isDefault: false, source: 'builtin',
  createdAt: '', updatedAt: ''
}
const tool = (over: Partial<Tool>): Tool => ({ ...base, ...over })

describe('buildToolOptions', () => {
  const tools = [
    tool({ name: 'Bash', source: 'builtin' }),
    tool({ name: 'Read', source: 'builtin' }),
    tool({ name: 'SayHello', source: 'custom', artifactStatus: 'ready' }),
    tool({ name: 'Legacy', source: 'custom', artifactStatus: 'missing' })
  ]
  const defaults = new Set(['Read'])

  it('groups builtin before custom with counts implied by group length', () => {
    const groups = buildToolOptions(tools, [], defaults)
    expect(groups).toHaveLength(2)
    expect(groups[0]).toMatchObject({ label: '内置工具' })
    expect(groups[1]).toMatchObject({ label: '自定义工具' })
    expect(groups[0].options.map((o) => o.value)).toEqual(['Bash', 'Read'])
  })

  it('disables default tools and unselected missing custom tools with hint label', () => {
    const groups = buildToolOptions(tools, [], defaults)
    const custom = groups[1].options
    const legacy = custom.find((o) => o.value === 'Legacy')
    expect(legacy?.disabled).toBe(true)
    expect(legacy?.label).toBe('Legacy（缺少工具文件）')
    const read = groups[0].options.find((o) => o.value === 'Read')
    expect(read?.disabled).toBe(true)
  })

  it('keeps already-associated missing tool selectable so user can explicitly remove it', () => {
    const groups = buildToolOptions(tools, ['Legacy'], defaults)
    const legacy = groups[1].options.find((o) => o.value === 'Legacy')
    expect(legacy?.disabled).toBe(false)
    expect(legacy?.label).toBe('Legacy（缺少工具文件）')
  })
})
