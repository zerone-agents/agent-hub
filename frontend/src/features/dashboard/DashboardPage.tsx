import { useState, type ComponentType } from 'react'
import {
  Books,
  Cpu,
  Cube,
  Chats,
  FilmSlate,
  PlugsConnected,
  Robot,
  Sparkle,
  Wrench
} from '@phosphor-icons/react'
import { Spin } from 'antd'
import { createStyles } from 'antd-style'
import { useNavigate } from 'react-router-dom'
import { useDashboardStats } from '@/queries/useDashboardStats'
import { formatTime } from '@/utils/time'
import { tokens as t } from '@/styles/tokens'
import type {
  Agent,
  Tool,
  Skill,
  Scene,
  Provider,
  Mcp,
  KnowledgeDataset,
} from '@/queries/useDashboardStats'

const useStyles = createStyles(({ css }) => ({
  page: css`
    max-width: 1480px;
    margin: 0 auto;
    animation: pageIn 0.35s ease;
    @keyframes pageIn {
      from { opacity: 0; transform: translateY(6px); }
      to { opacity: 1; transform: translateY(0); }
    }
  `,
  header: css`
    display: flex;
    align-items: flex-end;
    justify-content: space-between;
    gap: 24px;
    margin-bottom: 24px;
  `,
  headerTitle: css`
    margin: 0;
    color: ${t.text};
    font-size: clamp(28px, 3vw, 42px);
    font-weight: 750;
    letter-spacing: -0.045em;
    line-height: 1;
  `,
  headerSub: css`
    max-width: 48ch;
    margin-top: 10px;
    color: ${t.textTertiary};
    font-size: ${t.textBase};
    line-height: 1.6;
  `,
  status: css`
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 8px 11px;
    border: 1px solid var(--border);
    border-radius: ${t.radiusSm}px;
    color: ${t.textSecondary};
    background: color-mix(in srgb, var(--card) 84%, transparent);
    font-size: 12px;
    white-space: nowrap;
  `,
  statusDot: css`
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: var(--chart-2);
    box-shadow: 0 0 0 4px color-mix(in srgb, var(--chart-2) 14%, transparent);
  `,
  heroGrid: css`
    display: grid;
    grid-template-columns: minmax(0, 1.6fr) minmax(290px, 0.8fr);
    gap: 16px;
    margin-bottom: 16px;
    @media (max-width: 980px) {
      grid-template-columns: 1fr;
    }
  `,
  heroCard: css`
    position: relative;
    min-height: 280px;
    overflow: hidden;
    padding: 26px;
    border: 1px solid color-mix(in srgb, var(--primary) 26%, var(--border));
    border-radius: ${t.radius}px;
    background:
      radial-gradient(circle at 86% 18%, color-mix(in srgb, var(--primary) 17%, transparent), transparent 34%),
      var(--card);
  `,
  heroPattern: css`
    position: absolute;
    inset: 0;
    opacity: 0.34;
    pointer-events: none;
    background-image:
      linear-gradient(color-mix(in srgb, var(--border) 36%, transparent) 1px, transparent 1px),
      linear-gradient(90deg, color-mix(in srgb, var(--border) 36%, transparent) 1px, transparent 1px);
    background-size: 28px 28px;
    mask-image: linear-gradient(90deg, transparent 15%, black 100%);
  `,
  heroContent: css`
    position: relative;
    z-index: 1;
    display: flex;
    height: 100%;
    flex-direction: column;
    justify-content: space-between;
  `,
  heroLabel: css`
    color: ${t.textTertiary};
    font-size: 12px;
    font-weight: 600;
  `,
  heroNumberRow: css`
    display: flex;
    align-items: baseline;
    gap: 10px;
    margin-top: 8px;
  `,
  heroNumber: css`
    color: ${t.text};
    font-size: clamp(52px, 7vw, 82px);
    font-weight: 760;
    font-variant-numeric: tabular-nums;
    letter-spacing: -0.07em;
    line-height: 0.95;
  `,
  heroUnit: css`
    color: ${t.textMuted};
    font-size: 13px;
  `,
  heroBottom: css`
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: 10px;
    margin-top: 32px;
    @media (max-width: 680px) {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
  `,
  heroMetric: css`
    padding-top: 12px;
    border-top: 1px solid var(--border);
  `,
  heroMetricValue: css`
    color: ${t.text};
    font-size: 19px;
    font-weight: 700;
    font-variant-numeric: tabular-nums;
  `,
  heroMetricLabel: css`
    margin-top: 3px;
    color: ${t.textMuted};
    font-size: 11px;
  `,
  readinessCard: css`
    display: flex;
    min-height: 280px;
    flex-direction: column;
    justify-content: space-between;
    padding: 24px;
    border: 1px solid var(--border);
    border-radius: ${t.radius}px;
    background: var(--card);
  `,
  cardKicker: css`
    color: ${t.textMuted};
    font-size: 11px;
    font-weight: 650;
    letter-spacing: 0.08em;
  `,
  readinessBody: css`
    display: flex;
    align-items: center;
    gap: 24px;
    margin: 22px 0;
  `,
  ring: css`
    --progress: 0deg;
    position: relative;
    display: grid;
    width: 120px;
    height: 120px;
    flex: 0 0 auto;
    place-items: center;
    border-radius: 50%;
    background: conic-gradient(var(--primary) var(--progress), var(--muted) 0);
    &::after {
      position: absolute;
      width: 88px;
      height: 88px;
      border-radius: 50%;
      background: var(--card);
      content: '';
    }
  `,
  ringValue: css`
    position: relative;
    z-index: 1;
    color: ${t.text};
    font-size: 26px;
    font-weight: 740;
    font-variant-numeric: tabular-nums;
  `,
  readinessList: css`
    display: flex;
    min-width: 0;
    flex: 1;
    flex-direction: column;
    gap: 10px;
  `,
  readinessItem: css`
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    color: ${t.textSecondary};
    font-size: 12px;
  `,
  readinessValue: css`
    color: ${t.text};
    font-weight: 650;
    font-variant-numeric: tabular-nums;
  `,
  readinessNote: css`
    color: ${t.textMuted};
    font-size: 11px;
    line-height: 1.6;
  `,
  statsGrid: css`
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    overflow: hidden;
    margin-bottom: 16px;
    border: 1px solid var(--border);
    border-radius: ${t.radius}px;
    background: var(--card);
    @media (max-width: 760px) {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
  `,
  statItem: css`
    position: relative;
    display: flex;
    min-width: 0;
    align-items: center;
    gap: 12px;
    padding: 17px 16px;
    border: 0;
    border-right: 1px solid var(--border);
    background: transparent;
    color: inherit;
    text-align: left;
    cursor: pointer;
    transition: background 0.18s ease;
    &:nth-child(4n) { border-right: 0; }
    &:nth-child(-n + 4) { border-bottom: 1px solid var(--border); }
    &:hover { background: var(--accent); }
    &:active { transform: translateY(1px); }
    &:focus-visible {
      z-index: 1;
      outline: 2px solid var(--ring);
      outline-offset: -2px;
    }
    @media (max-width: 760px) {
      &:nth-child(4n) { border-right: 1px solid var(--border); }
      &:nth-child(even) { border-right: 0; }
      &:nth-child(-n + 6) { border-bottom: 1px solid var(--border); }
    }
  `,
  statIcon: css`
    display: grid;
    width: 34px;
    height: 34px;
    flex: 0 0 auto;
    place-items: center;
    border-radius: ${t.radiusSm}px;
    background: var(--accent);
    color: var(--primary);
  `,
  statValue: css`
    display: block;
    color: ${t.text};
    font-size: 20px;
    font-weight: 720;
    font-variant-numeric: tabular-nums;
    line-height: 1;
  `,
  statLabel: css`
    display: block;
    margin-top: 5px;
    overflow: hidden;
    color: ${t.textMuted};
    font-size: 11px;
    text-overflow: ellipsis;
    white-space: nowrap;
  `,
  dashboardGrid: css`
    display: grid;
    grid-template-columns: minmax(0, 1.45fr) minmax(330px, 0.85fr);
    gap: 16px;
    align-items: start;
    @media (max-width: 1080px) {
      grid-template-columns: 1fr;
    }
  `,
  column: css`
    display: flex;
    min-width: 0;
    flex-direction: column;
    gap: 16px;
  `,
  panel: css`
    border: 1px solid var(--border);
    border-radius: ${t.radius}px;
    background: var(--card);
  `,
  panelHeader: css`
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 16px;
    padding: 20px 20px 0;
  `,
  panelTitle: css`
    color: ${t.text};
    font-size: ${t.textBase};
    font-weight: 650;
  `,
  panelSub: css`
    margin-top: 4px;
    color: ${t.textMuted};
    font-size: 11px;
  `,
  trendChart: css`
    display: flex;
    height: 230px;
    align-items: flex-end;
    gap: clamp(7px, 2vw, 18px);
    padding: 30px 24px 18px;
    background-image: linear-gradient(to top, color-mix(in srgb, var(--border) 50%, transparent) 1px, transparent 1px);
    background-size: 100% 25%;
  `,
  barGroup: css`
    display: flex;
    min-width: 0;
    height: 100%;
    flex: 1;
    flex-direction: column;
    justify-content: flex-end;
    gap: 8px;
  `,
  barValue: css`
    color: ${t.textMuted};
    font-size: 10px;
    text-align: center;
    font-variant-numeric: tabular-nums;
  `,
  barTrack: css`
    position: relative;
    min-height: 3px;
    border-radius: 5px 5px 2px 2px;
    background: linear-gradient(to top, var(--primary), color-mix(in srgb, var(--primary) 44%, var(--card)));
    box-shadow: 0 6px 18px color-mix(in srgb, var(--primary) 13%, transparent);
  `,
  barLabel: css`
    overflow: hidden;
    color: ${t.textMuted};
    font-size: 10px;
    text-align: center;
    text-overflow: ellipsis;
    white-space: nowrap;
  `,
  composition: css`
    display: grid;
    grid-template-columns: 180px 1fr;
    gap: 26px;
    align-items: center;
    padding: 22px 24px 26px;
    @media (max-width: 560px) {
      grid-template-columns: 1fr;
      justify-items: center;
    }
  `,
  donutWrap: css`
    position: relative;
    width: 164px;
    height: 164px;
    overflow: visible;
  `,
  donutSvg: css`
    width: 100%;
    height: 100%;
    overflow: visible;
    transform: rotate(-90deg);
  `,
  donutSegment: css`
    cursor: pointer;
    transition: d 200ms ease;
  `,
  donutCenter: css`
    position: absolute;
    top: 50%;
    left: 50%;
    transform: translate(-50%, -50%);
    z-index: 1;
    text-align: center;
    pointer-events: none;
  `,
  donutValue: css`
    color: ${t.text};
    font-size: 30px;
    font-weight: 740;
    line-height: 1;
    font-variant-numeric: tabular-nums;
  `,
  donutLabel: css`
    margin-top: 6px;
    color: ${t.textMuted};
    font-size: 10px;
  `,
  legend: css`
    display: grid;
    width: 100%;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 13px 20px;
  `,
  legendItem: css`
    display: flex;
    align-items: center;
    justify-content: space-between;
    color: ${t.textSecondary};
    font-size: 11px;
    cursor: pointer;
  `,
  legendLeft: css`
    display: flex;
    align-items: center;
    gap: 8px;
    transform-origin: left center;
    transition: transform 200ms ease;
  `,
  legendLeftActive: css`
    transform: scale(1.3);
  `,
  legendDot: css`
    width: 8px;
    height: 8px;
    border-radius: 2px;
    background: var(--dot);
    flex-shrink: 0;
  `,
  legendValue: css`
    color: ${t.text};
    font-weight: 650;
    font-variant-numeric: tabular-nums;
    transform-origin: right center;
    transition: transform 200ms ease;
  `,
  legendValueActive: css`
    transform: scale(1.3);
  `,
  feed: css`
    padding: 8px 20px 14px;
  `,
  activity: css`
    display: grid;
    grid-template-columns: 26px minmax(0, 1fr) auto;
    gap: 10px;
    align-items: center;
    padding: 11px 0;
    border-bottom: 1px solid color-mix(in srgb, var(--border) 72%, transparent);
    &:last-child { border-bottom: 0; }
  `,
  activityMark: css`
    display: grid;
    width: 26px;
    height: 26px;
    place-items: center;
    border-radius: 4px;
    background: var(--accent);
    color: var(--primary);
  `,
  activityTitle: css`
    overflow: hidden;
    color: ${t.text};
    font-size: 12px;
    font-weight: 560;
    text-overflow: ellipsis;
    white-space: nowrap;
  `,
  activityMeta: css`
    margin-top: 3px;
    color: ${t.textMuted};
    font-size: 10px;
  `,
  activityTime: css`
    color: ${t.textMuted};
    font-size: 10px;
    white-space: nowrap;
  `,
  empty: css`
    padding: 42px 20px;
    color: ${t.textMuted};
    font-size: 12px;
    text-align: center;
  `,
  loadingWrap: css`
    display: grid;
    min-height: 460px;
    place-items: center;
  `
}))

