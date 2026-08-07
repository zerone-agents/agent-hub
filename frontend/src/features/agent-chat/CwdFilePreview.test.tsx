import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import { ConfigProvider } from 'antd'
import { antdTheme } from '@/lib/antd-theme'
import CwdFilePreview from './CwdFilePreview'
import type { FileEntry } from '@/api/agent-files'

vi.mock('@/api/agent-files', () => ({
  agentFilesApi: {
    head: vi.fn(),
    getContent: vi.fn(),
  },
}))

import { agentFilesApi } from '@/api/agent-files'

function renderWith(ui: React.ReactElement) {
  return render(<ConfigProvider theme={antdTheme}>{ui}</ConfigProvider>)
}

function makeHeaders(obj: Record<string, string>): Headers {
  return new Headers(obj)
}

/**
 * Build a mock Response object covering the surface the component touches:
 * `ok`, `headers`, `body.getReader()` (preferred) and `text()` fallback.
 * Blob is added for completeness though preview never reads blob for text.
 */
function mockGetContent(text: string, mime = 'text/plain', extra?: Record<string, string>) {
  const headers = makeHeaders({ 'content-type': mime, ...extra })
  const encoder = new TextEncoder()
  const bytes = encoder.encode(text)
  // A simple reader that yields the whole payload once.
  let done = false
  const reader = {
    read: async () => {
      if (done) return { done: true, value: undefined }
      done = true
      return { done: false, value: bytes as unknown as Uint8Array }
    },
    cancel: vi.fn().mockResolvedValue(undefined),
  }
  const body = { getReader: () => reader }
  return {
    ok: true,
    status: 200,
    headers,
    body,
    text: async () => text,
    blob: async () => new Blob([bytes]),
  } as unknown as Response
}

function mockHead(contentLength: number, mime = 'text/plain', extra?: Record<string, string>) {
  return Promise.resolve({
    ok: true,
    status: 200,
    headers: makeHeaders({
      'content-length': String(contentLength),
      'content-type': mime,
      ...(extra ?? {}),
    }),
  } as unknown as Response)
}

function fileEntry(over: Partial<FileEntry> = {}): FileEntry {
  return {
    name: 'README.md',
    type: 'file',
    size: 11,
    mime: 'text/plain',
    ...over,
  }
}

