import {
  fireEvent,
  render,
  screen,
  waitFor,
  within
} from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { memoryApi, type MemoryApplicationReceipt } from '@/api/memory'
import CreateMemoryApplicationModal from './CreateMemoryApplicationModal'

afterEach(() => {
  vi.restoreAllMocks()
})

const active: MemoryApplicationReceipt = {
  application: {
    tenantId: 'default',
    applicationId: 'page-app',
    name: '页面测试应用',
    status: 'active',
    policyVersion: 'synthetic-v1',
    dataClass: 'synthetic'
  },
  applicationToken: 'secret-test-application-token'
}

function renderModal(empty = false) {
  vi.spyOn(memoryApi, 'applicationModels').mockResolvedValue({
    chat: empty ? [] : [{ fullId: '356', name: 'Chat', factory: 'Provider' }],
    embedding: empty
      ? []
      : [{ fullId: '337', name: 'Embedding', factory: 'Provider' }]
  })
  const onChanged = vi.fn()
  const onClose = vi.fn()
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } }
  })
  render(
    <QueryClientProvider client={client}>
      <CreateMemoryApplicationModal
        queryPrefix={['memory', 'test']}
        onChanged={onChanged}
        onClose={onClose}
      />
    </QueryClientProvider>
  )
  return { onChanged, onClose }
}

async function fillAndSubmit() {
  await waitFor(() =>
    expect(screen.getByRole('button', { name: '创建记忆应用' })).toBeEnabled()
  )
  fireEvent.change(screen.getByLabelText('应用名称'), {
    target: { value: '页面测试应用' }
  })
  fireEvent.change(screen.getByLabelText('应用标识'), {
    target: { value: 'page-app' }
  })
  fireEvent.mouseDown(screen.getByLabelText('记忆提取模型'))
  fireEvent.click(await screen.findByText('Chat · Provider (356)'))
  fireEvent.mouseDown(screen.getByLabelText('向量模型'))
  fireEvent.click(await screen.findByText('Embedding · Provider (337)'))
  fireEvent.click(screen.getByRole('button', { name: '创建记忆应用' }))
}

describe('memory application creation', () => {
  it('creates from selected models without a manager token and shows the application credential', async () => {
    const create = vi
      .spyOn(memoryApi, 'createApplication')
      .mockResolvedValue(active)
    const { onChanged, onClose } = renderModal()
    await fillAndSubmit()
    expect(await screen.findByText('记忆应用已创建')).toBeInTheDocument()
    expect(create).toHaveBeenCalledTimes(1)
    expect(create.mock.calls[0]?.[0]).toEqual({
      operationId: expect.any(String),
      applicationId: 'page-app',
      name: '页面测试应用',
      chatModel: '356',
      embeddingModel: '337'
    })
    expect(onChanged).toHaveBeenCalledWith(active.application)
    expect(screen.getByLabelText('应用令牌')).toHaveValue(
      active.applicationToken
    )
    fireEvent.click(screen.getByRole('button', { name: '已保存，完成' }))
    expect(onClose).toHaveBeenCalledOnce()
  })

  it('checks an uncertain creation with the identical operation and frozen model selection', async () => {
    const create = vi
      .spyOn(memoryApi, 'createApplication')
      .mockResolvedValueOnce({
        ...active,
        application: { ...active.application, status: 'unknown' }
      })
      .mockResolvedValueOnce(active)
    renderModal()
    await fillAndSubmit()
    expect(await screen.findByText('创建结果待确认')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '核对创建结果' }))
    expect(await screen.findByText('记忆应用已创建')).toBeInTheDocument()
    expect(create.mock.calls[1]?.[0]).toEqual(create.mock.calls[0]?.[0])
  })

  it('retains the same operation after a lost response and prevents duplicate submit while waiting', async () => {
    let reject!: (error: Error) => void
    const create = vi
      .spyOn(memoryApi, 'createApplication')
      .mockImplementationOnce(
        () =>
          new Promise((_, fail) => {
            reject = fail
          })
      )
      .mockResolvedValueOnce(active)
    renderModal()
    await fillAndSubmit()
    await waitFor(() => {
      expect(create).toHaveBeenCalledTimes(1)
    })
    fireEvent.click(screen.getByRole('button', { name: '核对创建结果' }))
    expect(create).toHaveBeenCalledTimes(1)
    reject(new Error('response lost'))
    expect(await screen.findByText('response lost')).toBeInTheDocument()
    expect(screen.getByLabelText('应用名称')).toBeDisabled()
    fireEvent.click(screen.getByRole('button', { name: '核对创建结果' }))
    expect(await screen.findByText('记忆应用已创建')).toBeInTheDocument()
    expect(create.mock.calls[1]?.[0]).toEqual(create.mock.calls[0]?.[0])
  })

  it('explains missing models and disables creation', async () => {
    const create = vi.spyOn(memoryApi, 'createApplication')
    renderModal(true)
    const dialog = screen.getByRole('dialog')
    expect(
      await screen.findByText(/请先在模型管理中配置并同步/)
    ).toBeInTheDocument()
    expect(
      within(dialog).getByRole('button', { name: '创建记忆应用' })
    ).toBeDisabled()
    expect(create).not.toHaveBeenCalled()
  })
})
