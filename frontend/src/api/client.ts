import axios from 'axios'
import type { AxiosInstance, AxiosResponse, AxiosError, InternalAxiosRequestConfig } from 'axios'

const TOKEN_KEY = 'access_token'
const REFRESH_TOKEN_KEY = 'refresh_token'

export function getAccessToken(): string | null {
  return localStorage.getItem(TOKEN_KEY)
}

export function getRefreshToken(): string | null {
  return localStorage.getItem(REFRESH_TOKEN_KEY)
}

export function setTokens(accessToken: string, refreshToken?: string) {
  localStorage.setItem(TOKEN_KEY, accessToken)
  if (refreshToken) {
    localStorage.setItem(REFRESH_TOKEN_KEY, refreshToken)
  }
}

export function clearTokens() {
  localStorage.removeItem(TOKEN_KEY)
  localStorage.removeItem(REFRESH_TOKEN_KEY)
}

const apiClient: AxiosInstance = axios.create({
  baseURL: '',
  timeout: 10000,
  headers: {
    'Content-Type': 'application/json'
  }
})

apiClient.interceptors.request.use(
  (config: InternalAxiosRequestConfig) => {
    const token = getAccessToken()
    if (token) {
      config.headers.Authorization = `Bearer ${token}`
    }
    return config
  }
)

apiClient.interceptors.response.use(
  (response: AxiosResponse) => {
    return response
  },
  async (error: AxiosError) => {
    const originalRequest = error.config as (InternalAxiosRequestConfig & { headers: Record<string, string> }) | undefined

    // 凭证校验类端点的 401 语义是"密码错误"，不是"会话过期"——
    // 不能走 refresh / 强制跳登录页，否则错误提示会被整页刷新吞掉。
    const CREDENTIAL_CHECK_PATHS = ['/auth/login', '/auth/setup', '/auth/register', '/auth/change-password']
    const isCredentialCheck = CREDENTIAL_CHECK_PATHS.some((p) => originalRequest?.url?.includes(p))

    if (error.response?.status === 401 && originalRequest && !isCredentialCheck) {
      if (originalRequest.headers['X-Refresh-Attempt']) {
        clearTokens()
        window.location.href = '/static/login'
        return Promise.reject(error)
      }

      const refreshToken = getRefreshToken()
      if (!refreshToken) {
        clearTokens()
        window.location.href = '/static/login'
        return Promise.reject(error)
      }

      try {
        const response = await axios.post('/auth/refresh', {
          refresh_token: refreshToken
        })

        const body = response.data as { success: boolean; data?: { accessToken: string; refreshToken: string } }
        if (body.success) {
          const { accessToken, refreshToken: newRefreshToken } = body.data ?? { accessToken: '', refreshToken: '' }
          setTokens(accessToken, newRefreshToken)

          originalRequest.headers['X-Refresh-Attempt'] = 'true'
          originalRequest.headers.Authorization = `Bearer ${accessToken}`
          return await apiClient(originalRequest)
        }
      } catch {
        clearTokens()
        window.location.href = '/static/login'
        return Promise.reject(error)
      }
    }

    return Promise.reject(error)
  }
)

export default apiClient

/**
 * Backend response envelope. All admin API responses share this shape:
 * `{ success: boolean, data?: T, message?: string, error?: string }`.
 */
export interface ApiEnvelope<T = unknown> {
  success: boolean
  data?: T
  message?: string
  error?: string
}

/**
 * Unwrap an axios response into its data payload, throwing on backend errors.
 * Eliminates `any` propagation from `res.data` across query/mutation hooks.
 */
// eslint-disable-next-line @typescript-eslint/no-unnecessary-type-parameters -- T is the caller-specified return type; the rule doesn't recognize this ergonomic pattern
export function unwrapResponse<T>(res: { data: unknown }): T {
  const body = res.data as ApiEnvelope<T>
  if (!body.success) {
    throw new Error(body.error ?? body.message ?? '请求失败')
  }
  return body.data as T
}

/**
 * Convert any thrown value into a user-facing zh-CN message.
 * Used as the global mutation onError handler in QueryClient.
 *
 * Backend error envelope is `{ success: false, error: "..." }`. We also
 * accept `message` for compatibility with third-party services that use
 * the more common REST shape.
 */
export function parseApiError(err: unknown): string {
  if (axios.isAxiosError(err)) {
    const status = err.response?.status
    const data = err.response?.data as { error?: string; message?: string } | undefined

    // Prefer the backend's `error` field; fall back to `message` for
    // third-party / proxy responses.
    if (data?.error) return data.error
    if (data?.message) return data.message

    if (status === 401) return '登录已过期，请重新登录'
    if (status === 403) return '没有权限执行此操作'
    if (status === 404) return '资源不存在或已被删除'
    if (status && status >= 500) return '服务器繁忙，请稍后重试'
    if (err.code === 'ECONNABORTED') return '请求超时，请检查网络'
    if (!err.response) return '网络连接失败'
  }
  if (err instanceof Error) return err.message
  return '操作失败，请重试'
}
