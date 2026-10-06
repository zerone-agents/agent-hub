import axios, { AxiosError, type AxiosResponse, type InternalAxiosRequestConfig } from 'axios'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import apiClient, { clearTokens, getAccessToken, getRefreshToken, setTokens } from './client'

const originalAdapter = apiClient.defaults.adapter
const interceptorIds: number[] = []
let redirects: string[] = []
function response(config: InternalAxiosRequestConfig, status = 200): AxiosResponse<unknown> {
  return { data: { success: true, data: { selected: 'a' } }, status, statusText: String(status), headers: {}, config }
}
function unauthorized(config: InternalAxiosRequestConfig) {
  return new AxiosError('expired', 'ERR_BAD_REQUEST', config, undefined, response(config, 401))
}
function refreshed() {
  return { data: { success: true, data: { accessToken: 'access-a2', refreshToken: 'refresh-a2' } } }
}
function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (error: unknown) => void
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}
function credentials() { return [getAccessToken(), getRefreshToken()] }
beforeEach(() => {
  redirects = []
  const location = { pathname: '/static/knowledge', search: '?page=2', hash: '#scope', get href() { return '/static/knowledge' }, set href(value: string) { redirects.push(value) } }
  vi.stubGlobal('window', { location })
  clearTokens()
  setTokens('access-a', 'refresh-a')
})
afterEach(() => {
  apiClient.defaults.adapter = originalAdapter
  interceptorIds.forEach((id) => { apiClient.interceptors.request.eject(id) })
  interceptorIds.length = 0
  clearTokens()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

it('rejects an A 401 arriving after B login without using B refresh credentials', async () => {
  const gate = deferred<AxiosResponse<unknown>>()
  const adapter = vi.fn((_config: InternalAxiosRequestConfig) => gate.promise)
  apiClient.defaults.adapter = adapter
  const refresh = vi.spyOn(axios, 'post')
  const pending = apiClient.get('/selected-a').catch((error: unknown) => error)
  await vi.waitFor(() => { expect(adapter).toHaveBeenCalledTimes(1) })
  const config = adapter.mock.calls[0][0]
  expect(config.headers.get('Authorization')).toBe('Bearer access-a')
  setTokens('access-b', 'refresh-b')
  gate.reject(unauthorized(config))
  expect(await pending).toBeInstanceOf(AxiosError)
  expect(refresh).not.toHaveBeenCalled()
  expect(credentials()).toEqual(['access-b', 'refresh-b'])
  expect(redirects).toEqual([])
})

it.each(['success', 'network failure', 'business failure', 'malformed success'])('does not replace or clear B after an old refresh returns %s', async (outcome) => {
  const gate = deferred<ReturnType<typeof refreshed>>()
  const refresh = vi.spyOn(axios, 'post').mockImplementation(() => gate.promise)
  const adapter = vi.fn((config: InternalAxiosRequestConfig) => Promise.reject(unauthorized(config)))
  apiClient.defaults.adapter = adapter
  const pending = apiClient.get('/selected-a').catch((error: unknown) => error)
  await vi.waitFor(() => { expect(refresh).toHaveBeenCalledTimes(1) })
  expect(refresh.mock.calls[0][1]).toEqual({ refresh_token: 'refresh-a' })
  setTokens('access-b', 'refresh-b')
  if (outcome === 'network failure') gate.reject(new Error('offline'))
  else if (outcome === 'business failure') gate.resolve({ data: { success: false, data: { accessToken: '', refreshToken: '' } } })
  else if (outcome === 'malformed success') gate.resolve({ data: { success: true, data: { accessToken: '', refreshToken: '' } } })
  else gate.resolve(refreshed())
  expect(await pending).toBeInstanceOf(AxiosError)
  expect(adapter).toHaveBeenCalledTimes(1)
  expect(credentials()).toEqual(['access-b', 'refresh-b'])
  expect(redirects).toEqual([])
})

it('recognizes a new login generation even if a caller supplies identical token strings', async () => {
  const gate = deferred<ReturnType<typeof refreshed>>()
  const refresh = vi.spyOn(axios, 'post').mockImplementation(() => gate.promise)
  const adapter = vi.fn((config: InternalAxiosRequestConfig) => Promise.reject(unauthorized(config)))
  apiClient.defaults.adapter = adapter
  const pending = apiClient.get('/selected-a').catch((error: unknown) => error)
  await vi.waitFor(() => { expect(refresh).toHaveBeenCalledTimes(1) })
  setTokens('access-a', 'refresh-a')
  gate.resolve(refreshed())
  await pending
  expect(adapter).toHaveBeenCalledTimes(1)
  expect(credentials()).toEqual(['access-a', 'refresh-a'])
  expect(redirects).toEqual([])
})

it.each(['success', 'failure'])('does not mutate credentials or replay an aborted request after refresh %s', async (outcome) => {
  const gate = deferred<ReturnType<typeof refreshed>>()
  const refresh = vi.spyOn(axios, 'post').mockImplementation(() => gate.promise)
  const adapter = vi.fn((config: InternalAxiosRequestConfig) => Promise.reject(unauthorized(config)))
  apiClient.defaults.adapter = adapter
  const controller = new AbortController()
  const pending = apiClient.get('/selected-a', { signal: controller.signal }).catch((error: unknown) => error)
  await vi.waitFor(() => { expect(refresh).toHaveBeenCalledTimes(1) })
  expect(refresh.mock.calls[0][2]?.signal).toBe(controller.signal)
  controller.abort()
  if (outcome === 'failure') gate.reject(new Error('aborted refresh'))
  else gate.resolve(refreshed())
  await pending
  expect(adapter).toHaveBeenCalledTimes(1)
  expect(credentials()).toEqual(['access-a', 'refresh-a'])
  expect(redirects).toEqual([])
})

it.each(['different tokens', 'identical new generation'])('blocks replay before its adapter when a new login uses %s', async (scenario) => {
  interceptorIds.push(apiClient.interceptors.request.use((config) => {
    if (config.headers.get('X-Refresh-Attempt')) {
      if (scenario === 'different tokens') setTokens('access-b', 'refresh-b')
      else setTokens('access-a2', 'refresh-a2')
    }
    return config
  }))
  const refresh = vi.spyOn(axios, 'post').mockResolvedValue(refreshed())
  const adapter = vi.fn((config: InternalAxiosRequestConfig) => Promise.reject(unauthorized(config)))
  apiClient.defaults.adapter = adapter
  const error: unknown = await apiClient.get('/selected-a').catch((reason: unknown) => reason)
  expect(axios.isCancel(error)).toBe(true)
  expect(refresh).toHaveBeenCalledTimes(1)
  expect(adapter).toHaveBeenCalledTimes(1)
  expect(credentials()).toEqual(scenario === 'different tokens' ? ['access-b', 'refresh-b'] : ['access-a2', 'refresh-a2'])
  expect(redirects).toEqual([])
})

it.each([401, 503])('leaves B credentials intact after A replay returns late %s', async (status) => {
  const gate = deferred<AxiosResponse<unknown>>()
  const adapter = vi.fn((config: InternalAxiosRequestConfig) => config.headers.get('X-Refresh-Attempt') ? gate.promise : Promise.reject(unauthorized(config)))
  apiClient.defaults.adapter = adapter
  const refresh = vi.spyOn(axios, 'post').mockResolvedValue(refreshed())
  const pending = apiClient.get('/selected-a').catch((error: unknown) => error)
  await vi.waitFor(() => { expect(adapter).toHaveBeenCalledTimes(2) })
  const replay = adapter.mock.calls[1][0]
  expect(replay.headers.get('Authorization')).toBe('Bearer access-a2')
  setTokens('access-b', 'refresh-b')
  gate.reject(new AxiosError('late replay', 'ERR_BAD_RESPONSE', replay, undefined, response(replay, status)))
  expect(await pending).toBeInstanceOf(AxiosError)
  expect(refresh).toHaveBeenCalledTimes(1)
  expect(credentials()).toEqual(['access-b', 'refresh-b'])
  expect(redirects).toEqual([])
})

it('still refreshes and replays the same request normally for the initiating account', async () => {
  const refresh = vi.spyOn(axios, 'post').mockResolvedValue(refreshed())
  const adapter = vi.fn((config: InternalAxiosRequestConfig) => config.headers.get('X-Refresh-Attempt') ? Promise.resolve(response(config)) : Promise.reject(unauthorized(config)))
  apiClient.defaults.adapter = adapter
  const result = await apiClient.get('/selected-a', { params: { dataset_ids: 'a-only' } })
  expect(result.status).toBe(200)
  expect(adapter.mock.calls.map(([config]) => ({ url: config.url, token: config.headers.get('Authorization'), ids: config.params?.dataset_ids }))).toEqual([
    { url: '/selected-a', token: 'Bearer access-a', ids: 'a-only' },
    { url: '/selected-a', token: 'Bearer access-a2', ids: 'a-only' },
  ])
  expect(refresh.mock.calls[0][1]).toEqual({ refresh_token: 'refresh-a' })
  expect(credentials()).toEqual(['access-a2', 'refresh-a2'])
  expect(redirects).toEqual([])
})

it.each(['refresh failure', 'replay 401'])('expires only the active initiating session on %s', async (failure) => {
  const refresh = vi.spyOn(axios, 'post')
  if (failure === 'refresh failure') refresh.mockRejectedValue(new Error('invalid refresh'))
  else refresh.mockResolvedValue(refreshed())
  const adapter = vi.fn((config: InternalAxiosRequestConfig) => Promise.reject(unauthorized(config)))
  apiClient.defaults.adapter = adapter
  await expect(apiClient.get('/selected-a')).rejects.toThrow('expired')
  expect(credentials()).toEqual([null, null])
  expect(redirects).toEqual(['/static/login?redirect=%2Fknowledge%3Fpage%3D2%23scope'])
  expect(adapter).toHaveBeenCalledTimes(failure === 'refresh failure' ? 1 : 2)
  expect(refresh).toHaveBeenCalledTimes(1)
})

it('does not refresh a failed credential check or clear an existing session', async () => {
  const refresh = vi.spyOn(axios, 'post')
  apiClient.defaults.adapter = async (config) => Promise.reject(unauthorized(config))
  await expect(apiClient.post('/auth/login', { username: 'wrong' })).rejects.toThrow('expired')
  expect(refresh).not.toHaveBeenCalled()
  expect(credentials()).toEqual(['access-a', 'refresh-a'])
  expect(redirects).toEqual([])
})

it('never retains an A refresh token for an access-only B login', () => {
  setTokens('access-b')
  expect(credentials()).toEqual(['access-b', null])
})

it('compares the refresh credential even when access token and local generation have not changed', async () => {
  const gate = deferred<ReturnType<typeof refreshed>>()
  const refresh = vi.spyOn(axios, 'post').mockImplementation(() => gate.promise)
  const adapter = vi.fn((config: InternalAxiosRequestConfig) => Promise.reject(unauthorized(config)))
  apiClient.defaults.adapter = adapter
  const pending = apiClient.get('/selected-a').catch((error: unknown) => error)
  await vi.waitFor(() => { expect(refresh).toHaveBeenCalledTimes(1) })
  localStorage.setItem('refresh_token', 'refresh-new-session')
  gate.resolve(refreshed())
  await pending
  expect(adapter).toHaveBeenCalledTimes(1)
  expect(credentials()).toEqual(['access-a', 'refresh-new-session'])
  expect(redirects).toEqual([])
})

it('honors CancelToken while waiting for refresh without discarding the active login', async () => {
  const gate = deferred<ReturnType<typeof refreshed>>()
  const refresh = vi.spyOn(axios, 'post').mockImplementation(() => gate.promise)
  const adapter = vi.fn((config: InternalAxiosRequestConfig) => Promise.reject(unauthorized(config)))
  apiClient.defaults.adapter = adapter
  const source = axios.CancelToken.source()
  const pending = apiClient.get('/selected-a', { cancelToken: source.token }).catch((error: unknown) => error)
  await vi.waitFor(() => { expect(refresh).toHaveBeenCalledTimes(1) })
  expect(refresh.mock.calls[0][2]?.cancelToken).toBe(source.token)
  source.cancel('Scope changed')
  gate.resolve(refreshed())
  await pending
  expect(adapter).toHaveBeenCalledTimes(1)
  expect(credentials()).toEqual(['access-a', 'refresh-a'])
  expect(redirects).toEqual([])
})

it('rejects a malformed active refresh response instead of installing empty credentials', async () => {
  vi.spyOn(axios, 'post').mockResolvedValue({ data: { success: true, data: { accessToken: 'missing-refresh' } } })
  apiClient.defaults.adapter = async (config) => Promise.reject(unauthorized(config))
  await expect(apiClient.get('/selected-a')).rejects.toThrow('expired')
  expect(credentials()).toEqual([null, null])
  expect(redirects).toHaveLength(1)
})

it('requires a real successful refresh envelope instead of a truthy business error', async () => {
  vi.spyOn(axios, 'post').mockResolvedValue({ data: { success: 'false', data: { accessToken: 'unexpected', refreshToken: 'unexpected' } } })
  const adapter = vi.fn((config: InternalAxiosRequestConfig) => Promise.reject(unauthorized(config)))
  apiClient.defaults.adapter = adapter
  await expect(apiClient.get('/selected-a')).rejects.toThrow('expired')
  expect(adapter).toHaveBeenCalledTimes(1)
  expect(credentials()).toEqual([null, null])
})
