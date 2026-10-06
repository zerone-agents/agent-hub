import { afterEach, describe, expect, it, vi } from 'vitest'
import { AxiosHeaders, type AxiosResponse } from 'axios'
import { multiragApi, type MultiRAGModel } from '@/api/multirag'
import { providerApi, type CatalogModel, type Provider } from '@/api/providers'
import type { RequestOwner } from '@/api/requestOwnership'
import { getParserSyncModels, ParserModelSyncError, syncParserModel } from './parserModelSync'

const model = (modelId: string, modelType: CatalogModel['modelType'] = 'ocr', status: CatalogModel['status'] = '1'): CatalogModel => ({ modelId, displayName: modelId, modelType, status })
const provider = (models: CatalogModel[], protocol: Provider['protocol'] = 'mineru', baseUrl = 'https://parser.example.invalid'): Provider => ({
  id: 7, key: 'configured-provider', name: 'Configured provider', description: '', descriptionEn: '', protocol,
  authStyle: 'no_auth', baseUrl, defaultModels: models, fields: [], iconKey: '', builtin: false, attributes: {}, lockedApiKey: '', createdAt: '', updatedAt: '',
})
const response = (data: unknown): AxiosResponse<{ success: boolean; data: unknown }> => ({
  data: { success: true, data }, status: 200, statusText: 'OK', headers: {}, config: { headers: new AxiosHeaders() },
})
const confirmed = (modelName: string, factoryName = 'MinerU') => response({ success: true, factoryName, callCount: 1, perCall: [{ modelName, ok: true }] })
const stored = (name: string, factory = 'MinerU', type = 'ocr', status = '1'): MultiRAGModel => ({ name, factory, type, status, fullId: `${name}@${factory}` })
function ownerState() {
  let current = true
  const owner: RequestOwner = {
    signal: new AbortController().signal,
    isCurrent: () => current,
    assertCurrent: vi.fn(() => { if (!current) throw new Error('owner changed') }),
  }
  return { owner, loseAccess: () => { current = false } }
}
afterEach(() => vi.restoreAllMocks())

describe('parser models eligible for precise sync', () => {
  it('keeps both OCR models in one factory and offers visual models through supported vision protocols', () => {
    const models = [model('ocr-one'), model('ocr-two'), model('vision', 'vlm'), model('embedding', 'embedding'), model('chat', 'llm'), model('disabled', 'ocr', '0')]
    expect(getParserSyncModels(provider(models))).toEqual(models.slice(0, 2))
    expect(getParserSyncModels(provider(models, 'paddleocr'))).toEqual(models.slice(0, 2))
    expect(getParserSyncModels(provider(models, 'anthropic'))).toEqual([models[2]])
    expect(getParserSyncModels(provider(models, 'openai'))).toEqual([models[2]])
  })

  it.each([['mineru', 'vlm'], ['openai', 'ocr']] as const)('does not offer or sync the unsupported %s/%s combination', async (protocol, type) => {
    const candidate = model('unsupported-parser', type)
    const p = provider([candidate], protocol)
    const sync = vi.spyOn(providerApi, 'syncMultiRAG')
    expect(getParserSyncModels(p)).toEqual([])
    await expect(syncParserModel(p, candidate, ownerState().owner)).rejects.toMatchObject({ code: 'invalid-model' })
    expect(sync).not.toHaveBeenCalled()
  })

  it.each(['1', '0'] as const)('rejects duplicate visual IDs even when the second selection has status %s', async status => {
    const vision = { ...model('vision-one', 'vlm'), selectionId: 'first' }
    const duplicate = { ...vision, selectionId: 'second', status }
    const p = provider([vision, duplicate], 'openai')
    const sync = vi.spyOn(providerApi, 'syncMultiRAG')
    const read = vi.spyOn(multiragApi, 'getModels')
    expect(getParserSyncModels(p)).toEqual([])
    await expect(syncParserModel(p, vision, ownerState().owner)).rejects.toMatchObject({ code: 'invalid-model' })
    expect(sync).not.toHaveBeenCalled()
    expect(read).not.toHaveBeenCalled()
    const independent = model('vision-two', 'vlm')
    expect(getParserSyncModels({ ...p, defaultModels: [...p.defaultModels, independent] })).toEqual([independent])
  })

  it('rejects a modelId shared with embedding even when the embedding row is disabled', async () => {
    const vision = model('shared', 'vlm')
    const p = provider([vision, model('shared', 'embedding', '0'), model('independent', 'vlm')], 'openai')
    expect(getParserSyncModels(p).map(candidate => candidate.modelId)).toEqual(['independent'])
    const sync = vi.spyOn(providerApi, 'syncMultiRAG')
    await expect(syncParserModel(p, vision, ownerState().owner)).rejects.toMatchObject({ code: 'invalid-model' })
    expect(sync).not.toHaveBeenCalled()
  })

  it('never syncs an embedding-only selection', async () => {
    const embedding = model('embedding', 'embedding')
    const sync = vi.spyOn(providerApi, 'syncMultiRAG')
    await expect(syncParserModel(provider([embedding], 'openai'), embedding, ownerState().owner)).rejects.toMatchObject({ code: 'invalid-model' })
    expect(sync).not.toHaveBeenCalled()
  })

  it('requires an OCR service URL before issuing any request', async () => {
    const ocr = model('ocr-one')
    const sync = vi.spyOn(providerApi, 'syncMultiRAG')
    await expect(syncParserModel(provider([ocr], 'mineru', '  '), ocr, ownerState().owner)).rejects.toMatchObject({ code: 'needs-config' })
    expect(sync).not.toHaveBeenCalled()
  })
})

