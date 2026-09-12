import { useEffect, useMemo, useState } from 'react'
import { Alert, Button, Collapse, Empty, Form, Input, Modal, Select, Skeleton, Tag } from 'antd'
import {
  ArrowRightIcon,
  ArrowBendUpRightIcon,
  ArrowsLeftRightIcon,
  CheckCircleIcon,
  ChatCircleTextIcon,
  ClockCounterClockwiseIcon,
  PauseCircleIcon,
  PlayCircleIcon,
  RobotIcon,
  EyeIcon,
  StackIcon,
  PlusIcon,
} from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import { useNavigate, useParams } from 'react-router'
import type { AgentMessage, PromptSnapshot, Run, RunActivity, RunEventItem, RunState, RunStateChange, RunStatus, ToolResultRecord } from '@/api/runs'
import { parseApiError } from '@/api/client'
import PrimaryButton from '@/components/PrimaryButton'
import { useCanWrite } from '@/hooks/useCanWrite'
import { useAgents } from '@/queries/useAgents'
import { useAddRunAgent, useComposeRunPrompt, useCreateRun, useEnabledCapabilityPackages, useRun, useRunActivities, useRunAgentMessages, useRunEvents, useRuns, useRunStateChanges, useRunToolResults, useTransitionRun } from '@/queries/useRuns'
import { formatTime } from '@/utils/time'
import { tokens as t } from '@/styles/tokens'

const STATUS: Record<RunStatus, { label: string; tone: string; Icon: typeof PlayCircleIcon }> = {
  draft: { label: '待开始', tone: 'var(--text-muted)', Icon: ClockCounterClockwiseIcon },
  running: { label: '进行中', tone: 'var(--success)', Icon: PlayCircleIcon },
  paused: { label: '已暂停', tone: 'var(--warning)', Icon: PauseCircleIcon },
  completed: { label: '已完成', tone: 'var(--success)', Icon: CheckCircleIcon },
  archived: { label: '已归档', tone: 'var(--text-muted)', Icon: StackIcon },
}

