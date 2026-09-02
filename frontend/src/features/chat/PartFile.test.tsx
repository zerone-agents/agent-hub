import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import PartFile from './PartFile'

vi.mock('@/api/agent-chat', () => ({
  authFetchBlob: vi.fn(),
}))
import { authFetchBlob } from '@/api/agent-chat'

const filePart = {
  type: 'file',
  id: 'f-1',
  name: 'chart.png',
  mime: 'image/png',
  size: 2048,
  path: '.zerone-uploads/chart.png',
}

beforeEach(() => {
  vi.mocked(authFetchBlob).mockReset()
})

describe('PartFile', () => {
  it('renders an image from an authed blob fetch', async () => {
    vi.mocked(authFetchBlob).mockResolvedValue(new Blob(['img'], { type: 'image/png' }))
    render(<PartFile part={filePart} buildAttachmentUrl={(p) => `/proxy?path=${p}`} />)
    const img = await waitFor(() => screen.getByRole('img', { name: 'chart.png' }))
    expect(img.getAttribute('src')).toMatch(/^blob:/)
    expect(authFetchBlob).toHaveBeenCalledWith('/proxy?path=.zerone-uploads/chart.png')
  })

  it('shows unavailable state when the blob fetch fails (runtime rebuilt)', async () => {
    vi.mocked(authFetchBlob).mockRejectedValue(new Error('HTTP 404'))
    render(<PartFile part={filePart} buildAttachmentUrl={(p) => `/proxy?path=${p}`} />)
    await waitFor(() => expect(screen.getByText(/临时文件已不可用/)).toBeInTheDocument())
    expect(screen.getByText(/chart.png/)).toBeInTheDocument() // 元数据仍可见
  })

  it('renders a metadata card for plain files with a download action', () => {
    vi.mocked(authFetchBlob).mockResolvedValue(new Blob(['x']))
    render(
      <PartFile
        part={{ ...filePart, name: 'report.pdf', mime: 'application/pdf' }}
        buildAttachmentUrl={(p) => `/proxy?path=${p}`}
      />
    )
    expect(screen.getByText('report.pdf')).toBeInTheDocument()
    expect(screen.getByText('2.0 KB')).toBeInTheDocument()
    expect(screen.getByTitle('下载')).toBeInTheDocument()
  })

  it('renders metadata only when no builder is injected', () => {
    render(<PartFile part={{ ...filePart, name: 'doc.pdf', mime: 'application/pdf' }} />)
    expect(screen.getByText('doc.pdf')).toBeInTheDocument()
    expect(screen.queryByTitle('下载')).not.toBeInTheDocument()
    expect(screen.queryByText(/临时文件已不可用/)).not.toBeInTheDocument()
  })
})