interface ResourceItem {
  label: string
  value: number
  icon: ComponentType<{ size?: number; weight?: 'regular' | 'fill' }>
  path: string
  color: string
}

interface ActivityItem {
  title: string
  time: string
  type: 'agent' | 'tool' | 'skill' | 'scene' | 'provider' | 'mcp' | 'knowledge'
}

const typeLabel: Record<ActivityItem['type'], string> = {
  agent: 'Agent',
  tool: 'Tool',
  skill: 'Skill',
  scene: 'Scene',
  provider: 'Provider',
  mcp: 'MCP',
  knowledge: '知识库'
}

const typeIcon: Record<ActivityItem['type'], ComponentType<{ size?: number; weight?: 'fill' | 'regular' | 'bold' | 'light' | 'thin' | 'duotone' }>> = {
  agent: Robot,
  tool: Wrench,
  skill: Sparkle,
  scene: FilmSlate,
  provider: PlugsConnected,
  mcp: Cube,
  knowledge: Books
}

function buildTrend(activities: ActivityItem[]) {
  const valid = activities.filter((item) => Number.isFinite(+new Date(item.time)))
  const anchor = valid.length
    ? Math.max(...valid.map((item) => +new Date(item.time)))
    : Date.now()
  const day = 86_400_000
  return Array.from({ length: 8 }, (_, index) => {
    const start = anchor - (7 - index) * 7 * day
    const end = start + 7 * day
    const date = new Date(start)
    return {
      label: `${date.getMonth() + 1}/${date.getDate()}`,
      value: valid.filter((item) => {
        const timestamp = +new Date(item.time)
        return timestamp >= start && timestamp < end
      }).length
    }
  })
}

