import { useMemo, useState } from 'react'
import { Alert, Empty, Input, Select, Skeleton, Tag } from 'antd'
import { ArrowRightIcon, CheckCircleIcon, MagnifyingGlassIcon, WarningCircleIcon } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import { useSearchParams } from 'react-router'
import type { AgentMessageAuditItem, MessageChainSelector } from '@/api/agent-message-audit'
import { parseApiError } from '@/api/client'
import PrimaryButton from '@/components/PrimaryButton'
import { useAgentMessageChain } from '@/queries/useAgentMessageAudit'
import { formatTime } from '@/utils/time'
import { tokens as t } from '@/styles/tokens'

const useStyles = createStyles(({ css }) => ({
  page: css`max-width: 1180px; margin: 0 auto;`,
  title: css`margin: 0; color: ${t.text}; font-size: ${t.text3xl}; letter-spacing: -.035em;`,
  subtitle: css`max-width: 72ch; margin: 8px 0 20px; color: ${t.textTertiary}; line-height: 1.65;`,
  search: css`display: grid; grid-template-columns: 150px minmax(260px, 1fr) auto; gap: 10px; padding: 16px; border: 1px solid var(--border); border-radius: ${t.radiusSm}px; background: var(--card); @media(max-width: 650px){grid-template-columns:1fr;}`,
  summary: css`display:flex; flex-wrap:wrap; gap:10px; align-items:center; margin:20px 0 12px; color:${t.textSecondary};`,
  chain: css`overflow:hidden; border:1px solid var(--border); border-radius:${t.radiusSm}px; background:var(--card);`,
  row: css`display:grid; grid-template-columns:minmax(210px,.8fr) minmax(240px,1.2fr) auto; gap:16px; padding:14px; border-bottom:1px solid var(--border); &:last-child{border-bottom:0;} @media(max-width:760px){grid-template-columns:1fr;}`,
  route: css`display:flex; align-items:center; gap:8px; min-width:0; color:${t.text}; font-weight:650; span{overflow:hidden;text-overflow:ellipsis;white-space:nowrap;}`,
  detail: css`color:${t.textTertiary}; font-size:12px; line-height:1.65;`,
  status: css`display:flex; align-items:flex-start; flex-wrap:wrap; gap:6px;`,
  content: css`grid-column:1/-1; display:grid; grid-template-columns:1fr 1fr; gap:10px; @media(max-width:650px){grid-template-columns:1fr;}`,
  evidence: css`padding:10px 12px; border-radius:8px; background:var(--background); color:${t.textSecondary}; font-size:12px; line-height:1.6; white-space:pre-wrap; word-break:break-word;`,
  empty: css`margin-top:20px; padding:50px 20px; border:1px dashed var(--border); border-radius:${t.radiusSm}px;`,
}))

const STATUS: Record<string, { label: string; color: string }> = {
  queued: { label: '等待处理', color: 'processing' }, running: { label: '处理中', color: 'processing' },
  completed: { label: '已完成', color: 'success' }, failed: { label: '失败', color: 'error' }, guarded: { label: '已拦截', color: 'warning' },
}

function buildTree(messages: AgentMessageAuditItem[]) {
  const ids = new Set(messages.map((item) => item.id))
  const children = new Map<string, AgentMessageAuditItem[]>()
  messages.forEach((item) => {
    const parent = item.parentMessageId && ids.has(item.parentMessageId) ? item.parentMessageId : '__root__'
    children.set(parent, [...(children.get(parent) ?? []), item])
  })
  const result: { item: AgentMessageAuditItem; depth: number }[] = []
  const append = (parent: string, depth: number): void => {
    (children.get(parent) ?? []).sort((a, b) => a.createdAt.localeCompare(b.createdAt)).forEach((item) => { result.push({ item, depth }); append(item.id, depth + 1) })
  }
  append('__root__', 0)
  return result
}

