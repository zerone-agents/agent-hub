import { describe, it, expect } from 'vitest'
import {
  buildEmbeddingCandidates,
  buildLayoutCandidates,
  decodeCandidateValue,
  BUILTIN_LAYOUTS,
  isAllowedLayoutSelection,
  retainLayoutCandidates,
} from './candidates'
import type { Provider } from '@/api/providers'
import type { MultiRAGModel } from '@/api/multirag'

describe('buildEmbeddingCandidates', () => {
  it('shows MultiRAG and local groups, dedupes by full id', () => {
    const multirag: MultiRAGModel[] = [
      { name: 'bge-m3', factory: 'Anthropic', type: 'embedding', status: '1', fullId: 'bge-m3@Anthropic' },
    ]
    const localProviders: Provider[] = [
      {
        id: 42, key: 'glm-cn', name: 'ZhipuAI', protocol: 'anthropic', authStyle: 'api_key',
        baseUrl: '', description: '', descriptionEn: '', iconKey: '', builtin: false,
        lockedApiKey: '', attributes: {}, createdAt: '', updatedAt: '',
        defaultModels: [
          { modelId: 'bge-large-zh', displayName: 'BGE Large ZH', modelType: 'embedding' },
          { modelId: 'bge-m3', displayName: 'BGE M3', modelType: 'embedding' }, // dup of multirag, factory Anthropic via GLM — deduped
        ],
        fields: [],
      },
    ]
    const result = buildEmbeddingCandidates(multirag, localProviders)
    // Expect 2 groups: 'MultiRAG 已有' with 1 option, '本地待同步' with 1 option (bge-large-zh)
    expect(result).toHaveLength(2)
    const mrGroup = result.find(g => g.label === 'knowledge.candidates.multirag')!
    expect(mrGroup.options).toHaveLength(1)
    expect(mrGroup.options[0].value).toBe('multirag:bge-m3@Anthropic')
    const localGroup = result.find(g => g.label === 'knowledge.candidates.local')!
    expect(localGroup.options).toHaveLength(1)
    expect(localGroup.options[0].value).toBe('local:42:bge-large-zh')
  })

  it('returns empty array when both sources are empty', () => {
    expect(buildEmbeddingCandidates([], [])).toEqual([])
  })
})

