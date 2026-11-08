import { afterEach, describe, expect, it, vi } from 'vitest'
import apiClient, { parseApiError } from '@/api/client'
import i18next from '@/i18n'
import { memoryApi, memoryApplicationKey, memoryScopeKey } from '@/api/memory'

const scope = {
  tenantId: 'tenant-a',
  applicationId: 'app-a',
  userId: 'user-a',
  accountLifecycleId: 'lifecycle-a'
}
const mutation = { scope, operationId: 'op-a', expectedVersion: 3 }
const response = (data: unknown) => ({ data: { success: true, data } })

afterEach(() => {
  vi.restoreAllMocks()
})

describe('memory admin HTTP contract', () => {
  it('lists the delegated applications and actual subjects using their dedicated envelope', async () => {
    const applications = [
      {
        tenantId: 'tenant-a',
        applicationId: 'app-a',
        policyVersion: 'synthetic-v1',
        dataClass: 'synthetic'
      }
    ]
    const subjects = {
      subjects: [{ scope, state: 'active', revision: 1 }],
      missingScopeCount: 2,
      limited: false,
      limit: 1000
    }
    const get = vi
      .spyOn(apiClient, 'get')
      .mockResolvedValueOnce(response(applications))
      .mockResolvedValueOnce(response(subjects))
    await expect(memoryApi.applications()).resolves.toEqual(applications)
    await expect(
      memoryApi.subjects({
        tenantId: scope.tenantId,
        applicationId: scope.applicationId
      })
    ).resolves.toEqual(subjects)
    expect(get).toHaveBeenNthCalledWith(
      1,
      '/api/v1/admin/memory/applications',
      { signal: undefined }
    )
    expect(get).toHaveBeenNthCalledWith(2, '/api/v1/admin/memory/subjects', {
      params: { tenantId: 'tenant-a', applicationId: 'app-a' },
      signal: undefined
    })
  })

  it('sends every identity field for fact and operation reads, encoding the operation ID', async () => {
    const signal = new AbortController().signal
    const get = vi.spyOn(apiClient, 'get').mockResolvedValue(response([]))
    await memoryApi.facts(scope, signal)
    await memoryApi.operation(scope, 'op/1', signal)
    expect(get).toHaveBeenNthCalledWith(1, '/api/v1/admin/memory/facts', {
      params: scope,
      signal
    })
    expect(get).toHaveBeenNthCalledWith(
      2,
      '/api/v1/admin/memory/operations/op%2F1',
      { params: scope, signal }
    )
  })

  it('preserves the CAS version and idempotency key in edit and DELETE JSON bodies', async () => {
    const patch = vi
      .spyOn(apiClient, 'patch')
      .mockResolvedValue(response({ factId: 'fact/1', version: 4 }))
    const remove = vi
      .spyOn(apiClient, 'delete')
      .mockResolvedValue(response({ operationId: 'op-a' }))
    await memoryApi.update('fact/1', { ...mutation, content: '用户原话' })
    await memoryApi.delete('fact/1', mutation)
    expect(patch).toHaveBeenCalledWith('/api/v1/admin/memory/facts/fact%2F1', {
      ...mutation,
      content: '用户原话'
    })
    expect(remove).toHaveBeenCalledWith('/api/v1/admin/memory/facts/fact%2F1', {
      data: mutation
    })
  })

  it('uses only the Hub gateway for consent withdrawal and retrieval', async () => {
    const post = vi.spyOn(apiClient, 'post').mockResolvedValue(response({}))
    const revoke = { scope, consentRef: 'consent-a', operationId: 'op-a' }
    const recall = { scope, query: '用户喜欢什么？' }
    await memoryApi.revoke(revoke)
    await memoryApi.context(recall)
    expect(post).toHaveBeenNthCalledWith(
      1,
      '/api/v1/admin/memory/consents/revoke',
      revoke
    )
    expect(post).toHaveBeenNthCalledWith(
      2,
      '/api/v1/admin/memory/context',
      recall
    )
  })

  it('rejects an HTTP-success envelope containing a business failure', async () => {
    vi.spyOn(apiClient, 'delete').mockResolvedValue({
      data: { success: false, error: '版本冲突' }
    })
    await expect(memoryApi.delete('fact-a', mutation)).rejects.toThrow(
      '版本冲突'
    )
  })

  it('separates delimiter-containing identity tuples', () => {
    expect(
      memoryScopeKey({ ...scope, userId: 'u|l', accountLifecycleId: 'c' })
    ).not.toBe(
      memoryScopeKey({ ...scope, userId: 'u', accountLifecycleId: 'l|c' })
    )
    expect(
      memoryApplicationKey({ tenantId: 't|a', applicationId: 'b' })
    ).not.toBe(memoryApplicationKey({ tenantId: 't', applicationId: 'a|b' }))
  })

  it('translates stable memory error codes for the English page', async () => {
    await i18next.changeLanguage('en')
    try {
      expect(
        parseApiError({
          isAxiosError: true,
          response: {
            status: 409,
            data: { code: 'memory_conflict', error: '版本冲突' }
          }
        })
      ).toBe(
        'The memory version, task or operation ID conflicts. Refresh before retrying.'
      )
      expect(
        parseApiError({
          isAxiosError: true,
          response: {
            status: 501,
            data: { code: 'MEMORY_OPERATION_UNSUPPORTED', error: '本期不支持' }
          }
        })
      ).toBe('This memory operation is unavailable in the current phase.')
    } finally {
      await i18next.changeLanguage('zh')
    }
  })
})
