import { useEffect, useRef, useState, useCallback, useMemo } from 'react'
import { Modal, Button, Steps, Alert, Checkbox, Tag, Space, Typography, message } from 'antd'
import {
  RocketIcon,
  StopIcon,
  TrashIcon,
  ArrowClockwiseIcon,
  ChatsCircleIcon,
  CheckIcon,
  EyeIcon,
  EyeSlashIcon,
  CopyIcon,
  PlayIcon,
} from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import PrimaryButton from '@/components/PrimaryButton'
import { agentApi } from '@/api/agents'
import { copyOrManual } from '@/utils/clipboard'
import { parseApiError } from '@/api/client'
import { useKnowledgeList } from '@/queries/useKnowledge'
import { useCanWrite } from '@/hooks/useCanWrite'
import type { Agent, DeploymentStatus } from '@/api/agents'
import type { Provider } from '@/api/providers'
import { tokens as t } from '@/styles/tokens'

const { Text } = Typography

/**
 * No-Kong mode returns a hub-relative runtime path — casdoor shape
 * /runtime/<org>/<agent>, builtin shape /runtime/<agent> (issue #114: no
 * default tenant segment); resolve it against the current origin for display
 * and clipboard. Kong-mode absolute URLs pass through unchanged.
 */
const absoluteRuntimeUrl = (url: string): string =>
  url.startsWith('/') ? `${window.location.origin}${url}` : url

interface DeployModalProps {
  agent: Agent
  providers: Provider[]
  open: boolean
  onClose: () => void
}

const POLL_FAST_MS = 2000   // mid-state: creating / starting / unknown
const POLL_SLOW_MS = 15000  // terminal: running+healthy / stopped / error / not_found

const stepLabels = ['准备配置', '创建容器', '等待运行', '健康检查通过']

const useStyles = createStyles(({ css }) => ({
  infoCard: css`
    background: ${t.paper};
    border: 1px solid color-mix(in srgb, var(--foreground) 8%, transparent);
    border-radius: ${t.radius}px;
    padding: 14px 16px;
    margin-bottom: 16px;
  `,
  infoHead: css`
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    gap: 12px;
    margin-bottom: 12px;
  `,
  infoTitle: css`
    font-size: ${t.textBase};
    font-weight: 600;
    color: ${t.text};
    line-height: 1.3;
    margin: 0 0 2px 0;
  `,
  infoDesc: css`
    font-size: ${t.textXs};
    color: ${t.textTertiary};
    margin: 0;
    line-height: 1.4;
  `,
  statusTag: css`
    flex-shrink: 0;
    font-size: 11px;
    font-weight: 600;
    padding: 2px 8px;
    border-radius: 10px;
    text-transform: uppercase;
    letter-spacing: 0.02em;
  `,
  statusRunning: css`background: rgba(5, 150, 105, 0.10); color: ${t.success};`,
  statusStopped: css`background: rgba(107, 114, 128, 0.10); color: ${t.textTertiary};`,
  statusError: css`background: rgba(220, 38, 38, 0.10); color: ${t.danger};`,
  statusArchived: css`background: rgba(245, 158, 11, 0.10); color: ${t.warning};`,
  statusDefault: css`background: color-mix(in srgb, var(--primary) 7%, transparent); color: ${t.ink};`,
  infoGrid: css`
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 12px;
    font-size: ${t.textXs};
  `,
  infoLabel: css`
    color: ${t.textMuted};
    font-size: 11px;
    margin-bottom: 2px;
  `,
  infoValue: css`
    color: ${t.text};
    font-weight: 500;
  `,
  capabilityWrap: css`
    display: flex;
    gap: 12px;
    flex-wrap: wrap;
    margin-bottom: 16px;
  `,
  capabilityBlock: css`
    flex: 1;
    min-width: 140px;
    border: 1px solid color-mix(in srgb, var(--foreground) 8%, transparent);
    border-radius: ${t.radiusSm}px;
    padding: 10px 12px;
    background: var(--card);
  `,
  fullBlock: css`
    width: 100%;
    border: 1px solid color-mix(in srgb, var(--foreground) 8%, transparent);
    border-radius: ${t.radiusSm}px;
    padding: 10px 12px;
    background: var(--card);
    margin-bottom: 12px;
  `,
  capabilityTitle: css`
    font-size: 11px;
    font-weight: 600;
    color: ${t.textTertiary};
    text-transform: uppercase;
    letter-spacing: 0.03em;
    margin-bottom: 8px;
  `,
  capabilityTags: css`
    display: flex;
    flex-wrap: wrap;
    gap: 5px;
  `,
  capabilityTag: css`
    font-size: 11px;
    font-weight: 500;
    color: ${t.text} !important;
    background: ${t.inkSubtle} !important;
    border-color: color-mix(in srgb, var(--foreground) 10%, transparent) !important;
    margin-inline-end: 0 !important;
  `,
  emptyText: css`
    font-size: ${t.textXs};
    color: ${t.textMuted};
  `,
  apiCard: css`
    border: 1px solid color-mix(in srgb, var(--foreground) 8%, transparent);
    border-radius: ${t.radiusSm}px;
    padding: 10px 12px;
    background: var(--card);
    margin-bottom: 16px;
  `,
  apiRow: css`
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 12px;
    padding: 4px 0;
    &:not(:last-child) {
      margin-bottom: 4px;
    }
  `,
  apiLabel: css`
    color: ${t.textMuted};
    width: 72px;
    flex-shrink: 0;
    white-space: nowrap;
    font-weight: 500;
  `,
  apiValue: css`
    color: ${t.text};
    font-family: ${t.fontMono};
    font-size: 11px;
    flex: 1;
    min-width: 0;
    overflow-wrap: anywhere;
    user-select: all;
  `,
  apiAction: css`
    border: none;
    background: transparent;
    cursor: pointer;
    color: ${t.textTertiary};
    padding: 2px;
    display: inline-flex;
    align-items: center;
    flex-shrink: 0;
    &:hover {
      color: ${t.ink};
    }
  `,
  copied: css`
    color: ${t.success};
    font-size: 11px;
    flex-shrink: 0;
  `,
  footer: css`
    display: flex;
    gap: 8px;
    justify-content: flex-end;
    flex-wrap: wrap;
    .ant-btn:not(.ant-btn-primary) {
      box-shadow: none;
    }
    @media (max-width: 480px) {
      .ant-btn {
        flex: 1 1 calc(50% - 8px);
      }
    }
  `,
}))

