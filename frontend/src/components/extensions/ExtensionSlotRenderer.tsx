// ExtensionSlotRenderer 是 H7.2 声明式 UI 插槽的通用渲染器（安全边界）：
// 扩展只能声明四种静态组件（stat-card / link-list / key-value / markdown），
// 前端永不执行扩展提供的任何 JS/CSS。每个组件包独立错误边界——单个组件
// 加载失败只显示「该扩展内容加载失败」小卡片，不影响宿主页面；动态数据
// 组件（dataSource）经平台代理拉取扩展自己声明的 GET 端点，失败同样降级。
// 布局为响应式卡片栅格：PC 3 列 / 平板 2 列 / 手机 1 列。
import { Component, type ReactNode } from 'react'
import { QueryClient, QueryClientProvider, useQuery } from '@tanstack/react-query'
import { Skeleton, theme } from 'antd'
import { createStyles } from 'antd-style'
import { fetchExtensionDataSource, fetchExtensionSlots, type ExtensionSlotItem } from '@/api/extensionSlots'

const useStyles = createStyles(({ css }) => ({
  grid: css`
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 12px;
    margin: 12px 0;
    @media (max-width: 1100px) {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
    @media (max-width: 640px) {
      grid-template-columns: 1fr;
    }
  `,
  card: css`
    border: 1px solid color-mix(in srgb, var(--border) 70%, transparent);
    border-radius: 10px;
    padding: 14px 16px;
    background: color-mix(in srgb, var(--card) 65%, transparent);
    min-width: 0;
  `,
  cardTitle: css`
    font-size: 13px;
    font-weight: 600;
    color: color-mix(in srgb, var(--foreground) 68%, transparent);
    margin-bottom: 8px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  `,
  statValue: css`
    font-size: 24px;
    font-weight: 700;
    line-height: 1.2;
    margin-bottom: 4px;
    overflow-wrap: anywhere;
  `,
  statDesc: css`
    font-size: 12px;
    color: color-mix(in srgb, var(--foreground) 55%, transparent);
  `,
  linkList: css`
    display: flex;
    flex-direction: column;
    gap: 6px;
    margin: 0;
    padding: 0;
    list-style: none;
  `,
  kvTable: css`
    width: 100%;
    border-collapse: collapse;
    font-size: 13px;
    td {
      padding: 3px 0;
      vertical-align: top;
    }
    td:first-child {
      color: color-mix(in srgb, var(--foreground) 55%, transparent);
      padding-right: 12px;
      white-space: nowrap;
    }
  `,
  markdown: css`
    font-size: 13px;
    line-height: 1.6;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  `,
  errorCard: css`
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 12px;
    color: ${theme.getDesignToken().colorTextSecondary};
  `,
  extName: css`
    font-size: 11px;
    color: color-mix(in srgb, var(--foreground) 40%, transparent);
    margin-top: 8px;
  `
}))

// SlotErrorBoundary：单个组件失败不拖垮宿主页面。
class SlotErrorBoundary extends Component<{ children: ReactNode }, { failed: boolean }> {
  state = { failed: false }

  static getDerivedStateFromError(): { failed: boolean } {
    return { failed: true }
  }

  componentDidCatch(error: Error) {
    console.error('[ExtensionSlot]', error)
  }

  render() {
    if (this.state.failed) {
      return <ExtensionErrorCard />
    }
    return this.props.children
  }
}

function ExtensionErrorCard() {
  const { styles } = useStyles()
  return (
    <div className={styles.card} role="alert">
      <div className={styles.errorCard}>该扩展内容加载失败</div>
    </div>
  )
}

function asString(v: unknown): string {
  if (v === null || v === undefined) return ''
  if (typeof v === 'string') return v
  if (typeof v === 'number' || typeof v === 'boolean') return String(v)
  try {
    return JSON.stringify(v)
  } catch {
    return ''
  }
}

function StatCard({ data }: { data?: Record<string, unknown> }) {
  const { styles } = useStyles()
  return (
    <>
      <div className={styles.statValue}>{asString(data?.value) || '—'}</div>
      {asString(data?.description) && <div className={styles.statDesc}>{asString(data?.description)}</div>}
    </>
  )
}

function LinkList({ data }: { data?: Record<string, unknown> }) {
  const { styles } = useStyles()
  const links = Array.isArray(data?.links) ? (data.links as Record<string, unknown>[]) : []
  if (links.length === 0) return <div className={styles.statDesc}>暂无链接</div>
  return (
    <ul className={styles.linkList}>
      {links.map((link, i) => {
        const href = asString(link?.href)
        const label = asString(link?.label) || href
        return (
          <li key={`${href}-${i}`}>
            {href ? (
              <a href={href} target="_blank" rel="noreferrer noopener">
                {label}
              </a>
            ) : (
              label
            )}
          </li>
        )
      })}
    </ul>
  )
}

