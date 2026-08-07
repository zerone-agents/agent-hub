import { describe, it, expect } from 'vitest'
import {
  buildEmbeddingCandidates,
  buildLayoutCandidates,
  decodeCandidateValue,
  BUILTIN_LAYOUTS,
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
    const mrGroup = result.find(g => g.label === 'MultiRAG 已有')!
    expect(mrGroup.options).toHaveLength(1)
    expect(mrGroup.options[0].value).toBe('multirag:bge-m3@Anthropic')
    const localGroup = result.find(g => g.label === '本地待同步')!
    expect(localGroup.options).toHaveLength(1)
    expect(localGroup.options[0].value).toBe('local:42:bge-large-zh')
  })

  it('returns empty array when both sources are empty', () => {
    expect(buildEmbeddingCandidates([], [])).toEqual([])
  })
})

describe('buildLayoutCandidates', () => {
  it('shows 3 groups: builtin + multirag + local', () => {
    const multirag: MultiRAGModel[] = [
      { name: 'mineru-x', factory: 'MinerU', type: 'ocr', status: '1', fullId: 'mineru-x@MinerU' },
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
    const result = buildLayoutCandidates(multirag, localProviders)
    expect(result).toHaveLength(3)
    const builtinGroup = result.find(g => g.label === '内置')!
    expect(builtinGroup.options.map(o => o.value)).toEqual(['builtin:DeepDOC', 'builtin:Plain Text'])
    const mrGroup = result.find(g => g.label === 'MultiRAG 已有')!
    expect(mrGroup.options).toHaveLength(1)
    expect(mrGroup.options[0].value).toBe('multirag:MinerU')
    const localGroup = result.find(g => g.label === '本地待同步')!
    expect(localGroup.options).toHaveLength(1)
    expect(localGroup.options[0].value).toBe('local:7:paddleocr')
  })

  it('dedupes local MinerU when already in MultiRAG', () => {
    const multirag: MultiRAGModel[] = [
      { name: 'mineru-default', factory: 'MinerU', type: 'ocr', status: '1', fullId: 'mineru-default@MinerU' },
    ]
    const localProviders: Provider[] = [
      {
        id: 7, key: 'mineru', name: 'MinerU', protocol: 'mineru', authStyle: 'api_key',
        baseUrl: '', description: '', descriptionEn: '', iconKey: '', builtin: true,
        lockedApiKey: '', attributes: {}, createdAt: '', updatedAt: '',
        defaultModels: [{ modelId: 'mineru', displayName: 'MinerU', modelType: 'ocr' }],
        fields: [],
      },
    ]
    const result = buildLayoutCandidates(multirag, localProviders)
    const localGroup = result.find(g => g.label === '本地待同步')
    // MinerU factory is already in MultiRAG, so local entry is dropped.
    // If local group exists, it must be empty.
    if (localGroup) {
      expect(localGroup.options).toHaveLength(0)
    }
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