const useStyles = createStyles(({ css }) => ({
  page: css`
    width: 100%; max-width: 1480px; margin: 0 auto;
    animation: runPageIn .28s ease;
    @keyframes runPageIn { from { opacity: 0; transform: translateY(5px); } }
    @media (prefers-reduced-motion: reduce) { animation: none; }
  `,
  head: css`
    display: flex; align-items: flex-end; justify-content: space-between; gap: 24px; margin-bottom: 20px;
    @media (max-width: 720px) { align-items: flex-start; flex-direction: column; }
  `,
  title: css`margin: 0; color: ${t.text}; font-size: ${t.text3xl}; font-weight: 720; letter-spacing: -.035em; line-height: 1.12;`,
  subtitle: css`max-width: 68ch; margin: 7px 0 0; color: ${t.textTertiary}; font-size: ${t.textBase}; line-height: 1.6;`,
  scope: css`
    display: flex; align-items: center; gap: 8px; padding: 8px 11px; border: 1px solid var(--border);
    border-radius: ${t.radiusSm}px; background: var(--card); color: ${t.textSecondary}; font-size: 12px; white-space: nowrap;
  `,
  scopeDot: css`width: 7px; height: 7px; border-radius: 50%; background: var(--success); box-shadow: 0 0 0 4px color-mix(in srgb, var(--success) 14%, transparent);`,
  shell: css`
    display: grid; grid-template-columns: minmax(280px, .78fr) minmax(0, 2fr); min-height: 670px;
    overflow: hidden; border: 1px solid var(--border); border-radius: ${t.radius}px; background: var(--card); box-shadow: ${t.elevation1};
    @media (max-width: 900px) { grid-template-columns: 1fr; overflow: visible; }
  `,
  rail: css`
    min-width: 0; border-right: 1px solid var(--border); background: color-mix(in srgb, var(--background) 64%, var(--card));
    @media (max-width: 900px) { border-right: 0; border-bottom: 1px solid var(--border); }
  `,
  railHead: css`padding: 16px; border-bottom: 1px solid var(--border);`,
  railTitle: css`margin-bottom: 10px; color: ${t.text}; font-size: ${t.textSm}; font-weight: 650;`,
  runList: css`max-height: 610px; overflow-y: auto;`,
  runButton: css`
    width: 100%; padding: 16px; border: 0; border-bottom: 1px solid var(--border); background: transparent; text-align: left; cursor: pointer;
    transition: background .16s ease, box-shadow .16s ease;
    &:hover { background: var(--card); }
    &:focus-visible { outline: 2px solid var(--ring); outline-offset: -2px; }
  `,
  runButtonActive: css`background: var(--card); box-shadow: inset 3px 0 0 var(--primary);`,
  runTop: css`display: flex; align-items: flex-start; justify-content: space-between; gap: 10px;`,
  runName: css`color: ${t.text}; font-size: ${t.textBase}; font-weight: 650; line-height: 1.35;`,
  runDesc: css`display: -webkit-box; margin-top: 6px; overflow: hidden; color: ${t.textTertiary}; font-size: 12px; line-height: 1.5; -webkit-line-clamp: 2; -webkit-box-orient: vertical;`,
  runMeta: css`display: flex; align-items: center; gap: 10px; margin-top: 11px; color: ${t.textMuted}; font-size: 11px;`,
  status: css`display: inline-flex; align-items: center; gap: 5px; flex: 0 0 auto; font-size: 11px; font-weight: 650;`,
  detail: css`min-width: 0; padding: 24px; @media (max-width: 620px) { padding: 17px; }`,
  detailHead: css`display: flex; align-items: flex-start; justify-content: space-between; gap: 18px; padding-bottom: 20px; border-bottom: 1px solid var(--border);`,
  detailActions: css`display: flex; align-items: center; justify-content: flex-end; flex-wrap: wrap; gap: 8px;`,
  detailTitle: css`margin: 0; color: ${t.text}; font-size: ${t.text2xl}; font-weight: 700; letter-spacing: -.025em;`,
  detailDesc: css`max-width: 70ch; margin: 7px 0 0; color: ${t.textTertiary}; font-size: ${t.textSm}; line-height: 1.6;`,
  facts: css`
    display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 0; margin: 20px 0;
    border: 1px solid var(--border); border-radius: ${t.radiusSm}px; background: var(--background);
    @media (max-width: 700px) { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  `,
  bindings: css`display: flex; flex-wrap: wrap; gap: 8px; margin: -7px 0 20px;`,
  binding: css`display: flex; align-items: center; gap: 8px; padding: 8px 10px; border: 1px solid var(--border); border-radius: ${t.radiusSm}px; background: var(--card); color: ${t.textSecondary}; font-size: 12px; strong { color: ${t.text}; font-weight: 640; }`,
  fact: css`padding: 13px 15px; border-right: 1px solid var(--border); &:last-child { border-right: 0; } @media (max-width: 700px) { &:nth-child(2) { border-right: 0; } &:nth-child(-n+2) { border-bottom: 1px solid var(--border); } }`,
  factLabel: css`color: ${t.textMuted}; font-size: 11px;`,
  factValue: css`margin-top: 3px; overflow: hidden; color: ${t.text}; font-size: ${t.textSm}; font-weight: 620; text-overflow: ellipsis; white-space: nowrap;`,
  grid: css`display: grid; grid-template-columns: minmax(230px, .78fr) minmax(0, 1.4fr); gap: 20px; @media (max-width: 760px) { grid-template-columns: 1fr; }`,
  section: css`min-width: 0;`,
  sectionTitle: css`display: flex; align-items: center; gap: 7px; margin: 0 0 10px; color: ${t.text}; font-size: ${t.textBase}; font-weight: 680;`,
  quietList: css`overflow: hidden; border: 1px solid var(--border); border-radius: ${t.radiusSm}px;`,
  person: css`display: flex; align-items: center; gap: 10px; padding: 11px 12px; border-bottom: 1px solid var(--border); &:last-child { border-bottom: 0; }`,
  avatar: css`display: grid; width: 30px; height: 30px; flex: 0 0 auto; place-items: center; border-radius: 9px; background: var(--primary-soft); color: var(--primary);`,
  personName: css`min-width: 0; flex: 1; color: ${t.text}; font-size: ${t.textSm}; font-weight: 620;`,
  role: css`color: ${t.textMuted}; font-size: 11px;`,
  promptIntro: css`margin: 0 0 16px; color: ${t.textTertiary}; font-size: ${t.textBase}; line-height: 1.65;`,
  promptSummary: css`display: flex; flex-wrap: wrap; gap: 16px; padding: 12px 0 16px; border-bottom: 1px solid var(--border); color: ${t.textSecondary}; font-size: ${t.textSm};`,
  promptSource: css`padding: 13px 0; border-bottom: 1px solid var(--border); &:last-child { border-bottom: 0; }`,
  promptSourceTop: css`display: flex; align-items: baseline; justify-content: space-between; gap: 12px;`,
  promptSourceName: css`color: ${t.text}; font-size: ${t.textBase}; font-weight: 650;`,
  promptSourceMeta: css`margin-top: 4px; color: ${t.textMuted}; font-size: ${t.textSm}; line-height: 1.5;`,
  promptText: css`max-height: 310px; margin: 0; overflow: auto; color: ${t.textSecondary}; font-family: ${t.fontMono}; font-size: 12px; line-height: 1.7; white-space: pre-wrap; word-break: break-word;`,
  addAgent: css`display: grid; grid-template-columns: minmax(0, 1fr) 110px auto; gap: 8px; margin-bottom: 10px; @media (max-width: 600px) { grid-template-columns: 1fr; }`,
  stateCard: css`padding: 12px; border-bottom: 1px solid var(--border); &:last-child { border-bottom: 0; }`,
  stateHead: css`display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 9px; color: ${t.textSecondary}; font-size: 12px;`,
  stateGrid: css`display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 7px;`,
  stateValue: css`padding: 8px 9px; border-radius: 7px; background: var(--background);`,
  stateKey: css`overflow: hidden; color: ${t.textMuted}; font-size: 10px; text-overflow: ellipsis; white-space: nowrap;`,
  stateData: css`margin-top: 2px; overflow: hidden; color: ${t.text}; font-family: ${t.fontMono}; font-size: 12px; text-overflow: ellipsis; white-space: nowrap;`,
  timelineSection: css`margin-top: 22px;`,
  timeline: css`position: relative; margin-left: 7px; padding-left: 23px; border-left: 1px solid var(--border);`,
  change: css`position: relative; padding: 0 0 19px 2px; &::before { position: absolute; top: 4px; left: -29px; width: 10px; height: 10px; border: 2px solid var(--card); border-radius: 50%; background: var(--primary); box-shadow: 0 0 0 1px var(--border); content: ''; }`,
  changeTop: css`display: flex; align-items: baseline; justify-content: space-between; gap: 12px;`,
  changeTitle: css`color: ${t.text}; font-size: ${t.textSm}; font-weight: 630;`,
  changeTime: css`color: ${t.textMuted}; font-size: 11px; white-space: nowrap;`,
  changeBody: css`margin-top: 4px; color: ${t.textTertiary}; font-size: 12px; line-height: 1.5;`,
  future: css`margin-top: 20px; padding-top: 15px; border-top: 1px dashed var(--border); color: ${t.textMuted}; font-size: 12px; line-height: 1.6;`,
  collaboration: css`margin-top: 22px; border-top: 1px solid var(--border); padding-top: 20px;`,
  collaborationHead: css`display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; margin-bottom: 12px; @media (max-width: 620px) { flex-direction: column; }`,
  collaborationHelp: css`margin: -5px 0 0; color: ${t.textTertiary}; font-size: 12px; line-height: 1.6;`,
  chain: css`overflow: hidden; border: 1px solid var(--border); border-radius: ${t.radiusSm}px;`,
  chainHead: css`display: flex; align-items: center; flex-wrap: wrap; gap: 7px; padding: 10px 12px; border-bottom: 1px solid var(--border); background: var(--background); color: ${t.textSecondary}; font-size: 12px;`,
  chainNode: css`color: ${t.text}; font-weight: 650;`,
  hopRow: css`display: grid; grid-template-columns: 56px minmax(150px, .8fr) minmax(170px, 1fr) minmax(150px, .8fr); align-items: center; gap: 12px; padding: 12px; border-bottom: 1px solid var(--border); &:last-child { border-bottom: 0; } @media (max-width: 720px) { grid-template-columns: 48px minmax(0, 1fr); }`,
  hopNumber: css`color: ${t.textMuted}; font-family: ${t.fontMono}; font-size: 11px;`,
  route: css`display: flex; align-items: center; gap: 7px; min-width: 0; color: ${t.text}; font-size: ${t.textSm}; font-weight: 620; span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }`,
  hopMeta: css`min-width: 0; color: ${t.textTertiary}; font-size: 12px; line-height: 1.5; @media (max-width: 720px) { grid-column: 2; }`,
  hopStatus: css`display: flex; justify-content: flex-end; gap: 7px; align-items: center; color: ${t.textSecondary}; font-size: 12px; @media (max-width: 720px) { grid-column: 2; justify-content: flex-start; }`,
  guide: css`padding: 16px; border: 1px dashed var(--border); border-radius: ${t.radiusSm}px; background: var(--background);`,
  guideSteps: css`display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 0; margin-bottom: 14px; @media (max-width: 720px) { grid-template-columns: 1fr; }`,
  guideStep: css`padding: 0 14px; border-right: 1px solid var(--border); color: ${t.textTertiary}; font-size: 12px; line-height: 1.55; &:first-child { padding-left: 0; } &:last-child { padding-right: 0; border-right: 0; } strong { display: block; margin-bottom: 3px; color: ${t.text}; font-size: ${t.textSm}; } @media (max-width: 720px) { padding: 10px 0; border-right: 0; border-bottom: 1px solid var(--border); &:first-child { padding-top: 0; } &:last-child { padding-bottom: 0; border-bottom: 0; } }`,
  center: css`display: grid; min-height: 470px; place-items: center; padding: 30px;`,
}))