function KeyValue({ data }: { data?: Record<string, unknown> }) {
  const { styles } = useStyles()
  const entries = Array.isArray(data?.entries)
    ? (data.entries as Record<string, unknown>[])
    : Object.entries(data ?? {})
        .filter(([, v]) => typeof v !== 'object' || v === null)
        .map(([k, v]) => ({ key: k, value: v }))
  if (entries.length === 0) return <div className={styles.statDesc}>暂无数据</div>
  return (
    <table className={styles.kvTable}>
      <tbody>
        {entries.map((entry, i) => {
          const pair = entry as unknown as Record<string, unknown> | unknown[]
          const isPair = Array.isArray(pair)
          const k = asString(isPair ? pair[0] : (pair as Record<string, unknown>)?.key)
          const v = asString(isPair ? pair[1] : (pair as Record<string, unknown>)?.value)
          return (
            <tr key={`${k}-${i}`}>
              <td>{k}</td>
              <td>{v}</td>
            </tr>
          )
        })}
      </tbody>
    </table>
  )
}

function MarkdownText({ data }: { data?: Record<string, unknown> }) {
  const { styles } = useStyles()
  // 静态文本：纯文本渲染，不解析 HTML（XSS 安全边界）。
  return <div className={styles.markdown}>{asString(data?.content)}</div>
}

function StaticBody({ item, data }: { item: ExtensionSlotItem; data?: Record<string, unknown> }) {
  switch (item.component) {
    case 'stat-card':
      return <StatCard data={data} />
    case 'link-list':
      return <LinkList data={data} />
    case 'key-value':
      return <KeyValue data={data} />
    case 'markdown':
      return <MarkdownText data={data} />
    default:
      return null // 未知组件类型不渲染（后端白名单兜底，前端双保险）
  }
}

// DataSourceBody：动态数据组件。失败（网络/代理 502/超限）抛给错误边界降级。
function DataSourceBody({ item }: { item: ExtensionSlotItem }) {
  const { data, isLoading, isError } = useQuery({
    queryKey: ['extension-slot-data', item.extensionName, item.dataSource?.path],
    queryFn: () => fetchExtensionDataSource(item.dataSource?.path ?? ''),
    retry: 1,
    staleTime: 30_000
  })
  if (isLoading) return <Skeleton active paragraph={{ rows: 2 }} title={false} />
  if (isError) throw new Error('扩展数据源加载失败')
  const payload = (data ?? {}) as Record<string, unknown>
  if (payload && typeof payload === 'object' && !Array.isArray(payload) && 'data' in payload) {
    return <StaticBody item={item} data={(payload.data ?? {}) as Record<string, unknown>} />
  }
  return <StaticBody item={item} data={payload} />
}

function SlotCard({ item }: { item: ExtensionSlotItem }) {
  const { styles } = useStyles()
  return (
    <div className={styles.card}>
      {item.title && <div className={styles.cardTitle}>{item.title}</div>}
      <SlotErrorBoundary>
        {item.dataSource ? <DataSourceBody item={item} /> : <StaticBody item={item} data={item.data} />}
      </SlotErrorBoundary>
      <div className={styles.extName}>{item.extensionName}@{item.version}</div>
    </div>
  )
}

// 模块级 QueryClient 单例：插槽渲染器可能被宿主页面在任何上下文
// （包括无全局 Provider 的测试环境）渲染，自带轻量 Provider。
// 导出供测试在 beforeEach 中 clear()，避免用例间缓存串扰。
export const slotQueryClient = new QueryClient({
  defaultOptions: { queries: { retry: 1, staleTime: 30_000, refetchOnWindowFocus: false } }
})

export interface ExtensionSlotRendererProps {
  slot: string
  // 预留上下文（如 runId/agentId）：声明式组件当前不用，宿主页面传了也不影响。
  context?: Record<string, string>
}

// 通用插槽渲染器：查询当前租户该插槽的组件列表，逐个渲染。
export default function ExtensionSlotRenderer({ slot }: ExtensionSlotRendererProps) {
  return (
    <QueryClientProvider client={slotQueryClient}>
      <ExtensionSlotRendererInner slot={slot} />
    </QueryClientProvider>
  )
}

function ExtensionSlotRendererInner({ slot }: { slot: string }) {
  const { styles } = useStyles()
  const { data, isLoading, isError } = useQuery({
    queryKey: ['extension-slot', slot],
    queryFn: () => fetchExtensionSlots(slot),
    staleTime: 30_000
  })

  if (isError) {
    // 插槽列表本身加载失败：静默降级，不打扰宿主页面。
    return null
  }
  if (isLoading) {
    return (
      <div className={styles.grid} aria-label={`扩展内容加载中-${slot}`}>
        <div className={styles.card}>
          <Skeleton active paragraph={{ rows: 2 }} />
        </div>
      </div>
    )
  }
  if (!data || data.length === 0) return null
  return (
    <div className={styles.grid} data-extension-slot={slot}>
      {data.map((item, i) => (
        <SlotCard key={`${item.extensionName}-${item.component}-${item.title}-${i}`} item={item} />
      ))}
    </div>
  )
}
