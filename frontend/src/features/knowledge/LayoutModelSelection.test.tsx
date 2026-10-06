import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ConfigProvider } from 'antd'
import { MemoryRouter } from 'react-router'
import { useAuthStore } from '@/stores/auth'
import KnowledgeForm from './KnowledgeForm'
import type { KnowledgeDataset } from '@/api/knowledge'
import type { MultiRAGModel } from '@/api/multirag'

const h = vi.hoisted(() => ({
  create: vi.fn(), update: vi.fn(), sync: vi.fn(), retry: vi.fn(), queried: vi.fn(),
  ocr: [] as MultiRAGModel[], vision: [] as MultiRAGModel[], visionError: false,
}))
vi.mock('@/queries/useKnowledge', () => ({
  useCreateKnowledge: () => ({ mutateAsync: h.create }),
  useUpdateKnowledge: () => ({ mutateAsync: h.update }),
}))
vi.mock('@/queries/useProviders', () => ({
  useProviders: () => ({ data: [], refetch: h.retry }),
  useSyncProviderMultiRAG: () => ({ mutateAsync: h.sync }),
}))
vi.mock('@/queries/useMultirag', () => ({
  useMultiragModels: (type: string) => {
    h.queried(type)
    return { data: type === 'ocr' ? h.ocr : type === 'image2text' ? h.vision : [], isError: type === 'image2text' && h.visionError, refetch: h.retry }
  },
}))

const model = (name: string, factory: string, type: string, status = '1'): MultiRAGModel => ({ name, factory, type, status, fullId: `${name}@${factory}` })
const dataset = (layout = 'vision-a@OpenAI-API-Compatible'): KnowledgeDataset => ({
  id: 'kb-layout', name: 'Saved KB', display_name: 'Saved KB', collection_name: 'saved', description: '', permission: 'me',
  doc_num: 0, chunk_num: 0, parser_id: 'naive', embd_id: 'embed-a@OpenAI-API-Compatible', parser_config: { layout_recognize: layout, retained_extension: { enabled: false } },
})
const tree = (editing: KnowledgeDataset | null = null) => <ConfigProvider><MemoryRouter><KnowledgeForm open editing={editing} onClose={vi.fn()} /></MemoryRouter></ConfigProvider>
async function openAdvanced(user: ReturnType<typeof userEvent.setup>) {
  const header = screen.getByText('高级配置').closest<HTMLElement>('.ant-collapse-header')!
  if (header.getAttribute('aria-expanded') !== 'true') await user.click(header)
}
function layoutControl() { return screen.getByRole('combobox', { name: '解析布局' }) }
async function choose(user: ReturnType<typeof userEvent.setup>, text: string) {
  await user.click(layoutControl())
  await user.click(await screen.findByText(text))
}