function RunStatusLabel({ status }: { status: RunStatus }) {
  const { styles } = useStyles()
  const config = STATUS[status] ?? STATUS.draft
  const Icon = config.Icon
  return <span className={styles.status} style={{ color: config.tone }}><Icon size={14} weight="fill" />{config.label}</span>
}

function valueLabel(value: unknown) {
  if (value === null) return '空'
  if (typeof value === 'object') return JSON.stringify(value)
  if (typeof value === 'boolean') return value ? '是' : '否'
  return String(value)
}

function StateList({ states }: { states: RunState[] }) {
  const { styles } = useStyles()
  if (states.length === 0) return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="这次运行尚未初始化状态" />
  return <div className={styles.quietList}>{states.map((state) => (
    <div className={styles.stateCard} key={state.id}>
      <div className={styles.stateHead}><span>{state.subjectType}{state.subjectId ? ` · ${state.subjectId}` : ''}</span><Tag variant="filled">v{state.revision}</Tag></div>
      <div className={styles.stateGrid}>{Object.entries(state.data ?? {}).map(([key, value]) => (
        <div className={styles.stateValue} key={key}><div className={styles.stateKey}>{key}</div><div className={styles.stateData} title={valueLabel(value)}>{valueLabel(value)}</div></div>
      ))}</div>
    </div>
  ))}</div>
}

