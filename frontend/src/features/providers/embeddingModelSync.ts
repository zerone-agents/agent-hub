import { unwrapResponse } from '@/api/client'
import { multiragApi, type MultiRAGModel } from '@/api/multirag'
import { modelReference } from '@/api/modelIdentity'
import { providerApi, type Provider } from '@/api/providers'
import type { RequestOwner } from '@/api/requestOwnership'

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

export async function syncEmbeddingModel(provider: Provider, modelId: string, owner: RequestOwner): Promise<MultiRAGModel> {
  owner.assertCurrent()
  const factory = provider.protocol === 'openai' ? 'OpenAI-API-Compatible' : provider.protocol === 'anthropic' ? 'Anthropic' : undefined
  const rows = provider.defaultModels.filter(model => model.modelId === modelId)
  if (!factory || rows.length !== 1 || rows[0].modelType !== 'embedding' || rows[0].status === '0') throw new Error('Invalid embedding model')
  // add_llm already probes before persisting. A separate verify would repeat
  // paid model calls. Do not send is_tools: preserve the saved explicit value.
  const response = await providerApi.syncMultiRAG(provider.id, { verifyOnly: false, modelIds: [modelId] }, owner)
  owner.assertCurrent()
  const result = unwrapResponse<unknown>(response)
  if (!isRecord(result) || result.success !== true || result.factoryName !== factory || result.callCount !== 1
    || !Array.isArray(result.perCall) || result.perCall.length !== 1 || !isRecord(result.perCall[0])
    || result.perCall[0].ok !== true || result.perCall[0].modelName !== modelId) throw new Error('Embedding sync failed')
  const reference = result.perCall[0].reference
  if (result.endpoint === 'instances' && (typeof reference !== 'string' || typeof result.instanceName !== 'string' || !result.instanceName)) throw new Error('Embedding identity unconfirmed')
  const inventory = await multiragApi.getModels('embedding', owner)
  owner.assertCurrent()
  const models = unwrapResponse<MultiRAGModel[]>(inventory)
  const name = factory === 'OpenAI-API-Compatible' ? `${modelId}___OpenAI-API` : modelId
  const matches = Array.isArray(models) ? models.filter(model => model.factory === factory
    && model.type === 'embedding' && model.status === '1' && modelReference(model)
    && (result.endpoint === 'instances' ? model.reference === reference && model.instanceName === result.instanceName
      : model.name === name && model.fullId === `${name}@${factory}`)) : []
  const stored = matches.length === 1 ? matches[0] : undefined
  if (!stored) throw new Error('Embedding readback failed')
  return stored
}
