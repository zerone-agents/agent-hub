import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Routes, Route } from 'react-router'
import { ConfigProvider } from 'antd'
import { antdTheme } from '@/lib/antd-theme'
import { setAuthRole } from '@/test/auth-store-mock'
import KnowledgeRetrievalPage from './KnowledgeRetrievalPage'

const h = vi.hoisted(() => ({
  retrievalMock: vi.fn(),
  data: undefined as unknown
}))

vi.mock('@/queries/useKnowledge', () => ({
  useRetrievalTest: () => ({ mutate: h.retrievalMock, isPending: false, data: h.data })
}))

vi.mock('@/stores/auth', async () => (await import('@/test/auth-store-mock')).createAuthStoreMock())

const sampleResult = {
  total: 1,
  chunks: [
    {
      id: 'c1',
      content: 'matched chunk text',
      document_id: 'd1',
      document_name: 'guide.pdf',
      similarity: 0.873,
      vector_similarity: 0.9,
      term_similarity: 0.8
    }
  ],
  doc_aggs: [],
  labels: {}
}

function renderPage() {
  return render(
    <ConfigProvider theme={antdTheme}>
      <MemoryRouter initialEntries={['/knowledge/kb1/retrieval']}>
        <Routes>
          <Route path="/knowledge/:id/retrieval" element={<KnowledgeRetrievalPage />} />
        </Routes>
      </MemoryRouter>
    </ConfigProvider>
  )
}

describe('KnowledgeRetrievalPage', () => {
  beforeEach(() => {
    setAuthRole('admin')
    h.retrievalMock.mockReset()
    h.data = undefined
  })

  it('submits the retrieval form with the dataset id', async () => {
    const user = userEvent.setup()
    renderPage()

    await user.type(screen.getByRole('textbox'), '如何退货？')
    await user.click(screen.getByRole('button', { name: /检索测试/ }))

    await waitFor(() => { expect(h.retrievalMock).toHaveBeenCalled(); })
    expect(h.retrievalMock).toHaveBeenCalledWith(
      expect.objectContaining({ question: '如何退货？', dataset_ids: ['kb1'] })
    )
  })

  it('renders retrieval results', () => {
    h.data = sampleResult
    renderPage()
    expect(screen.getByText('共召回 1 条分块')).toBeInTheDocument()
    expect(screen.getByText('matched chunk text')).toBeInTheDocument()
    expect(screen.getByText('guide.pdf')).toBeInTheDocument()
    expect(screen.getByText(/相似度 0.873/)).toBeInTheDocument()
  })

  it('member: hides the retrieval test submit button but keeps results visible', () => {
    setAuthRole('member')
    h.data = sampleResult
    renderPage()

    // 写操作入口（检索测试）隐藏；已渲染的检索结果区不受影响
    expect(screen.queryByRole('button', { name: /检索测试/ })).not.toBeInTheDocument()
    expect(screen.getByText('共召回 1 条分块')).toBeInTheDocument()
    expect(screen.getByText('matched chunk text')).toBeInTheDocument()
  })
})
