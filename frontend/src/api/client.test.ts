import { describe, it, expect } from 'vitest'
import axios from 'axios'
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

  it('returns fallback for unknown shapes', () => {
    expect(parseApiError('weird')).toBe('操作失败，请重试')
  })
})
