// 手写 SVG 多序列折线图（无图表依赖，antd v6 主题变量取色）。
// 数据为按天聚合的 trends：x=日期，y=指标值（calls/tokens/cost/errors），
// 每个 kind 一条线 + 图例。
import { useMemo, useState } from 'react'
import { createStyles } from 'antd-style'
import type { DailyTrend } from '@/api/usage'
import { USAGE_KIND_LABELS } from '@/api/usage'
import { tokens as t } from '@/styles/tokens'

const KIND_COLORS = ['#0ea5e9', '#8b5cf6', '#f59e0b', '#10b981', '#ef4444', '#64748b']

const useStyles = createStyles(({ css }) => ({
  wrap: css`
    width: 100%;
  `,
  legend: css`
    display: flex;
    flex-wrap: wrap;
    gap: 14px;
    margin-bottom: 8px;
    font-size: ${t.textSm};
    color: ${t.textSecondary};
  `,
  legendItem: css`
    display: inline-flex;
    align-items: center;
    gap: 6px;
    cursor: pointer;
    user-select: none;
    opacity: 1;
  `,
  legendItemOff: css`
    opacity: 0.35;
  `,
  dot: css`
    width: 10px;
    height: 10px;
    border-radius: 50%;
    display: inline-block;
  `,
  empty: css`
    color: ${t.textTertiary};
    font-size: ${t.textSm};
    padding: 32px 0;
    text-align: center;
  `
}))

interface Props {
  trends: DailyTrend[]
  metric: 'calls' | 'tokens' | 'costMicros' | 'errors'
  metricLabel: string
}

const W = 720
const H = 240
const PAD = { left: 48, right: 16, top: 16, bottom: 28 }

export default function UsageTrendChart({ trends, metric, metricLabel }: Props) {
  const { styles, cx } = useStyles()
  const [hidden, setHidden] = useState<Record<string, boolean>>({})

  const { days, series } = useMemo(() => {
    const daySet = new Map<string, Record<string, number>>()
    for (const tr of trends) {
      const bucket = daySet.get(tr.date) ?? {}
      bucket[tr.kind] = (bucket[tr.kind] ?? 0) + (tr[metric] ?? 0)
      daySet.set(tr.date, bucket)
    }
    const sortedDays = [...daySet.keys()].sort()
    const kinds = [...new Set(trends.map((tr) => tr.kind))]
    return { days: sortedDays, series: kinds.map((kind, i) => ({ kind, color: KIND_COLORS[i % KIND_COLORS.length] })) }
  }, [trends, metric])

  if (days.length === 0) {
    return <div className={styles.empty}>所选时间范围内暂无用量数据</div>
  }

  const maxV = Math.max(
    1,
    ...days.flatMap((d) =>
      series.filter((s) => !hidden[s.kind]).map((s) => trendsValue(trends, d, s.kind, metric))
    )
  )

  const x = (i: number) => PAD.left + (i * (W - PAD.left - PAD.right)) / Math.max(1, days.length - 1)
  const y = (v: number) => H - PAD.bottom - (v * (H - PAD.top - PAD.bottom)) / maxV

  const pathFor = (kind: string) =>
    days
      .map((d, i) => `${i === 0 ? 'M' : 'L'}${x(i).toFixed(1)},${y(trendsValue(trends, d, kind, metric)).toFixed(1)}`)
      .join(' ')

  return (
    <div className={styles.wrap}>
      <div className={styles.legend}>
        <span style={{ marginRight: 4 }}>{metricLabel}</span>
        {series.map((s) => (
          <span
            key={s.kind}
            className={cx(styles.legendItem, hidden[s.kind] && styles.legendItemOff)}
            onClick={() => setHidden((h) => ({ ...h, [s.kind]: !h[s.kind] }))}
          >
            <span className={styles.dot} style={{ background: s.color }} />
            {USAGE_KIND_LABELS[s.kind] ?? s.kind}
          </span>
        ))}
      </div>
      <svg viewBox={`0 0 ${W} ${H}`} width="100%" role="img" aria-label="用量趋势图">
        {[0, 0.5, 1].map((f) => (
          <g key={f}>
            <line x1={PAD.left} x2={W - PAD.right} y1={y(maxV * f)} y2={y(maxV * f)} stroke="var(--border)" strokeDasharray={f === 1 ? '' : '4 4'} />
            <text x={PAD.left - 6} y={y(maxV * f) + 4} textAnchor="end" fontSize={10} fill={t.textTertiary}>
              {compact(maxV * f)}
            </text>
          </g>
        ))}
        {days.map((d, i) =>
          i % Math.ceil(days.length / 8) === 0 ? (
            <text key={d} x={x(i)} y={H - 8} textAnchor="middle" fontSize={10} fill={t.textTertiary}>
              {d.slice(5)}
            </text>
          ) : null
        )}
        {series
          .filter((s) => !hidden[s.kind])
          .map((s) => (
            <path key={s.kind} d={pathFor(s.kind)} fill="none" stroke={s.color} strokeWidth={2} />
          ))}
      </svg>
    </div>
  )
}

function trendsValue(trends: DailyTrend[], date: string, kind: string, metric: Props['metric']): number {
  let sum = 0
  for (const tr of trends) {
    if (tr.date === date && tr.kind === kind) sum += tr[metric] ?? 0
  }
  return sum
}

function compact(v: number): string {
  if (v >= 1_000_000) return `${(v / 1_000_000).toFixed(1)}M`
  if (v >= 1_000) return `${(v / 1_000).toFixed(1)}k`
  return `${Math.round(v)}`
}
