import {
  fireEvent,
  render,
  screen,
  waitFor,
  within
} from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ConfigProvider } from 'antd'
import { memoryApi, type NativeMemoryMessage } from '@/api/memory'
import { antdTheme } from '@/lib/antd-theme'
import NativeMemoryPanel from './NativeMemoryPanel'

vi.mock('@/api/memory', async (original) => ({
  ...(await original<typeof import('@/api/memory')>()),
  memoryApi: {
    nativeMessages: vi.fn(),
    nativeContext: vi.fn(),
    nativeSearch: vi.fn(),
    setNativeStatus: vi.fn()
  }
}))
const subject = {
  scope: {
    tenantId: 't',
    applicationId: 'app',
    userId: 'alice',
    accountLifecycleId: 'life'
  },
  state: 'active',
  revision: 1
}
const rows: NativeMemoryMessage[] = (
  ['raw', 'semantic', 'episodic', 'procedural'] as const
).map((type, i) => ({
  id: String(i + 1),
  sourceId: i ? '1' : '0',
  sourceSessionId: 'life-turn',
  type,
  content: type + ' actual source content',
  createdAt: '2026-11-08 10:00:00',
  status: true,
  nativeStatus: true,
  statusChangeAllowed: true,
  parentStatus: i ? true : undefined,
  extractionStatus: i ? undefined : 'derivedAvailable'
}))
const clients: QueryClient[] = []
function mount(state = 'active', management = false) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } }
  })
  clients.push(client)
  return render(
    <ConfigProvider theme={antdTheme}>
      <QueryClientProvider client={client}>
        <NativeMemoryPanel
          subject={{ ...subject, state }}
          application={
            management
              ? {
                  tenantId: 't',
                  applicationId: 'app',
                  policyVersion: 'synthetic-v1',
                  dataClass: 'synthetic',
                  nativeStatusSupported: true,
                  nativeDiagnosticsSupported: true
                }
              : undefined
          }
          queryPrefix={['native-test', crypto.randomUUID()]}
        />
      </QueryClientProvider>
    </ConfigProvider>
  )
}
beforeEach(() => {
  vi.resetAllMocks()
  vi.mocked(memoryApi.nativeMessages).mockResolvedValue(rows)
  vi.mocked(memoryApi.nativeContext).mockResolvedValue({
    messages: [rows[1]],
    status: 'ready',
    budgetLimited: false,
    recentSourceIncluded: true,
    currentnessNote: 'Inspect latest source.'
  })
})
afterEach(() => {
  clients.forEach((client) => {
    client.clear()
  })
  clients.length = 0
})
describe('native memory provenance', () => {
  it('shows all actual memory types and source status without manual entry or delete controls', async () => {
    mount()
    await screen.findByText('semantic actual source content')
    expect(
      screen.getByText('episodic actual source content')
    ).toBeInTheDocument()
    expect(
      screen.getByText('procedural actual source content')
    ).toBeInTheDocument()
    expect(screen.getByText('已形成派生记忆')).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: '删除' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: '添加' })
    ).not.toBeInTheDocument()
    expect(memoryApi.nativeMessages).toHaveBeenCalledWith(
      subject.scope,
      expect.any(AbortSignal)
    )
  })
  it('can explicitly refresh derived records until native completion is observed', async () => {
    vi.mocked(memoryApi.nativeMessages)
      .mockResolvedValueOnce(rows)
      .mockResolvedValueOnce([{ ...rows[0], extractionStatus: 'completed' }])
    mount()
    await screen.findByText('已形成派生记忆')
    fireEvent.click(screen.getByRole('button', { name: '刷新提取状态' }))
    await screen.findByText('提取完成')
    expect(memoryApi.nativeMessages).toHaveBeenCalledTimes(2)
  })
  it('does not show an earlier query response after the question changes', async () => {
    let complete!: (
      value: Awaited<ReturnType<typeof memoryApi.nativeContext>>
    ) => void
    vi.mocked(memoryApi.nativeContext).mockReturnValue(
      new Promise((resolve) => {
        complete = resolve
      })
    )
    mount()
    await screen.findByText('raw actual source content')
    fireEvent.click(screen.getByRole('tab', { name: '召回诊断' }))
    const input = screen.getByRole('textbox', { name: '检索原生记忆' })
    fireEvent.change(input, { target: { value: 'old question' } })
    fireEvent.click(screen.getByRole('button', { name: '检索记忆' }))
    await waitFor(() => {
      expect(memoryApi.nativeContext).toHaveBeenCalledWith({
        scope: subject.scope,
        query: 'old question',
        limit: 5
      })
    })
    fireEvent.change(input, { target: { value: 'new question' } })
    complete({
      messages: [{ ...rows[1], content: 'old asynchronous response' }],
      status: 'ready',
      budgetLimited: false,
      recentSourceIncluded: false,
      currentnessNote: ''
    })
    await waitFor(() => {
      expect(
        screen.queryByText('old asynchronous response')
      ).not.toBeInTheDocument()
    })
  })
  it('does not read provider memories for an inactive lifecycle', () => {
    mount('frozen')
    expect(memoryApi.nativeMessages).not.toHaveBeenCalled()
    expect(memoryApi.nativeContext).not.toHaveBeenCalled()
  })
})

