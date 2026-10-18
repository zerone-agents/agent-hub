import { beforeEach, describe, expect, it, vi } from 'vitest'
import { checkDatasetEmbedding } from './knowledgeEmbedding'

const h = vi.hoisted(() => ({ post: vi.fn() }))
vi.mock('./client', () => ({ default: { post: h.post, defaults: {} }, unwrapResponse: (response: { data: unknown }) => response.data }))
const owner = () => ({ signal: new AbortController().signal, assertCurrent: vi.fn(), isCurrent: () => true })
const result = { code: 10, compatible: false, summary: { kb_id: 'kb/C', model: 'precise@factory', sampled: 2, valid: 2, avg_cos_sim: 0, min_cos_sim: 0, max_cos_sim: 0, match_mode: 'content_only' }, results: [] }

describe('embedding check HTTP contract', () => {
  beforeEach(() => { h.post.mockReset().mockResolvedValue({ data: result }) })
  it('preserves incompatible business evidence without treating it as transport failure', async () => {
    const guard = owner()
    await expect(checkDatasetEmbedding('kb/C', 'precise@factory', guard)).resolves.toEqual(result)
    expect(h.post).toHaveBeenCalledExactlyOnceWith('/api/v1/admin/knowledge/datasets/kb%2FC/embedding/check', { embd_id: 'precise@factory' }, expect.objectContaining({ signal: guard.signal, timeout: 65000 }))
  })
  it.each([null, { ...result, code: 0 }, { ...result, summary: { ...result.summary, kb_id: 'foreign' } }, { ...result, summary: { ...result.summary, model: 'other@factory' } }, { ...result, summary: { ...result.summary, avg_cos_sim: Number.NaN } }])('rejects malformed or cross-dataset evidence %j', async (data) => {
    h.post.mockResolvedValue({ data })
    await expect(checkDatasetEmbedding('kb/C', 'precise@factory', owner())).rejects.toThrow('Invalid')
  })
  it('rejects a response after the initiating credentials become obsolete', async () => {
    const guard = owner()
    h.post.mockImplementation(async () => { guard.assertCurrent.mockImplementation(() => { throw new Error('owner changed') }); return { data: result } })
    await expect(checkDatasetEmbedding('kb/C', 'precise@factory', guard)).rejects.toThrow('owner changed')
  })
})
