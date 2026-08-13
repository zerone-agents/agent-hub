import { useEffect, useState } from 'react'
import { Input, Spin } from 'antd'
import { createStyles } from 'antd-style'
import { useNavigate, useSearchParams } from 'react-router'
import PasswordInput from '@/components/PasswordInput'
import { authApi } from '@/api/auth'
import { parseApiError, setTokens } from '@/api/client'
import { tokens as t } from '@/styles/tokens'
import BrandMark from '@/components/BrandMark'
import ThemeControls from '@/components/ThemeControls'

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
  note: css`
    font-size: ${t.textSm};
    color: ${t.textMuted};
    background: color-mix(in srgb, ${t.paper} 60%, transparent);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 8px 12px;
    margin-bottom: 20px;
  `,
  field: css`
    margin-bottom: 12px;
    text-align: left;
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
  `,
  link: css`
    display: inline-block;
    margin-top: 16px;
    font-size: ${t.textSm};
    color: ${t.softAccent};
    text-decoration: none;
    &:hover {
      text-decoration: underline;
    }
  `
}))

type State = 'loading' | 'ok' | 'invalid'

export default function RegisterPage() {
  const { styles } = useStyles()
  const [searchParams] = useSearchParams()
  const inviteToken = searchParams.get('token') ?? ''
  const navigate = useNavigate()
  const [state, setState] = useState<State>('loading')
  const [note, setNote] = useState('')
  const [username, setUsername] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    if (!inviteToken) {
      setState('invalid')
      return
    }
    authApi
      .precheckInvite(inviteToken)
      .then((res) => {
        setNote(res.note)
        setState('ok')
      })
      .catch(() => setState('invalid'))
  }, [inviteToken])

  const handleSubmit = async () => {
    setError('')
    setLoading(true)
    try {
      const pair = await authApi.register(inviteToken, username, password, displayName || undefined)
      setTokens(pair.accessToken, pair.refreshToken)
      void Promise.resolve(navigate('/', { replace: true }))
    } catch (err) {
      setError(parseApiError(err))
    } finally {
      setLoading(false)
    }
  }

  const renderForm = () => (
    <div className={styles.card}>
      <div className={styles.brand}>
        <div className={styles.logoMark}>
          <BrandMark size={56} />
        </div>
        <div className={styles.brandTitle}>Zerone Agent Hub</div>
        <div className={styles.brandSubtitle}>AI Agent 管理平台</div>
      </div>
      <div className={styles.body}>
        <div className={styles.bodyTitle}>加入 Agent Hub</div>
        <div className={styles.bodySubtitle}>填写信息完成注册</div>
        {note && <div className={styles.note}>邀请备注：{note}</div>}
        {error && <div className={styles.error}>{error}</div>}
        <div className={styles.field}>
          <Input
            placeholder="用户名（3-32 位字母数字下划线连字符）"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            size="large"
          />
        </div>
        <div className={styles.field}>
          <Input
            placeholder="昵称（可选）"
            value={displayName}
            onChange={(e) => setDisplayName(e.target.value)}
            size="large"
          />
        </div>
        <div className={styles.field}>
          <PasswordInput
            placeholder="密码（至少 8 位，含字母和数字）"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            onPressEnter={handleSubmit}
            size="large"
          />
        </div>
        <button
          type="button"
          className={styles.submitBtn}
          onClick={handleSubmit}
          disabled={loading || !username || !password}
        >
          {loading ? <Spin size="small" /> : '注册并登录'}
        </button>
      </div>
    </div>
  )

  return (
    <div className={styles.page}>
      <div className={styles.themeControls}>
        <ThemeControls />
      </div>
      {state === 'loading' && <Spin />}
      {state === 'ok' && renderForm()}
      {state === 'invalid' && (
        <div className={styles.card}>
          <div className={styles.brand}>
            <div className={styles.logoMark}>
              <BrandMark size={56} />
            </div>
            <div className={styles.brandTitle}>邀请链接无效</div>
          </div>
          <div className={styles.body}>
            <div className={styles.bodySubtitle}>
              邀请链接无效或已失效，请联系管理员重新获取。
            </div>
            <a
              className={styles.link}
              href="/static/login"
              onClick={(e) => {
                e.preventDefault()
                void Promise.resolve(navigate('/login', { replace: true }))
              }}
            >
              返回登录
            </a>
          </div>
        </div>
      )}
    </div>
  )
}
