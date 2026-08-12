import { useEffect, useState } from 'react'
import { Spin } from 'antd'
import { createStyles } from 'antd-style'
import { useNavigate } from 'react-router'
import { authApi } from '@/api/auth'
import { getAccessToken } from '@/api/client'
import { useUserInfo } from '@/queries/useUserInfo'
import LoadingState from '@/components/LoadingState'
import { tokens as t } from '@/styles/tokens'
import ThemeControls from '@/components/ThemeControls'
import BrandMark from '@/components/BrandMark'

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
  `
}))

export default function LoginPage() {
  const { styles } = useStyles()
  const [loading, setLoading] = useState(false)
  const navigate = useNavigate()
  const token = getAccessToken()
  const { data: user, isLoading } = useUserInfo({ enabled: !!token })

  useEffect(() => {
    if (token && !isLoading && user) {
      void Promise.resolve(navigate('/', { replace: true }))
    }
  }, [token, isLoading, user, navigate])

  const handleLogin = () => {
    setLoading(true)
    authApi.login()
  }

  if (token && isLoading) {
    return (
      <div className={styles.page}>
        <LoadingState />
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
          <div className={styles.bodyTitle}>欢迎回来</div>
          <div className={styles.bodySubtitle}>使用 Zerone 统一账号认证登录</div>
          <button
            type="button"
            className={styles.loginBtn}
            onClick={handleLogin}
            disabled={loading}
          >
            {loading ? <Spin size="small" /> : '登录 Agent Hub'}
          </button>
        </div>
        <div className={styles.foot}>由 Zerone 认证服务保障安全</div>
      </div>
    </div>
  )
}
