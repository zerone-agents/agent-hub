import { describe, it, expect, afterEach } from 'vitest'
import axios from 'axios'
import i18next from '@/i18n'
import { parseApiError } from './client'

describe('parseApiError', () => {
  it('returns backend error field (real backend shape)', () => {
    const err = new axios.AxiosError('bad', 'ERR_BAD_REQUEST', undefined, undefined, {
      status: 400,
      data: { success: false, error: '技能 标识只能包含字母、数字、点、下划线和横线' }
    } as any)
    expect(parseApiError(err)).toBe('技能 标识只能包含字母、数字、点、下划线和横线')
  })

  it('falls back to message field for third-party services', () => {
    const err = new axios.AxiosError('bad', 'ERR_BAD_REQUEST', undefined, undefined, {
      status: 400,
      data: { message: '名称已存在' }
    } as any)
    expect(parseApiError(err)).toBe('名称已存在')
  })

  it('prefers error over message when both present', () => {
    const err = new axios.AxiosError('bad', 'ERR_BAD_REQUEST', undefined, undefined, {
      status: 400,
      data: { error: 'backend-msg', message: 'third-party-msg' }
    } as any)
    expect(parseApiError(err)).toBe('backend-msg')
  })

  it('returns zh-CN message for 401', () => {
    const err = new axios.AxiosError('unauth', 'ERR_BAD_REQUEST', undefined, undefined, {
      status: 401,
      data: {}
    } as any)
    expect(parseApiError(err)).toBe('登录已过期，请重新登录')
  })

  it('returns zh-CN message for 403', () => {
    const err = new axios.AxiosError('forbidden', 'ERR_BAD_REQUEST', undefined, undefined, {
      status: 403,
      data: {}
    } as any)
    expect(parseApiError(err)).toBe('没有权限执行此操作')
  })

  it('returns zh-CN message for 404', () => {
    const err = new axios.AxiosError('missing', 'ERR_BAD_REQUEST', undefined, undefined, {
      status: 404,
      data: {}
    } as any)
    expect(parseApiError(err)).toBe('资源不存在或已被删除')
  })

  it('returns server-busy message for 5xx', () => {
    const err = new axios.AxiosError('boom', 'ERR_BAD_RESPONSE', undefined, undefined, {
      status: 500,
      data: {}
    } as any)
    expect(parseApiError(err)).toBe('服务器繁忙，请稍后重试')
  })

  it('returns timeout message for ECONNABORTED', () => {
    const err = new axios.AxiosError('timeout', 'ECONNABORTED')
    expect(parseApiError(err)).toBe('请求超时，请检查网络')
  })

  it('returns network-failure message when no response', () => {
    const err = new axios.AxiosError('network', undefined)
    expect(parseApiError(err)).toBe('网络连接失败')
  })

  it('returns generic Error.message for non-axios errors', () => {
    expect(parseApiError(new Error('boom'))).toBe('boom')
  })

  it('uses the product service name for upstream error messages', () => {
    const err = new axios.AxiosError('bad', 'ERR_BAD_RESPONSE', undefined, undefined, {
      status: 502,
      data: { error: 'MultiRAG error 102: parser unavailable' },
    } as any)
    expect(parseApiError(err)).toBe('知识库服务 error 102: parser unavailable')
    expect(parseApiError(new Error('RAGFlow is unavailable'))).toBe('知识库服务 is unavailable')
  })

  it('returns fallback for unknown shapes', () => {
    expect(parseApiError('weird')).toBe('操作失败，请重试')
  })
})

describe('parseApiError 稳定码翻译（#201 B 档：en 模式按码插值翻译）', () => {
  const prevLang = i18next.language
  afterEach(async () => {
    await i18next.changeLanguage(prevLang)
  })

  it('en 模式：命中码键时用插值翻译（params 透传）', async () => {
    await i18next.changeLanguage('en')
    const err = new axios.AxiosError('bad', 'ERR_BAD_REQUEST', undefined, undefined, {
      status: 400,
      data: { success: false, error: "子 Agent 'ghost' 不存在", code: 'subagent_not_found', params: { name: 'ghost' } }
    } as any)
    expect(parseApiError(err)).toBe('Subagent "ghost" does not exist')
  })

  it('en 模式：404 sentinel 码 agent_not_found 无插值（通用文案）', async () => {
    await i18next.changeLanguage('en')
    const err = new axios.AxiosError('bad', 'ERR_BAD_REQUEST', undefined, undefined, {
      status: 404,
      data: { success: false, error: 'Agent 不存在', code: 'agent_not_found' }
    } as any)
    expect(parseApiError(err)).toBe('Agent does not exist or has been deleted')
  })

  it('en 模式：400 引用拆键 agent_reference_not_found 带 name 插值（PR #204 评审）', async () => {
    await i18next.changeLanguage('en')
    const err = new axios.AxiosError('bad', 'ERR_BAD_REQUEST', undefined, undefined, {
      status: 400,
      data: { success: false, error: "Agent 'ghost-parent' 不存在", code: 'agent_reference_not_found', params: { name: 'ghost-parent' } }
    } as any)
    expect(parseApiError(err)).toBe('Agent "ghost-parent" does not exist')
  })

  it('en 键存在性契约：agent 域全部稳定码都有 apiErrors 键（防注册表/键漂移）', async () => {
    await i18next.changeLanguage('en')
    // 与 domain/agent/errors.go 码常量清单保持同步（26 项，#205 长尾补齐）
    const codes = [
      'agent_name_required', 'agent_name_too_long', 'agent_name_invalid', 'agent_name_exists',
      'agent_not_found', 'agent_reference_not_found', 'subagent_not_found', 'subagent_self_reference',
      'system_prompt_required', 'config_key_renamed', 'invalid_permission_mode', 'max_turns_negative',
      'max_turns_too_large', 'provider_id_not_found', 'model_selection_not_found', 'model_not_found',
      'model_type_mismatch', 'config_required', 'icon_field_too_long', 'field_overrides_requires_provider',
      'field_overrides_invalid_key', 'disallowed_tools_too_many', 'disallowed_tools_invalid_item',
      'disallowed_tools_entry_too_long', 'disallowed_tools_duplicate', 'provider_not_bound'
    ]
    for (const code of codes) {
      const key = `apiErrors.${code.replace(/_([a-z])/g, (_, ch: string) => ch.toUpperCase())}`
      expect(i18next.exists(key), `missing en key ${key} for code ${code}`).toBe(true)
    }
  })

  it('en 模式：旧 key 哨兵码插值 oldKey/newKey', async () => {
    await i18next.changeLanguage('en')
    const err = new axios.AxiosError('bad', 'ERR_BAD_REQUEST', undefined, undefined, {
      status: 400,
      data: {
        success: false,
        error: '配置项 maxSessionTurns 已更名为 maxSessionQueries，请更新调用方后重试',
        code: 'config_key_renamed',
        params: { oldKey: 'maxSessionTurns', newKey: 'maxSessionQueries' }
      }
    } as any)
    expect(parseApiError(err)).toBe('Config key maxSessionTurns has been renamed to maxSessionQueries, please update and retry')
  })

  it('zh 模式：带码错误仍直出后端中文原文（信息不降级）', async () => {
    await i18next.changeLanguage('zh')
    const err = new axios.AxiosError('bad', 'ERR_BAD_REQUEST', undefined, undefined, {
      status: 400,
      data: { success: false, error: "子 Agent 'ghost' 不存在", code: 'subagent_not_found', params: { name: 'ghost' } }
    } as any)
    expect(parseApiError(err)).toBe("子 Agent 'ghost' 不存在")
  })
})
