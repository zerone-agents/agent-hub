import { useEffect, useState } from 'react'
import { Input, Spin } from 'antd'
import { createStyles } from 'antd-style'
import { useNavigate } from 'react-router'
import PasswordInput from '@/components/PasswordInput'
import { authApi } from '@/api/auth'
import { getAccessToken, parseApiError } from '@/api/client'
import { useUserInfo } from '@/queries/useUserInfo'
import { useAuthStore } from '@/stores/auth'
import LoadingState from '@/components/LoadingState'
import { tokens as t } from '@/styles/tokens'
import ThemeControls from '@/components/ThemeControls'
import BrandMark from '@/components/BrandMark'
import { useAuthMode } from './useAuthMode'

const useStyles = createStyles(({ css }) => ({
  page: css`
    min-height: 100vh;
    position: relative;
    display: flex;
    align-items: center;
    justify-content: center;
    background: ${t.paper};
    padding: 24px;
  `,
  themeControls: css`
    position: absolute;
    top: 20px;
    right: 20px;
    padding: 4px;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: color-mix(in srgb, var(--card) 88%, transparent);
    box-shadow: var(--elevation-1);
  `,
  card: css`
    width: 380px;
    max-width: 100%;
    text-align: center;
    padding: 40px;
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    background: var(--card);
    box-shadow: var(--elevation-3);
    animation: fadeIn 0.5s ease;
    @keyframes fadeIn {
      from {
        opacity: 0;
        transform: translateY(12px);
      }
      to {
        opacity: 1;
        transform: translateY(0);
      }
    }
  `,
  brand: css`
    margin-bottom: 40px;
  `,
  logoMark: css`
    display: flex;
    justify-content: center;
    margin-bottom: 14px;
  `,
  brandTitle: css`
    font-family: ${t.fontSans};
    font-size: 22px;
    font-weight: 700;
    color: ${t.ink};
    letter-spacing: -0.03em;
    margin-bottom: 4px;
  `,
  brandSubtitle: css`
    font-size: ${t.textSm};
    color: ${t.textMuted};
  `,
  body: css`
    margin-bottom: 28px;
  `,
  bodyTitle: css`
    font-size: ${t.textLg};
    font-weight: 600;
    color: ${t.text};
    margin-bottom: 4px;
    letter-spacing: -0.02em;
  `,
  bodySubtitle: css`
    font-size: ${t.textSm};
    color: ${t.textTertiary};
    margin-bottom: 24px;
  `,
  field: css`
    margin-bottom: 12px;
    text-align: left;
    .ant-input,
    .ant-input-affix-wrapper {
      font-size: 14px;
    }
  `,
  error: css`
    color: #d4380d;
    font-size: ${t.textSm};
    margin-bottom: 12px;
  `,
  loginBtn: css`
    width: 100%;
    height: 46px;
    background: ${t.ink};
    color: var(--primary-foreground);
    border: none;
    border-radius: 999px;
    font-family: ${t.fontSans};
    font-size: ${t.textSm};
    font-weight: 600;
    cursor: pointer;
    transition: all 0.2s ease;
    display: flex;
    align-items: center;
    justify-content: center;
    &:hover {
      background: ${t.inkHover};
      box-shadow: ${t.elevation2};
    }
    &:active {
      transform: scale(0.98);
    }
    &:disabled {
      opacity: 0.7;
      cursor: not-allowed;
    }
  `,
  foot: css`
    font-size: 11px;
    color: ${t.textMuted};
  `,
  moreSection: css`
    margin-top: 12px;
  `,
  moreLink: css`
    background: none;
    border: none;
    padding: 0;
    font-size: ${t.textSm};
    color: ${t.textMuted};
    cursor: pointer;
    &:hover {
      color: ${t.text};
    }
  `
}))