describe('memory management interactions', () => {
  it('filters derived records and traces a selected record to its exact original', async () => {
    mount()
    await screen.findByText('semantic actual source content')
    fireEvent.mouseDown(screen.getByRole('combobox', { name: '筛选记忆类型' }))
    fireEvent.click(
      screen.getByText('语义记忆', {
        selector: '.ant-select-item-option-content'
      })
    )
    expect(
      screen.getByText('semantic actual source content')
    ).toBeInTheDocument()
    expect(
      screen.queryByText('episodic actual source content')
    ).not.toBeInTheDocument()
    expect(
      screen.queryByText('raw actual source content')
    ).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '查看记忆 2 详情' }))
    const drawer = await screen.findByRole('dialog', { name: '记忆详情' })
    expect(
      within(drawer).getByText('semantic actual source content', {
        selector: 'pre'
      })
    ).toBeInTheDocument()
    fireEvent.click(within(drawer).getByRole('button', { name: '追溯原文 #1' }))
    expect(
      within(drawer).getByText('raw actual source content', { selector: 'pre' })
    ).toBeInTheDocument()
    fireEvent.click(
      within(drawer).getByRole('button', { name: '查看关联记忆 3' })
    )
    expect(
      within(drawer).getByText('episodic actual source content', {
        selector: 'pre'
      })
    ).toBeInTheDocument()
  })
  it('shows retained derived content when its extraction task failed', async () => {
    vi.mocked(memoryApi.nativeMessages).mockResolvedValue([
      {
        ...rows[0],
        extractionStatus: 'failed',
        extractionError: 'NATIVE_EXTRACTION_FAILED'
      },
      ...rows.slice(1)
    ])
    mount()
    await screen.findByText('semantic actual source content')
    expect(
      screen.getByText(/提取任务失败，但已有派生内容保留/)
    ).toBeInTheDocument()
    fireEvent.change(screen.getByRole('textbox', { name: '搜索记忆记录' }), {
      target: { value: 'episodic' }
    })
    expect(
      screen.queryByText('semantic actual source content')
    ).not.toBeInTheDocument()
    expect(
      screen.getByText('episodic actual source content')
    ).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '清除筛选' }))
    expect(
      screen.getByText('semantic actual source content')
    ).toBeInTheDocument()
  })
  it('confirms source disabling and refreshes effective child supply without changing child switches', async () => {
    const disabled = rows.map((item, index) => ({
      ...item,
      status: false,
      nativeStatus: index > 0,
      parentStatus: index > 0 ? false : undefined
    }))
    vi.mocked(memoryApi.setNativeStatus).mockImplementation(async () => {
      vi.mocked(memoryApi.nativeMessages).mockResolvedValue(disabled)
      return disabled[0]
    })
    mount('active', true)
    await screen.findByText('raw actual source content')
    fireEvent.click(screen.getByRole('switch', { name: '切换记忆 1 启用状态' }))
    const modal = await screen.findByRole('dialog', {
      name: '停用这条记忆的供给？'
    })
    expect(within(modal).getByText(/同源派生内容供给/)).toBeInTheDocument()
    expect(within(modal).getByText(/不暂停异步抽取/)).toBeInTheDocument()
    expect(memoryApi.setNativeStatus).not.toHaveBeenCalled()
    fireEvent.click(within(modal).getByRole('button', { name: '停用供给' }))
    await waitFor(() => {
      expect(memoryApi.setNativeStatus).toHaveBeenCalledWith('1', {
        scope: subject.scope,
        enabled: false,
        expectedEnabled: true
      })
    })
    await waitFor(() => {
      expect(
        screen.getByRole('switch', { name: '切换记忆 1 启用状态' })
      ).not.toBeChecked()
    })
    expect(
      screen.getByRole('switch', { name: '切换记忆 2 启用状态' })
    ).toBeChecked()
    expect(
      screen.getByRole('switch', { name: '切换记忆 2 启用状态' })
    ).toBeEnabled()
    expect(
      screen.getByText('semantic actual source content')
    ).toBeInTheDocument()
  })
  it('blocks enabling a disabled child while its original is disabled', async () => {
    vi.mocked(memoryApi.nativeMessages).mockResolvedValue(
      rows.map((item, index) => ({
        ...item,
        status: false,
        nativeStatus: index > 1,
        parentStatus: index ? false : undefined
      }))
    )
    mount('active', true)
    await screen.findByText('semantic actual source content')
    expect(
      screen.getByRole('switch', { name: '切换记忆 2 启用状态' })
    ).toBeDisabled()
    expect(
      screen.getByRole('switch', { name: '切换记忆 3 启用状态' })
    ).toBeEnabled()
    expect(memoryApi.setNativeStatus).not.toHaveBeenCalled()
  })
  it('keeps an unconfirmed toggle from being replayed and does not pretend the switch changed', async () => {
    vi.mocked(memoryApi.setNativeStatus).mockRejectedValue(
      new Error('unconfirmed')
    )
    mount('active', true)
    await screen.findByText('raw actual source content')
    fireEvent.click(screen.getByRole('switch', { name: '切换记忆 2 启用状态' }))
    const modal = await screen.findByRole('dialog', {
      name: '停用这条记忆的供给？'
    })
    fireEvent.click(within(modal).getByRole('button', { name: '停用供给' }))
    await within(modal).findByText(/操作结果未确认或状态已变化/)
    expect(
      within(modal).getByRole('button', { name: '停用供给' })
    ).toBeDisabled()
    expect(
      screen.getByRole('switch', { name: '切换记忆 2 启用状态' })
    ).toBeChecked()
    expect(memoryApi.setNativeStatus).toHaveBeenCalledTimes(1)
  })
  it('shows unknown statistics on read failure instead of zero memories', async () => {
    vi.mocked(memoryApi.nativeMessages).mockRejectedValue(
      new Error('unavailable')
    )
    mount()
    await waitFor(() => {
      expect(
        screen.getByRole('region', { name: '当前账户记忆统计' })
      ).toHaveTextContent('—')
    })
    expect(
      screen.queryByText('暂无记忆，完成聊天后可刷新查看自动提取进度。')
    ).not.toBeInTheDocument()
  })
})

