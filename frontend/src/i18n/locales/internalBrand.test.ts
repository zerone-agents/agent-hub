import { describe, expect, it } from 'vitest'
import zh from './zh'
import en from './en'

function visibleText(value: unknown): string[] {
  if (typeof value === 'string') return [value]
  if (Array.isArray(value)) return value.flatMap(visibleText)
  if (value && typeof value === 'object') return Object.values(value).flatMap(visibleText)
  return []
}

describe('user-facing product copy', () => {
  it('does not expose the internal knowledge engine brand', () => {
    expect(visibleText({ zh, en }).filter(text => /multirag|ragflow|dataflow|casdoor|milvus|infinity/i.test(text))).toEqual([])
  })

  it('describes knowledge model choices by availability', () => {
    expect(zh.knowledge.candidates.multirag).toBe('已启用模型')
    expect(zh.knowledge.candidates.local).toContain('保存时验证')
    expect(zh.knowledge.form.embedModel).toBe('向量模型')
  })
})
