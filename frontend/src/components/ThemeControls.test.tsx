import { describe, expect, it } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import ThemeControls from '@/components/ThemeControls'

/**
 * PR #168 review 回归：第二个 Dropdown（明暗模式）曾把 key 化后的
 * appearanceOptions 原样传给 antd Menu，渲染裸 key 字符串。
 * 此处断言两个 Dropdown 的菜单 label 均为实际文案而非 i18n key。
 */
describe('ThemeControls', () => {
  it('appearance dropdown renders translated labels, not raw i18n keys', async () => {
    render(<ThemeControls />)
    fireEvent.click(screen.getByRole('button', { name: '切换明暗模式' }))
    expect(await screen.findByText('浅色')).toBeInTheDocument()
    expect(screen.getByText('深色')).toBeInTheDocument()
    expect(screen.getByText('跟随系统')).toBeInTheDocument()
    expect(screen.queryByText(/components\.themeControls\./)).not.toBeInTheDocument()
  })

  it('color-scheme dropdown renders theme labels without raw i18n keys', async () => {
    render(<ThemeControls />)
    fireEvent.click(screen.getByRole('button', { name: '切换配色' }))
    expect(await screen.findByRole('menu')).toBeInTheDocument()
    expect(screen.queryByText(/components\.themeControls\./)).not.toBeInTheDocument()
  })
})
