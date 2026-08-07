import { describe, it, expect } from 'vitest'
import { parseSkillFrontmatter } from './parseSkillFrontmatter'

describe('parseSkillFrontmatter', () => {
  it('parses frontmatter and returns body', () => {
    const { frontmatter, body } = parseSkillFrontmatter('---\nname: my-skill\ndescription: A test skill\n---\n# My Skill\nbody text')
    expect(frontmatter).toEqual({
      name: 'my-skill',
      description: 'A test skill'
    })
    expect(body).toBe('# My Skill\nbody text')
  })

  it('returns null frontmatter when no frontmatter', () => {
    const { frontmatter, body } = parseSkillFrontmatter('# My Skill\nbody text')
    expect(frontmatter).toBeNull()
    expect(body).toBe('# My Skill\nbody text')
  })

  it('returns null frontmatter when closing delimiter is missing', () => {
    const { frontmatter, body } = parseSkillFrontmatter('---\nname: my-skill\n# My Skill')
    expect(frontmatter).toBeNull()
    expect(body).toBe('---\nname: my-skill\n# My Skill')
  })

  it('ignores empty and comment lines', () => {
    const { frontmatter } = parseSkillFrontmatter('---\nname: my-skill\n# comment\n\ndescription: desc\n---\n')
    expect(frontmatter).toEqual({
      name: 'my-skill',
      description: 'desc'
    })
  })

  it('splits on the first colon only', () => {
    const { frontmatter } = parseSkillFrontmatter('---\nurl: https://example.com:8080\n---\n')
    expect(frontmatter).toEqual({
      url: 'https://example.com:8080'
    })
  })

  it('parses multi-line folded scalar as a single value', () => {
    const input = `---
name: pharmaceutical-care-pathway
description: >
  药学监护路径生成技能。This skill should be used when the user needs to generate a pharmaceutical care pathway
  (药学监护路径) for a specific clinical diagnosis. The workflow includes: (1) analyzing clinical guidelines or
  pathways for the diagnosis to generate a treatment timeline diagram.
  Trigger when user mentions: 药学监护路径、临床路径药学、药学查房路径、药学监护计划、某疾病的药学监护。
agent_created: true
---
# 药学监护路径生成技能
`
    const { frontmatter, body } = parseSkillFrontmatter(input)
    expect(frontmatter).toEqual({
      name: 'pharmaceutical-care-pathway',
      description:
        '药学监护路径生成技能。This skill should be used when the user needs to generate a pharmaceutical care pathway (药学监护路径) for a specific clinical diagnosis. The workflow includes: (1) analyzing clinical guidelines or pathways for the diagnosis to generate a treatment timeline diagram. Trigger when user mentions: 药学监护路径、临床路径药学、药学查房路径、药学监护计划、某疾病的药学监护。',
      agent_created: 'true'
    })
    expect(body).toBe('# 药学监护路径生成技能\n')
  })

  it('parses literal block scalar preserving newlines', () => {
    const input = `---
name: my-skill
description: |
  line one
  line two
agent_created: true
---
body`
    const { frontmatter } = parseSkillFrontmatter(input)
    expect(frontmatter).toEqual({
      name: 'my-skill',
      description: 'line one\nline two',
      agent_created: 'true'
    })
  })
})
