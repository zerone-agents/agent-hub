import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ConfigProvider } from 'antd'
import { antdTheme } from '@/lib/antd-theme'
import EntityCard from './EntityCard'

describe('EntityCard', () => {
  it('renders title, subtitle, description, footer and default first-letter icon', () => {
    render(
      <ConfigProvider theme={antdTheme}>
        <EntityCard
          title="执行命令"
          subtitle="Bash"
          description="在持久化的 shell 会话中执行 bash 命令"
          footerLeft="27 分钟前"
          footerRight={<button>edit</button>}
        />
      </ConfigProvider>
    )

    expect(screen.getByText('执行命令')).toBeInTheDocument()
    expect(screen.getByText('Bash')).toBeInTheDocument()
    expect(screen.getByText('在持久化的 shell 会话中执行 bash 命令')).toBeInTheDocument()
    expect(screen.getByText('27 分钟前')).toBeInTheDocument()
    expect(screen.getByText('edit')).toBeInTheDocument()
    expect(screen.getByText('执')).toBeInTheDocument()
  })

  it('renders custom icon and header extra', () => {
    render(
      <ConfigProvider theme={antdTheme}>
        <EntityCard
          icon={<span data-testid="custom-icon">🔌</span>}
          title="Git MCP"
          subtitle="git-mcp"
          headerExtra={<span data-testid="tag">STDIO</span>}
        />
      </ConfigProvider>
    )

    expect(screen.getByTestId('custom-icon')).toBeInTheDocument()
    expect(screen.getByTestId('tag')).toBeInTheDocument()
  })
})
