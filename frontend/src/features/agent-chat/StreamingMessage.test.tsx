import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import StreamingMessage from './StreamingMessage'

describe('StreamingMessage retry notice', () => {
  it('shows attempt count, error type and delay while the runtime is retrying', () => {
    render(
      <StreamingMessage
        parts={[]}
        phase="streaming"
        retry={{ attempt: 2, errorType: 'rate_limit', delayMs: 5646.997 }}
      />
    )
    expect(screen.getByText(/rate_limit/)).toBeInTheDocument()
    expect(screen.getByText(/5\.6 秒/)).toBeInTheDocument()
    expect(screen.getByText(/第 2 次/)).toBeInTheDocument()
  })
})
