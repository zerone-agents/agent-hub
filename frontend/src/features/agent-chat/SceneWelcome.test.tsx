import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { ConfigProvider } from 'antd'
import { antdTheme } from '@/lib/antd-theme'
import SceneWelcome from './SceneWelcome'
import type { Scene } from '@/api/scenes'

const mockScenes: Scene[] = [
  { id: 1, name: 'alpha', agent: 'demo', title: 'Alpha 场景', titleEn: '', prompt: 'You are an alpha assistant.', promptEn: '', enabled: true, createdAt: '', updatedAt: '' },
  { id: 2, name: 'beta', agent: 'demo', title: 'Beta 场景', titleEn: '', prompt: 'You are a beta assistant. '.repeat(20), promptEn: '', enabled: true, createdAt: '', updatedAt: '' }
]

vi.mock('@/queries/useScenes', () => ({
  useAgentScenes: vi.fn()
}))

import { useAgentScenes } from '@/queries/useScenes'

function renderWith(ui: React.ReactElement) {
  return render(<ConfigProvider theme={antdTheme}>{ui}</ConfigProvider>)
}

describe('SceneWelcome', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders Spin while scenes are loading', () => {
    ;(useAgentScenes as any).mockReturnValue({ data: undefined, isLoading: true })
    renderWith(<SceneWelcome agentName="demo" onPick={() => {}} />)
    expect(document.querySelector('.ant-spin')).toBeInTheDocument()
  })

  it('renders the minimal hint when there are no scenes', async () => {
    ;(useAgentScenes as any).mockReturnValue({ data: [], isLoading: false })
    renderWith(<SceneWelcome agentName="demo" onPick={() => {}} />)
    expect(screen.getByText('直接输入消息开始对话')).toBeInTheDocument()
    expect(screen.queryByText('你可以试试以下场景：')).not.toBeInTheDocument()
  })

  it('renders grid with header, footer, and one card per scene', () => {
    ;(useAgentScenes as any).mockReturnValue({ data: mockScenes, isLoading: false })
    renderWith(<SceneWelcome agentName="demo" onPick={() => {}} />)
    expect(screen.getByText('你可以试试以下场景：')).toBeInTheDocument()
    expect(screen.getByText('（也可直接在下方输入）')).toBeInTheDocument()
    expect(screen.getByText('Alpha 场景')).toBeInTheDocument()
    expect(screen.getByText('Beta 场景')).toBeInTheDocument()
  })

  it('invokes onPick with the scene when a card is clicked', () => {
    const onPick = vi.fn()
    ;(useAgentScenes as any).mockReturnValue({ data: mockScenes, isLoading: false })
    renderWith(<SceneWelcome agentName="demo" onPick={onPick} />)
    fireEvent.click(screen.getByText('Alpha 场景'))
    expect(onPick).toHaveBeenCalledOnce()
    expect(onPick).toHaveBeenCalledWith(mockScenes[0])
  })

  it('does not invoke onPick when disabled', () => {
    const onPick = vi.fn()
    ;(useAgentScenes as any).mockReturnValue({ data: mockScenes, isLoading: false })
    renderWith(<SceneWelcome agentName="demo" onPick={onPick} disabled />)
    fireEvent.click(screen.getByText('Alpha 场景'))
    expect(onPick).not.toHaveBeenCalled()
  })
})
