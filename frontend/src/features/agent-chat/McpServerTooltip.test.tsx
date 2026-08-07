import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ConfigProvider } from 'antd'
import { antdTheme } from '@/lib/antd-theme'
import { McpServerTooltipOverlay } from './McpServerTooltip'
import type { McpServerSummary } from '@/api/agents'

function renderWith(ui: React.ReactElement) {
  return render(<ConfigProvider theme={antdTheme}>{ui}</ConfigProvider>)
}

describe('McpServerTooltipOverlay', () => {
  it('renders stdio server with command/args/env', () => {
    const server: McpServerSummary = {
      transport: 'stdio',
      command: 'mcp-server-github',
      args: ['--owner', 'myorg'],
      env: { GITHUB_TOKEN: '***', ZERONE_AGENT_API_KEY: '***' },
    }
    renderWith(<McpServerTooltipOverlay name="github" server={server} />)

    expect(screen.getByText('github')).toBeInTheDocument()
    expect(screen.getByText('stdio')).toBeInTheDocument()
    expect(screen.getByText('mcp-server-github')).toBeInTheDocument()
    expect(screen.getByText('--owner myorg')).toBeInTheDocument()
    // env keys shown with redacted values
    expect(screen.getByText('GITHUB_TOKEN')).toBeInTheDocument()
    expect(screen.getAllByText('***')).toHaveLength(2)
  })

  it('renders sse server with url/headers', () => {
    const server: McpServerSummary = {
      transport: 'sse',
      url: 'https://mcp.example.com/sse',
      headers: { Authorization: '***' },
    }
    renderWith(<McpServerTooltipOverlay name="remote" server={server} />)

    expect(screen.getByText('remote')).toBeInTheDocument()
    expect(screen.getByText('sse')).toBeInTheDocument()
    expect(screen.getByText('https://mcp.example.com/sse')).toBeInTheDocument()
    expect(screen.getByText('Authorization')).toBeInTheDocument()
    expect(screen.getByText('***')).toBeInTheDocument()
  })

  it('omits env section when env is missing on stdio server', () => {
    const server: McpServerSummary = {
      transport: 'stdio',
      command: 'simple-server',
    }
    renderWith(<McpServerTooltipOverlay name="simple" server={server} />)

    expect(screen.getByText('simple')).toBeInTheDocument()
    expect(screen.getByText('simple-server')).toBeInTheDocument()
    // No env keys should be rendered
    expect(screen.queryByText('env')).not.toBeInTheDocument()
  })

  it('joins multiple args with spaces', () => {
    const server: McpServerSummary = {
      transport: 'stdio',
      command: 'srv',
      args: ['--port=8080', '--verbose', '--log=debug'],
    }
    renderWith(<McpServerTooltipOverlay name="srv" server={server} />)

    expect(screen.getByText('--port=8080 --verbose --log=debug')).toBeInTheDocument()
  })
})
