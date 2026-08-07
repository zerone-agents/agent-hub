import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ConfigProvider } from 'antd'
import { antdTheme } from '@/lib/antd-theme'
import GenericProviderForm from './GenericProviderForm'
import type { Provider } from '@/api/providers'

vi.mock('@/queries/useProviders', () => ({
  useCreateProvider: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useUpdateProvider: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useProbeProvider: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useProbeConfig: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useProviderAttrRules: () => ({ data: {} }),
}))

// GenericProviderForm reads models from `editingProvider.defaultModels` and
// shows them as editable rows. The AIGC chip column is read-only, surfaced
// from the backend-assigned aigcCode on each model.
const editingProvider: Provider = {
  id: 1,
  key: 'openai-thirdparty',
  name: 'Test Provider',
  description: '',
  descriptionEn: '',
  protocol: 'openai',
  authStyle: 'api_key',
  baseUrl: 'https://api.example.com',
  defaultModels: [
    {
      modelId: 'glm-4.5',
      displayName: 'GLM 4.5',
      modelType: 'llm',
      aigcCode: '0001',
    },
    {
      modelId: 'new-unsaved-model',
      displayName: 'New Model',
      modelType: 'llm',
      // no aigcCode — should render the — placeholder
    },
  ],
  fields: [],
  attributes: {},
  iconKey: 'openai',
  builtin: false,
  lockedApiKey: '',
  createdAt: '',
  updatedAt: '',
}

function renderForm() {
  return render(
    <ConfigProvider theme={antdTheme}>
      <GenericProviderForm
        open
        editingProvider={editingProvider}
        onClose={vi.fn()}
      />
    </ConfigProvider>,
  )
}

describe('GenericProviderForm AIGC column', () => {
  it('renders the AIGC column header', () => {
    renderForm()
    expect(screen.getByText('AIGC')).toBeInTheDocument()
  })

  it('renders the model aigcCode as a purple chip', () => {
    renderForm()
    const chip = screen.getByText('0001')
    expect(chip).toBeInTheDocument()
    expect(chip).toHaveAttribute(
      'title',
      '由系统自动分配，用于 AIGC 内容标识',
    )
  })

  it('renders the — placeholder when aigcCode is absent', () => {
    renderForm()
    // The unsaved model row should show the em dash placeholder.
    const placeholder = screen.getByText('—')
    expect(placeholder).toBeInTheDocument()
    expect(placeholder).toHaveAttribute(
      'title',
      '由系统自动分配，用于 AIGC 内容标识',
    )
  })
})