describe('CwdFilePreview', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    ;(agentFilesApi.head as any).mockReset?.()
    ;(agentFilesApi.getContent as any).mockReset?.()
  })

  it('renders 选择文件预览 placeholder when no file selected', () => {
    renderWith(<CwdFilePreview agentName="x" selectedFile={null} />)
    expect(screen.getByText('选择文件预览')).toBeInTheDocument()
  })

  it('fetches and renders plain text content', async () => {
    ;(agentFilesApi.head as any).mockImplementation(() => mockHead(11))
    ;(agentFilesApi.getContent as any).mockImplementation(() =>
      Promise.resolve(mockGetContent('hello world'))
    )

    renderWith(<CwdFilePreview agentName="x" selectedFile={fileEntry()} />)

    await waitFor(() => {
      expect(screen.getByText(/hello world/)).toBeInTheDocument()
    })
  })

  it('prettifies JSON content for json mime', async () => {
    ;(agentFilesApi.head as any).mockImplementation(() => mockHead(20, 'application/json'))
    ;(agentFilesApi.getContent as any).mockImplementation(() =>
      Promise.resolve(
        mockGetContent(JSON.stringify({ a: 1, b: [1, 2] }), 'application/json')
      )
    )

    renderWith(
      <CwdFilePreview
        agentName="x"
        selectedFile={fileEntry({ name: 'package.json', mime: 'application/json' })}
      />
    )

    await waitFor(() => {
      // Prettified JSON has newlines + 2-space indent.
      expect(screen.getByText(/"a": 1/)).toBeInTheDocument()
      expect(screen.getByText(/"b": \[/)).toBeInTheDocument()
    })
  })

  it('shows 文件较大 download-only notice for files > 512KB and does NOT GET', async () => {
    const big = 1024 * 1024
    ;(agentFilesApi.head as any).mockImplementation(() => mockHead(big))

    renderWith(
      <CwdFilePreview
        agentName="x"
        selectedFile={fileEntry({ name: 'big.bin', size: big, mime: 'application/octet-stream' })}
      />
    )

    await waitFor(() => {
      expect(screen.getByText(/文件较大/)).toBeInTheDocument()
    })
    expect(agentFilesApi.getContent).not.toHaveBeenCalled()
  })

  it('shows truncated notice when content arrives over the cap', async () => {
    // HEAD says < cap so GET fires; but streamed body itself exceeds cap.
    ;(agentFilesApi.head as any).mockImplementation(() => mockHead(11))
    // Build a body over 512KB so the stream-read loop hits the cap.
    const huge = 'a'.repeat(600 * 1024)
    ;(agentFilesApi.getContent as any).mockImplementation(() =>
      Promise.resolve(mockGetContent(huge))
    )

    renderWith(<CwdFilePreview agentName="x" selectedFile={fileEntry()} />)

    await waitFor(() => {
      expect(screen.getByText(/已截断/)).toBeInTheDocument()
    })
  })

  it('renders image preview when mime is image/png (no <pre>)', async () => {
    ;(agentFilesApi.head as any).mockImplementation(() => mockHead(1024, 'image/png'))
    ;(agentFilesApi.getContent as any).mockImplementation(() => {
      const headers = makeHeaders({ 'content-type': 'image/png' })
      const blob = new Blob([new Uint8Array([0, 1, 2])], { type: 'image/png' })
      return Promise.resolve({
        ok: true,
        status: 200,
        headers,
        body: undefined,
        blob: async () => blob,
        text: async () => '',
      } as unknown as Response)
    })

    const { container } = renderWith(
      <CwdFilePreview
        agentName="x"
        selectedFile={fileEntry({ name: 'logo.png', mime: 'image/png', size: 1024 })}
      />
    )

    await waitFor(() => {
      expect(container.querySelector('img')).not.toBeNull()
    })
    expect(container.querySelector('pre')).toBeNull()
  })

  it('renders 不支持预览 notice for unknown/binary mime (application/octet-stream)', async () => {
    ;(agentFilesApi.head as any).mockImplementation(() =>
      mockHead(128, 'application/octet-stream')
    )
    ;(agentFilesApi.getContent as any).mockImplementation(() =>
      Promise.resolve(mockGetContent('\x00\x01\x02', 'application/octet-stream'))
    )

    renderWith(
      <CwdFilePreview
        agentName="x"
        selectedFile={fileEntry({
          name: 'blob.bin',
          size: 128,
          mime: 'application/octet-stream',
        })}
      />
    )

    await waitFor(() => {
      expect(screen.getByText(/不支持预览/)).toBeInTheDocument()
    })
  })

  it('renders PDF in <object> for application/pdf', async () => {
    ;(agentFilesApi.head as any).mockImplementation(() => mockHead(2048, 'application/pdf'))
    ;(agentFilesApi.getContent as any).mockImplementation(() => {
      const headers = makeHeaders({ 'content-type': 'application/pdf' })
      const blob = new Blob([new Uint8Array([0, 1, 2])], { type: 'application/pdf' })
      return Promise.resolve({
        ok: true,
        status: 200,
        headers,
        body: undefined,
        blob: async () => blob,
        text: async () => '',
      } as unknown as Response)
    })

    const { container } = renderWith(
      <CwdFilePreview
        agentName="x"
        selectedFile={fileEntry({ name: 'doc.pdf', mime: 'application/pdf', size: 2048 })}
      />
    )

    await waitFor(() => {
      expect(container.querySelector('object')).not.toBeNull()
    })
  })

  it('does NOT render HTML as live DOM', async () => {
    const html = '<script>alert("xss")</script><p>hello</p>'
    ;(agentFilesApi.head as any).mockImplementation(() => mockHead(html.length, 'text/html'))
    ;(agentFilesApi.getContent as any).mockImplementation(() =>
      Promise.resolve(mockGetContent(html, 'text/html'))
    )

    const { container } = renderWith(
      <CwdFilePreview
        agentName="x"
        selectedFile={fileEntry({ name: 'page.html', mime: 'text/html' })}
      />
    )

    await waitFor(() => {
      expect(screen.getByText(/hello/)).toBeInTheDocument()
    })
    // The script tag must never appear as a live DOM node.
    expect(container.querySelector('script')).toBeNull()
  })

  it('clicking 下载 fetches blob and triggers synthetic anchor click', async () => {
    ;(agentFilesApi.head as any).mockImplementation(() =>
      mockHead(1024 * 1024, 'application/octet-stream', {
        'content-disposition': "attachment; filename=\"big.bin\"",
      })
    )
    ;(agentFilesApi.getContent as any).mockResolvedValue({
      ok: true,
      status: 200,
      headers: new Headers({ 'content-disposition': "attachment; filename=\"big.bin\"" }),
      blob: async () => new Blob(['fake-bytes']),
    })

    const createObjectURLSpy = vi
      .spyOn(URL, 'createObjectURL')
      .mockReturnValue('blob:fake-url')
    const revokeObjectURLSpy = vi
      .spyOn(URL, 'revokeObjectURL')
      .mockImplementation(() => {})
    const anchorClickSpy = vi
      .spyOn(HTMLAnchorElement.prototype, 'click')
      .mockImplementation(() => {})

    renderWith(
      <CwdFilePreview
        agentName="x"
        selectedFile={fileEntry({ name: 'big.bin', size: 1024 * 1024, mime: 'application/octet-stream' })}
      />
    )

    await waitFor(() => {
      expect(screen.getByText(/文件较大/)).toBeInTheDocument()
    })

    // The download trigger is a <button>, not a native <a href>. Clicking it
    // must invoke the fetch+blob pipeline, not browser-native navigation.
    const btn = screen.getByRole('button', { name: /下载/ })
    fireEvent.click(btn)

    await waitFor(() => {
      expect(agentFilesApi.getContent).toHaveBeenCalled()
    })
    expect(createObjectURLSpy).toHaveBeenCalled()
    expect(anchorClickSpy).toHaveBeenCalled()
    expect(revokeObjectURLSpy).toHaveBeenCalled()

    createObjectURLSpy.mockRestore()
    revokeObjectURLSpy.mockRestore()
    anchorClickSpy.mockRestore()
  })
})