export default function CollaborationAuditPage() {
  const { styles } = useStyles()
  const [params, setParams] = useSearchParams()
  const initialKind = params.has('root_message_id') ? 'root' : 'conversation'
  const initialValue = params.get('root_message_id') || params.get('conversation_id') || ''
  const [kind, setKind] = useState<MessageChainSelector['kind']>(initialKind)
  const [value, setValue] = useState(initialValue)
  const [selector, setSelector] = useState<MessageChainSelector | undefined>(initialValue ? { kind: initialKind, value: initialValue } : undefined)
  const query = useAgentMessageChain(selector)
  const tree = useMemo(() => buildTree(query.data?.messages ?? []), [query.data?.messages])
  const search = () => {
    const normalized = value.trim()
    if (!normalized) return
    const next = { kind, value: normalized } as MessageChainSelector
    setSelector(next)
    setParams(kind === 'conversation' ? { conversation_id: normalized } : { root_message_id: normalized })
  }
  return <main className={styles.page}>
    <h1 className={styles.title}>协作审计</h1>
    <p className={styles.subtitle}>核验 Agent 是否真的发出消息、收到回复并继续转交。这里展示 Hub 留下的系统证据，不采用 Agent 自己对执行过程的描述。</p>
    <div className={styles.search}>
      <Select aria-label="线索类型" value={kind} onChange={setKind} options={[{ value: 'conversation', label: '会话 ID' }, { value: 'root', label: '根消息 ID' }]} />
      <Input aria-label="审计线索" value={value} onChange={(event) => setValue(event.target.value)} onPressEnter={search} placeholder={kind === 'conversation' ? '输入 conversation_id' : '输入 root_message_id'} />
      <PrimaryButton icon={<MagnifyingGlassIcon />} onClick={search} disabled={!value.trim()}>查询证据</PrimaryButton>
    </div>
    {!selector ? <div className={styles.empty}><Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="输入聊天工具卡片或运行档案中的会话/根消息 ID" /></div> : query.isLoading ? <div className={styles.empty}><Skeleton active paragraph={{ rows: 5 }} /></div> : query.isError ? <Alert style={{ marginTop: 20 }} type="error" showIcon title="没有找到协作证据" description={parseApiError(query.error)} /> : query.data && <>
      <div className={styles.summary}>{query.data.verified ? <CheckCircleIcon color="var(--success)" weight="fill" /> : <WarningCircleIcon color="var(--warning)" weight="fill" />}<strong>{query.data.verified ? '整条协作链已验证' : '协作链仍有未验证结果'}</strong><Tag>{query.data.messageCount} 次联络</Tag><span>会话 {query.data.conversationId}</span><span>根消息 {query.data.rootMessageId}</span></div>
      <div className={styles.chain}>{tree.map(({ item, depth }) => {
        const status = STATUS[item.status] ?? { label: item.status, color: 'default' }
        return <div className={styles.row} key={item.id} style={{ marginLeft: Math.min(depth, 5) * 22 }}>
          <div className={styles.route}><span>{item.sourceAgent}</span><ArrowRightIcon /><span>{item.targetAgent}</span></div>
          <div className={styles.detail}>第 {item.hop}/{item.maxHops} 跳 · {item.action} · {item.deliveryPolicy === 'sync' ? '同步' : '异步'}<br />消息 {item.id}{item.parentMessageId ? ` · 上一步 ${item.parentMessageId}` : ''}<br />{formatTime(item.createdAt)}{item.routeDeviation ? ` · 路径偏离：${item.routeDeviation}` : ''}</div>
          <div className={styles.status}><Tag color={status.color}>{status.label}</Tag><Tag color={item.verified ? 'success' : 'warning'}>{item.verified ? '已验证' : '未验证'}</Tag></div>
          {(item.content || item.reply || item.error || item.verification) && <div className={styles.content}><div className={styles.evidence}><strong>请求与系统判定</strong><br />{item.content || '未保存请求正文'}{item.error ? `\n错误：${item.error}` : ''}<br />核验：{item.verification || '尚无核验结论'}</div><div className={styles.evidence}><strong>目标 Agent 回复</strong><br />{item.reply || '尚未取得回复'}</div></div>}
        </div>
      })}</div>
    </>}
  </main>
}