// Step indices: 0 准备配置 / 1 创建容器 / 2 等待运行 / 3 健康检查通过 / 4 全部完成。
// "等待运行" covers everything up to fully healthy: the pulse stays there
// while the runtime is created/running-but-starting; the last step only
// lights up once the health check actually passes.
function getStepIndex(status: string, health?: string): number {
  if (status === 'running' && health === 'healthy') return 4
  if (status === 'running') return 2
  if (status === 'error') return 3
  if (status === 'created' || status === 'restarting' || status === 'paused') return 2
  if (status === 'not_found') return 0
  return 1
}

function getStepStatus(
  index: number,
  current: number,
  deploymentStatus: string,
  health?: string
): 'wait' | 'process' | 'finish' | 'error' {
  if (index < current) return 'finish'
  if (index === current) {
    if (deploymentStatus === 'error' || health === 'unhealthy') return 'error'
    return 'process'
  }
  return 'wait'
}

// buildStatusLine composes the live one-line summary shown under the step bar,
// e.g. "容器运行中 · 健康检查中 · Kong 网关路由检测中".
function buildStatusLine(s: DeploymentStatus | null): string {
  if (!s?.status) return ''
  const parts: string[] = []
  if (s.status === 'not_found') parts.push('未部署')
  else if (s.status === 'running') parts.push('容器运行中')
  else if (s.status === 'stopped' || s.status === 'exited') parts.push('容器已停止')
  else if (s.status === 'archived') parts.push('已归档')
  else if (s.status === 'error') parts.push('部署出错')
  else parts.push('容器创建中')
  if (s.status === 'running') {
    if (s.health === 'starting') parts.push('健康检查中')
    else if (s.health === 'healthy') parts.push('健康检查通过')
    else if (s.health === 'unhealthy') parts.push('健康检查异常')
  }
  if (s.message) parts.push(s.message)
  return parts.join(' · ')
}

