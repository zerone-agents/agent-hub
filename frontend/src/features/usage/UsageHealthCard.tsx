// 系统健康状态卡片：绿/黄/红合成状态 + DB / 队列 / 今日错误率 / 扩展启停 / uptime。
import { Card, Statistic, Tag } from 'antd'
import { createStyles } from 'antd-style'
import { useUsageHealth } from '@/queries/useUsage'
import { tokens as t } from '@/styles/tokens'

const useStyles = createStyles(({ css }) => ({
  card: css`
    border-radius: ${t.radius}px;
  `,
  statusDot: css`
    display: inline-block;
    width: 12px;
    height: 12px;
    border-radius: 50%;
    margin-right: 8px;
  `,
  grid: css`
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
    gap: 16px;
    margin-top: 16px;
  `
}))

const STATUS_META: Record<string, { label: string; color: string }> = {
  green: { label: '健康', color: 'var(--success)' },
  yellow: { label: '警告', color: 'var(--warning)' },
  red: { label: '异常', color: 'var(--destructive)' }
}

export function formatUptime(sec: number): string {
  if (sec < 60) return `${sec} 秒`
  if (sec < 3600) return `${Math.floor(sec / 60)} 分钟`
  if (sec < 86400) return `${(sec / 3600).toFixed(1)} 小时`
  return `${(sec / 86400).toFixed(1)} 天`
}

export default function UsageHealthCard() {
  const { styles } = useStyles()
  const { data, isLoading } = useUsageHealth()
  if (isLoading || !data) return null
  const meta = STATUS_META[data.status] ?? STATUS_META.green

  return (
    <Card className={styles.card} loading={isLoading}>
      <div>
        <span className={styles.statusDot} style={{ background: meta.color }} />
        <strong style={{ fontSize: t.textLg }}>{meta.label}</strong>
        <Tag style={{ marginLeft: 8 }} color={data.status === 'green' ? 'success' : data.status === 'yellow' ? 'warning' : 'error'}>
          {data.status.toUpperCase()}
        </Tag>
      </div>
      <div className={styles.grid}>
        <Statistic title="数据库" value={data.db === 'ok' ? '连通' : '异常'} valueStyle={{ fontSize: t.textBase }} />
        <Statistic title="采集队列积压" value={data.queueBacklog} valueStyle={{ fontSize: t.textBase }} />
        <Statistic title="丢弃埋点" value={data.queueDropped} valueStyle={{ fontSize: t.textBase }} />
        <Statistic title="今日错误率" value={data.todayErrorRatePct} precision={1} suffix="%" valueStyle={{ fontSize: t.textBase }} />
        <Statistic title="扩展启用" value={`${data.extensionsOn} / ${data.extensionsTotal}`} valueStyle={{ fontSize: t.textBase }} />
        <Statistic title="运行时长" value={formatUptime(data.uptimeSec)} valueStyle={{ fontSize: t.textBase }} />
      </div>
    </Card>
  )
}