describe('verify, persist and independently read a single parser model', () => {
  it('syncs two same-factory OCR models separately without including embedding', async () => {
    const models = [model('ocr-one'), model('ocr-two'), model('embedding', 'embedding')]
    const p = provider(models)
    const { owner } = ownerState()
    const sync = vi.spyOn(providerApi, 'syncMultiRAG').mockImplementation(async (_id, body) => confirmed(body?.modelIds?.[0] ?? ''))
    const read = vi.spyOn(multiragApi, 'getModels').mockResolvedValue(response([stored('ocr-one'), stored('ocr-two')]))
    for (const candidate of getParserSyncModels(p)) await expect(syncParserModel(p, candidate, owner)).resolves.toEqual(stored(candidate.modelId))
    expect(sync.mock.calls).toEqual([
      [7, { verifyOnly: true, modelIds: ['ocr-one'] }, owner], [7, { verifyOnly: false, modelIds: ['ocr-one'] }, owner],
      [7, { verifyOnly: true, modelIds: ['ocr-two'] }, owner], [7, { verifyOnly: false, modelIds: ['ocr-two'] }, owner],
    ])
    expect(read.mock.calls).toEqual([['ocr', owner], ['ocr', owner]])
    expect(owner.assertCurrent).toHaveBeenCalled()
  })

  it.each([
    ['openai', 'OpenAI-API-Compatible', 'vision@preview___OpenAI-API'],
    ['anthropic', 'Anthropic', 'vision@preview'],
  ] as const)('allows %s visual service defaults and reads its exact stored reference', async (protocol, factory, name) => {
    const vision = model('vision@preview', 'vlm')
    const p = provider([vision, model('embedding', 'embedding')], protocol, '')
    const { owner } = ownerState()
    const sync = vi.spyOn(providerApi, 'syncMultiRAG').mockResolvedValue(confirmed(vision.modelId, factory))
    const expected = stored(name, factory, 'image2text')
    const read = vi.spyOn(multiragApi, 'getModels').mockResolvedValue(response([stored('other-vision', factory, 'image2text'), expected]))
    await expect(syncParserModel(p, vision, owner)).resolves.toEqual(expected)
    expect(sync.mock.calls).toEqual([[7, { verifyOnly: true, modelIds: ['vision@preview'] }, owner], [7, { verifyOnly: false, modelIds: ['vision@preview'] }, owner]])
    expect(read).toHaveBeenCalledWith('image2text', owner)
  })

  it('uses the PaddleOCR protocol factory for a specific model', async () => {
    const ocr = model('PP-OCRv5')
    vi.spyOn(providerApi, 'syncMultiRAG').mockResolvedValue(confirmed(ocr.modelId, 'PaddleOCR'))
    vi.spyOn(multiragApi, 'getModels').mockResolvedValue(response([stored(ocr.modelId, 'PaddleOCR')]))
    await expect(syncParserModel(provider([ocr], 'paddleocr'), ocr, ownerState().owner)).resolves.toEqual(stored(ocr.modelId, 'PaddleOCR'))
  })

  it.each([
    { success: false, factoryName: 'MinerU', callCount: 1, perCall: [{ modelName: 'ocr-one', ok: true }] },
    { success: true, factoryName: 'MinerU', callCount: 0, perCall: [] },
    { success: true, factoryName: 'MinerU', callCount: 1, perCall: [{ modelName: 'ocr-one', ok: false, error: 'fixture-secret' }] },
    { success: true, factoryName: 'MinerU', callCount: 2, perCall: [{ modelName: 'ocr-one', ok: true }, { modelName: 'embedding', ok: true }] },
    { success: true, factoryName: 'MinerU', callCount: 1, perCall: [{ modelName: 'other-ocr', ok: true }] },
    { success: true, factoryName: 'PaddleOCR', callCount: 1, perCall: [{ modelName: 'ocr-one', ok: true }] },
    { success: true, factoryName: 'MinerU', callCount: 1 },
  ])('stops after an unconfirmed verification result %#', async (result) => {
    const ocr = model('ocr-one')
    const sync = vi.spyOn(providerApi, 'syncMultiRAG').mockResolvedValue(response(result))
    const read = vi.spyOn(multiragApi, 'getModels')
    await expect(syncParserModel(provider([ocr]), ocr, ownerState().owner)).rejects.toMatchObject({ code: 'verify-failed' })
    expect(sync).toHaveBeenCalledTimes(1)
    expect(read).not.toHaveBeenCalled()
  })

  it('does not report a successful verify-only response as successful persistence', async () => {
    const ocr = model('ocr-one')
    const sync = vi.spyOn(providerApi, 'syncMultiRAG').mockResolvedValueOnce(confirmed(ocr.modelId)).mockResolvedValueOnce(response({ success: false, callCount: 1, perCall: [{ ok: false, error: 'fixture-secret' }] }))
    const read = vi.spyOn(multiragApi, 'getModels')
    await expect(syncParserModel(provider([ocr]), ocr, ownerState().owner)).rejects.toMatchObject({ code: 'sync-failed' })
    expect(sync).toHaveBeenCalledTimes(2)
    expect(read).not.toHaveBeenCalled()
  })

  it.each([
    null, [], [stored('other-ocr')], [stored('ocr-one', 'PaddleOCR')],
    [stored('ocr-one', 'MinerU', 'image2text')], [stored('ocr-one', 'MinerU', 'ocr', '0')],
    [{ ...stored('ocr-one'), fullId: 'different@MinerU' }],
  ].map(data => ({ data })))('rejects an unconfirmed exact readback %#', async ({ data }) => {
    const ocr = model('ocr-one')
    vi.spyOn(providerApi, 'syncMultiRAG').mockResolvedValue(confirmed(ocr.modelId))
    vi.spyOn(multiragApi, 'getModels').mockResolvedValue(response(data))
    await expect(syncParserModel(provider([ocr]), ocr, ownerState().owner)).rejects.toMatchObject({ code: 'readback-failed' })
  })

  it('does not expose transport or business error payloads', async () => {
    const ocr = model('ocr-one')
    vi.spyOn(providerApi, 'syncMultiRAG').mockRejectedValue(new Error('fixture-secret in upstream payload'))
    const error = await syncParserModel(provider([ocr]), ocr, ownerState().owner).catch((failure: unknown) => failure)
    expect(error).toBeInstanceOf(ParserModelSyncError)
    expect(error).toMatchObject({ code: 'verify-failed' })
    expect(String(error)).not.toContain('fixture-secret')
    expect(error).not.toHaveProperty('cause')
  })

  it.each(['verify', 'persist', 'readback'] as const)('stops when ownership changes after %s', async (stage) => {
    const ocr = model('ocr-one')
    const state = ownerState()
    const sync = vi.spyOn(providerApi, 'syncMultiRAG').mockImplementation(async (_id, body) => {
      if ((stage === 'verify' && body?.verifyOnly) || (stage === 'persist' && body?.verifyOnly === false)) state.loseAccess()
      return confirmed(ocr.modelId)
    })
    const read = vi.spyOn(multiragApi, 'getModels').mockImplementation(async () => {
      state.loseAccess()
      return response([stored(ocr.modelId)])
    })
    await expect(syncParserModel(provider([ocr]), ocr, state.owner)).rejects.toMatchObject({ code: 'owner-changed' })
    expect(sync).toHaveBeenCalledTimes(stage === 'verify' ? 1 : 2)
    expect(read).toHaveBeenCalledTimes(stage === 'readback' ? 1 : 0)
  })
})
