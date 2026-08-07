import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import AigcHint from './AigcHint'

describe('AigcHint', () => {
  it('renders the fixed AI-content notice', () => {
    render(<AigcHint />)
    expect(screen.getByText('内容由 AI 生成，请仔细甄别')).toBeInTheDocument()
  })
})
