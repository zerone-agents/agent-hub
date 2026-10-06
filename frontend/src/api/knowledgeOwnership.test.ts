import axios, { AxiosError, type AxiosResponse, type InternalAxiosRequestConfig } from 'axios'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { waitFor } from '@testing-library/react'
import apiClient from './client'
import { knowledgeApi, type KnowledgeWriteOwner } from './knowledge'

const originalAdapter = apiClient.defaults.adapter
let current = true
function owner(): KnowledgeWriteOwner {
  return { signal: new AbortController().signal, isCurrent: () => current, assertCurrent: () => { if (!current) throw new Error('owner changed') } }
}
function response(config: InternalAxiosRequestConfig): AxiosResponse<unknown> {
  return { data: JSON.stringify({ success: true, data: { id: 'owner-a-kb', name: 'A draft' } }), status: 200, statusText: 'OK', headers: {}, config }
}
beforeEach(() => { current = true; localStorage.setItem('access_token', 'token-a') })
afterEach(() => { apiClient.defaults.adapter = originalAdapter; localStorage.clear(); vi.restoreAllMocks() })

it('rechecks ownership at dispatch after the token interceptor runs', async () => {
  const adapter = vi.fn((config: InternalAxiosRequestConfig) => Promise.resolve(response(config)))
  apiClient.defaults.adapter = adapter
  const pending = knowledgeApi.datasets.create({ name: 'A private draft' }, owner())
  current = false
  localStorage.setItem('access_token', 'token-b')
  await expect(pending).rejects.toThrow('owner changed')
  expect(adapter).not.toHaveBeenCalled()
})

it('keeps the original JSON contract and credential on a valid owned request', async () => {
  const adapter = vi.fn((config: InternalAxiosRequestConfig) => Promise.resolve(response(config)))
  apiClient.defaults.adapter = adapter
  await expect(knowledgeApi.datasets.create({ name: 'A draft' }, owner())).resolves.toMatchObject({ id: 'owner-a-kb' })
  const config = adapter.mock.calls[0][0]
  expect(config.headers.get('Authorization')).toBe('Bearer token-a')
  expect(JSON.parse(config.data as string)).toEqual({ name: 'A draft' })
})

it('does not issue a readback GET under another account after an owned PUT', async () => {
  let release!: (value: { data: { success: boolean; data: { id: string } } }) => void
  vi.spyOn(apiClient, 'put').mockImplementation(() => new Promise((resolve) => { release = resolve }))
  const get = vi.spyOn(apiClient, 'get')
  const pending = knowledgeApi.datasets.update('owner-a-kb', { name: 'A draft' }, owner())
  current = false
  localStorage.setItem('access_token', 'token-b')
  release({ data: { success: true, data: { id: 'owner-a-kb' } } })
  await expect(pending).rejects.toThrow('owner changed')
  expect(get).not.toHaveBeenCalled()
})

it('does not refresh B credentials for a late unauthorized response from A', async () => {
  let reject!: (error: AxiosError) => void
  const adapter = vi.fn((_config: InternalAxiosRequestConfig) => new Promise<AxiosResponse<unknown>>((_resolve, rejectPromise) => { reject = rejectPromise }))
  apiClient.defaults.adapter = adapter
  const refresh = vi.spyOn(axios, 'post').mockResolvedValue({ data: { success: true, data: { accessToken: 'unexpected', refreshToken: 'unexpected' } } })
  const pending = knowledgeApi.datasets.create({ name: 'A private draft' }, owner())
  await waitFor(() => { expect(adapter).toHaveBeenCalledTimes(1) })
  current = false
  localStorage.setItem('access_token', 'token-b')
  localStorage.setItem('refresh_token', 'refresh-b')
  const config = adapter.mock.calls[0][0]
  reject(new AxiosError('unauthorized', 'ERR_BAD_REQUEST', config, undefined, { ...response(config), status: 401 }))
  await expect(pending).rejects.toThrow('owner changed')
  expect(refresh).not.toHaveBeenCalled()
  expect(localStorage.getItem('access_token')).toBe('token-b')
})
