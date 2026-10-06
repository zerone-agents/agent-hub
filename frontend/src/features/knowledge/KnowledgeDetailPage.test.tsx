import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router'
import KnowledgeDetailPage from './KnowledgeDetailPage'

const h = vi.hoisted(() => ({ data: undefined as Record<string, unknown> | undefined, loading: false, error: false, refetch: vi.fn() }))
vi.mock('@/queries/useKnowledge', () => ({ useKnowledgeDetail: () => ({ data: h.data, isLoading: h.loading, isError: h.error, refetch: h.refetch }) }))
function show() {
  return render(<MemoryRouter initialEntries={['/knowledge/kb1/documents']}><Routes><Route path="/knowledge/:id" element={<KnowledgeDetailPage />}><Route path="documents" element={<div>Document outlet</div>} /><Route path="ingestions" element={<div>Task outlet</div>} /></Route></Routes></MemoryRouter>)
}
describe('KnowledgeDetailPage', () => {
  beforeEach(() => { h.data = undefined; h.loading = false; h.error = false; h.refetch.mockReset() })
  it('does not render a default name or child page after a failed read', async () => {
    h.error = true; show()
    expect(screen.getByText('知识库加载失败')).toBeInTheDocument()
    expect(screen.queryByRole('tab')).not.toBeInTheDocument()
    expect(screen.queryByText('Document outlet')).not.toBeInTheDocument()
    await userEvent.setup().click(screen.getByRole('button', { name: /重\s*试/ }))
    expect(h.refetch).toHaveBeenCalledTimes(1)
  })
  it('shows a loading state before child pages', () => {
    h.loading = true; show()
    expect(screen.getByRole('status', { name: '加载中...' })).toBeInTheDocument()
    expect(screen.queryByText('Document outlet')).not.toBeInTheDocument()
  })
  it('shows tasks as a first-level destination', async () => {
    h.data = { name: 'Actual dataset', doc_num: 0, chunk_num: 0 }; show()
    expect(screen.getByRole('heading', { name: 'Actual dataset' })).toBeInTheDocument()
    await userEvent.setup().click(screen.getByRole('tab', { name: '任务与日志' }))
    expect(screen.getByText('Task outlet')).toBeInTheDocument()
  })
})
