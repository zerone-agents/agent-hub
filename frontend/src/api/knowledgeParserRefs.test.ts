import { afterEach, describe, expect, it, vi } from 'vitest'
import apiClient from './client'
import { knowledgeApi } from './knowledge'
import { multiragApi } from './multirag'

afterEach(() => { vi.restoreAllMocks() })

describe('parser model API contract', () => {
  it('requests vision candidates independently from embedding candidates', async () => {
    const get = vi.spyOn(apiClient, 'get').mockResolvedValue({ data: { success: true, data: [] } })
    await multiragApi.getModels('image2text')
    await multiragApi.getModels('embedding')
    expect(get.mock.calls).toEqual([
      ['/api/v1/admin/knowledge/multirag/models', { params: { type: 'image2text' } }],
      ['/api/v1/admin/knowledge/multirag/models', { params: { type: 'embedding' } }],
    ])
  })

  it.each(['vision@preview@OpenAI', 'ocr-one@MinerU', 'ocr-two@MinerU', 'removed-model@PaddleOCR'])('saves and independently reads back %s without changing embedding', async (layout) => {
    const input = { embd_id: 'embedding@BAAI', parser_config: { layout_recognize: layout, custom: { preserve: true } } }
    const saved = { id: 'kb/1', ...input }
    const put = vi.spyOn(apiClient, 'put').mockResolvedValue({ data: { success: true, data: saved } })
    const get = vi.spyOn(apiClient, 'get').mockResolvedValue({ data: { success: true, data: saved } })
    await expect(knowledgeApi.datasets.update('kb/1', input)).resolves.toMatchObject(saved)
    expect(put).toHaveBeenCalledWith('/api/v1/admin/knowledge/datasets/kb%2F1', input)
    expect(get).toHaveBeenCalledWith('/api/v1/admin/knowledge/datasets/kb%2F1')
  })

  it.each([
    { embd_id: 'embedding@BAAI', parser_config: { layout_recognize: 'ocr-two@MinerU' } },
    { embd_id: 'other@BAAI', parser_config: { layout_recognize: 'ocr-one@MinerU' } },
  ])('rejects a different exact reference even when write and read responses agree', async (returned) => {
    const saved = { id: 'kb1', ...returned }
    vi.spyOn(apiClient, 'put').mockResolvedValue({ data: { success: true, data: saved } })
    vi.spyOn(apiClient, 'get').mockResolvedValue({ data: { success: true, data: saved } })
    await expect(knowledgeApi.datasets.update('kb1', { embd_id: 'embedding@BAAI', parser_config: { layout_recognize: 'ocr-one@MinerU' } })).rejects.toThrow('配置读回未确认')
  })

  it.each(['DeepDOC', 'Plain Text', 'MinerU', 'PaddleOCR', 'OpenDataLoader', 'Docling', 'TCADP Parser'])('does not let matching PUT and GET responses replace %s with a model', async (layout) => {
    const saved = { id: 'kb1', parser_config: { layout_recognize: 'wrong-model@OpenAI' } }
    vi.spyOn(apiClient, 'put').mockResolvedValue({ data: { success: true, data: saved } })
    vi.spyOn(apiClient, 'get').mockResolvedValue({ data: { success: true, data: saved } })
    await expect(knowledgeApi.datasets.update('kb1', { parser_config: { layout_recognize: layout } })).rejects.toThrow('配置读回未确认')
  })

  it.each([['DeepDOC', 'MinerU'], ['Plain Text', 'PaddleOCR'], ['MinerU', 'PaddleOCR'], ['PaddleOCR', 'MinerU'], ['OpenDataLoader', 'MinerU']])('rejects a factory switch from %s to %s in both responses', async (layout, returned) => {
    const saved = { id: 'kb1', parser_config: { layout_recognize: returned } }
    vi.spyOn(apiClient, 'put').mockResolvedValue({ data: { success: true, data: saved } })
    vi.spyOn(apiClient, 'get').mockResolvedValue({ data: { success: true, data: saved } })
    await expect(knowledgeApi.datasets.update('kb1', { parser_config: { layout_recognize: layout } })).rejects.toThrow('配置读回未确认')
  })

  it.each(['MinerU', 'PaddleOCR', 'OpenDataLoader', 'Docling'])('preserves the legacy local layout conversion to %s', async (factory) => {
    const saved = { id: 'kb1', parser_config: { layout_recognize: factory } }
    vi.spyOn(apiClient, 'put').mockResolvedValue({ data: { success: true, data: saved } })
    vi.spyOn(apiClient, 'get').mockResolvedValue({ data: { success: true, data: saved } })
    await expect(knowledgeApi.datasets.update('kb1', { parser_config: { layout_recognize: 'legacy-local-ocr' } })).resolves.toMatchObject(saved)
  })

  it('rejects changing a legacy local layout ID to a full model reference', async () => {
    const saved = { id: 'kb1', parser_config: { layout_recognize: 'other-ocr@MinerU' } }
    vi.spyOn(apiClient, 'put').mockResolvedValue({ data: { success: true, data: saved } })
    vi.spyOn(apiClient, 'get').mockResolvedValue({ data: { success: true, data: saved } })
    await expect(knowledgeApi.datasets.update('kb1', { parser_config: { layout_recognize: 'legacy-local-ocr' } })).rejects.toThrow('配置读回未确认')
  })

  it('confirms a local embedding model name with a revision and only its appended factory', async () => {
    const saved = { id: 'kb1', embd_id: 'embed@revision@Anthropic' }
    const put = vi.spyOn(apiClient, 'put').mockResolvedValue({ data: { success: true, data: saved } })
    const get = vi.spyOn(apiClient, 'get').mockResolvedValue({ data: { success: true, data: saved } })
    await expect(knowledgeApi.datasets.update('kb1', { embd_id: 'embed@revision' })).resolves.toMatchObject(saved)
    expect(put).toHaveBeenCalledWith('/api/v1/admin/knowledge/datasets/kb1', { embd_id: 'embed@revision' })
    expect(get).toHaveBeenCalledTimes(1)
  })

  it.each([
    ['embedding@BAAI', 'embedding@BAAI@Anthropic'],
    ['embedding@OpenAI-API-Compatible', 'embedding@OpenAI-API-Compatible@Anthropic'],
    ['embedding@OpenAI', 'embedding@OpenAI@Anthropic'],
    ['embed@revision', 'other@revision@Anthropic'],
    ['embed@revision', 'embed@revision@UnknownFactory'],
  ])('rejects changing %s to %s even when both responses agree', async (input, returned) => {
    const saved = { id: 'kb1', embd_id: returned }
    vi.spyOn(apiClient, 'put').mockResolvedValue({ data: { success: true, data: saved } })
    vi.spyOn(apiClient, 'get').mockResolvedValue({ data: { success: true, data: saved } })
    await expect(knowledgeApi.datasets.update('kb1', { embd_id: input })).rejects.toThrow('配置读回未确认')
  })
})