function ChangeTimeline({ changes }: { changes: RunStateChange[] }) {
  const { styles } = useStyles()
  if (changes.length === 0) return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="状态尚未发生变化" />
  return <div className={styles.timeline}>{changes.map((change) => (
    <div className={styles.change} key={change.id}>
      <div className={styles.changeTop}><span className={styles.changeTitle}>{change.reason || '更新了运行状态'}</span><time className={styles.changeTime}>{formatTime(change.createdAt)}</time></div>
      <div className={styles.changeBody}>{change.source ? `${change.source} · ` : ''}修订 {change.revisionBefore} → {change.revisionAfter}</div>
    </div>
  ))}</div>
}

const ACTIVITY_LABEL: Record<string, string> = {
  participant: '参与运行', started: '开始执行', tool_started: '调用工具',
  tool_finished: '工具完成', completed: '执行完成', failed: '执行失败',
}

function ActivityTimeline({ activities }: { activities: RunActivity[] }) {
  const { styles } = useStyles()
  if (activities.length === 0) return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="尚未开始执行" />
  return <div className={styles.timeline}>{activities.map((activity) => (
    <div className={styles.change} key={activity.id}>
      <div className={styles.changeTop}><span className={styles.changeTitle}>{ACTIVITY_LABEL[activity.kind] ?? activity.kind}{activity.name ? ` · ${activity.name}` : ''}</span><time className={styles.changeTime}>{formatTime(activity.occurredAt)}</time></div>
      <div className={styles.changeBody}>{activity.actorId ? `${activity.actorId} · ` : ''}{activity.error ? `失败原因：${activity.error}` : activity.status || '已记录'}</div>
    </div>
  ))}</div>
}

const DELIVERY_LABEL: Record<string, string> = { pending: '等待处理', processing: '处理中', retry: '正在重试', delivered: '已处理', cancelled: '已取消', dead_letter: '处理失败' }

function EventTimeline({ items }: { items: RunEventItem[] }) {
  const { styles } = useStyles()
  if (items.length === 0) return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="还没有形成可追溯的因果事件" />
  return <div className={styles.timeline}>{items.map(({ event, delivery }) => <div className={styles.change} key={event.id}>
    <div className={styles.changeTop}><span className={styles.changeTitle}>{event.type.replace(/\.v\d+$/, '').replaceAll('.', ' · ')}</span><time className={styles.changeTime}>{formatTime(event.occurredAt || event.recordedAt)}</time></div>
    <div className={styles.changeBody}>{event.actor?.id ? `${event.actor.id} 发起 · ` : ''}{DELIVERY_LABEL[delivery?.status ?? ''] ?? '已记录'}{event.causationId ? ' · 由上一事件触发' : ' · 因果链起点'}</div>
  </div>)}</div>
}

const TOOL_DECISION: Record<string, string> = { accepted: '等待规则确认', rejected: '未生效', applied: '已生效' }

function ToolDecisionList({ items }: { items: ToolResultRecord[] }) {
  const { styles } = useStyles()
  if (items.length === 0) return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="工具尚未提出状态变化" />
  return <div className={styles.quietList}>{items.map((item) => <div className={styles.stateCard} key={item.id}>
    <div className={styles.stateHead}><span>{item.actorId ? `${item.actorId} 使用了 ` : ''}{item.toolName}</span><Tag color={item.status === 'applied' ? 'success' : item.status === 'rejected' ? 'error' : 'processing'} variant="filled">{TOOL_DECISION[item.status] ?? item.status}</Tag></div>
    <div className={styles.changeBody}>{item.status === 'rejected' ? `原因：${item.decisionReason || '未通过规则校验'}` : item.stateProposals?.length ? `${item.stateProposals.length} 项状态建议，${item.committedChangeIds?.length ?? 0} 项已经写入` : '工具只返回结果，没有要求修改状态'}</div>
  </div>)}</div>
}

const MESSAGE_STATUS: Record<string, { label: string; color?: string }> = {
  queued: { label: '等待接收', color: 'default' }, running: { label: '处理中', color: 'processing' },
  completed: { label: '已回复', color: 'success' }, failed: { label: '执行失败', color: 'error' }, guarded: { label: '已拦截', color: 'warning' },
}
const GUARD_REASON: Record<string, string> = {
  route_not_found: '双方没有允许这次联络的关系', action_not_allowed: '这段关系不允许该动作',
  run_participant_denied: '目标 Agent 未加入本次运行',
  max_hops_exceeded: '已达到最大传递层数', event_budget_exceeded: '本次协作的消息额度已用完',
  token_budget_exceeded: '本次协作的内容额度已用完', deadline_exceeded: '已超过本次协作的截止时间',
  sync_wait_cycle: '同步等待会形成死锁', repeated_agent: '检测到可能反复传递的路径',
}
const ACTION_LABEL: Record<string, string> = { inform: '告知', consult: '征询', assign: '指派', report: '汇报', submit: '提交', review: '审核', challenge: '质疑', handoff: '移交', escalate: '越级汇报', invite: '邀请' }

