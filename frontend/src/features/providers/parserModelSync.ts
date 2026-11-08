import { unwrapResponse } from '@/api/client'
import { multiragApi, type MultiRAGModel } from '@/api/multirag'
import { modelReference } from '@/api/modelIdentity'
import { providerApi, type CatalogModel, type Provider } from '@/api/providers'
import type { RequestOwner } from '@/api/requestOwnership'

export type ParserModelSyncErrorCode = 'needs-config' | 'invalid-model' | 'verify-failed' | 'sync-failed' | 'readback-failed' | 'owner-changed'

export class ParserModelSyncError extends Error {
  constructor(readonly code: ParserModelSyncErrorCode) {
    // Provider probe responses may include raw credentials or upstream payloads.
    // The caller translates this code without displaying the original error.
    super('Parser model verification or sync failed')
    this.name = 'ParserModelSyncError'
  }
}

const FACTORIES: Record<string, string | undefined> = {
  anthropic: 'Anthropic',
  openai: 'OpenAI-API-Compatible',
  mineru: 'MinerU',
  paddleocr: 'PaddleOCR',
}

export function getParserSyncModels(provider: Provider): CatalogModel[] {
  const factory = FACTORIES[provider.protocol]
  if (!factory) return []
  const modelType = factory === 'MinerU' || factory === 'PaddleOCR' ? 'ocr' : 'vlm'
  const rowsByID = new Map<string, number>()
  for (const model of provider.defaultModels) {
    rowsByID.set(model.modelId, (rowsByID.get(model.modelId) ?? 0) + 1)
  }
  // The sync endpoint selects every row with this modelId, including disabled
  // rows. Repeated IDs can trigger duplicate paid probes or sync embedding too.
  return provider.defaultModels.filter(model => model.modelType === modelType
    && (model.status === undefined || model.status === '1')
    && model.modelId.trim() !== '' && rowsByID.get(model.modelId) === 1)
}

function assertOwner(owner: RequestOwner): void {
  try {
    owner.assertCurrent()
    if (owner.signal.aborted || !owner.isCurrent()) throw new Error()
  } catch {
    throw new ParserModelSyncError('owner-changed')
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function confirmedSync(value: unknown, factory: string, modelID: string): boolean {
  if (!isRecord(value) || value.success !== true || value.factoryName !== factory || value.callCount !== 1) return false
  const calls = value.perCall
  if (!Array.isArray(calls) || calls.length !== 1) return false
  return isRecord(calls[0]) && calls[0].ok === true && calls[0].modelName === modelID
}

export async function syncParserModel(provider: Provider, model: CatalogModel, owner: RequestOwner): Promise<MultiRAGModel> {
  let phase: ParserModelSyncErrorCode = 'invalid-model'
  try {
    assertOwner(owner)
    const factory = FACTORIES[provider.protocol]
    if (!factory || !getParserSyncModels(provider).some(candidate => candidate.modelId === model.modelId && candidate.modelType === model.modelType)) {
      throw new ParserModelSyncError('invalid-model')
    }
    if (model.modelType === 'ocr' && !provider.baseUrl.trim()) throw new ParserModelSyncError('needs-config')

    // Both legacy add_llm and atomic instance creation already probe. Sending a
    // separate verify first would repeat a paid inference without improving ack.
    phase = 'sync-failed'
    assertOwner(owner)
    const response = await providerApi.syncMultiRAG(provider.id, { verifyOnly: false, modelIds: [model.modelId] }, owner)
    assertOwner(owner)
    const receipt = unwrapResponse<unknown>(response)
    if (!confirmedSync(receipt, factory, model.modelId) || !isRecord(receipt)) throw new ParserModelSyncError(phase)
    const calls = receipt.perCall as Record<string, unknown>[]
    const reference = calls[0].reference
    if (receipt.endpoint === 'instances' && (typeof reference !== 'string' || typeof receipt.instanceName !== 'string' || !receipt.instanceName)) throw new ParserModelSyncError(phase)

    phase = 'readback-failed'
    const type = model.modelType === 'ocr' ? 'ocr' : 'image2text'
    // MultiRAG persists the OpenAI-compatible factory's legacy name suffix.
    const name = factory === 'OpenAI-API-Compatible' ? `${model.modelId}___OpenAI-API` : model.modelId
    assertOwner(owner)
    const inventory = await multiragApi.getModels(type, owner)
    assertOwner(owner)
    const models = unwrapResponse<unknown>(inventory)
    if (!Array.isArray(models)) throw new ParserModelSyncError('readback-failed')
    const matches = models.filter((candidate: unknown) => isRecord(candidate)
      && candidate.factory === factory && candidate.type === type && candidate.status === '1'
      && modelReference(candidate as unknown as MultiRAGModel)
      && (receipt.endpoint === 'instances' ? candidate.reference === reference && candidate.instanceName === receipt.instanceName
        : candidate.name === name && candidate.fullId === `${name}@${factory}`)) as MultiRAGModel[]
    const stored = matches.length === 1 ? matches[0] : undefined
    if (!stored) throw new ParserModelSyncError('readback-failed')
    assertOwner(owner)
    return stored
  } catch (error) {
    assertOwner(owner)
    if (error instanceof ParserModelSyncError) throw error
    throw new ParserModelSyncError(phase)
  }
}
