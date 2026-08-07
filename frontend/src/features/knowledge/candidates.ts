import type { MultiRAGModel } from '@/api/multirag'
import type { Provider } from '@/api/providers'

// Build constants — locked per spec.
export const BUILTIN_LAYOUTS = ['DeepDOC', 'Plain Text'] as const

export type CandidateSource = 'builtin' | 'multirag' | 'local'

export interface CandidateOption {
  label: string
  value: string
  // Internal metadata (not displayed; carried for submit logic).
  source: CandidateSource
  providerId?: number
  // For multirag options, the raw MultiRAG fullId / factory.
  // For local options, the raw modelId.
  rawValue: string
}

export interface CandidateGroup {
  label: string
  options: CandidateOption[]
}

export interface DecodedValue {
  source: CandidateSource
  rawValue: string
  providerId: number | null
}

// MultiRAG factory name is determined by protocol, not provider key.
// This mirrors the backend's MultiRAGFactoryName() which is protocol-based:
//   anthropic → Anthropic, openai → OpenAI-API-Compatible, etc.
const PROTOCOL_TO_FACTORY: Record<string, string> = {
  'anthropic': 'Anthropic',
  'openai': 'OpenAI-API-Compatible',
  'mineru': 'MinerU',
  'paddleocr': 'PaddleOCR',
}

function providerToFactory(p: Provider): string | undefined {
  return PROTOCOL_TO_FACTORY[p.protocol]
}

export function buildEmbeddingCandidates(
  multirag: MultiRAGModel[],
  localProviders: Provider[],
): CandidateGroup[] {
  const multiragFullIDs = new Set(multirag.map(m => m.fullId))

  const multiragOptions: CandidateOption[] = multirag
    .filter(m => m.type === 'embedding')
    .map(m => ({
      label: `${m.name} (${m.factory})`,
      value: `multirag:${m.fullId}`,
      source: 'multirag' as const,
      rawValue: m.fullId,
    }))

  const localOptions: CandidateOption[] = []
  for (const p of localProviders) {
    const factory = providerToFactory(p)
    if (!factory) continue
    for (const m of p.defaultModels) {
      if (m.modelType !== 'embedding') continue
      const fullID = `${m.modelId}@${factory}`
      if (multiragFullIDs.has(fullID)) continue // dedup
      localOptions.push({
        label: `${m.displayName} (${p.name})`,
        value: `local:${p.id}:${m.modelId}`,
        source: 'local',
        providerId: p.id,
        rawValue: m.modelId,
      })
    }
  }

  return buildGroups({
    'MultiRAG 已有': multiragOptions,
    '本地待同步': localOptions,
  })
}

export function buildLayoutCandidates(
  multirag: MultiRAGModel[],
  localProviders: Provider[],
): CandidateGroup[] {
  const multiragFactories = new Set(
    multirag.filter(m => m.type === 'ocr').map(m => m.factory)
  )

  const builtinOptions: CandidateOption[] = BUILTIN_LAYOUTS.map(v => ({
    label: v,
    value: `builtin:${v}`,
    source: 'builtin' as const,
    rawValue: v,
  }))

  const multiragOptions: CandidateOption[] = multirag
    .filter(m => m.type === 'ocr')
    .map(m => ({
      label: `${m.factory} (${m.name})`,
      value: `multirag:${m.factory}`,
      source: 'multirag' as const,
      rawValue: m.factory,
    }))

  const localOptions: CandidateOption[] = []
  for (const p of localProviders) {
    const factory = providerToFactory(p)
    if (!factory) continue
    if (multiragFactories.has(factory)) continue // dedup by factory
    for (const m of p.defaultModels) {
      if (m.modelType !== 'ocr') continue
      localOptions.push({
        label: `${m.displayName} (${p.name})`,
        value: `local:${p.id}:${m.modelId}`,
        source: 'local',
        providerId: p.id,
        rawValue: m.modelId,
      })
      break // one option per provider (factory-level dedup)
    }
  }

  return buildGroups({
    '内置': builtinOptions,
    'MultiRAG 已有': multiragOptions,
    '本地待同步': localOptions,
  })
}

// buildGroups filters out empty groups and returns them in the order given.
function buildGroups(groups: Record<string, CandidateOption[]>): CandidateGroup[] {
  const order = Object.keys(groups)
  return order
    .map(label => ({ label, options: groups[label] }))
    .filter(g => g.options.length > 0)
}

export function decodeCandidateValue(value: string | undefined | null): DecodedValue | null {
  if (!value) return null
  if (value.startsWith('builtin:')) {
    return { source: 'builtin', rawValue: value.slice('builtin:'.length), providerId: null }
  }
  if (value.startsWith('multirag:')) {
    return { source: 'multirag', rawValue: value.slice('multirag:'.length), providerId: null }
  }
  if (value.startsWith('local:')) {
    const rest = value.slice('local:'.length)
    const colonIdx = rest.indexOf(':')
    if (colonIdx === -1) return null
    const pidStr = rest.slice(0, colonIdx)
    const raw = rest.slice(colonIdx + 1)
    const pid = Number(pidStr)
    if (!Number.isInteger(pid)) return null
    if (raw === '') return null
    return { source: 'local', rawValue: raw, providerId: pid }
  }
  return null
}