describe('PDF layout model selection', () => {
  beforeEach(() => {
    useAuthStore.setState({ user: null })
    localStorage.clear()
    for (const mock of [h.create, h.update, h.sync, h.retry, h.queried]) mock.mockReset()
    h.create.mockResolvedValue({ id: 'kb-created' })
    h.update.mockResolvedValue({ id: 'kb-layout' })
    h.ocr = [model('PP-OCRv5', 'PaddleOCR', 'ocr'), model('PaddleOCR-VL', 'PaddleOCR', 'ocr')]
    h.vision = [model('vision-a', 'OpenAI-API-Compatible', 'image2text')]
    h.visionError = false
  })

  it('loads visual models independently and saves their complete reference without changing embedding', async () => {
    const editing = dataset('DeepDOC')
    render(tree(editing))
    const user = userEvent.setup()
    await openAdvanced(user)
    expect(new Set(h.queried.mock.calls.map(call => call[0]))).toEqual(new Set(['embedding', 'ocr', 'image2text']))
    await choose(user, 'vision-a (OpenAI-API-Compatible)')
    expect(screen.getByText('实验性')).toBeInTheDocument()
    expect(screen.getByText(/视觉模型按 PDF 页面调用/)).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /^保\s*存$/ }))
    await waitFor(() => { expect(h.update).toHaveBeenCalledTimes(1); })
    expect(h.update.mock.calls[0][0]).toEqual({ id: 'kb-layout', data: { parser_config: { layout_recognize: 'vision-a@OpenAI-API-Compatible' } } })
    expect(h.sync).not.toHaveBeenCalled()
  })

  it('keeps two OCR models from the same factory and submits the selected model', async () => {
    render(tree(dataset('DeepDOC')))
    const user = userEvent.setup()
    await openAdvanced(user)
    await user.click(layoutControl())
    expect(await screen.findByText('PP-OCRv5 (PaddleOCR)')).toBeInTheDocument()
    await user.click(screen.getByText('PaddleOCR-VL (PaddleOCR)'))
    await user.click(screen.getByRole('button', { name: /^保\s*存$/ }))
    await waitFor(() => { expect(h.update).toHaveBeenCalledTimes(1); })
    expect(h.update.mock.calls[0][0].data.parser_config.layout_recognize).toBe('PaddleOCR-VL@PaddleOCR')
    expect(h.sync).not.toHaveBeenCalled()
  })

  it('exposes MinerU-specific fields for an exact model reference', async () => {
    h.ocr = [model('mineru-a', 'MinerU', 'ocr')]
    render(tree(dataset('DeepDOC')))
    const user = userEvent.setup()
    await openAdvanced(user)
    await choose(user, 'mineru-a (MinerU)')
    await user.click(screen.getByText('内容增强与专用解析参数'))
    expect(screen.getByRole('combobox', { name: '解析方式' })).toBeInTheDocument()
  })

  it('retains an unavailable saved reference without resubmitting parser defaults', async () => {
    h.vision = [model('vision-a', 'OpenAI-API-Compatible', 'image2text', '0')]
    render(tree(dataset()))
    const user = userEvent.setup()
    await openAdvanced(user)
    expect(screen.getByText(/vision-a@OpenAI-API-Compatible.*当前未提供/)).toBeInTheDocument()
    await user.click(layoutControl())
    const retained = (await screen.findAllByText(/vision-a@OpenAI-API-Compatible.*当前未提供/)).map(element => element.closest('.ant-select-item-option-disabled')).find(Boolean)
    expect(retained).toHaveTextContent('vision-a@OpenAI-API-Compatible')
    await user.keyboard('{Escape}')
    await user.type(screen.getByRole('textbox', { name: '描述' }), 'New note')
    await user.click(screen.getByRole('button', { name: /^保\s*存$/ }))
    await waitFor(() => { expect(h.update).toHaveBeenCalledTimes(1); })
    expect(h.update.mock.calls[0][0].data).toEqual({ description: 'New note' })
  })

  it('hides stale failed visual candidates, preserves the selection and offers retry', async () => {
    const editing = dataset()
    const view = render(tree(editing))
    const user = userEvent.setup()
    await openAdvanced(user)
    await waitFor(() => expect(within(layoutControl().closest('.ant-select')!).getByText('vision-a (OpenAI-API-Compatible)')).toBeInTheDocument())
    h.visionError = true
    view.rerender(tree(editing))
    expect(screen.getByText('模型候选读取失败')).toBeInTheDocument()
    expect(screen.getByText(/vision-a@OpenAI-API-Compatible.*当前未提供/)).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /^重\s*试$/ }))
    expect(h.retry).toHaveBeenCalled()
    await user.type(screen.getByRole('textbox', { name: '描述' }), 'Keep ref')
    await user.click(screen.getByRole('button', { name: /^保\s*存$/ }))
    await waitFor(() => { expect(h.update).toHaveBeenCalledTimes(1); })
    expect(h.update.mock.calls[0][0].data).toEqual({ description: 'Keep ref' })
  })

  it('selects and saves a visual model using the keyboard', async () => {
    render(tree(dataset('DeepDOC')))
    const user = userEvent.setup()
    await openAdvanced(user)
    await user.click(layoutControl())
    await user.type(layoutControl(), 'vision-a', { skipClick: true })
    await screen.findByText('vision-a (OpenAI-API-Compatible)')
    // rc-select reads legacy keyCode as real browsers provide it; jsdom's
    // user-event keyboard omits it. Keep the event contract faithful here.
    fireEvent.keyDown(layoutControl(), { key: 'ArrowDown', code: 'ArrowDown', keyCode: 40, which: 40 })
    fireEvent.keyDown(layoutControl(), { key: 'Enter', code: 'Enter', keyCode: 13, which: 13 })
    await user.tab()
    expect(await screen.findByText('实验性')).toBeInTheDocument()
    expect(layoutControl()).toHaveAttribute('aria-describedby')
    await user.click(screen.getByRole('button', { name: /^保\s*存$/ }))
    await waitFor(() => { expect(h.update).toHaveBeenCalledTimes(1); })
    expect(h.update.mock.calls[0][0].data.parser_config.layout_recognize).toBe('vision-a@OpenAI-API-Compatible')
  })

  it.each(['1', '0'])('preserves a real model-name prefix when the saved model has status %s', async (status) => {
    h.vision = [model('multirag:vision-a', 'Anthropic', 'image2text', status)]
    render(tree(dataset('multirag:vision-a@Anthropic')))
    const user = userEvent.setup()
    await openAdvanced(user)
    await user.type(screen.getByRole('textbox', { name: '描述' }), 'Keep model prefix')
    await user.click(screen.getByRole('button', { name: /^保\s*存$/ }))
    await waitFor(() => { expect(h.update).toHaveBeenCalledTimes(1); })
    expect(h.update.mock.calls[0][0].data).toEqual({ description: 'Keep model prefix' })
  })
})
