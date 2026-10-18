import type { MultiRAGModel } from '@/api/multirag'
import type { Provider } from '@/api/providers'

// Build constants — locked per spec.
export const BUILTIN_LAYOUTS = ['DeepDOC', 'Plain Text'] as const

export type CandidateSource = 'builtin' | 'multirag' | 'local'
export type LayoutModelKind = 'builtin' | 'ocr' | 'image2text'

export interface CandidateOption {
  label: string
  value: string
  // Internal metadata (not displayed; carried for submit logic).
  source: CandidateSource
  providerId?: number
  // For multirag options, the raw MultiRAG fullId / factory.
  // For local options, the raw modelId.
  rawValue: string
  kind?: LayoutModelKind
  disabled?: boolean
  unavailable?: boolean
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
  const enabled = multirag.filter(m => m.type === 'embedding' && m.status === '1' && m.name && m.factory && m.fullId === `${m.name}@${m.factory}`)
  const multiragFullIDs = new Set(enabled.map(m => m.fullId))
  const multiragOptions: CandidateOption[] = enabled.map(m => ({
    label: `${m.name} (${m.factory})`, value: `multirag:${m.fullId}`,
    source: 'multirag', rawValue: m.fullId,
  }))

  // Python stores tenant+factory+name, not Hub provider instance IDs. Never
  // offer two local instances which would overwrite the same registration.
  const counts = new Map<string, number>()
  for (const p of localProviders) {
    const factory = providerToFactory(p)
    for (const m of p.defaultModels) {
      if (factory && m.modelType === 'embedding') {
        const key = `${factory}:${m.modelId}`
        counts.set(key, (counts.get(key) ?? 0) + 1)
      }
    }
  }
  const localOptions: CandidateOption[] = []
  for (const p of localProviders) {
    const factory = providerToFactory(p)
    if (!factory || !['OpenAI-API-Compatible', 'Anthropic'].includes(factory)) continue
    for (const m of p.defaultModels) {
      if (m.modelType !== 'embedding' || !m.modelId.trim() || (m.status !== undefined && m.status !== '1')) continue
      if (counts.get(`${factory}:${m.modelId}`) !== 1) continue
      const name = factory === 'OpenAI-API-Compatible' ? `${m.modelId}___OpenAI-API` : m.modelId
      if (multiragFullIDs.has(`${name}@${factory}`) || multiragFullIDs.has(`${m.modelId}@${factory}`)) continue
      localOptions.push({ label: `${m.displayName} (${p.name})`, value: `local:${p.id}:${m.modelId}`,
        source: 'local', providerId: p.id, rawValue: m.modelId })
    }
  }

  return buildGroups({
    'knowledge.candidates.multirag': multiragOptions,
    'knowledge.candidates.local': localOptions,
  })
}

export function buildLayoutCandidates(
  multirag: MultiRAGModel[],
): CandidateGroup[] {
  const builtinOptions: CandidateOption[] = BUILTIN_LAYOUTS.map(v => ({
    label: v,
    value: `builtin:${v}`,
    source: 'builtin' as const,
    rawValue: v,
    kind: 'builtin',
  }))

  // Frozen Python normalizes one parser suffix, then resolves the remaining
  // full name@factory. Keep both suffixes to bind same-name OCR registrations.
  const ocrFactories = ['MinerU', 'PaddleOCR', 'OpenDataLoader']
  const seen = new Set<string>()
  const external: CandidateOption[] = []
  for (const model of multirag) {
    if (model.status !== '1' || !model.name || !model.factory) continue
    const kind = model.type
    if (kind !== 'ocr' && kind !== 'image2text') continue
    if (kind === 'ocr' && !ocrFactories.includes(model.factory)) continue
    // Reserved OCR suffixes dispatch to OCR, regardless of the model's type.
    if (kind === 'image2text' && ocrFactories.includes(model.factory)) continue
    const fullId = kind === 'ocr' ? `${model.name}@${model.factory}@${model.factory}` : `${model.name}@${model.factory}`
    if (seen.has(fullId)) continue
    seen.add(fullId)
    external.push({ label: `${model.name} (${model.factory})`, value: `multirag:${fullId}`, source: 'multirag', rawValue: fullId, kind })
  }

  return buildGroups({
    'knowledge.candidates.builtin': builtinOptions,
    'knowledge.candidates.ocr': external.filter(option => option.kind === 'ocr'),
    'knowledge.candidates.vision': external.filter(option => option.kind === 'image2text'),
  })
}

export function isAllowedLayoutSelection(value: string, groups: CandidateGroup[], saved?: string): boolean {
  // Keep an existing unavailable parser on unrelated settings edits, but do
  // not offer it as a new choice or sync an old local candidate draft.
  const raw = decodeCandidateValue(value)?.rawValue ?? value
  if (saved && (value === saved || raw === saved)) return true
  return groups.some(group => group.options.some(option => !option.disabled && (option.value === value || option.rawValue === value)))
}

// A query refresh can remove a selected encoded option. Keep that exact value
// visible and disabled, while validation permits only the persisted reference.
export function retainLayoutCandidates(groups: CandidateGroup[], saved?: string, current?: string): CandidateGroup[] {
  const retained: CandidateOption[] = []
  const rawCurrent = current === saved ? saved : decodeCandidateValue(current)?.rawValue ?? current
  for (const [index, value] of [current, saved].entries()) {
    if (!value) continue
    const raw = index === 1 || value === saved ? value : decodeCandidateValue(value)?.rawValue ?? value
    if (index === 1 && current && raw === rawCurrent) continue
    if (groups.some(group => group.options.some(option => option.value === value || option.rawValue === value))) continue
    retained.push({ label: raw, value, rawValue: raw, source: 'multirag', disabled: true, unavailable: true })
  }
  return retained.length ? [{ label: 'knowledge.settings.currentValue', options: retained }, ...groups] : groups
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
