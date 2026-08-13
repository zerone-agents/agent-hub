import { useEffect, useState } from 'react'
import { Spin } from 'antd'
import { createStyles } from 'antd-style'
import { useNavigate } from 'react-router'
import PasswordInput from '@/components/PasswordInput'
import { authApi } from '@/api/auth'
import { parseApiError, setTokens } from '@/api/client'
import { tokens as t } from '@/styles/tokens'
import ThemeControls from '@/components/ThemeControls'
import BrandMark from '@/components/BrandMark'
import { useAuthMode } from '@/features/login/useAuthMode'

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
  `,
  brand: css`
    margin-bottom: 32px;
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
    margin-bottom: 24px;
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
  submitBtn: css`
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
    margin-top: 8px;
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
  `
}))

export default function SetupPage() {
  const { styles } = useStyles()
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const navigate = useNavigate()
  const { data: mode, isLoading } = useAuthMode()

  // Already initialized (or casdoor mode) → bounce to login.
  useEffect(() => {
    if (mode && (mode.mode !== 'builtin' || mode.initialized)) {
      void Promise.resolve(navigate('/login', { replace: true }))
    }
  }, [mode, navigate])

  const handleSubmit = async () => {
    setError('')
    if (password.length < 8) {
      setError('密码至少 8 位，且需包含字母和数字')
      return
    }
    if (password !== confirm) {
      setError('两次输入的密码不一致')
      return
    }
    setLoading(true)
    try {
      const pair = await authApi.setup(password, confirm)
      setTokens(pair.accessToken, pair.refreshToken)
      void Promise.resolve(navigate('/', { replace: true }))
    } catch (err) {
      setError(parseApiError(err))
    } finally {
      setLoading(false)
    }
  }

  if (isLoading) {
    return (
      <div className={styles.page}>
        <Spin />
      </div>
    )
  }

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
          <div className={styles.bodyTitle}>初始化系统</div>
          <div className={styles.bodySubtitle}>创建管理员账号（用户名固定为 admin）</div>
          {error && <div className={styles.error}>{error}</div>}
          <form noValidate onSubmit={(e) => { e.preventDefault(); void handleSubmit(); }}>
            {/* 用户名固定为 admin。渲染一个离屏 username 输入框，
                让密码管理器能把凭证关联到正确的用户名上。 */}
            <input
              type="text"
              name="username"
              autoComplete="username"
              value="admin"
              readOnly
              tabIndex={-1}
              aria-hidden="true"
              style={{ position: 'absolute', opacity: 0, pointerEvents: 'none', height: 0, width: 0 }}
            />
            <div className={styles.field}>
              <PasswordInput
                placeholder="设置管理员密码"
                name="password"
                value={password}
                onChange={(e) => { setPassword(e.target.value); }}
                autoComplete="new-password"
                size="large"
              />
            </div>
            <div className={styles.field}>
              <PasswordInput
                placeholder="确认密码"
                name="confirmPassword"
                value={confirm}
                onChange={(e) => { setConfirm(e.target.value); }}
                autoComplete="new-password"
                size="large"
              />
            </div>
            <button
              type="submit"
              className={styles.submitBtn}
              disabled={loading || !password || !confirm}
            >
              {loading ? <Spin size="small" /> : '创建并登录'}
            </button>
          </form>
        </div>
      </div>
    </div>
  )
}
