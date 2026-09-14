// H7.5 用量总览页：kind 卡片 + 按天趋势折线 + 扩展排行 + 错误聚合 +
// 系统健康卡片 + 时间范围选择（7天/30天/自定义）+ CSV 导出。
import { useMemo, useState } from 'react'
import { Alert, Card, DatePicker, Segmented, Select, Space, Table, message } from 'antd'
import { ChartLineIcon, DownloadIcon } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import dayjs, { type Dayjs } from 'dayjs'
import PrimaryButton from '@/components/PrimaryButton'
import { parseApiError } from '@/api/client'
import {
  USAGE_KIND_LABELS,
  formatMicros,
  usageApi,
  type UsageRange
} from '@/api/usage'
import {
  useUsageByExtension,
  useUsageErrors,
  useUsageSummary
} from '@/queries/useUsage'
import UsageHealthCard from './UsageHealthCard'
import UsageTrendChart from './UsageTrendChart'
import { tokens as t } from '@/styles/tokens'

const { RangePicker } = DatePicker

const useStyles = createStyles(({ css }) => ({
  page: css`
    width: 100%;
    max-width: 1200px;
    margin: 0 auto;
  `,
  header: css`
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 24px;
    margin-bottom: 20px;
  `,
  title: css`
    margin: 0;
    color: ${t.text};
    font-size: ${t.text2xl};
    font-weight: 650;
  `,
  subtitle: css`
    margin: 5px 0 0;
    color: ${t.textTertiary};
    font-size: ${t.textBase};
  `,
  toolbar: css`
    display: flex;
    flex-wrap: wrap;
    gap: 12px;
    align-items: center;
    margin-bottom: 16px;
  `,
  cards: css`
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(170px, 1fr));
    gap: 12px;
    margin-bottom: 16px;
  `,
  kindCard: css`
    border-radius: ${t.radius}px;
  `,
  kindName: css`
    color: ${t.textSecondary};
    font-size: ${t.textSm};
  `,
  kindValue: css`
    font-size: ${t.textXl};
    font-weight: 650;
    color: ${t.text};
  `,
  kindMeta: css`
    margin-top: 4px;
    color: ${t.textTertiary};
    font-size: ${t.textXs};
    line-height: 1.6;
  `,
  section: css`
    margin-bottom: 16px;
  `
}))

type Preset = '7d' | '30d' | 'custom'

const TREND_METRICS = [
  { value: 'calls', label: '调用次数' },
  { value: 'tokens', label: 'Token 数' },
  { value: 'costMicros', label: '费用（微分）' },
  { value: 'errors', label: '错误数' }
] as const

type Metric = (typeof TREND_METRICS)[number]['value']

