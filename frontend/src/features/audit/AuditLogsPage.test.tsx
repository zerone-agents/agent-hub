import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, fireEvent, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { ConfigProvider } from 'antd'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { antdTheme } from '@/lib/antd-theme'
import AuditLogsPage from './AuditLogsPage'
import { auditApi, type AuditLog, type AuditLogQuery } from '@/api/audit'

vi.mock('@/api/audit', () => ({
  auditApi: { listLogs: vi.fn() }
}))

const mocked = vi.mocked(auditApi.listLogs)

function mkItem(id: string, action: string, status: 'success' | 'failure' | 'partial'): AuditLog {
  return {
    id, tenantId: 'default', userId: '7', userName: 'alice', category: 'user',
    action, targetType: 'user', targetId: '2', targetName: 'bob', status,
    detail: { field: 'role' }, remoteIp: '127.0.0.1', userAgent: 'ua', createdAt: '2026-09-10T12:00:00Z'
  }
}

function renderPage() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <ConfigProvider theme={antdTheme}>
      <QueryClientProvider client={qc}>
        <MemoryRouter>
          <AuditLogsPage />
        </MemoryRouter>
      </QueryClientProvider>
    </ConfigProvider>
  )
}

// total=25 > PAGE_SIZE(20)，分页器才有第 2 页按钮（翻页断言的前提）。
beforeEach(() => {
  mocked.mockReset()
  mocked.mockResolvedValue({ items: [mkItem('1', 'user.update_role', 'success')], total: 25, snapshotId: '5' } as never)
})

describe('AuditLogsPage', () => {
  it('渲染列表：动作、用户、partial 状态 Tag', async () => {
    // snapshotId 进 queryKey：首屏会连发两次请求（无快照 → 存 '9' → 带快照重查），
    // 用 mockResolvedValue 让两次返回同一份数据，避免第二次覆盖掉断言内容。
    mocked.mockResolvedValue({
      items: [mkItem('1', 'agent.deploy', 'success'), mkItem('2', 'user.update_role', 'partial')],
      total: 2, snapshotId: '9'
    } as never)
    renderPage()
    expect(await screen.findByText('agent.deploy')).toBeInTheDocument()
    expect(screen.getByText('部分生效')).toBeInTheDocument() // partial 橙 Tag
  })

  it('翻页按 string 原样回传服务端 snapshotId，不转 number', async () => {
    renderPage()
    await screen.findByText('user.update_role')
    // antd v6 页码项是 <li title="2"><a>2</a></li>（非 button）；「对象」列的
    // <span title={targetId}> 也可能等于 '2'，须在分页器范围内查找。
    const pagination = document.querySelector('.ant-pagination') as HTMLElement
    const page2 = within(pagination).getByTitle('2')
    await userEvent.setup().click(page2)
    await waitFor(() => {
      const call = mocked.mock.calls[mocked.mock.calls.length - 1][0]
      expect(call.snapshotId).toBe('5')
      expect(typeof call.snapshotId).toBe('string')
      expect(call.page).toBe(2)
    })
  })

  it('category 筛选触发 refetch（沿用同一 snapshotId）', async () => {
    renderPage()
    await screen.findByText('user.update_role')
    // antd v6 Select 无 .ant-select-selector；触发器是 role=combobox 的 input（同 UsersPage.test）
    fireEvent.mouseDown(screen.getByRole('combobox'))
    const option = await screen.findByText('agent', { selector: '.ant-select-item-option-content' })
    fireEvent.click(option)
    await waitFor(() => {
      const call = mocked.mock.calls[mocked.mock.calls.length - 1][0]
      expect(call.category).toBe('agent')
      expect(call.snapshotId).toBe('5')
    })
  })

  it('行展开渲染 detail 对象', async () => {
    renderPage()
    await screen.findByText('user.update_role')
    const expandBtn = document.querySelector('.ant-table-row-expand-icon') as HTMLElement
    await userEvent.setup().click(expandBtn)
    await waitFor(() => expect(screen.getByText(/"field"/)).toBeInTheDocument())
  })

  // PR #150 二轮审查 P2：刷新必须真正重开快照——旧缓存回放旧 snapshotId 会使
  // 刷新沦为空转、新记录不可见。
  it('刷新重开快照：无 snapshotId 重新请求、捕获新快照、新记录可见', async () => {
    const user = userEvent.setup()
    let reopenCalls = 0 // 不带 snapshotId 的首屏/重开请求计数
    mocked.mockImplementation(async (params: AuditLogQuery) => {
      if (params.snapshotId === undefined) {
        reopenCalls++
        if (reopenCalls === 1) {
          // 初始快照 5：只有旧记录
          return { items: [mkItem('1', 'user.update_role', 'success')], total: 1, snapshotId: '5' }
        }
        // 刷新后的重开请求：服务端已写入新记录，快照推进到 6
        return {
          items: [mkItem('1', 'user.update_role', 'success'), mkItem('2', 'agent.deploy', 'success')],
          total: 2, snapshotId: '6'
        }
      }
      // 会话内续传：echo 当前快照内容
      const items = params.snapshotId === '6'
        ? [mkItem('1', 'user.update_role', 'success'), mkItem('2', 'agent.deploy', 'success')]
        : [mkItem('1', 'user.update_role', 'success')]
      return { items, total: items.length, snapshotId: params.snapshotId ?? '5' }
    })

    renderPage()
    expect(await screen.findByText('user.update_role')).toBeInTheDocument()
    expect(screen.queryByText('agent.deploy')).not.toBeInTheDocument() // 旧快照内无新记录

    const undefinedBefore = mocked.mock.calls.filter(([p]) => p.snapshotId === undefined).length
    await user.click(screen.getByRole('button', { name: /刷新/ }))

    // 1) 刷新后必须发起一次不带 snapshotId 的重开请求
    await waitFor(() => {
      expect(mocked.mock.calls.filter(([p]) => p.snapshotId === undefined).length)
        .toBeGreaterThan(undefinedBefore)
    })
    // 2) 新记录最终可见（缺陷情形：旧快照 5 被缓存回放写回，新记录永不出现）
    expect(await screen.findByText('agent.deploy')).toBeInTheDocument()
    // 3) 会话最终携带新快照 6（重开捕获后的续传请求）
    await waitFor(() => {
      const last = mocked.mock.calls[mocked.mock.calls.length - 1][0]
      expect(last.snapshotId).toBe('6')
    })
  })
})
