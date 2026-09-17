import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it } from 'vitest'
import LanguageSwitch from './LanguageSwitch'
import { setAppLanguage } from '@/i18n'

describe('LanguageSwitch', () => {
  beforeEach(() => {
    setAppLanguage('zh')
  })

  it('渲染切换按钮（可访问名=切换语言）', () => {
    render(<LanguageSwitch />)
    expect(screen.getByRole('button', { name: '切换语言' })).toBeInTheDocument()
  })

  it('点击展开菜单并可切换到 English（html lang 与文案联动）', async () => {
    const user = userEvent.setup()
    render(<LanguageSwitch />)
    await user.click(screen.getByRole('button', { name: '切换语言' }))
    await user.click(screen.getByRole('menuitem', { name: 'English' }))
    expect(document.documentElement.lang).toBe('en')
    // 切英文后按钮的可访问名也随语言变
    expect(
      screen.getByRole('button', { name: 'Switch language' })
    ).toBeInTheDocument()
    // 还原，避免影响后续用例
    setAppLanguage('zh')
  })
})