export default function UsageOverviewPage() {
  const { styles } = useStyles()
  const [preset, setPreset] = useState<Preset>('7d')
  const [customRange, setCustomRange] = useState<[Dayjs, Dayjs] | null>(null)
  const [metric, setMetric] = useState<Metric>('calls')

  const range: UsageRange = useMemo(() => {
    if (preset === 'custom' && customRange) {
      return { from: customRange[0].format('YYYY-MM-DD'), to: customRange[1].format('YYYY-MM-DD') }
    }
    const days = preset === '30d' ? 30 : 7
    return { from: dayjs().subtract(days - 1, 'day').format('YYYY-MM-DD'), to: dayjs().format('YYYY-MM-DD') }
  }, [preset, customRange])

  const summary = useUsageSummary(range)
  const byExtension = useUsageByExtension(range)
  const errors = useUsageErrors(range)

  const kinds = summary.data?.kinds ?? []
  const totalCalls = kinds.reduce((s, k) => s + k.totalCalls, 0)
  const totalErrors = kinds.reduce((s, k) => s + k.totalErrors, 0)
  const errorRate = totalCalls > 0 ? (totalErrors / totalCalls) * 100 : 0

  return (
    <div className={styles.page}>
      <div className={styles.header}>
        <div>
          <h1 className={styles.title}>用量总览</h1>
          <p className={styles.subtitle}>
            Token / 费用 / 错误率按天聚合（实时统计）· 范围 {range.from} ~ {range.to}
          </p>
        </div>
        <PrimaryButton
          icon={<DownloadIcon size={16} />}
          onClick={() => {
            try {
              window.open(usageApi.exportUrl(range), '_blank')
            } catch (e) {
              message.error(parseApiError(e))
            }
          }}
        >
          导出 CSV
        </PrimaryButton>
      </div>

      <div className={styles.toolbar}>
        <Segmented
          value={preset}
          onChange={(v) => setPreset(v as Preset)}
          options={[
            { label: '近 7 天', value: '7d' },
            { label: '近 30 天', value: '30d' },
            { label: '自定义', value: 'custom' }
          ]}
        />
        {preset === 'custom' && (
          <RangePicker
            value={customRange}
            onChange={(v) => setCustomRange(v as [Dayjs, Dayjs] | null)}
            allowClear={false}
          />
        )}
      </div>

      {summary.error && <Alert type="error" showIcon message={parseApiError(summary.error)} style={{ marginBottom: 16 }} />}

      <div className={styles.cards}>
        <Card className={styles.kindCard}>
          <div className={styles.kindName}>总调用</div>
          <div className={styles.kindValue}>{totalCalls}</div>
        </Card>
        {kinds.map((k) => (
          <Card key={k.kind} className={styles.kindCard}>
            <div className={styles.kindName}>{USAGE_KIND_LABELS[k.kind] ?? k.kind}</div>
            <div className={styles.kindValue}>{k.totalCalls}</div>
            <div className={styles.kindMeta}>
              Token {k.totalTokens} · 费用 {formatMicros(k.totalCostMicros)} 分
              <br />
              错误 {k.totalErrors} · 均延迟 {k.avgLatencyMs.toFixed(0)} ms
              {k.deniedCount > 0 ? ` · 超限 ${k.deniedCount}` : ''}
            </div>
          </Card>
        ))}
        <Card className={styles.kindCard}>
          <div className={styles.kindName}>错误率</div>
          <div className={styles.kindValue}>{errorRate.toFixed(1)}%</div>
          <div className={styles.kindMeta}>错误 {totalErrors} / 调用 {totalCalls}</div>
        </Card>
      </div>

      <Card
        className={styles.section}
        title={
          <Space>
            <ChartLineIcon size={18} />
            按天趋势
          </Space>
        }
        extra={
          <Select<Metric> value={metric} onChange={setMetric} options={[...TREND_METRICS]} style={{ width: 140 }} />
        }
      >
        <UsageTrendChart
          trends={summary.data?.trends ?? []}
          metric={metric}
          metricLabel={TREND_METRICS.find((m) => m.value === metric)?.label ?? ''}
        />
      </Card>

      <Card className={styles.section} title="扩展调用排行">
        <Table
          rowKey="key"
          size="small"
          loading={byExtension.isLoading}
          dataSource={byExtension.data?.items ?? []}
          pagination={{ pageSize: 10, total: byExtension.data?.total ?? 0, hideOnSinglePage: true }}
          columns={[
            { title: '扩展', dataIndex: 'key' },
            { title: '调用', dataIndex: 'totalCalls', width: 100 },
            { title: 'Token', dataIndex: 'totalTokens', width: 120 },
            { title: '费用（分）', dataIndex: 'totalCostMicros', width: 120, render: (v: number) => formatMicros(v) },
            { title: '错误', dataIndex: 'totalErrors', width: 90 },
            { title: '均延迟（ms）', dataIndex: 'avgLatencyMs', width: 120, render: (v: number) => v.toFixed(0) }
          ]}
        />
      </Card>

      <Card className={styles.section} title="错误聚合">
        <Table
          rowKey={(row) => `${row.kind}:${row.error}`}
          size="small"
          loading={errors.isLoading}
          dataSource={errors.data?.items ?? []}
          pagination={{ pageSize: 10, hideOnSinglePage: true }}
          locale={{ emptyText: '所选范围内没有错误记录' }}
          columns={[
            { title: '类型', dataIndex: 'kind', width: 140, render: (v: string) => USAGE_KIND_LABELS[v] ?? v },
            { title: '错误', dataIndex: 'error' },
            { title: '次数', dataIndex: 'count', width: 90 },
            { title: '最近发生', dataIndex: 'lastAt', width: 200, render: (v: string) => dayjs(v).format('YYYY-MM-DD HH:mm:ss') }
          ]}
        />
      </Card>

      <UsageHealthCard />
    </div>
  )
}
