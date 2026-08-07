import { describe, it, expect } from 'vitest'
import JSZip from 'jszip'
import { parseSkillMd } from './parseSkillMd'

async function makeTestZip(files: Record<string, string>): Promise<File> {
  const zip = new JSZip()
  for (const [name, content] of Object.entries(files)) {
    zip.file(name, content)
  }
  const blob = await zip.generateAsync({ type: 'blob' })
  return new File([blob], 'test.zip', { type: 'application/zip' })
}

describe('parseSkillMd', () => {
  it('extracts single SKILL.md from nested layout', async () => {
    const file = await makeTestZip({
      'my-skill/SKILL.md': '# My Skill\nbody text',
      'my-skill/script.sh': 'echo hello',
    })
    const entries = await parseSkillMd(file)
    expect(entries).toHaveLength(1)
    expect(entries[0].path).toBe('my-skill/SKILL.md')
    expect(entries[0].content).toBe('# My Skill\nbody text')
  })

  it('extracts single SKILL.md from flat layout', async () => {
    const file = await makeTestZip({
      'SKILL.md': '# Flat',
      'README.md': 'other',
    })
    const entries = await parseSkillMd(file)
    expect(entries).toHaveLength(1)
    expect(entries[0].path).toBe('SKILL.md')
    expect(entries[0].content).toBe('# Flat')
  })

  it('throws when SKILL.md is missing', async () => {
    const file = await makeTestZip({
      'my-skill/README.md': 'no skill here',
    })
    await expect(parseSkillMd(file)).rejects.toThrow('SKILL.md')
  })

  it('extracts multiple SKILL.md at various depths, sorted by path', async () => {
    const file = await makeTestZip({
      'team/sub/deploy/SKILL.md': '# Deploy',
      'commit/SKILL.md': '# Commit',
      'team/review/SKILL.md': '# Review',
    })
    const entries = await parseSkillMd(file)
    expect(entries.map((e) => e.path)).toEqual([
      'commit/SKILL.md',
      'team/review/SKILL.md',
      'team/sub/deploy/SKILL.md',
    ])
    expect(entries.map((e) => e.content)).toEqual(['# Commit', '# Review', '# Deploy'])
  })

  it('excludes SKILL.md under .git / node_modules / dist / build / .zerone-uploads', async () => {
    const file = await makeTestZip({
      '.git/SKILL.md': '# Ghost',
      'node_modules/pkg/SKILL.md': '# Dep',
      'dist/SKILL.md': '# Built',
      'build/x/SKILL.md': '# Art',
      '.zerone-uploads/scratch/SKILL.md': '# Scratch',
      'real/SKILL.md': '# Real',
    })
    const entries = await parseSkillMd(file)
    expect(entries).toHaveLength(1)
    expect(entries[0].path).toBe('real/SKILL.md')
  })

  it('throws when every SKILL.md is under excluded dirs', async () => {
    const file = await makeTestZip({
      '.git/SKILL.md': '# Ghost',
      'node_modules/x/SKILL.md': '# Dep',
      'README.md': 'nothing real',
    })
    await expect(parseSkillMd(file)).rejects.toThrow('SKILL.md')
  })
})
