import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Provider } from '@/api/providers'
import { syncEmbeddingModel } from './embeddingModelSync'

const h = vi.hoisted(() => ({ sync: vi.fn(), inventory: vi.fn() }))
vi.mock('@/api/providers', () => ({ providerApi: { syncMultiRAG: h.sync } }))
vi.mock('@/api/multirag', () => ({ multiragApi: { getModels: h.inventory } }))
const provider = { id: 42, protocol: 'openai', defaultModels: [{ modelId: 'embed', modelType: 'embedding' }] } as Provider
const stored = { name: 'embed___OpenAI-API', factory: 'OpenAI-API-Compatible', type: 'embedding', status: '1', fullId: 'embed___OpenAI-API@OpenAI-API-Compatible' }
const owner = () => ({ signal: new AbortController().signal, assertCurrent: vi.fn(), isCurrent: () => true })
const wrap = (data: unknown) => ({ data: { success: true, data } })

describe('embedding sync', () => {
  beforeEach(() => {
    h.sync.mockReset().mockResolvedValue(wrap({ success: true, factoryName: stored.factory, callCount: 1, perCall: [{ ok: true, modelName: 'embed' }] }))
    h.inventory.mockReset().mockResolvedValue(wrap([stored]))
  })
  it('persists once and independently reads the canonical factory-qualified registration', async () => {
    const guard = owner()
    await expect(syncEmbeddingModel(provider, 'embed', guard)).resolves.toEqual(stored)
    expect(h.sync).toHaveBeenCalledExactlyOnceWith(42, { verifyOnly: false, modelIds: ['embed'] }, guard)
    expect(h.inventory).toHaveBeenCalledExactlyOnceWith('embedding', guard)
  })
  it.each([{ rows: [] }, { rows: [{ ...stored, factory: 'Anthropic' }] }, { rows: [{ ...stored, status: '0' }] }])('rejects missing, wrong-factory or disabled readback %j', async ({ rows }) => {
    h.inventory.mockResolvedValue(wrap(rows))
    await expect(syncEmbeddingModel(provider, 'embed', owner())).rejects.toThrow('readback')
  })
  it('does not read inventory after a business sync failure', async () => {
    h.sync.mockResolvedValue(wrap({ success: false }))
    await expect(syncEmbeddingModel(provider, 'embed', owner())).rejects.toThrow()
    expect(h.inventory).not.toHaveBeenCalled()
  })
  it('rejects duplicate registrations before paid calls', async () => {
    await expect(syncEmbeddingModel({ ...provider, defaultModels: [...provider.defaultModels, ...provider.defaultModels] }, 'embed', owner())).rejects.toThrow()
    expect(h.sync).not.toHaveBeenCalled()
  })
  it('does not perform readback when credentials become obsolete after sync', async () => {
    const guard = owner()
    h.sync.mockImplementation(async () => { guard.assertCurrent.mockImplementation(() => { throw new Error('owner changed') }); return wrap({}) })
    await expect(syncEmbeddingModel(provider, 'embed', guard)).rejects.toThrow('owner changed')
    expect(h.inventory).not.toHaveBeenCalled()
  })
})
