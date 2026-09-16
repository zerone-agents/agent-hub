// ExtensionSidebarSection 渲染 sidebar 插槽：扩展经 manifest ui.slots
// 声明的 link-list 组件会在这里以导航链接形式出现在侧边栏底部。
// 只有链接项会被采纳，其他组件类型在侧边栏不渲染；逐项包错误边界。
import { QueryClient, QueryClientProvider, useQuery } from '@tanstack/react-query'
import { fetchExtensionSlots, type ExtensionSlotItem } from '@/api/extensionSlots'
import { safeExternalHref } from '@/utils/url'
import SlotItemErrorBoundary from './SlotItemErrorBoundary'

interface SidebarLink {
  label: string
  href: string
}

function collectLinks(item: ExtensionSlotItem): SidebarLink[] {
  if (item.component !== 'link-list' || !Array.isArray(item.data?.links)) return []
  return (item.data.links as Record<string, unknown>[])
    .map((link) => ({
      label: String(link?.label ?? link?.href ?? ''),
      // 扩展提供的 href 先过协议白名单：manifest 的 data 是自由 map，
      // 原样进 <a href> 会让 javascript: 直接可点。校验不过的项被过滤掉。
      href: safeExternalHref(link?.href)
    }))
    .filter((link) => link.href)
}

function SidebarLinkItem({ item }: { item: ExtensionSlotItem }) {
  const links = collectLinks(item)
  if (links.length === 0) return null
  return (
    <>
      {links.map((link, i) => (
        <a
          key={`${item.extensionName}-${link.href}-${i}`}
          href={link.href}
          target="_blank"
          rel="noreferrer noopener"
          style={{ display: 'block', padding: '8px 18px', fontSize: 13, color: 'inherit', opacity: 0.75 }}
        >
          {link.label}
        </a>
      ))}
    </>
  )
}

// 自带轻量 QueryClient（见 ExtensionSlotRenderer）。
const sidebarQueryClient = new QueryClient({
  defaultOptions: { queries: { retry: 1, staleTime: 60_000, refetchOnWindowFocus: false } }
})

export default function ExtensionSidebarSection() {
  return (
    <QueryClientProvider client={sidebarQueryClient}>
      <ExtensionSidebarSectionInner />
    </QueryClientProvider>
  )
}

function ExtensionSidebarSectionInner() {
  const { data } = useQuery({
    queryKey: ['extension-slot', 'sidebar'],
    queryFn: () => fetchExtensionSlots('sidebar'),
    staleTime: 60_000,
    retry: 1
  })
  if (!data || data.length === 0) return null
  return (
    <div style={{ flexShrink: 0, padding: '6px 0', borderTop: '1px solid color-mix(in srgb, var(--sidebar-border) 72%, transparent)' }}>
      {data.map((item, i) => (
        <SlotItemErrorBoundary key={`${item.extensionName}-${i}`}>
          <SidebarLinkItem item={item} />
        </SlotItemErrorBoundary>
      ))}
    </div>
  )
}