export default function LoginPage() {
  const { styles } = useStyles()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [showOrg, setShowOrg] = useState(false)
  const [org, setOrg] = useState('')
  const [orgError, setOrgError] = useState('')
  const [orgChecking, setOrgChecking] = useState(false)
  const navigate = useNavigate()
  const token = getAccessToken()
  const { data: user, isLoading } = useUserInfo({ enabled: !!token })
  const { data: mode, isLoading: modeLoading } = useAuthMode()
  const loginWithPassword = useAuthStore((s) => s.loginWithPassword)

  useEffect(() => {
    if (token && !isLoading && user) {
      void Promise.resolve(navigate('/', { replace: true }))
    }
  }, [token, isLoading, user, navigate])

  // builtin mode + uninitialized → force the setup flow
  useEffect(() => {
    if (mode && mode.mode === 'builtin' && !mode.initialized) {
      void Promise.resolve(navigate('/setup', { replace: true }))
    }
  }, [mode, navigate])

  const handleBuiltinLogin = async () => {
    setError('')
    setLoading(true)
    try {
      await loginWithPassword(username, password)
      void Promise.resolve(navigate('/', { replace: true }))
    } catch (err) {
      setError(parseApiError(err))
    } finally {
      setLoading(false)
    }
  }

  const handleCasdoorLogin = () => {
    setLoading(true)
    authApi.login()
  }

  // 多组织确认：空 → 默认组织直接跳转；非空 → 先预检，未注册就地报错不跳转。
  const handleOrgConfirm = async () => {
    const value = org.trim()
    setOrgError('')
    if (!value) {
      setLoading(true)
      authApi.login()
      return
    }
    setOrgChecking(true)
    try {
      await authApi.checkOrg(value)
      setLoading(true)
      authApi.login(value)
    } catch {
      setOrgError('组织不存在或未注册，请检查后重试')
    } finally {
      setOrgChecking(false)
    }
  }

  if ((token && isLoading) || modeLoading) {
    return (
      <div className={styles.page}>
        <LoadingState />
      </div>
    )
  }

  const isCasdoor = mode?.mode === 'casdoor'

  return (
    <div className={styles.page}>
      <div className={styles.themeControls}>
        <ThemeControls />
      </div>
      <div className={styles.card}>
        <div className={styles.brand}>
          <div className={styles.logoMark}>
            <BrandMark size={56} />
          </div>
          <div className={styles.brandTitle}>Zerone Agent Hub</div>
          <div className={styles.brandSubtitle}>AI Agent 管理平台</div>
        </div>
        <div className={styles.body}>
          <div className={styles.bodyTitle}>欢迎回来</div>
          <div className={styles.bodySubtitle}>
            {isCasdoor ? '使用 Zerone 统一账号认证登录' : '使用账号登录'}
          </div>
          {error && <div className={styles.error}>{error}</div>}
          {isCasdoor ? (
            <>
              <button
                type="button"
                className={styles.loginBtn}
                onClick={handleCasdoorLogin}
                disabled={loading}
              >
                {loading ? <Spin size="small" /> : '登录 Agent Hub'}
              </button>
              {mode.multiOrg === true && (
                <div className={styles.moreSection}>
                  <button
                    type="button"
                    className={styles.moreLink}
                    onClick={() => { setShowOrg((v) => !v); setOrgError('') }}
                  >
                    {showOrg ? '收起' : '更多'}
                  </button>
                  {showOrg && (
                    <div className={styles.field}>
                      <Input
                        placeholder="留空使用默认组织"
                        value={org}
                        onChange={(e) => { setOrg(e.target.value); setOrgError('') }}
                        size="large"
                        aria-label="组织"
                      />
                      {orgError && <div className={styles.error}>{orgError}</div>}
                      <button
                        type="button"
                        className={styles.loginBtn}
                        style={{ marginTop: 12 }}
                        onClick={() => { void handleOrgConfirm() }}
                        disabled={orgChecking}
                      >
                        {orgChecking ? <Spin size="small" /> : '确认'}
                      </button>
                    </div>
                  )}
                </div>
              )}
            </>
          ) : (
            <form noValidate onSubmit={(e) => { e.preventDefault(); void handleBuiltinLogin(); }}>
              <div className={styles.field}>
                <Input
                  placeholder="用户名"
                  name="username"
                  value={username}
                  onChange={(e) => { setUsername(e.target.value); }}
                  autoComplete="username"
                  size="large"
                />
              </div>
              <div className={styles.field}>
                <PasswordInput
                  placeholder="密码"
                  name="password"
                  value={password}
                  onChange={(e) => { setPassword(e.target.value); }}
                  autoComplete="current-password"
                  size="large"
                />
              </div>
              <button
                type="submit"
                className={styles.loginBtn}
                disabled={loading || !username || !password}
              >
                {loading ? <Spin size="small" /> : '登录'}
              </button>
            </form>
          )}
        </div>
        <div className={styles.foot}>由 Zerone 认证服务保障安全</div>
      </div>
    </div>
  )
}
