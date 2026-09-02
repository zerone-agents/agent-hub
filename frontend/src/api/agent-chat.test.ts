import { describe, it, expect, vi, beforeEach } from 'vitest'
import { agentChatApi, attachmentContentUrl } from './agent-chat'

const fetchMock = vi.fn()
vi.stubGlobal('fetch', fetchMock)

describe('agentChatApi.uploadFiles', () => {
  beforeEach(() => {
    fetchMock.mockReset()
    localStorage.setItem('access_token', 'tok')
  })

  it('posts FormData with bearer and returns descriptors', async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          success: true,
          data: { files: [{ id: '1', name: 'a.pdf', mime: 'application/pdf', size: 3, path: '.zerone-uploads/a.pdf' }] },
        }),
        { status: 201 }
      )
    )
    const files = await agentChatApi.uploadFiles('min', 's1', [
      new File(['abc'], 'a.pdf', { type: 'application/pdf' }),
    ])
    expect(files).toHaveLength(1)
    expect(files[0]?.path).toBe('.zerone-uploads/a.pdf')
    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect((init.headers as Record<string, string>).Authorization).toBe('Bearer tok')
    expect(init.body).toBeInstanceOf(FormData)
  })

  it('throws an error carrying the backend code (413 limit)', async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(
        JSON.stringify({ success: false, error: 'too many files', code: 'upload_limit_exceeded' }),
        { status: 413 }
      )
    )
    await expect(
      agentChatApi.uploadFiles('min', 's1', [new File(['x'], 'a.txt')])
    ).rejects.toMatchObject({ code: 'upload_limit_exceeded', status: 413 })
  })
})

describe('attachmentContentUrl', () => {
  it('encodes path and ids', () => {
    expect(attachmentContentUrl('my agent', 's 1', '.zerone-uploads/a b.png')).toBe(
      '/api/v1/agents/my%20agent/chat/sessions/s%201/attachments/content?path=.zerone-uploads%2Fa%20b.png'
    )
  })
})