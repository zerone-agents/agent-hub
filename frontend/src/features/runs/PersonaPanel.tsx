import { useMemo } from 'react'
import { Alert, Empty, Skeleton, Tag } from 'antd'
import { BrainIcon, ClockCounterClockwiseIcon, HeartIcon, UsersIcon, WarningDiamondIcon } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import type { RunAgent, RunState, RunStateChange } from '@/api/runs'
import { parseApiError } from '@/api/client'
import { useRunBeliefDisputes, useRunPersonaState } from '@/queries/useRuns'
import { formatTime } from '@/utils/time'
import { tokens as t } from '@/styles/tokens'

export const PERSONA_NAMESPACES = new Set([
  'io.zerone.emotion',
  'io.zerone.belief',
  'io.zerone.subjective-memory',
  'io.zerone.relationship-dynamics',
])

const MOOD_LABEL: Record<string, string> = {
  calm: '平静', wary: '警惕', tense: '紧张', angry: '愤怒', elated: '振奋', grieving: '悲伤',
}

const BELIEF_STATUS: Record<string, { label: string; color: string }> = {
  known: { label: '已知', color: 'default' },
  believed: { label: '相信', color: 'success' },
  doubted: { label: '怀疑', color: 'warning' },
  disputed: { label: '有争议', color: 'error' },
  forgotten: { label: '已遗忘', color: 'default' },
}

const STANCE_LABEL: Record<string, string> = {
  hostile: '敌对', wary: '警惕', neutral: '中立', friendly: '友好', allied: '同盟',
}

const MOOD_COLOR: Record<string, string> = {
  calm: '#22c55e',
  elated: '#14b8a6',
  wary: '#eab308',
  tense: '#f97316',
  angry: '#ef4444',
  grieving: '#a855f7',
}

const useStyles = createStyles(({ css }) => ({
  persona: css`margin-top: 22px; padding-top: 20px; border-top: 1px solid var(--border);`,
  personaHead: css`margin-bottom: 12px;`,
  sectionTitle: css`display: flex; align-items: center; gap: 7px; margin: 0 0 6px; color: ${t.text}; font-size: ${t.textBase}; font-weight: 680;`,
  personaHelp: css`margin: 0; color: ${t.textTertiary}; font-size: 12px; line-height: 1.6;`,
  disputeCard: css`
    margin-bottom: 14px; padding: 12px 14px; border: 1px solid color-mix(in srgb, var(--warning) 45%, var(--border));
    border-radius: ${t.radiusSm}px; background: color-mix(in srgb, var(--warning) 7%, var(--card));
  `,
  disputeTitle: css`display: flex; align-items: center; gap: 7px; margin: 0 0  8px; color: ${t.text}; font-size: ${t.textSm}; font-weight: 650;`,
  disputeRow: css`display: flex; align-items: center; flex-wrap: wrap; gap: 7px; padding: 6px 0; border-top: 1px dashed var(--border); color: ${t.textSecondary}; font-size: 12px; &:first-of-type { border-top: 0; }`,
  disputeFact: css`color: ${t.text}; font-weight: 620; overflow-wrap: anywhere;`,
  personaGrid: css`display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16px; @media (max-width: 980px) { grid-template-columns: 1fr; }`,
  block: css`min-width: 0;`,
  blockTitle: css`display: flex; align-items: center; gap: 6px; margin: 0 0 8px; color: ${t.text}; font-size: ${t.textSm}; font-weight: 650;`,
  quietList: css`overflow: hidden; border: 1px solid var(--border); border-radius: ${t.radiusSm}px;`,
  row: css`padding: 12px; border-bottom: 1px solid var(--border); &:last-child { border-bottom: 0; }`,
  rowTop: css`display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 8px;`,
  rowName: css`color: ${t.text}; font-size: ${t.textSm}; font-weight: 630; overflow-wrap: anywhere;`,
  rowMeta: css`display: flex; align-items: center; flex-wrap: wrap; gap: 6px;`,
  moodDot: css`display: inline-block; width: 8px; height: 8px; flex: 0 0 auto; border-radius: 50%;`,
  rowBody: css`margin-top: 6px; color: ${t.textTertiary}; font-size: 12px; line-height: 1.6; overflow-wrap: anywhere;`,
  rowTime: css`margin-top: 5px; color: ${t.textMuted}; font-size: 11px;`,
  intensity: css`display: inline-flex; align-items: center; gap: 7px; color: ${t.textSecondary}; font-size: 12px;`,
  bar: css`width: 74px; height: 6px; overflow: hidden; border-radius: 999px; background: var(--background);`,
  barFill: css`height: 100%; border-radius: 999px; background: var(--primary);`,
  footer: css`margin-top: 14px; color: ${t.textMuted}; font-size: 11px; line-height: 1.6;`,
  timeline: css`position: relative; margin-left: 7px; padding-left: 23px; border-left: 1px solid var(--border);`,
  change: css`position: relative; padding: 0 0 15px 2px; &::before { position: absolute; top: 4px; left: -29px; width: 10px; height: 10px; border: 2px solid var(--card); border-radius: 50%; background: var(--primary); box-shadow: 0 0 0 1px var(--border); content: ''; } &:last-child { padding-bottom: 0; }`,
  changeTop: css`display: flex; align-items: baseline; justify-content: space-between; gap: 12px;`,
  changeTitle: css`color: ${t.text}; font-size: ${t.textSm}; font-weight: 630;`,
  changeTime: css`color: ${t.textMuted}; font-size: 11px; white-space: nowrap;`,
  changeBody: css`margin-top: 3px; color: ${t.textTertiary}; font-size: 12px; line-height: 1.5;`,
}))