function elapsed(from?: string, to?: string) {
  if (!from || !to) return '—'
  const milliseconds = Math.max(0, new Date(to).getTime() - new Date(from).getTime())
  if (milliseconds < 1000) return `${milliseconds}ms`
  if (milliseconds < 60_000) return `${(milliseconds / 1000).toFixed(milliseconds < 10_000 ? 1 : 0)} 秒`
  return `${Math.floor(milliseconds / 60_000)} 分 ${Math.round((milliseconds % 60_000) / 1000)} 秒`
}

function CollaborationTimeline({ items }: { items: AgentMessage[] }) {
  const { styles } = useStyles()
  const conversations = useMemo(() => {
    const grouped = new Map<string, AgentMessage[]>()
    items.forEach((item) => grouped.set(item.conversationId || item.rootMessageId || item.id, [...(grouped.get(item.conversationId || item.rootMessageId || item.id) ?? []), item]))
    return [...grouped.values()].map((messages) => messages.sort((a, b) => a.hop - b.hop || a.createdAt.localeCompare(b.createdAt)))
  }, [items])
  return <div style={{ display: 'grid', gap: 12 }}>{conversations.map((messages) => {
    const first = messages[0]
    const routeNames = [first.sourceAgent, ...messages.map((item) => item.targetAgent)]
    return <div className={styles.chain} key={first.conversationId || first.id}>
      <div className={styles.chainHead}><span>协作路径</span>{routeNames.map((name, index) => <span key={`${name}-${index}`} style={{ display: 'contents' }}>{index > 0 && <ArrowRightIcon size={12} />}<span className={styles.chainNode}>{name}</span></span>)}</div>
      {messages.map((item) => {
        const status = MESSAGE_STATUS[item.status] ?? { label: item.status }
        const isReturn = item.hop > 1 && item.targetAgent === first.sourceAgent
        const reason = item.guardDescription || (item.guardReason ? (GUARD_REASON[item.guardReason] ?? item.guardReason) : item.error)
        return <div className={styles.hopRow} key={item.id}>
          <div className={styles.hopNumber}>第 {item.hop} 跳</div>
          <div className={styles.route}><span>{item.sourceAgent}</span>{item.action === 'escalate' ? <ArrowBendUpRightIcon size={15} /> : <ArrowRightIcon size={15} />}<span>{item.targetAgent}</span></div>
          <div className={styles.hopMeta}>{isReturn ? '回报发起人 · ' : ''}{ACTION_LABEL[item.action] ?? item.action} · {item.deliveryPolicy === 'sync' ? '同步等待' : '异步投递'}<br />排队 {elapsed(item.createdAt, item.startedAt)}{item.startedAt ? ` · 处理 ${elapsed(item.startedAt, item.completedAt)}` : ''}{reason ? ` · 停止原因：${reason}` : ''}</div>
          <div className={styles.hopStatus}><Tag color={status.color} variant="filled">{status.label}</Tag><span>{item.hop}/{item.maxHops} 跳</span></div>
        </div>
      })}
    </div>
  })}</div>
}

function CollaborationGuide({ run, onConfigure }: { run: Run; onConfigure: () => void }) {
  const { styles } = useStyles()
  const firstAgent = run.agents?.[0]
  const navigate = useNavigate()
  return <div className={styles.guide}>
    <div className={styles.guideSteps}>
      <div className={styles.guideStep}><strong>1. 配置传递方向</strong>为 A→B、B→C 和需要的 C→A 分别建立关系，并选择允许的动作。</div>
      <div className={styles.guideStep}><strong>2. 从 A 发起任务</strong>进入本次运行对话，用自然语言要求 A 交给 B；Agent 会根据关系决定是否继续传递。</div>
      <div className={styles.guideStep}><strong>3. 回到这里复盘</strong>页面自动刷新每一跳，并显示越级、拦截、排队和预算停止原因。</div>
    </div>
    <div className={styles.detailActions} style={{ justifyContent: 'flex-start' }}><Button onClick={onConfigure}>配置 Agent 关系</Button>{run.status === 'running' && firstAgent && <PrimaryButton icon={<ChatCircleTextIcon size={16} />} onClick={() => void navigate(`/agents/${encodeURIComponent(firstAgent.agentNameSnapshot)}/chat?runId=${encodeURIComponent(run.id)}`)}>从 {firstAgent.agentNameSnapshot} 发起测试</PrimaryButton>}</div>
  </div>
}

const NEXT_ACTION: Partial<Record<RunStatus, { label: string; target: RunStatus }[]>> = {
  draft: [{ label: '开始运行', target: 'running' }, { label: '归档', target: 'archived' }],
  running: [{ label: '暂停', target: 'paused' }, { label: '完成运行', target: 'completed' }],
  paused: [{ label: '继续运行', target: 'running' }, { label: '完成运行', target: 'completed' }],
  completed: [{ label: '归档', target: 'archived' }],
}

