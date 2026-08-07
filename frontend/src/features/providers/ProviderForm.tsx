import type { ComponentType } from 'react'
import type { Provider } from '@/api/providers'
import GenericProviderForm from './presets/GenericProviderForm'

interface ProviderFormProps {
  open: boolean
  editingProvider: Provider | null
  onClose: () => void
}

const PRESET_FORMS: Partial<Record<string, ComponentType<any>>> = {
  'glm-cn': GenericProviderForm,
  'kimi-cn': GenericProviderForm,
  'bailian': GenericProviderForm,
  'anthropic-thirdparty': GenericProviderForm,
  'openai-thirdparty': GenericProviderForm,
  'mineru': GenericProviderForm,
  'paddleocr': GenericProviderForm,
}

export default function ProviderForm({ open, editingProvider, onClose }: ProviderFormProps) {
  const FormComponent = editingProvider
    ? PRESET_FORMS[editingProvider.key] || GenericProviderForm
    : GenericProviderForm

  return <FormComponent open={open} editingProvider={editingProvider} onClose={onClose} />
}