function str(value: unknown): string {
  return typeof value === 'string' ? value : ''
}

function num(value: unknown): number | undefined {
  return typeof value === 'number' ? value : undefined
}

function agentName(agents: RunAgent[], agentId: string | number | undefined): string {
  const id = Number(agentId)
  const hit = agents.find((agent) => agent.agentId === id)
  return hit?.agentNameSnapshot || `Agent ${id}`
}

function sourceBadge(source: string): { label: string; subjective: boolean } {
  if (source === 'claim') return { label: '主观声称', subjective: true }
  if (source === 'delivery' || source === 'observation') return { label: '事实来源', subjective: false }
  return { label: source || '未知来源', subjective: false }
}

export function PersonaPanel({ runId, agents, states, changes }: { runId: string; agents: RunAgent[]; states: RunState[]; changes: RunStateChange[] }) {
  const { styles } = useStyles()
  const persona = useRunPersonaState(runId)
  const disputes = useRunBeliefDisputes(runId)

  const emotion = useMemo(() => persona.data?.emotion ?? [], [persona.data])
  const belief = useMemo(() => persona.data?.belief ?? [], [persona.data])
  const memory = useMemo(() => persona.data?.memory ?? [], [persona.data])
  const relationDynamics = useMemo(() => persona.data?.relationDynamics ?? [], [persona.data])

  const personaChanges = useMemo(() => {
    const namespaceByStateId = new Map<number, string>()
    states.forEach((state) => namespaceByStateId.set(state.id, state.namespace))
    return changes.filter((change) => change.runStateId !== undefined && PERSONA_NAMESPACES.has(namespaceByStateId.get(change.runStateId) ?? ''))
  }, [states, changes])

  if (persona.isError) {
    return <section className={styles.persona}><Alert type="error" showIcon title="人物状态加载失败" description={`${parseApiError(persona.error)}。确认后端已升级到 H6 版本后重试。`} /></section>
  }
  if (persona.isLoading) {
    return <section className={styles.persona}><h3 className={styles.sectionTitle}><HeartIcon size={17} />人物状态</h3><Skeleton active paragraph={{ rows: 4 }} /></section>
  }

  const hasState = emotion.length > 0 || belief.length > 0 || memory.length > 0 || relationDynamics.length > 0
  const disputeList = (disputes.data ?? []).filter((dispute) => Array.isArray(dispute.entries))

  return <section className={styles.persona} aria-label="人物状态">
    <div className={styles.personaHead}>
      <h3 className={styles.sectionTitle}><HeartIcon size={17} />人物状态</h3>
      <p className={styles.personaHelp}>每个 Agent 在本次运行中的动态状态：情绪、认知与立场、主观记忆和相互态度。它们随互动实时变化，和静态人格不是一回事。</p>
    </div>
    {!hasState && disputeList.length === 0 ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="尚未产生人物状态——Agent 开始互动后这里会显示情绪、认知、记忆和关系变化" /> : <>
      {disputeList.length > 0 && <div className={styles.disputeCard} role="status">
        <h4 className={styles.disputeTitle}><WarningDiamondIcon size={15} />认知争议</h4>
        {disputeList.map((dispute) => <div className={styles.disputeRow} key={dispute.factRef}>
          <span className={styles.disputeFact}>{dispute.factRef}</span>
          {dispute.entries.map((entry) => <Tag key={entry.agentId} color={(BELIEF_STATUS[entry.status]?.color) ?? 'default'} variant="filled">{agentName(agents, entry.agentId)} · {BELIEF_STATUS[entry.status]?.label ?? entry.status}（置信 {entry.confidence}）</Tag>)}
        </div>)}
      </div>}
      <div className={styles.personaGrid}>
        <div className={styles.block}>
          <h4 className={styles.blockTitle}><HeartIcon size={15} />当前情绪</h4>
          {emotion.length === 0 ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="还没有情绪记录" /> : <div className={styles.quietList}>{emotion.map((entry) => {
            const mood = str(entry.data.mood)
            const intensity = num(entry.data.intensity)
            return <div className={styles.row} key={`${entry.namespace}-${entry.subjectId}`}>
              <div className={styles.rowTop}><span className={styles.rowName}>{agentName(agents, entry.subjectId)}</span><span className={styles.rowMeta}><span className={styles.moodDot} style={{ background: MOOD_COLOR[mood] ?? 'var(--text-muted)' }} aria-hidden="true" /><Tag color="processing" variant="filled">{MOOD_LABEL[mood] ?? mood}</Tag>{intensity !== undefined && <span className={styles.intensity}><span className={styles.bar}><span className={styles.barFill} style={{ width: `${intensity}%` }} /></span>{intensity}/100</span>}</span></div>
              <div className={styles.rowBody}>{str(entry.data.narration) || '暂无叙述'}</div>
              <div className={styles.rowTime}>最近变化 {formatTime(str(entry.data.updatedAt) || entry.updatedAt)}</div>
            </div>
          })}</div>}
        </div>
        <div className={styles.block}>
          <h4 className={styles.blockTitle}><BrainIcon size={15} />认知与立场</h4>
          {belief.length === 0 ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="还没有形成认知记录" /> : <div className={styles.quietList}>{belief.map((entry) => {
            const status = BELIEF_STATUS[str(entry.data.status)] ?? { label: str(entry.data.status) || '未知', color: 'default' }
            const source = sourceBadge(str(entry.data.source))
            return <div className={styles.row} key={`${entry.namespace}-${entry.subjectId}`}>
              <div className={styles.rowTop}><span className={styles.rowName}>{agentName(agents, entry.subjectId)} · <span>{str(entry.data.factRef)}</span></span><span className={styles.rowMeta}><Tag color={status.color} variant="filled">{status.label}</Tag>{num(entry.data.confidence) !== undefined && <span className={styles.intensity}><span className={styles.bar}><span className={styles.barFill} style={{ width: `${num(entry.data.confidence)}%` }} /></span>置信 {num(entry.data.confidence)}</span>}<Tag color={source.subjective ? 'warning' : 'success'} variant="filled">{source.label}</Tag></span></div>
              {str(entry.data.statement) && <div className={styles.rowBody}>{str(entry.data.statement)}</div>}
              <div className={styles.rowTime}>最近事件 {formatTime(str(entry.data.lastEventAt) || entry.updatedAt)}</div>
            </div>
          })}</div>}
        </div>
        <div className={styles.block}>
          <h4 className={styles.blockTitle}><BrainIcon size={15} />主观记忆</h4>
          {memory.length === 0 ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="还没有留下记忆" /> : <div className={styles.quietList}>{memory.map((entry) => <div className={styles.row} key={`${entry.namespace}-${entry.subjectId}`}>
            <div className={styles.rowTop}><span className={styles.rowName}>{agentName(agents, num(entry.data.agentId) ?? entry.subjectId)} · <span>{str(entry.data.factRef)}</span></span><span className={styles.rowMeta}><Tag variant="filled">重要度 {num(entry.data.importance) ?? 0}</Tag>{num(entry.data.recallCount) !== undefined && <Tag variant="filled">回忆 {num(entry.data.recallCount)} 次</Tag>}</span></div>
            <div className={styles.rowBody}>{str(entry.data.interpretation)}</div>
            <div className={styles.rowTime}>记录于 {formatTime(str(entry.data.recordedAt) || entry.updatedAt)}</div>
          </div>)}</div>}
        </div>
        <div className={styles.block}>
          <h4 className={styles.blockTitle}><UsersIcon size={15} />动态关系</h4>
          {relationDynamics.length === 0 ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="关系态度尚未发生变化" /> : <><p className={styles.personaHelp} style={{ marginBottom: 8 }}>关系是单向的：A 对 B 的态度不一定等于 B 对 A。</p><div className={styles.quietList}>{relationDynamics.map((entry) => {
            const [fromId, toId] = entry.subjectId.split(':')
            const score = num(entry.data.score)
            return <div className={styles.row} key={`${entry.namespace}-${entry.subjectId}`}>
              <div className={styles.rowTop}><span className={styles.rowName}>{agentName(agents, fromId)} → {agentName(agents, toId)}</span><span className={styles.rowMeta}><Tag color={score !== undefined && score < -20 ? 'error' : score !== undefined && score > 20 ? 'success' : 'default'} variant="filled">{STANCE_LABEL[str(entry.data.stance)] ?? str(entry.data.stance) ?? '中立'}{score !== undefined ? `（${score}）` : ''}</Tag></span></div>
              <div className={styles.rowBody}>{str(entry.data.narration) || '暂无叙述'}</div>
              <div className={styles.rowTime}>最近变化 {formatTime(entry.updatedAt)}</div>
            </div>
          })}</div></>}
        </div>
      </div>
      <div className={styles.block} style={{ marginTop: 16 }}>
        <h4 className={styles.blockTitle}><ClockCounterClockwiseIcon size={15} />变化记录</h4>
        {personaChanges.length === 0 ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="人物状态尚未发生变化" /> : <div className={styles.timeline}>{personaChanges.map((change) => <div className={styles.change} key={change.id}>
          <div className={styles.changeTop}><span className={styles.changeTitle}>{change.reason || '人物状态已更新'}</span><time className={styles.changeTime}>{formatTime(change.createdAt)}</time></div>
          <div className={styles.changeBody}>{change.source ? `${change.source} · ` : ''}修订 {change.revisionBefore} → {change.revisionAfter}</div>
        </div>)}</div>}
      </div>
      <p className={styles.footer}>以上状态仅注入该 Agent 本人的提示词，其他成员与管理员可见此处视图。标注「事实来源」的内容来自投递或观察，标注「主观声称」的内容来自 Agent 自己的表述。</p>
    </>}
  </section>
}