const PROMPT_STAGE_LABEL: Record<string, string> = {
  platform_safety: '平台安全边界', identity: '身份', responsibilities: '职责',
  personality_baseline: '人格基线', organization_policy: '组织角色', dynamic_state: '运行中的状态',
  relationship_context: '与其他 Agent 的关系', application_context: '本次任务背景',
}

function PromptExplanation({ snapshot }: { snapshot: PromptSnapshot }) {
  const { styles } = useStyles()
  const totalTokens = snapshot.provenance.reduce((sum, item) => sum + item.tokenEstimate, 0)
  const deliveryLabel = snapshot.deliveryStatus === 'delivered' ? '本次对话实际使用' : snapshot.deliveryStatus === 'failed' ? '发送失败，未被 Agent 使用' : '预览，尚未用于对话'
  return <>
    <p className={styles.promptIntro}>这是平台按本次运行档案生成的判断背景。内容按固定顺序合并，并保留每一部分的来源；人格和上下文不会赋予额外权限。</p>
    <div className={styles.promptSummary}><span>{deliveryLabel}</span><span>{snapshot.provenance.length} 个判断依据</span><span>约 {totalTokens} tokens</span><span>版本指纹 {snapshot.renderedHash.slice(0, 10)}</span></div>
    <div>{snapshot.provenance.map((item) => <div className={styles.promptSource} key={`${item.stage}-${item.sourceId}-${item.contentHash}`}>
      <div className={styles.promptSourceTop}><span className={styles.promptSourceName}>{PROMPT_STAGE_LABEL[item.stage] ?? item.label}</span><Tag variant="filled">约 {item.tokenEstimate} tokens</Tag></div>
      <div className={styles.promptSourceMeta}>{item.label} · 来源：{item.sourceType === 'platform' ? 'Agent Hub' : item.sourceId || '本次运行'}{item.sourceVersion ? ` · 版本 ${item.sourceVersion}` : ''}</div>
    </div>)}</div>
    <Collapse ghost size="small" items={[{ key: 'rendered', label: '查看完整合成文本', children: <pre className={styles.promptText}>{snapshot.renderedText}</pre> }]} />
  </>
}

