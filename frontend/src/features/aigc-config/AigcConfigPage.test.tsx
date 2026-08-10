import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { ConfigProvider } from 'antd'
import { antdTheme } from '@/lib/antd-theme'
import AigcConfigPage from './AigcConfigPage'

vi.mock('@/queries/useAigcConfig', () => ({
  useAigcConfig: () => ({
    data: {
      configured: true,
      uscc: '91320118MAK93FC72D',
      companyName: '南京测试科技有限公司',
      contentProducer: '001191320118MAK93FC72D10000',
      signingKeyConfigured: true
    },
    isLoading: false
  }),
  useSaveAigcConfig: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useRotateAigcKey: () => ({ mutate: vi.fn(), isPending: false }),
  useClearAigcConfig: () => ({ mutate: vi.fn(), isPending: false })
}))

const Wrapper = ({ children }: { children: React.ReactNode }) => (
  <ConfigProvider theme={antdTheme}>
    <MemoryRouter>{children}</MemoryRouter>
  </ConfigProvider>
)

describe('AigcConfigPage', () => {
  it('renders title and form fields', () => {
    render(<AigcConfigPage />, { wrapper: Wrapper })
    expect(screen.getByText('AIGC 标识配置')).toBeInTheDocument()
    expect(screen.getByText('统一社会信用代码')).toBeInTheDocument()
    expect(screen.getByText('公司完整名称')).toBeInTheDocument()
  })

  it('shows derived contentProducer and key status when configured', () => {
    render(<AigcConfigPage />, { wrapper: Wrapper })
    expect(screen.getByText('001191320118MAK93FC72D10000')).toBeInTheDocument()
    expect(screen.getByText(/已配置（由后端保管）/)).toBeInTheDocument()
  })

  it('renders rotate and clear actions', () => {
    render(<AigcConfigPage />, { wrapper: Wrapper })
    expect(screen.getByText('重新生成密钥')).toBeInTheDocument()
    expect(screen.getByText('清除配置')).toBeInTheDocument()
  })

  it('renders link to model management instead of modelCodes dump', () => {
    render(<AigcConfigPage />, { wrapper: Wrapper })
    const pattern = /模型 AIGC 码在[\s\S]*模型管理[\s\S]*自动分配/
    expect(
      screen.getByText((_, node) => {
         
        if (!node?.textContent || !pattern.test(node.textContent)) return false
        // pick the smallest matching element (no descendant also matches)
        return !Array.from(node.querySelectorAll('*')).some((c) =>
          pattern.test(c.textContent)
        )
      })
    ).toBeInTheDocument()
    expect(screen.queryByText('模型码映射')).not.toBeInTheDocument()
  })
})
