// ExtensionSlotRenderer 组件测试：四种声明式组件渲染、错误边界降级、
// 动态数据源失败降级、加载骨架屏、响应式栅格类名。
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { act, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ConfigProvider } from 'antd'
import { antdTheme } from '@/lib/antd-theme'
import ExtensionSlotRenderer, { slotQueryClient } from './ExtensionSlotRenderer'
import { fetchExtensionSlots, fetchExtensionDataSource, type ExtensionSlotItem } from '@/api/extensionSlots'

vi.mock('@/api/extensionSlots', () => ({
  fetchExtensionSlots: vi.fn(),
  fetchExtensionDataSource: vi.fn()
}))

const mockedFetchSlots = vi.mocked(fetchExtensionSlots)
const mockedFetchData = vi.mocked(fetchExtensionDataSource)

function item(partial: Partial<ExtensionSlotItem>): ExtensionSlotItem {
  return {
    slot: 'dashboard.card',
    component: 'stat-card',
    title: '',
    order: 0,
    visible: true,
    extensionName: 'io.zerone.test',
    extensionId: 1,
    version: '1.0.0',
    ...partial
  }
}

function renderSlot(slot = 'dashboard.card') {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <ConfigProvider theme={antdTheme}>
      <QueryClientProvider client={client}>
        <ExtensionSlotRenderer slot={slot} />
      </QueryClientProvider>
    </ConfigProvider>
  )
}