function RunDetailPanel({ id }: { id: string }) {
  const navigate = useNavigate()
  const { styles } = useStyles()
  const canWrite = useCanWrite()
  const detail = useRun(id)
  const history = useRunStateChanges(id)
  const activities = useRunActivities(id)
  const events = useRunEvents(id)
  const toolResults = useRunToolResults(id)
  const agentMessages = useRunAgentMessages(id)
  const agents = useAgents()
  const transition = useTransitionRun()
  const addAgent = useAddRunAgent()
  const composePrompt = useComposeRunPrompt()
  const [promptSnapshot, setPromptSnapshot] = useState<PromptSnapshot | null>(null)
  const [promptAgentName, setPromptAgentName] = useState('')
  const [agentId, setAgentId] = useState<number>()
  const [role, setRole] = useState('参与者')
  if (detail.isLoading) return <div className={styles.detail}><Skeleton active paragraph={{ rows: 10 }} /></div>
  if (detail.isError || !detail.data) return <div className={styles.center}><Alert type="error" showIcon title="无法打开这次运行" description={parseApiError(detail.error)} /></div>
  const { run, states } = detail.data
  const assigned = new Set((run.agents ?? []).map((agent) => agent.agentId))
  const agentOptions = (agents.data ?? []).filter((agent) => !assigned.has(agent.id)).map((agent) => ({ value: agent.id, label: agent.config.title?.['zh-CN'] || agent.name }))
  return <article className={styles.detail}>
    <header className={styles.detailHead}><div><h2 className={styles.detailTitle}>{run.name}</h2><p className={styles.detailDesc}>{run.description || '未填写运行说明'}</p></div><div className={styles.detailActions}><RunStatusLabel status={run.status} />{canWrite && (NEXT_ACTION[run.status] ?? []).map((action, index) => index === 0 ? <PrimaryButton key={action.target} loading={transition.isPending} onClick={() => transition.mutate({ id, status: action.target })}>{action.label}</PrimaryButton> : <Button key={action.target} disabled={transition.isPending} onClick={() => transition.mutate({ id, status: action.target })}>{action.label}</Button>)}</div></header>
    <div className={styles.facts}>
      <div className={styles.fact}><div className={styles.factLabel}>发起人</div><div className={styles.factValue}>{run.createdBy || '系统'}</div></div>
      <div className={styles.fact}><div className={styles.factLabel}>参与者</div><div className={styles.factValue}>{run.agents?.length ?? 0} 个 Agent</div></div>
      <div className={styles.fact}><div className={styles.factLabel}>创建时间</div><div className={styles.factValue}>{formatTime(run.createdAt)}</div></div>
      <div className={styles.fact}><div className={styles.factLabel}>能力版本锁定</div><div className={styles.factValue}>{run.capabilityBindings?.length ?? 0} 项</div></div>
    </div>
    {(run.capabilityBindings?.length ?? 0) > 0 && <div className={styles.bindings} aria-label="已锁定能力包">{run.capabilityBindings?.map((binding) => <div className={styles.binding} key={`${binding.namespace}-${binding.version}`}><strong>{binding.packageName}</strong><span>{binding.version}</span><Tag color="success" variant="filled">本次固定</Tag></div>)}</div>}
    <div className={styles.grid}>
      <section className={styles.section}><h3 className={styles.sectionTitle}><RobotIcon size={17} />谁在参与</h3>
        {canWrite && run.status === 'draft' && <div className={styles.addAgent}><Select aria-label="选择 Agent" value={agentId} onChange={setAgentId} options={agentOptions} placeholder="选择 Agent" showSearch optionFilterProp="label" /><Input aria-label="参与角色" value={role} onChange={(event) => setRole(event.target.value)} placeholder="参与角色" /><PrimaryButton icon={<PlusIcon size={15} />} disabled={!agentId} loading={addAgent.isPending} onClick={() => agentId && addAgent.mutate({ id, agentId, role: role.trim() || '参与者' }, { onSuccess: () => setAgentId(undefined) })}>添加</PrimaryButton></div>}
        {(run.agents?.length ?? 0) === 0 ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="尚未添加 Agent" /> : <div className={styles.quietList}>{run.agents?.map((agent) => <div className={styles.person} key={agent.id}><span className={styles.avatar}><RobotIcon size={16} /></span><span className={styles.personName}>{agent.agentNameSnapshot || `Agent ${agent.agentId}`}</span><span className={styles.role}>{agent.role || '参与者'}</span>{run.status === 'running' && <Button size="small" icon={<ChatCircleTextIcon size={16} />} onClick={() => void navigate(`/agents/${encodeURIComponent(agent.agentNameSnapshot)}/chat?runId=${encodeURIComponent(id)}`)}>进入本次对话</Button>}<Button size="small" icon={<EyeIcon size={16} />} loading={composePrompt.isPending && promptAgentName === agent.agentNameSnapshot} onClick={() => { setPromptAgentName(agent.agentNameSnapshot); composePrompt.mutate({ id, agentId: agent.agentId }, { onSuccess: setPromptSnapshot }) }}>查看判断背景</Button></div>)}</div>}
      </section>
      <section className={styles.section}><h3 className={styles.sectionTitle}><StackIcon size={17} />当前状态</h3><StateList states={states ?? []} /></section>
    </div>
    <section className={styles.timelineSection}><h3 className={styles.sectionTitle}><PlayCircleIcon size={17} />执行过程</h3>
      {activities.isError ? <Alert type="error" showIcon title="执行过程加载失败" description={parseApiError(activities.error)} /> : activities.isLoading ? <Skeleton active paragraph={{ rows: 3 }} /> : <ActivityTimeline activities={activities.data ?? []} />}
    </section>
    <section className={styles.collaboration}>
      <div className={styles.collaborationHead}><div><h3 className={styles.sectionTitle}><ArrowsLeftRightIcon size={17} />Agent 协作链</h3><p className={styles.collaborationHelp}>展示实际发生的 A→B→C、回报和越级尝试。每一跳都单独经过关系、动作权限和运行预算检查。</p></div>{(agentMessages.data?.length ?? 0) > 0 && <Button onClick={() => void navigate('/relations')}>查看关系配置</Button>}</div>
      {agentMessages.isError ? <Alert type="error" showIcon title="协作记录加载失败" description={`${parseApiError(agentMessages.error)}。确认后端已升级到 H3 版本后重试。`} /> : agentMessages.isLoading ? <Skeleton active paragraph={{ rows: 4 }} /> : (agentMessages.data?.length ?? 0) > 0 ? <CollaborationTimeline items={agentMessages.data ?? []} /> : <CollaborationGuide run={run} onConfigure={() => void navigate('/relations')} />}
    </section>
    <section className={styles.timelineSection}><h3 className={styles.sectionTitle}><ClockCounterClockwiseIcon size={17} />状态变化</h3>
      {history.isError ? <Alert type="error" showIcon title="变化记录加载失败" description={parseApiError(history.error)} /> : history.isLoading ? <Skeleton active paragraph={{ rows: 3 }} /> : <ChangeTimeline changes={history.data ?? []} />}
    </section>
    <div className={styles.grid} style={{ marginTop: 22 }}>
      <section className={styles.section}><h3 className={styles.sectionTitle}>工具提出的变化是否生效</h3>{toolResults.isError ? <Alert type="error" showIcon title="工具结果加载失败" description={parseApiError(toolResults.error)} /> : toolResults.isLoading ? <Skeleton active paragraph={{ rows: 3 }} /> : <ToolDecisionList items={toolResults.data ?? []} />}</section>
      <section className={styles.section}><h3 className={styles.sectionTitle}>事情为什么会接着发生</h3>{events.isError ? <Alert type="error" showIcon title="因果记录加载失败" description={parseApiError(events.error)} /> : events.isLoading ? <Skeleton active paragraph={{ rows: 3 }} /> : <EventTimeline items={events.data ?? []} />}</section>
    </div>
    <div className={styles.future}>H3 在原有运行档案上增加了可控多跳协作：关系决定能不能传，预算决定什么时候停，每一跳都可以复盘。</div>
    <Modal title={`${promptAgentName || 'Agent'} 的判断背景`} open={promptSnapshot !== null} onCancel={() => setPromptSnapshot(null)} footer={null} width={760} destroyOnHidden>{promptSnapshot && <PromptExplanation snapshot={promptSnapshot} />}</Modal>
  </article>
}