function isMidState(s: DeploymentStatus | null): boolean {
  if (!s) return false
  if (s.status === 'not_found') return false
  if (s.status === 'archived') return false
  if (s.status === 'stopped' || s.status === 'exited') return false
  if (s.status === 'error') return false
  if (s.status === 'running' && s.health === 'healthy') return false
  return true
}

export default function DeployModal({ agent, providers, open, onClose }: DeployModalProps) {
  const { styles } = useStyles()
  const canWrite = useCanWrite()
  const [status, setStatus] = useState<DeploymentStatus | null>(null)
  // Whether the initial getDeployment request has resolved. Until it does, the
  // footer action buttons are hidden to prevent a flash of the "部署" button
  // (which appears whenever status === null) and avoid misclicks.
  const [statusLoaded, setStatusLoaded] = useState(false)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [showApiKey, setShowApiKey] = useState(false)
  const [copied, setCopied] = useState<'url' | 'key' | null>(null)
  const [rotateKey, setRotateKey] = useState(false)
  const [confirmOpen, setConfirmOpen] = useState(false)
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  // Generation token for the polling chain. clearTimer() bumps it so that
  // timers scheduled-but-not-yet-fired and polls with an in-flight fetch
  // (which clearTimeout cannot reach) invalidate themselves instead of
  // spawning duplicate chains (e.g. StrictMode double-mount, or clicking
  // 部署 before the initial fetch resolves).
  const pollGenRef = useRef(0)
  // After a deploy/restart, the deployer may return a stale "healthy" from
  // the old container before the new one's health check actually runs.
  // We suppress healthy status for a grace period to avoid false positives.
  const suppressHealthyRef = useRef(0)

  const clearTimer = useCallback(() => {
    pollGenRef.current++
    if (timerRef.current) {
      clearTimeout(timerRef.current)
      timerRef.current = null
    }
  }, [])

  const fetchStatus = useCallback(async () => {
    try {
      const res = await agentApi.getDeployment(agent.name)
      const payload = (res.data as { data?: DeploymentStatus; success?: boolean }).data ?? null
      if (payload) {
        setStatus(payload)
      }
      setError(null)
      return payload
    } catch (e: unknown) {
      setError(parseApiError(e))
      return null
    }
  }, [agent.name])

  // Single recursive chain: each tick captures the current generation at
  // schedule time and re-validates it both before fetching and before
  // rescheduling, so stale chains die instead of forking.
  //
  // The poll function is recursive (it reschedules itself on each tick). To
  // avoid referencing `schedulePoll` before its declaration (which violates
  // react-hooks/immutability) we hold the latest instance in a ref and have
  // the callback go through the ref. The ref is populated by a separate
  // effect below so the recursion always sees the current closure.
  const schedulePollRef = useRef<(delay: number) => void>(() => undefined)
  const schedulePoll = useCallback(
    (delay: number) => {
      const gen = pollGenRef.current
      timerRef.current = setTimeout(async () => {
        if (gen !== pollGenRef.current) return
        const current = await fetchStatus()
        if (gen !== pollGenRef.current) return
        if (current && Date.now() < suppressHealthyRef.current) {
          current.health = undefined
        }
        schedulePollRef.current(isMidState(current) ? POLL_FAST_MS : POLL_SLOW_MS)
      }, delay)
    },
    [fetchStatus]
  )
  useEffect(() => {
    schedulePollRef.current = schedulePoll
  }, [schedulePoll])

  useEffect(() => {
    if (open) {
      const gen = pollGenRef.current
      // Resetting error/loading state on open is intentional synchronization
      // with the modal lifecycle — these resets are coupled to the polling
      // kickoff below and must fire on the same effect tick.
      // eslint-disable-next-line react-hooks/set-state-in-effect -- modal-open lifecycle reset
      setError(null)
       
      setStatusLoaded(false)
      void fetchStatus().then(() => {
        // A close/re-mount (or StrictMode's extra effect cycle) bumps the
        // generation before this fetch resolves; only the live effect may
        // start a polling chain.
        if (gen !== pollGenRef.current) return
        setStatusLoaded(true)
        schedulePoll(POLL_SLOW_MS)
      })
    } else {
      clearTimer()
       
      setStatus(null)
       
      setStatusLoaded(false)
       
      setError(null)
    }
    return () => { clearTimer(); }
  }, [open, agent.name, clearTimer, fetchStatus, schedulePoll])

  const handleDeploy = async (force = false, rotateKey = false) => {
    setLoading(true)
    setError(null)
    setStatus(null)
    suppressHealthyRef.current = Date.now() + 4000
    clearTimer()
    try {
      const res = await agentApi.deploy(agent.name, force, rotateKey)
      const payload = (res.data as { data?: DeploymentStatus; success?: boolean }).data ?? null
      if (payload) {
        setStatus(payload)
      }
      // Kick off fast polling immediately after deploy response
      schedulePoll(POLL_FAST_MS)
    } catch (e: unknown) {
      setError(parseApiError(e))
    } finally {
      setLoading(false)
    }
  }

  const handleStop = async () => {
    setLoading(true)
    try {
      await agentApi.stopDeployment(agent.name)
      await fetchStatus()
    } catch (e: unknown) {
      setError(parseApiError(e))
    } finally {
      setLoading(false)
    }
  }

  const handleStart = async () => {
    setLoading(true)
    setError(null)
    suppressHealthyRef.current = Date.now() + 4000
    clearTimer()
    try {
      const res = await agentApi.startDeployment(agent.name)
      const payload = (res.data as { data?: DeploymentStatus; success?: boolean }).data ?? null
      if (payload) {
        setStatus(payload)
      }
      schedulePoll(POLL_FAST_MS)
    } catch (e: unknown) {
      setError(parseApiError(e))
    } finally {
      setLoading(false)
    }
  }

  const handleDelete = async (purge = false) => {
    setLoading(true)
    try {
      if (purge) {
        await agentApi.purgeDeployment(agent.name)
      } else {
        await agentApi.deleteDeployment(agent.name)
      }
      await fetchStatus()
    } catch (e: unknown) {
      setError(parseApiError(e))
    } finally {
      setLoading(false)
    }
  }

  const handleLaunch = () => {
    if (agent.name) {
      window.open(`/static/agents/${encodeURIComponent(agent.name)}/chat`, '_blank', 'noopener,noreferrer')
    }
  }

  const openConfirm = () => {
    setRotateKey(false)
    setConfirmOpen(true)
  }

  const isMissingConfig = !agent.config.providerId || !agent.config.modelId

  const deploymentStatus = status?.status ?? 'not_found'
  const isRunning = deploymentStatus === 'running'
  const isArchived = deploymentStatus === 'archived'
  const isStoppedOrError =
    deploymentStatus === 'stopped' ||
    deploymentStatus === 'exited' ||
    deploymentStatus === 'error' ||
    deploymentStatus === 'unknown'
  const isNotFound = deploymentStatus === 'not_found'
  const canLaunch = isRunning && status?.health === 'healthy'

  const provider = useMemo(
    () => providers.find((p) => p.id === agent.config.providerId),
    [providers, agent.config.providerId]
  )

  // agent.datasets 存的是 dataset UUID，展示时映射为知识库名称
  const { data: knowledgeData } = useKnowledgeList({ page: 1, page_size: 1000 })
  const datasetNameMap = useMemo(
    () => new Map((knowledgeData?.datasets ?? []).map((d) => [d.id, d.name])),
    [knowledgeData]
  )

  const description = useMemo(() => {
    const d = agent.config.description
    return d?.zh ?? d?.en ?? ''
  }, [agent.config.description])

  const statusClass = useMemo(() => {
    if (isRunning) return `${styles.statusTag} ${styles.statusRunning}`
    if (deploymentStatus === 'error') return `${styles.statusTag} ${styles.statusError}`
    if (isArchived) return `${styles.statusTag} ${styles.statusArchived}`
    if (isStoppedOrError) return `${styles.statusTag} ${styles.statusStopped}`
    return `${styles.statusTag} ${styles.statusDefault}`
  }, [isRunning, isArchived, isStoppedOrError, deploymentStatus, styles])

  const statusText = useMemo(() => {
    if (isRunning && status?.health === 'healthy') return '运行中'
    if (isRunning && status?.health === 'starting') return '启动中'
    if (isRunning) return '运行中'
    if (deploymentStatus === 'error') return '错误'
    if (deploymentStatus === 'stopped') return '已停止'
    if (deploymentStatus === 'exited') return '已退出'
    if (isArchived) return '已归档'
    if (deploymentStatus === 'not_found') return '未部署'
    return deploymentStatus || '未知'
  }, [isRunning, isArchived, deploymentStatus, status?.health])

  const statusLine = useMemo(() => buildStatusLine(status), [status])

  const renderCapabilityBlock = (title: string, items: string[]) => (
    <div className={styles.capabilityBlock} key={title}>
      <div className={styles.capabilityTitle}>{title}</div>
      <div className={styles.capabilityTags}>
        {items.length > 0 ? (
          items.map((item) => (
            <Tag key={item} className={styles.capabilityTag}>
              {item}
            </Tag>
          ))
        ) : (
          <span className={styles.emptyText}>无</span>
        )}
      </div>
    </div>
  )

  const handleCopy = async (which: 'url' | 'key', text: string) => {
    const result = await copyOrManual(text)
    if (result === 'copied') {
      setCopied(which)
      setTimeout(() => { setCopied(null); }, 1500)
    } else if (result === 'failed') {
      // 纯 HTTP + IP 等非安全上下文下两条复制路径都可能失败——静默吞错会让测试者误判
      message.error('复制失败，请手动选择复制')
    }
    // 'manual'：手动复制框已弹出，不显示成功态
  }

  return (
    <Modal
      open={open}
      onCancel={onClose}
      footer={null}
      closable={false}
      width={640}
      title={
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <RocketIcon size={20} weight="duotone" />
          <span>部署 Agent</span>
        </div>
      }
    >
      <div style={{ padding: '16px 0' }}>
        {isMissingConfig && (
          <Alert
            title="未配置模型"
            description="请先为 Agent 配置 Provider 和 Model，否则部署可能失败。"
            type="warning"
            showIcon
            style={{ marginBottom: 16 }}
          />
        )}

        {error && (
          <Alert
            title={error}
            type="error"
            showIcon
            closable={{ onClose: () => { setError(null); } }}
            style={{ marginBottom: 16 }}
          />
        )}

        {/* Agent 基本信息 */}
        <div className={styles.infoCard}>
          <div className={styles.infoHead}>
            <div>
              <h3 className={styles.infoTitle}>{agent.config.title?.zh ?? agent.config.title?.en ?? agent.name}</h3>
              {description && <p className={styles.infoDesc}>{description}</p>}
            </div>
            <span className={statusClass}>{statusText}</span>
          </div>
          <div className={styles.infoGrid}>
            <div>
              <div className={styles.infoLabel}>Provider</div>
              <div className={styles.infoValue}>{provider?.name ?? (agent.config.providerId != null ? `#${agent.config.providerId}` : '-')}</div>
            </div>
            <div>
              <div className={styles.infoLabel}>Model</div>
              <div className={styles.infoValue}>{agent.config.modelId ?? '-'}</div>
            </div>
            <div>
              <div className={styles.infoLabel}>端口</div>
              <div className={styles.infoValue}>{status?.hostPort ?? '-'}</div>
            </div>
          </div>
        </div>

        {/* 工具 / 子代理 */}
        {(() => {
          const tools = agent.tools ?? []
          const subagents = agent.subagents ?? []
          if (tools.length === 0 && subagents.length === 0) return null
          return (
            <div className={styles.capabilityWrap}>
              {tools.length > 0 && renderCapabilityBlock('工具', tools)}
              {subagents.length > 0 && renderCapabilityBlock('子代理', subagents)}
            </div>
          )
        })()}

        {/* 技能 */}
        {(() => {
          const items = agent.skills ?? []
          if (items.length === 0) return null
          return (
            <div className={styles.fullBlock}>
              <div className={styles.capabilityTitle}>技能</div>
              <div className={styles.capabilityTags}>
                {items.map((item) => (
                  <Tag key={item} className={styles.capabilityTag}>{item}</Tag>
                ))}
              </div>
            </div>
          )
        })()}

        {/* 知识库 */}
        {(() => {
          const items = agent.datasets ?? []
          if (items.length === 0) return null
          return (
            <div className={styles.fullBlock}>
              <div className={styles.capabilityTitle}>知识库</div>
              <div className={styles.capabilityTags}>
                {items.map((item) => (
                  <Tag key={item} className={styles.capabilityTag}>{datasetNameMap.get(item) ?? item}</Tag>
                ))}
              </div>
            </div>
          )
        })()}

        {/* MCP */}
        {(() => {
          const items = agent.mcps ?? []
          if (items.length === 0) return null
          return (
            <div className={styles.fullBlock}>
              <div className={styles.capabilityTitle}>MCP</div>
              <div className={styles.capabilityTags}>
                {items.map((item) => (
                  <Tag key={item} className={styles.capabilityTag}>{item}</Tag>
                ))}
              </div>
            </div>
          )
        })()}

        {/* Agent API 信息：部署成功后显示 URL 和 API Key */}
        {isRunning && status?.runtimeUrl && (
          <div className={styles.apiCard}>
            <div className={styles.capabilityTitle}>API 信息</div>
            <div className={styles.apiRow}>
              <span className={styles.apiLabel}>URL</span>
              <span className={styles.apiValue}>{absoluteRuntimeUrl(status.runtimeUrl)}</span>
              <button
                type="button"
                className={styles.apiAction}
                title="复制 URL"
                onClick={() => handleCopy('url', absoluteRuntimeUrl(status.runtimeUrl ?? ''))}
              >
                {copied === 'url' ? <span className={styles.copied}>已复制</span> : <CopyIcon size={13} />}
              </button>
            </div>
            <div className={styles.apiRow}>
              <span className={styles.apiLabel}>API Key</span>
              <span className={styles.apiValue}>
                {showApiKey ? status.apiKey : '••••••••••••••••'}
              </span>
              <button
                type="button"
                className={styles.apiAction}
                title={showApiKey ? '隐藏' : '显示'}
                onClick={() => { setShowApiKey(!showApiKey); }}
              >
                {showApiKey ? <EyeSlashIcon size={13} /> : <EyeIcon size={13} />}
              </button>
              {status.apiKey && (
                <button
                  type="button"
                  className={styles.apiAction}
                  title="复制 Key"
                  onClick={() => handleCopy('key', status.apiKey ?? '')}
                >
                  {copied === 'key' ? <span className={styles.copied}>已复制</span> : <CopyIcon size={13} />}
                </button>
              )}
            </div>
          </div>
        )}

        {/* 部署进度：loading（POST 在飞）时也显示，步骤完全由实时状态驱动 */}
        {(loading || (status && deploymentStatus !== 'not_found' && deploymentStatus !== 'archived')) && (
          <div style={{ marginBottom: 16 }}>
            {(() => {
              const current = loading && !status ? 1 : getStepIndex(deploymentStatus, status?.health)
              return (
                <Steps
                  size="small"
                  current={current}
                  status={
                    deploymentStatus === 'error' || status?.health === 'unhealthy'
                      ? 'error'
                      : 'process'
                  }
                  items={stepLabels.map((label, idx) => {
                    const state = getStepStatus(idx, current, deploymentStatus, status?.health)
                    return {
                      key: idx,
                      title: label,
                      status: state,
                      icon:
                        state === 'finish' ? (
                          <span className="deploy-step-icon deploy-step-icon-finish">
                            <CheckIcon size={13} weight="bold" />
                          </span>
                        ) : state === 'process' ? (
                          <span className="deploy-step-icon deploy-step-icon-process" />
                        ) : state === 'error' ? (
                          <span className="deploy-step-icon deploy-step-icon-error">!</span>
                        ) : (
                          <span className="deploy-step-icon deploy-step-icon-wait">{idx + 1}</span>
                        ),
                    }
                  })}
                />
              )
            })()}
            {statusLine && (
              <div key={statusLine} className="deploy-status-line" data-testid="deploy-status-line">
                {statusLine}
              </div>
            )}
          </div>
        )}

        {deploymentStatus === 'error' && status && (
          <div style={{ marginBottom: 16 }}>
            <Space orientation="vertical" size="small">
              <Text type="danger">
                状态: <Text strong>{status.status}</Text>
              </Text>
              {status.message && (
                <Text type="secondary">{status.message}</Text>
              )}
            </Space>
          </div>
        )}

        <div className={styles.footer}>
          {!statusLoaded ? (
            <Button disabled loading>
              加载中
            </Button>
          ) : (
            <>
          {isRunning && (
            <>
              <Button
                icon={<StopIcon size={16} />}
                onClick={handleStop}
                loading={loading}
                disabled={!canWrite || loading}
              >
                停止
              </Button>
              <Button
                danger
                icon={<TrashIcon size={16} />}
                onClick={() => handleDelete()}
                loading={loading}
                disabled={!canWrite || loading}
              >
                归档
              </Button>
              <Button
                icon={<ArrowClockwiseIcon size={16} />}
                onClick={openConfirm}
                loading={loading}
                disabled={!canWrite || loading}
              >
                重新部署
              </Button>
              <PrimaryButton
                icon={<ChatsCircleIcon size={16} weight="fill" />}
                onClick={handleLaunch}
                disabled={!canLaunch || loading}
              >
                聊天
              </PrimaryButton>
            </>
          )}

          {(isNotFound || !status) && (
            <PrimaryButton
              icon={<RocketIcon size={16} />}
              onClick={() => handleDeploy()}
              loading={loading}
              disabled={isMissingConfig || !canWrite || loading}
            >
              部署
            </PrimaryButton>
          )}

          {isArchived && (
            <>
              <Button
                danger
                icon={<TrashIcon size={16} />}
                onClick={() => handleDelete(true)}
                loading={loading}
                disabled={!canWrite || loading}
              >
                彻底删除
              </Button>
              <PrimaryButton
                icon={<ArrowClockwiseIcon size={16} />}
                onClick={openConfirm}
                loading={loading}
                disabled={isMissingConfig || !canWrite || loading}
              >
                重新部署
              </PrimaryButton>
            </>
          )}

          {isStoppedOrError && (
            <>
              <Button
                danger
                icon={<TrashIcon size={16} />}
                onClick={() => handleDelete()}
                loading={loading}
                disabled={!canWrite || loading}
              >
                归档
              </Button>
              {(deploymentStatus === 'stopped' || deploymentStatus === 'exited') ? (
                <PrimaryButton
                  icon={<PlayIcon size={16} weight="fill" />}
                  onClick={handleStart}
                  loading={loading}
                  disabled={!canWrite || loading}
                >
                  启动
                </PrimaryButton>
              ) : null}
              <Button
                icon={<ArrowClockwiseIcon size={16} />}
                onClick={openConfirm}
                loading={loading}
                disabled={isMissingConfig || !canWrite || loading}
              >
                重新部署
              </Button>
            </>
          )}
            </>
          )}
        </div>
      </div>

      <Modal
        open={confirmOpen}
        title={`重新部署 ${agent.config.title?.zh ?? agent.config.title?.en ?? agent.name}`}
        onCancel={() => {
          setConfirmOpen(false)
          setRotateKey(false)
        }}
        width={480}
        footer={
          <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 10 }}>
            <Button onClick={() => { setConfirmOpen(false); setRotateKey(false) }}>取消</Button>
            <PrimaryButton onClick={() => { void handleDeploy(true, rotateKey); setConfirmOpen(false); setRotateKey(false) }}>
              确认重新部署
            </PrimaryButton>
          </div>
        }
      >
        <Alert
          title="重新部署将重新创建容器"
          description="如果勾选下方选项，将生成新的 API Key，旧 API Key 会立即失效，使用旧 Key 的客户端需要重新配置。"
          type="warning"
          showIcon
          style={{ marginBottom: 16 }}
        />
        <Checkbox
          checked={rotateKey}
          onChange={(e) => { setRotateKey(e.target.checked); }}
          style={{ color: '#d48806' }}
        >
          同时轮转 API Key（旧 Key 将失效）
        </Checkbox>
      </Modal>
    </Modal>
  )
}