describe('ExtensionSlotRenderer', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    slotQueryClient.clear()
  })

  it('空插槽不渲染任何内容', async () => {
    mockedFetchSlots.mockResolvedValue([])
    const { container } = renderSlot()
    await waitFor(() => { expect(container.firstChild).toBeNull(); }, { timeout: 3000 })
  })

  it('渲染 stat-card（标题+数值+说明）', async () => {
    mockedFetchSlots.mockResolvedValue([
      item({ component: 'stat-card', title: '运行总数', data: { value: 42, description: '今天' } })
    ])
    renderSlot()
    expect(await screen.findByText('运行总数')).toBeInTheDocument()
    expect(screen.getByText('42')).toBeInTheDocument()
    expect(screen.getByText('今天')).toBeInTheDocument()
  })

  it('渲染 link-list 链接列表', async () => {
    mockedFetchSlots.mockResolvedValue([
      item({
        component: 'link-list',
        title: '扩展链接',
        data: { links: [{ label: '文档', href: 'https://example.com' }] }
      })
    ])
    renderSlot()
    const link = await screen.findByRole('link', { name: '文档' })
    expect(link).toHaveAttribute('href', 'https://example.com')
    expect(link).toHaveAttribute('rel', expect.stringContaining('noreferrer'))
  })

  // P2-15 回归：扩展 manifest 的 ui.slots[].data 是自由 map，href 原样进
  // <a href> 就等于把 javascript: 注入面交给扩展（点一下即 XSS）。
  it('link-list 拒绝非法协议的 href，退化为纯文本', async () => {
    mockedFetchSlots.mockResolvedValue([
      item({
        component: 'link-list',
        title: '扩展链接',
        data: {
          links: [
            { label: '恶意', href: 'javascript:alert(1)' },
            { label: '协议相对', href: '//evil.com/x' },
            { label: '正常', href: 'https://example.com/ok' }
          ]
        }
      })
    ])
    renderSlot()

    expect(await screen.findByRole('link', { name: '正常' })).toHaveAttribute(
      'href',
      'https://example.com/ok'
    )
    expect(screen.queryByRole('link', { name: '恶意' })).toBeNull()
    expect(screen.queryByRole('link', { name: '协议相对' })).toBeNull()
    // 链接降级成文本，不是整项消失（用户仍能看到扩展想展示什么）
    expect(screen.getByText('恶意')).toBeInTheDocument()
    expect(screen.getByText('协议相对')).toBeInTheDocument()
    expect(document.querySelectorAll('a')).toHaveLength(1)
  })

  it('渲染 key-value 键值对', async () => {
    mockedFetchSlots.mockResolvedValue([
      item({ component: 'key-value', title: '配置', data: { entries: [{ key: '区域', value: '华东' }] } })
    ])
    renderSlot()
    expect(await screen.findByText('区域')).toBeInTheDocument()
    expect(screen.getByText('华东')).toBeInTheDocument()
  })

  it('markdown 组件纯文本渲染，不解析 HTML', async () => {
    mockedFetchSlots.mockResolvedValue([
      item({ component: 'markdown', title: '说明', data: { content: '<img src=x onerror=alert(1)> 静态文本' } })
    ])
    renderSlot()
    expect(await screen.findByText(/静态文本/)).toBeInTheDocument()
    expect(document.querySelector('img')).toBeNull()
  })

  it('未知组件类型不渲染', async () => {
    mockedFetchSlots.mockResolvedValue([item({ component: 'evil-widget' })])
    const { container } = renderSlot()
    await waitFor(() => { expect(mockedFetchSlots).toHaveBeenCalled(); })
    expect(container.querySelector('[data-extension-slot]')).toBeNull()
  })

  it('动态数据源组件加载数据并渲染', async () => {
    mockedFetchSlots.mockResolvedValue([
      item({
        component: 'stat-card',
        title: '动态指标',
        dataSource: { path: '/api/v1/extensions/io.zerone.test/metrics' }
      })
    ])
    mockedFetchData.mockResolvedValue({ value: 7, description: '实时' })
    renderSlot()
    expect(await screen.findByText('7')).toBeInTheDocument()
    expect(screen.getByText('实时')).toBeInTheDocument()
    expect(mockedFetchData).toHaveBeenCalledWith('/api/v1/extensions/io.zerone.test/metrics')
  })

  it('动态数据源失败时降级提示，不影响其它组件', async () => {
    mockedFetchSlots.mockResolvedValue([
      item({ component: 'stat-card', title: '坏组件', dataSource: { path: '/x' } }),
      item({ component: 'stat-card', title: '好组件', data: { value: 1 } })
    ])
    mockedFetchData.mockRejectedValue(new Error('boom'))
    renderSlot()
    expect(await screen.findByText('该扩展数据加载失败', {}, { timeout: 5000 })).toBeInTheDocument()
    expect(screen.getByText('好组件')).toBeInTheDocument()
    expect(screen.getByText('1')).toBeInTheDocument()
  })

  // P2-16 回归：旧实现把失败 throw 进 SlotErrorBoundary，而边界没有复位逻辑，
  // 后续 refetch 成功也永久显示"加载失败"。现在按 isError 渲染，
  // 取数成功即自动恢复。
  it('动态数据源先失败后成功：错误提示可恢复', async () => {
    mockedFetchSlots.mockResolvedValue([
      item({ component: 'stat-card', title: '动态', dataSource: { path: '/y' } })
    ])
    mockedFetchData.mockRejectedValue(new Error('boom'))
    renderSlot()
    expect(await screen.findByText('该扩展数据加载失败', {}, { timeout: 5000 })).toBeInTheDocument()

    mockedFetchData.mockResolvedValue({ value: 9, description: '已恢复' })
    await act(async () => {
      await slotQueryClient.refetchQueries({ queryKey: ['extension-slot-data'] })
    })

    expect(await screen.findByText('9', {}, { timeout: 5000 })).toBeInTheDocument()
    expect(screen.getByText('已恢复')).toBeInTheDocument()
    expect(screen.queryByText('该扩展数据加载失败')).toBeNull()
  })

  it('插槽列表请求失败时静默降级为空', async () => {
    mockedFetchSlots.mockRejectedValue(new Error('network'))
    const { container } = renderSlot()
    await waitFor(() => { expect(container.firstChild).toBeNull(); }, { timeout: 3000 })
  })

  it('响应式栅格：PC 3 列 / 平板 2 列 / 手机 1 列', async () => {
    mockedFetchSlots.mockResolvedValue([item({ title: '卡片' })])
    renderSlot()
    await screen.findByText('卡片')
    const grid = document.querySelector('[data-extension-slot="dashboard.card"]') as HTMLElement
    const cssText = Array.from(document.styleSheets)
      .map((sheet) => {
        try {
          return Array.from(sheet.cssRules)
            .map((rule) => rule.cssText)
            .join('\n')
        } catch {
          return ''
        }
      })
      .join('\n')
    // antd-style 把媒体查询写进 stylesheet
    expect(cssText).toContain('repeat(3, minmax(0, 1fr))')
    expect(cssText).toMatch(/max-width: 1100px[\s\S]*repeat\(2, minmax\(0, 1fr\)\)/)
    expect(cssText).toMatch(/max-width: 640px[\s\S]*grid-template-columns: 1fr/)
    expect(grid).not.toBeNull()
  })

  it('加载中显示骨架屏', () => {
    mockedFetchSlots.mockReturnValue(new Promise(() => {}))
    renderSlot()
    expect(screen.getByLabelText('扩展内容加载中-dashboard.card')).toBeInTheDocument()
  })
})