export default function RunCenterPage() {
  const { styles, cx } = useStyles()
  const navigate = useNavigate()
  const canWrite = useCanWrite()
  const { runId } = useParams()
  const list = useRuns()
  const [search, setSearch] = useState('')
  const [createOpen, setCreateOpen] = useState(false)
  const [createForm] = Form.useForm<{ name: string; description?: string; capabilityPackageIds?: number[] }>()
  const createRun = useCreateRun()
  const packages = useEnabledCapabilityPackages()
  const selectedId = runId || undefined
  const runs = useMemo(() => (list.data ?? []).filter((run) => `${run.name} ${run.description ?? ''}`.toLowerCase().includes(search.trim().toLowerCase())), [list.data, search])
  useEffect(() => { if (!selectedId && runs[0]) void navigate(`/runs/${runs[0].id}`, { replace: true }) }, [navigate, runs, selectedId])

  return <main className={styles.page}>
    <header className={styles.head}><div><h1 className={styles.title}>运行中心</h1><p className={styles.subtitle}>每次任务都有独立档案。在这里查看谁把任务交给谁、为什么继续传递，以及系统在哪一跳阻止了风险。</p></div><div className={styles.detailActions}><div className={styles.scope}><span className={styles.scopeDot} />H3 · 多跳协作可控、可复盘</div>{canWrite && <PrimaryButton icon={<PlusIcon size={16} />} onClick={() => setCreateOpen(true)}>新建运行</PrimaryButton>}</div></header>
    <div className={styles.shell}>
      <aside className={styles.rail}><div className={styles.railHead}><div className={styles.railTitle}>运行档案</div><Input.Search allowClear value={search} onChange={(event) => setSearch(event.target.value)} placeholder="搜索任务" /></div>
        {list.isError ? <div className={styles.center}><Alert type="error" showIcon title="无法加载运行" description={parseApiError(list.error)} /></div> : list.isLoading ? <div style={{ padding: 16 }}><Skeleton active paragraph={{ rows: 7 }} /></div> : runs.length === 0 ? <div className={styles.center}><Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={search ? '没有匹配的运行' : '还没有运行记录'} /></div> : <div className={styles.runList}>{runs.map((run: Run) => <button type="button" className={cx(styles.runButton, run.id === selectedId && styles.runButtonActive)} key={run.id} onClick={() => void navigate(`/runs/${run.id}`)}><div className={styles.runTop}><span className={styles.runName}>{run.name}</span><RunStatusLabel status={run.status} /></div><p className={styles.runDesc}>{run.description || '未填写运行说明'}</p><div className={styles.runMeta}><span>{run.agents?.length ?? 0} 个 Agent</span><ArrowRightIcon size={12} /><time>{formatTime(run.updatedAt)}</time></div></button>)}</div>}
      </aside>
      {selectedId ? <RunDetailPanel id={selectedId} /> : <div className={styles.center}><Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="选择一次运行查看档案" /></div>}
    </div>
    <Modal title="新建运行" open={createOpen} onCancel={() => setCreateOpen(false)} footer={null} destroyOnHidden>
      <Form form={createForm} layout="vertical" onFinish={({ capabilityPackageIds, ...values }) => {
        const selected = new Set(capabilityPackageIds ?? [])
        const capabilityBindings = (packages.data ?? []).filter((item) => selected.has(item.id)).map((item) => ({ namespace: item.namespace, packageName: item.name, version: item.version }))
        createRun.mutate({ ...values, ...(capabilityBindings.length > 0 ? { capabilityBindings } : {}) }, { onSuccess: (run) => { setCreateOpen(false); createForm.resetFields(); void navigate(`/runs/${run.id}`) } })
      }}>
        <Form.Item name="name" label="运行名称" rules={[{ required: true, whitespace: true, message: '请输入运行名称' }]}><Input maxLength={160} placeholder="例如：新市场研究" /></Form.Item>
        <Form.Item name="description" label="运行说明"><Input.TextArea rows={3} placeholder="这次运行要完成什么？" /></Form.Item>
        <Form.Item name="capabilityPackageIds" label="本次使用的能力" extra="创建后会固定当前版本，以后升级不会改变这次运行的复盘结果。"><Select mode="multiple" allowClear loading={packages.isLoading} optionFilterProp="label" placeholder={packages.data?.length ? '可选，可多选' : '暂无已启用的能力包'} options={(packages.data ?? []).map((item) => ({ value: item.id, label: `${item.displayName || item.name} · ${item.version}` }))} /></Form.Item>
        <div className={styles.detailActions}><Button onClick={() => setCreateOpen(false)}>取消</Button><PrimaryButton htmlType="submit" loading={createRun.isPending}>创建运行</PrimaryButton></div>
      </Form>
    </Modal>
  </main>
}