function donutArcPath(
  cx: number,
  cy: number,
  outerR: number,
  innerR: number,
  startAngle: number,
  endAngle: number
) {
  const toRad = (deg: number) => ((deg - 90) * Math.PI) / 180
  const cos = (a: number) => Math.cos(toRad(a))
  const sin = (a: number) => Math.sin(toRad(a))
  const largeArc = endAngle - startAngle > 180 ? 1 : 0
  const ox1 = cx + outerR * cos(startAngle)
  const oy1 = cy + outerR * sin(startAngle)
  const ox2 = cx + outerR * cos(endAngle)
  const oy2 = cy + outerR * sin(endAngle)
  const ix1 = cx + innerR * cos(endAngle)
  const iy1 = cy + innerR * sin(endAngle)
  const ix2 = cx + innerR * cos(startAngle)
  const iy2 = cy + innerR * sin(startAngle)
  return [
    `M ${ox1} ${oy1}`,
    `A ${outerR} ${outerR} 0 ${largeArc} 1 ${ox2} ${oy2}`,
    `L ${ix1} ${iy1}`,
    `A ${innerR} ${innerR} 0 ${largeArc} 0 ${ix2} ${iy2}`,
    'Z'
  ].join(' ')
}

export default function DashboardPage() {
  const { styles } = useStyles()
  const navigate = useNavigate()
  const [hoveredIndex, setHoveredIndex] = useState<number | null>(null)
  const {
    agents,
    tools,
    skills,
    scenes,
    providers,
    mcps,
    knowledgeDatasets,
    chatSessionTotal,
    isLoading,
    isError
  } = useDashboardStats()

  if (isLoading) {
    return <div className={styles.loadingWrap}><Spin size="large" /></div>
  }

  if (isError) {
    return <div className={styles.empty}>仪表盘数据加载失败，请稍后刷新重试</div>
  }

  const modelCount = providers.reduce((sum, provider) => sum + (provider.defaultModels?.length ?? 0), 0)
  const documentCount = knowledgeDatasets.reduce((sum, dataset) => sum + dataset.doc_num, 0)
  const chunkCount = knowledgeDatasets.reduce((sum, dataset) => sum + dataset.chunk_num, 0)
  const resources: ResourceItem[] = [
    { label: 'Agent', value: agents.length, icon: Robot, path: '/agents', color: 'var(--chart-1)' },
    { label: '工具', value: tools.length, icon: Wrench, path: '/tools', color: 'var(--chart-2)' },
    { label: 'MCP 配置', value: mcps.length, icon: PlugsConnected, path: '/mcps', color: 'var(--primary)' },
    { label: '技能', value: skills.length, icon: Sparkle, path: '/skills', color: 'var(--chart-3)' },
    { label: '提供方', value: providers.length, icon: Cpu, path: '/providers', color: 'var(--chart-5)' },
    { label: '模型', value: modelCount, icon: Cube, path: '/providers', color: 'var(--chart-1)' },
    { label: '知识库', value: knowledgeDatasets.length, icon: Books, path: '/knowledge', color: 'var(--chart-2)' },
    { label: '场景', value: scenes.length, icon: FilmSlate, path: '/scenes', color: 'var(--chart-4)' }
  ]
  const resourceActivities: ActivityItem[] = [
    ...agents.map((item: Agent) => ({ title: item.config?.title?.zh || item.name, time: item.createdAt ?? '', type: 'agent' as const })),
    ...tools.map((item: Tool) => ({ title: item.title || item.name, time: item.createdAt ?? '', type: 'tool' as const })),
    ...skills.map((item: Skill) => ({ title: item.title || item.titleEn || item.name, time: item.createdAt ?? '', type: 'skill' as const })),
    ...scenes.map((item: Scene) => ({ title: item.title || item.titleEn || item.name, time: item.createdAt ?? '', type: 'scene' as const })),
    ...providers.map((item: Provider) => ({ title: item.name, time: item.createdAt ?? '', type: 'provider' as const })),
    ...mcps.map((item: Mcp) => ({ title: item.title || item.name, time: item.createdAt ?? '', type: 'mcp' as const })),
    ...knowledgeDatasets.map((item: KnowledgeDataset) => ({
      title: item.display_name || item.name,
      time: item.create_date || (item.create_time ? new Date(item.create_time).toISOString() : ''),
      type: 'knowledge' as const
    }))
  ].sort((a, b) => +new Date(b.time) - +new Date(a.time))
  const total = resources.reduce((sum, item) => sum + item.value, 0)
  const desktopAgents = agents.filter((item) => item.desktopEnabled).length
  const healthyMcps = mcps.filter((item) => item.probeStatus === 'success').length
  const readyChecks = [
    agents.length ? desktopAgents / agents.length : 0,
    providers.length ? 1 : 0,
    mcps.length ? healthyMcps / mcps.length : 0
  ]
  const readiness = Math.round((readyChecks.reduce((sum, value) => sum + value, 0) / readyChecks.length) * 100)
  const trend = buildTrend(resourceActivities)
  const maxTrend = Math.max(1, ...trend.map((item) => item.value))

  return (
    <div className={styles.page}>
      <header className={styles.header}>
        <div>
          <h1 className={styles.headerTitle}>仪表盘</h1>
          <div className={styles.headerSub}>查看智能体资源、配置健康度与最近变化。</div>
        </div>
        <div className={styles.status}><span className={styles.statusDot} /> 数据已同步</div>
      </header>

      <section className={styles.heroGrid} aria-label="系统总览">
        <article className={styles.heroCard}>
          <div className={styles.heroPattern} />
          <div className={styles.heroContent}>
            <div>
              <div className={styles.heroLabel}>已接入资源</div>
              <div className={styles.heroNumberRow}>
                <span className={styles.heroNumber}>{total}</span>
                <span className={styles.heroUnit}>项配置</span>
              </div>
            </div>
            <div className={styles.heroBottom}>
              <div className={styles.heroMetric}>
                <div className={styles.heroMetricValue}>{desktopAgents}/{agents.length}</div>
                <div className={styles.heroMetricLabel}>桌面端代理</div>
              </div>
              <div className={styles.heroMetric}>
                <div className={styles.heroMetricValue}>{modelCount}/{providers.length}</div>
                <div className={styles.heroMetricLabel}>模型 / 提供方</div>
              </div>
              <div className={styles.heroMetric}>
                <div className={styles.heroMetricValue}>{documentCount}/{chunkCount}</div>
                <div className={styles.heroMetricLabel}>知识文档 / 切块</div>
              </div>
              <div className={styles.heroMetric}>
                <div className={styles.heroMetricValue}>{chatSessionTotal}</div>
                <div className={styles.heroMetricLabel}><Chats size={11} /> 聊天会话</div>
              </div>
            </div>
          </div>
        </article>

        <article className={styles.readinessCard}>
          <div className={styles.cardKicker}>配置健康度</div>
          <div className={styles.readinessBody}>
            <div className={styles.ring} style={{ '--progress': `${readiness * 3.6}deg` } as React.CSSProperties}>
              <span className={styles.ringValue}>{readiness}%</span>
            </div>
            <div className={styles.readinessList}>
              <div className={styles.readinessItem}><span>桌面端代理</span><span className={styles.readinessValue}>{desktopAgents}/{agents.length}</span></div>
              <div className={styles.readinessItem}><span>Provider 接入</span><span className={styles.readinessValue}>{providers.length}</span></div>
              <div className={styles.readinessItem}><span>MCP 正常</span><span className={styles.readinessValue}>{healthyMcps}/{mcps.length}</span></div>
            </div>
          </div>
          <div className={styles.readinessNote}>基于桌面端代理占比、模型提供方接入和 MCP 探测结果综合计算。</div>
        </article>
      </section>

      <section className={styles.statsGrid} aria-label="资源统计">
        {resources.map((item) => {
          const Icon = item.icon
          return (
            <button key={item.label} type="button" className={styles.statItem} onClick={() => { navigate(item.path); }}>
              <span className={styles.statIcon}><Icon size={17} weight="fill" /></span>
              <span>
                <span className={styles.statValue}>{item.value}</span>
                <span className={styles.statLabel}>{item.label}</span>
              </span>
            </button>
          )
        })}
      </section>

      <div className={styles.dashboardGrid}>
        <div className={styles.column}>
          <section className={styles.panel}>
            <div className={styles.panelHeader}>
              <div>
                <div className={styles.panelTitle}>资源增长</div>
                <div className={styles.panelSub}>按七天聚合的新增配置</div>
              </div>
              <div className={styles.cardKicker}>8 WEEKS</div>
            </div>
            <div className={styles.trendChart} role="img" aria-label="最近八周资源新增趋势">
              {trend.map((item) => (
                <div className={styles.barGroup} key={item.label}>
                  <div className={styles.barValue}>{item.value}</div>
                  <div className={styles.barTrack} style={{ height: `${Math.max(3, (item.value / maxTrend) * 100)}%` }} />
                  <div className={styles.barLabel}>{item.label}</div>
                </div>
              ))}
            </div>
          </section>

          <section className={styles.panel}>
            <div className={styles.panelHeader}>
              <div>
                <div className={styles.panelTitle}>资源构成</div>
                <div className={styles.panelSub}>当前工作空间的能力分布</div>
              </div>
            </div>
            <div className={styles.composition}>
              <div className={styles.donutWrap}>
                <svg className={styles.donutSvg} viewBox="0 0 164 164">
                  {(() => {
                    let cursor = 0
                    return resources.map((item, i) => {
                      const start = total ? (cursor / total) * 360 : 0
                      cursor += item.value
                      const end = total ? (cursor / total) * 360 : 0
                      if (item.value === 0) return null
                      const isActive = hoveredIndex === i
                      const outerR = isActive ? 94 : 82
                      return (
                        <path
                          key={item.label}
                          d={donutArcPath(82, 82, outerR, 54, start, end)}
                          fill={item.color}
                          className={styles.donutSegment}
                          onMouseEnter={() => { setHoveredIndex(i); }}
                          onMouseLeave={() => { setHoveredIndex(null); }}
                        />
                      )
                    })
                  })()}
                </svg>
                <div className={styles.donutCenter}>
                  <div className={styles.donutValue}>{total}</div>
                  <div className={styles.donutLabel}>TOTAL</div>
                </div>
              </div>
              <div className={styles.legend}>
                {resources.map((item, i) => {
                  const active = hoveredIndex === i
                  return (
                    <div
                      className={styles.legendItem}
                      key={item.label}
                      onMouseEnter={() => { setHoveredIndex(i); }}
                      onMouseLeave={() => { setHoveredIndex(null); }}
                    >
                      <span className={`${styles.legendLeft} ${active ? styles.legendLeftActive : ''}`}>
                        <span className={styles.legendDot} style={{ '--dot': item.color } as React.CSSProperties} />
                        <span>{item.label}</span>
                      </span>
                      <span className={`${styles.legendValue} ${active ? styles.legendValueActive : ''}`}>
                        {item.value}
                      </span>
                    </div>
                  )
                })}
              </div>
            </div>
          </section>
        </div>

        <div className={styles.column}>
          <section className={styles.panel}>
            <div className={styles.panelHeader}>
              <div>
                <div className={styles.panelTitle}>最近活动</div>
                <div className={styles.panelSub}>跨资源的最新配置记录</div>
              </div>
            </div>
            <div className={styles.feed}>
              {resourceActivities.length === 0 ? (
                <div className={styles.empty}>暂无最近活动</div>
              ) : (
                resourceActivities.slice(0, 8).map((activity, index) => (
                  <div className={styles.activity} key={`${activity.type}-${activity.title}-${index}`}>
                    <span className={styles.activityMark}>
                      {(() => {
                        const Icon = typeIcon[activity.type]
                        return <Icon size={13} weight="fill" />
                      })()}
                    </span>
                    <span>
                      <div className={styles.activityTitle}>{activity.title}</div>
                      <div className={styles.activityMeta}>{typeLabel[activity.type]}</div>
                    </span>
                    <span className={styles.activityTime}>{formatTime(activity.time)}</span>
                  </div>
                ))
              )}
            </div>
          </section>
        </div>
      </div>
    </div>
  )
}