describe('buildLayoutCandidates', () => {
  it('shows only built-in and enabled MultiRAG OCR, never provider presets', () => {
    const multirag: MultiRAGModel[] = [
      { name: 'mineru-x', factory: 'MinerU', type: 'ocr', status: '1', fullId: 'mineru-x@MinerU' },
      { name: 'paddle-x', factory: 'PaddleOCR', type: 'ocr', status: '0', fullId: 'paddle-x@PaddleOCR' },
    ]
    const localProviders: Provider[] = [
      {
        id: 7, key: 'paddleocr', name: 'PaddleOCR', protocol: 'paddleocr', authStyle: 'no_auth',
        baseUrl: '', description: '', descriptionEn: '', iconKey: '', builtin: true,
        lockedApiKey: '', attributes: {}, createdAt: '', updatedAt: '',
        defaultModels: [{ modelId: 'paddleocr', displayName: 'PaddleOCR', modelType: 'ocr' }],
        fields: [],
      },
    ]
    const result = buildLayoutCandidates(multirag)
    expect(result).toHaveLength(2)
    const builtinGroup = result.find(g => g.label === 'knowledge.candidates.builtin')!
    expect(builtinGroup.options.map(o => o.value)).toEqual(['builtin:DeepDOC', 'builtin:Plain Text'])
    const mrGroup = result.find(g => g.label === 'knowledge.candidates.ocr')!
    expect(mrGroup.options).toHaveLength(1)
    expect(mrGroup.options[0].value).toBe('multirag:mineru-x@MinerU')
    expect(result.find(g => g.label === 'knowledge.candidates.local')).toBeUndefined()
    expect(result.flatMap(g => g.options).some(o => o.value.includes('PaddleOCR'))).toBe(false)
    expect(localProviders[0].baseUrl).toBe('')
  })

  it('keeps distinct enabled models from one OCR factory and rejects stale local selections', () => {
    const multirag: MultiRAGModel[] = [
      { name: 'mineru-default', factory: 'MinerU', type: 'ocr', status: '1', fullId: 'mineru-default@MinerU' },
      { name: 'mineru-other', factory: 'MinerU', type: 'ocr', status: '1', fullId: 'mineru-other@MinerU' },
    ]
    const result = buildLayoutCandidates(multirag)
    expect(result.find(g => g.label === 'knowledge.candidates.ocr')?.options.map(option => option.rawValue)).toEqual(['mineru-default@MinerU', 'mineru-other@MinerU'])
    expect(isAllowedLayoutSelection('local:7:mineru', result)).toBe(false)
    expect(isAllowedLayoutSelection('multirag:MinerU', result)).toBe(false)
    expect(isAllowedLayoutSelection('multirag:mineru-other@MinerU', result)).toBe(true)
    expect(isAllowedLayoutSelection('DeepDOC', result)).toBe(true)
    expect(isAllowedLayoutSelection('OldParser', result, 'OldParser')).toBe(true)
    expect(isAllowedLayoutSelection('multirag:OldParser', result, 'OldParser')).toBe(true)
  })

  it('offers configured enabled vision models with their exact names, independent of embedding', () => {
    const result = buildLayoutCandidates([
      { name: 'vision@preview', factory: 'OpenAI', type: 'image2text', status: '1', fullId: 'vision@preview@OpenAI' },
      { name: 'disabled', factory: 'OpenAI', type: 'image2text', status: '0', fullId: 'disabled@OpenAI' },
      { name: 'embedding', factory: 'OpenAI', type: 'embedding', status: '1', fullId: 'embedding@OpenAI' },
    ])
    expect(result.find(group => group.label === 'knowledge.candidates.vision')?.options).toEqual([
      expect.objectContaining({ rawValue: 'vision@preview@OpenAI', value: 'multirag:vision@preview@OpenAI', kind: 'image2text' }),
    ])
    expect(result.flatMap(group => group.options).some(option => option.rawValue === 'embedding@OpenAI')).toBe(false)
  })

  it('does not offer unsupported exact OCR references or ambiguous cross-factory names', () => {
    const result = buildLayoutCandidates([
      { name: 'shared', factory: 'MinerU', type: 'ocr', status: '1', fullId: 'shared@MinerU' },
      { name: 'shared', factory: 'PaddleOCR', type: 'ocr', status: '0', fullId: 'shared@PaddleOCR' },
      { name: 'loader', factory: 'OpenDataLoader', type: 'ocr', status: '1', fullId: 'loader@OpenDataLoader' },
    ])
    expect(result).toHaveLength(1)
    expect(isAllowedLayoutSelection('shared@MinerU', result)).toBe(false)
    expect(isAllowedLayoutSelection('OpenDataLoader', result, 'OpenDataLoader')).toBe(true)
  })

  it('keeps the encoded selected value visible and disabled after a candidate disappears', () => {
    const available = buildLayoutCandidates([])
    const result = retainLayoutCandidates(available, 'removed@OpenAI', 'multirag:removed@OpenAI')
    expect(result[0].options).toEqual([expect.objectContaining({ value: 'multirag:removed@OpenAI', rawValue: 'removed@OpenAI', disabled: true, unavailable: true })])
    expect(isAllowedLayoutSelection('multirag:removed@OpenAI', result)).toBe(false)
    expect(isAllowedLayoutSelection('multirag:removed@OpenAI', result, 'removed@OpenAI')).toBe(true)
    expect(retainLayoutCandidates(available, 'removed@OpenAI', 'removed@OpenAI')[0].options).toEqual([
      expect.objectContaining({ value: 'removed@OpenAI', disabled: true }),
    ])
  })

  it.each(['builtin:retired@OpenAI', 'multirag:retired@OpenAI', 'local:7:retired@OpenAI'])('does not decode the source-like prefix of a persisted name: %s', (saved) => {
    const groups = buildLayoutCandidates([])
    expect(isAllowedLayoutSelection(saved, groups, saved)).toBe(true)
    expect(retainLayoutCandidates(groups, saved, saved)[0].options[0]).toMatchObject({ label: saved, rawValue: saved, value: saved })
    expect(retainLayoutCandidates(groups, saved, `multirag:${saved}`)[0].options).toEqual([
      expect.objectContaining({ label: saved, rawValue: saved, value: `multirag:${saved}`, disabled: true }),
    ])
  })
})

describe('decodeCandidateValue', () => {
  it('parses builtin', () => {
    expect(decodeCandidateValue('builtin:DeepDOC')).toEqual({
      source: 'builtin', rawValue: 'DeepDOC', providerId: null,
    })
  })
  it('parses multirag', () => {
    expect(decodeCandidateValue('multirag:bge-m3@ZHIPU-AI')).toEqual({
      source: 'multirag', rawValue: 'bge-m3@ZHIPU-AI', providerId: null,
    })
  })
  it('parses local', () => {
    expect(decodeCandidateValue('local:42:bge-large-zh')).toEqual({
      source: 'local', rawValue: 'bge-large-zh', providerId: 42,
    })
  })
  it('parses local with model_id containing colon', () => {
    expect(decodeCandidateValue('local:7:weird:model:id')).toEqual({
      source: 'local', rawValue: 'weird:model:id', providerId: 7,
    })
  })
  it('rejects non-integer provider id (float)', () => {
    expect(decodeCandidateValue('local:3.14:foo')).toBeNull()
  })
  it('rejects non-numeric provider id', () => {
    expect(decodeCandidateValue('local:abc:foo')).toBeNull()
  })
  it('rejects empty raw value', () => {
    expect(decodeCandidateValue('local:42:')).toBeNull()
  })
})

describe('BUILTIN_LAYOUTS', () => {
  it('is locked to DeepDOC and Plain Text', () => {
    expect([...BUILTIN_LAYOUTS]).toEqual(['DeepDOC', 'Plain Text'])
  })
})
