import axios from 'axios'
import type { AxiosInstance, AxiosResponse, AxiosError, InternalAxiosRequestConfig } from 'axios'
import { loginRedirectUrl } from '@/lib/redirect'
// 非组件上下文（axios 拦截器/纯函数）：语言切换后下一次生成生效
import i18next from '@/i18n'

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
        window.location.href = loginRedirectUrl(window.location.pathname, window.location.search, window.location.hash)
        return Promise.reject(error)
      }

      const refreshToken = getRefreshToken()
      if (!refreshToken) {
        clearTokens()
        window.location.href = loginRedirectUrl(window.location.pathname, window.location.search, window.location.hash)
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
        window.location.href = loginRedirectUrl(window.location.pathname, window.location.search, window.location.hash)
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
    throw new Error(body.error ?? body.message ?? i18next.t('apiErrors.requestFailed'))
  }
  return body.data as T
}

/**
 * Convert any thrown value into a user-facing localized message.
 * Used as the global mutation onError handler in QueryClient.
 *
 * Backend error envelope is `{ success: false, error: "..." }`. We also
 * accept `message` for compatibility with third-party services that use
 * the more common REST shape.
 */
// snake_case 稳定码 → camelCase 资源键（invalid_parameter → invalidParameter）。
function errCodeToKey(code: string): string {
  return code.replace(/_([a-z])/g, (_, ch: string) => ch.toUpperCase())
}

export function parseApiError(err: unknown): string {
  if (axios.isAxiosError(err)) {
    const status = err.response?.status
    const data = err.response?.data as { error?: string; message?: string; code?: string } | undefined

    // 稳定码翻译（#149 P5 双写契约 / #175 勘误口径）：仅 en 模式且码命中
    // apiErrors 键时用翻译——zh 模式下后端 error 本就是更具体的中文原文，
    // 通用翻译会降级信息；语义码未建键的过渡期 exists 检测后回落原文。
    if (data?.code && i18next.language !== 'zh') {
      const key = `apiErrors.${errCodeToKey(data.code)}`
      if (i18next.exists(key)) return i18next.t(key)
    }

    // Prefer the backend's `error` field; fall back to `message` for
    // third-party / proxy responses.
    if (data?.error) return data.error
    if (data?.message) return data.message

    if (status === 401) return i18next.t('apiErrors.unauthorized')
    if (status === 403) return i18next.t('apiErrors.forbidden')
    if (status === 404) return i18next.t('apiErrors.notFound')
    if (status === 409) return i18next.t('apiErrors.conflict')
    if (status === 429) return i18next.t('apiErrors.rateLimited')
    if (status === 400) return i18next.t('apiErrors.invalidParameter')
    if (status && status >= 500) return i18next.t('apiErrors.serverBusy')
    if (err.code === 'ECONNABORTED') return i18next.t('apiErrors.timeout')
    if (!err.response) return i18next.t('apiErrors.networkError')
  }
  if (err instanceof Error) return err.message
  return i18next.t('apiErrors.operationFailed')
}