describe('native retrieval diagnostic honesty', () => {
  it('uses pure native retrieval with the selected real limit and no ineffective tuning controls', async () => {
    vi.mocked(memoryApi.nativeSearch).mockResolvedValue({
      messages: [rows[1]],
      status: 'ready',
      options: { limit: 3, similarityThreshold: 0.2, keywordWeight: 0.7 }
    })
    mount('active', true)
    await screen.findByText('raw actual source content')
    fireEvent.click(screen.getByRole('tab', { name: '召回诊断' }))
    fireEvent.click(
      screen.getByText('原生检索', { selector: '.ant-segmented-item-label' })
    )
    fireEvent.change(screen.getByRole('spinbutton', { name: '返回条数' }), {
      target: { value: '3' }
    })
    fireEvent.change(screen.getByRole('textbox', { name: '检索原生记忆' }), {
      target: { value: '茶' }
    })
    fireEvent.click(screen.getByRole('button', { name: '检索记忆' }))
    await waitFor(() => {
      expect(memoryApi.nativeSearch).toHaveBeenCalledWith({
        scope: subject.scope,
        query: '茶',
        limit: 3
      })
    })
    expect(memoryApi.nativeContext).not.toHaveBeenCalled()
    expect(
      screen.queryByRole('spinbutton', { name: /阈值/ })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('spinbutton', { name: /权重/ })
    ).not.toBeInTheDocument()
    fireEvent.click(screen.getByText('原生请求参数说明'))
    expect(
      await screen.findByText(/参数接收不等于当前引擎执行/)
    ).toBeInTheDocument()
  })
})
