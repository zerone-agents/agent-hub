import { useEffect, useMemo, useState } from 'react'
import { Modal, Form, Input, Select, Spin, message, Checkbox, Button } from 'antd'
import { XIcon, PlusIcon, TrashIcon, PlugIcon } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import PrimaryButton from '@/components/PrimaryButton'
import type { Provider, CatalogModel, PresetField, AttrValue, AttrRule } from '@/api/providers'
import EffortCell from './EffortCell'
import {
  useCreateProvider,
  useUpdateProvider,
  useProbeProvider,
  useProbeConfig,
  useProviderAttrRules,
} from '@/queries/useProviders'
import { identifierFormRules } from '@/utils/identifier'
import { tokens as t } from '@/styles/tokens'

const useStyles = createStyles(({ css }) => ({
  modalHead: css`
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 18px 24px;
    border-bottom: 1px solid color-mix(in srgb, var(--foreground) 5%, transparent);
  `,
  modalTitle: css`
    font-size: 18px;
    font-weight: 600;
    color: var(--text);
    letter-spacing: -0.02em;
  `,
  modalClose: css`
    width: 32px;
    height: 32px;
    display: flex;
    align-items: center;
    justify-content: center;
    border: none;
    background: var(--ink-subtle);
    border-radius: 4px;
    color: var(--text-tertiary);
    cursor: pointer;
    transition: all 0.15s;
    &:hover { background: var(--ink-light); color: var(--text); }
  `,
  modalBody: css`padding: 20px 24px 8px; max-height: 62vh; overflow-y: auto;`,
  sectionTitle: css`
    font-size: 11px;
    font-weight: 600;
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.05em;
    margin-bottom: 14px;
  `,
  modalFoot: css`
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 10px;
    padding: 14px 24px;
    border-top: 1px solid color-mix(in srgb, var(--foreground) 5%, transparent);
  `,
  footRight: css`display: flex; gap: 10px;`,
  // Model / field editors
  editorRow: css`
    display: grid;
    gap: 8px;
    margin-bottom: 8px;
    align-items: center;
  `,
  modelRow: css`grid-template-columns: 1fr 1fr 110px 90px minmax(150px, 1fr) 28px;`,
  modelRowOcr: css`grid-template-columns: 1fr 1fr 110px 28px;`,
  fieldRow: css`grid-template-columns: 120px 1fr 1fr 80px 80px 28px;`,
  editorHeader: css`
    display: grid;
    gap: 8px;
    align-items: center;
    font-size: 10px;
    font-weight: 600;
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.05em;
    margin-bottom: 6px;
  `,
  addBtn: css`
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 6px 12px;
    border: 1px dashed color-mix(in srgb, var(--foreground) 15%, transparent);
    border-radius: ${t.radiusSm}px;
    background: transparent;
    color: var(--text-tertiary);
    font-size: ${t.textSm};
    cursor: pointer;
    transition: all 0.15s;
    &:hover { border-color: ${t.ink}; color: ${t.ink}; }
  `,
  removeBtn: css`
    width: 28px;
    height: 28px;
    display: flex;
    align-items: center;
    justify-content: center;
    border: none;
    background: transparent;
    border-radius: ${t.radiusSm}px;
    color: var(--text-muted);
    cursor: pointer;
    transition: all 0.15s;
    &:hover { background: rgba(220, 38, 38, 0.06); color: ${t.danger}; }
  `,
  builtinRow: css`
    display: flex;
    gap: 24px;
    align-items: center;
  `,
}))

interface GenericProviderFormProps {
  open: boolean
  editingProvider: Provider | null
  onClose: () => void
}

interface FormValues {
  key: string
  name: string
  description: string
  descriptionEn: string
  iconKey: string
  protocol: string
  authStyle: string
  baseUrl: string
  builtin: boolean
  lockedApiKey: string
}

const FIELD_DEFINITIONS: Partial<Record<string, { label: string; labelEn: string; type: PresetField['type']; required: boolean; secret: boolean }>> = {
  name: { label: '名称', labelEn: 'Name', type: 'text', required: true, secret: false },
  base_url: { label: 'API 地址', labelEn: 'API URL', type: 'text', required: true, secret: false },
  api_key: { label: 'API 密钥', labelEn: 'API Key', type: 'password', required: false, secret: true },
}

const MODEL_TYPE_OPTIONS = [
  { label: 'LLM', value: 'llm' },
  { label: 'OCR', value: 'ocr' },
  { label: 'Embedding', value: 'embedding' },
  { label: 'VLM', value: 'vlm' },
]

const OCR_MODEL_TYPE_OPTIONS = [
  { label: 'OCR', value: 'ocr' },
]

// Protocol options shown in the form Select. Constrained to the four
// protocols the backend factory knows how to instantiate.
const PROTOCOL_OPTIONS = [
  { label: 'Anthropic', value: 'anthropic' },
  { label: 'OpenAI', value: 'openai' },
  { label: 'MinerU', value: 'mineru' },
  { label: 'PaddleOCR', value: 'paddleocr' },
]

export default function GenericProviderForm({ open, editingProvider, onClose }: GenericProviderFormProps) {
  const { styles } = useStyles()
  const [form] = Form.useForm<FormValues>()
  const createProvider = useCreateProvider()
  const updateProvider = useUpdateProvider()
  const probeProvider = useProbeProvider()
  const probeConfig = useProbeConfig()
  const { data: attrRules } = useProviderAttrRules()

  const [defaultModels, setDefaultModels] = useState<CatalogModel[]>([])
  const [fields, setFields] = useState<PresetField[]>([])
  const [attributes, setAttributes] = useState<Record<string, AttrValue>>({})
  const [probing, setProbing] = useState(false)
  const [initialMaskedAPIKey, setInitialMaskedAPIKey] = useState('')

  const submitting = createProvider.isPending || updateProvider.isPending
  const isEdit = !!editingProvider
  const protocol = Form.useWatch('protocol', form)
  const isOcr = protocol === 'mineru' || protocol === 'paddleocr'
  const modelTypeOptions = isOcr ? OCR_MODEL_TYPE_OPTIONS : MODEL_TYPE_OPTIONS

  const activeRules: AttrRule[] = useMemo(
    () => (protocol ? attrRules?.[protocol] ?? [] : []),
    [protocol, attrRules]
  )

  useEffect(() => {
    if (!open) return
    // eslint-disable-next-line react-hooks/set-state-in-effect -- merge attribute defaults from active rules into local state; functional update returns prev when nothing changed
    setAttributes((prev) => {
      const next = { ...prev }
      let changed = false
      for (const rule of activeRules) {
        // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- Record index returns T not T | undefined; first-time key access genuinely needs the default
        if (!next[rule.key] && rule.default) {
          next[rule.key] = { type: rule.type, value: rule.default }
          changed = true
        }
      }
      return changed ? next : prev
    })
  }, [open, activeRules])

  useEffect(() => {
    if (!open) return

    if (editingProvider) {
      form.setFieldsValue({
        key: editingProvider.key,
        name: editingProvider.name,
        description: editingProvider.description,
        descriptionEn: editingProvider.descriptionEn,
        iconKey: editingProvider.iconKey,
        protocol: editingProvider.protocol,
        authStyle: editingProvider.authStyle,
        baseUrl: editingProvider.baseUrl,
        builtin: editingProvider.builtin,
        lockedApiKey: editingProvider.lockedApiKey,
      })
      // eslint-disable-next-line react-hooks/set-state-in-effect -- sync local form state with the editing target on modal open; all four setStates are coupled to the antd form.setFieldsValue above
      setDefaultModels(editingProvider.defaultModels.map((m) => ({ ...m })))
      setFields(editingProvider.fields.map((f) => ({ ...f })))
      setAttributes({ ...editingProvider.attributes })
      setInitialMaskedAPIKey(editingProvider.lockedApiKey)
    } else {
      form.resetFields()
      form.setFieldsValue({
        protocol: 'anthropic',
        authStyle: 'api_key',
        iconKey: 'anthropic',
        builtin: false,
      })
       
      setDefaultModels([])
      setFields([])
      setAttributes({})
      setInitialMaskedAPIKey('')
    }
  }, [open, editingProvider, form])

  // ── Models editor ──
  const handleAddModel = () => {
    setDefaultModels([...defaultModels, {
      modelId: '',
      displayName: '',
      contextWindow: isOcr ? 0 : 200000,
      modelType: isOcr ? 'ocr' : 'llm',
    }])
  }
  const handleRemoveModel = (i: number) => {
    setDefaultModels(defaultModels.filter((_, idx) => idx !== i))
  }
  const handleModelChange = (i: number, field: keyof CatalogModel, value: string | number | string[]) => {
    setDefaultModels(defaultModels.map((m, idx) => (idx === i ? { ...m, [field]: value } : m)))
  }

  // ── Fields editor ──
  const handleAddField = () => {
    setFields([...fields, { key: 'api_key', label: 'API 密钥', labelEn: 'API Key', type: 'password', secret: true }])
  }
  const handleRemoveField = (i: number) => {
    setFields(fields.filter((_, idx) => idx !== i))
  }
  const handleFieldKeyChange = (i: number, key: string) => {
    const def = FIELD_DEFINITIONS[key]
    if (def) {
      setFields(fields.map((f, idx) => (idx === i ? { ...f, key, label: def.label, labelEn: def.labelEn, type: def.type, required: def.required, secret: def.secret } : f)))
    }
  }

  // ── Test connection ──
  const handleTestConnection = async () => {
    setProbing(true)
    try {
      if (editingProvider) {
        const values = form.getFieldsValue(['lockedApiKey', 'baseUrl']) as { lockedApiKey?: string; baseUrl?: string }
        const res = await probeProvider.mutateAsync({
          id: editingProvider.id,
          apiKey: values.lockedApiKey,
          baseUrl: values.baseUrl,
          models: defaultModels,
        })
        const result = (res as { data: { data?: { success?: boolean; latencyMs?: number; error?: string } } }).data.data
        if (result?.success) {
          message.success(`连接成功 · ${result.latencyMs}ms`)
        } else {
          message.error(`连接失败 · ${result?.error ?? '未知错误'}`)
        }
      } else {
        const values = await form.validateFields(['baseUrl', 'lockedApiKey', 'protocol', 'authStyle'])
        if (!values.baseUrl) {
          message.warning('请先填写 Base URL')
          return
        }
        const res = await probeConfig.mutateAsync({
          baseUrl: values.baseUrl,
          apiKey: values.lockedApiKey || '',
          protocol: values.protocol,
          authStyle: values.authStyle,
          models: defaultModels,
        })
        const result2 = (res as { data: { data?: { success?: boolean; latencyMs?: number; error?: string } } }).data.data
        if (result2?.success) {
          message.success(`连接成功 · ${result2.latencyMs}ms`)
        } else {
          message.error(`连接失败 · ${result2?.error ?? '未知错误'}`)
        }
      }
    } finally {
      setProbing(false)
    }
  }

  // ── Submit ──
  const handleSubmit = async () => {
    const values = await form.validateFields()

    if (editingProvider) {
      const data: Partial<Provider> = {
        name: values.name,
        description: values.description,
        descriptionEn: values.descriptionEn,
        protocol: values.protocol as 'anthropic' | 'openai' | 'mineru' | 'paddleocr',
        authStyle: values.authStyle as 'api_key' | 'auth_token' | 'no_auth',
        baseUrl: values.baseUrl,
        iconKey: values.iconKey,
        builtin: values.builtin,
        attributes,
        defaultModels,
        fields,
      }
      if (values.lockedApiKey !== initialMaskedAPIKey) {
        data.lockedApiKey = values.lockedApiKey
      }
      await updateProvider.mutateAsync({
        id: editingProvider.id,
        data,
      })
    } else {
      await createProvider.mutateAsync({
        key: values.key,
        name: values.name,
        description: values.description,
        descriptionEn: values.descriptionEn,
        protocol: values.protocol as 'anthropic' | 'openai' | 'mineru' | 'paddleocr',
        authStyle: values.authStyle as 'api_key' | 'auth_token' | 'no_auth',
        baseUrl: values.baseUrl,
        iconKey: values.iconKey,
        builtin: values.builtin,
        attributes,
        lockedApiKey: values.lockedApiKey,
        defaultModels,
        fields,
      })
    }
    onClose()
  }

  return (
    <Modal
      open={open}
      onCancel={onClose}
      footer={null}
      closable={false}
      width={760}
      styles={{ body: { padding: 0 } }}
      title={null}
      destroyOnHidden
    >
      <div className={styles.modalHead}>
        <div className={styles.modalTitle}>{isEdit ? '编辑 Provider' : '新建 Provider'}</div>
        <button type="button" className={styles.modalClose} onClick={onClose}>
          <XIcon size={18} />
        </button>
      </div>

      <Form
        form={form}
        layout="vertical"
        className={styles.modalBody}
        requiredMark={(label, { required }) => (
          <span>
            {required && <span style={{ color: t.danger, marginInlineEnd: 4 }}>*</span>}
            {label}
            {!required && (
              <span style={{ color: t.textMuted, fontSize: 12, marginInlineStart: 4 }}>（选填）</span>
            )}
          </span>
        )}
      >
        {/* Provider */}
        <div className={styles.sectionTitle} style={{ marginTop: 0 }}>Provider</div>
        <Form.Item label="Protocol" name="protocol" rules={[{ required: true, message: '请选择协议' }]}>
          <Select
            options={PROTOCOL_OPTIONS}
            placeholder="选择协议"
          />
        </Form.Item>

        {/* 基本信息 */}
        <div className={styles.sectionTitle} style={{ marginTop: 20 }}>基本信息</div>
        <Form.Item label="Key（唯一标识）" name="key" rules={identifierFormRules('Key')}>
          <Input placeholder="e.g. glm-cn" disabled={isEdit} />
        </Form.Item>
        <Form.Item label="名称" name="name" rules={[{ required: true, message: '请输入名称' }]}>
          <Input placeholder="e.g. GLM Coding Plan" />
        </Form.Item>
        <Form.Item label="中文描述" name="description">
          <Input.TextArea placeholder="e.g. 智谱 GLM Anthropic 兼容编码服务" rows={2} />
        </Form.Item>
        <Form.Item label="英文描述" name="descriptionEn">
          <Input.TextArea placeholder="e.g. Zhipu GLM Anthropic-compatible coding service" rows={2} />
        </Form.Item>
        <Form.Item label="图标 Key" name="iconKey">
          <Input placeholder="e.g. anthropic, zhipu, kimi, openai" />
        </Form.Item>

        {/* 协议配置 */}
        <div className={styles.sectionTitle} style={{ marginTop: 20 }}>协议配置</div>
        <Form.Item label="Base URL" name="baseUrl">
          <Input placeholder="https://api.example.com/v1" />
        </Form.Item>
        <Form.Item label="Auth Style" name="authStyle" rules={[{ required: true }]}>
          <Select
            options={[
              { label: 'API Key (X-Api-Key header)', value: 'api_key' },
              { label: 'Auth Token (Bearer header)', value: 'auth_token' },
              { label: 'No Auth', value: 'no_auth' },
            ]}
          />
        </Form.Item>
        <Form.Item label="内置 Provider" name="builtin" valuePropName="checked">
          <Checkbox>标记为内置（使用 Locked API Key，用户无需自行填写）</Checkbox>
        </Form.Item>
        <Form.Item label="Locked API Key" name="lockedApiKey">
          <Input placeholder="sk-..." />
        </Form.Item>

        {/* 默认模型 */}
        <div className={styles.sectionTitle} style={{ marginTop: 20 }}>默认模型</div>
        {defaultModels.length > 0 && (
          <div className={`${styles.editorHeader} ${isOcr ? styles.modelRowOcr : styles.modelRow}`}>
            <span>Model ID</span>
            <span>显示名称</span>
            <span>类型</span>
            {!isOcr && <span>上下文</span>}
            {!isOcr && <span>Effort</span>}
            <span />
          </div>
        )}
        {defaultModels.map((model, i) => (
          <div key={i} className={`${styles.editorRow} ${isOcr ? styles.modelRowOcr : styles.modelRow}`}>
            <Input
              size="small"
              placeholder="model-id"
              value={model.modelId}
              onChange={(e) => { handleModelChange(i, 'modelId', e.target.value); }}
            />
            <Input
              size="small"
              placeholder="Display Name"
              value={model.displayName}
              onChange={(e) => { handleModelChange(i, 'displayName', e.target.value); }}
            />
            <Select
              size="small"
              value={model.modelType}
              onChange={(v) => { handleModelChange(i, 'modelType', v); }}
              options={modelTypeOptions}
            />
            {!isOcr && (
              <Input
                size="small"
                placeholder="200000"
                value={model.contextWindow ?? ''}
                onChange={(e) => { handleModelChange(i, 'contextWindow', parseInt(e.target.value) || 0); }}
              />
            )}
            {!isOcr && (
              <EffortCell
                value={model.efforts}
                onChange={(efforts) => { handleModelChange(i, 'efforts', efforts); }}
              />
            )}
            <button type="button" className={styles.removeBtn} onClick={() => { handleRemoveModel(i); }}>
              <TrashIcon size={13} />
            </button>
          </div>
        ))}
        <button type="button" className={styles.addBtn} onClick={handleAddModel}>
          <PlusIcon size={14} /> 添加模型
        </button>

        {/* 表单字段定义 */}
        <div className={styles.sectionTitle} style={{ marginTop: 20 }}>表单字段定义</div>
        {fields.map((field, i) => (
          <div key={i} className={`${styles.editorRow} ${styles.fieldRow}`}>
            <Select
              size="small"
              value={field.key}
              onChange={(v) => { handleFieldKeyChange(i, v); }}
              options={[
                { label: 'name', value: 'name' },
                { label: 'base_url', value: 'base_url' },
                { label: 'api_key', value: 'api_key' },
              ]}
            />
            <Input size="small" value={field.label} disabled />
            <Input size="small" value={field.labelEn} disabled />
            <label style={{ display: 'flex', alignItems: 'center', gap: 4, fontSize: 12, color: '#6B7280', cursor: 'default' }}>
              必填 <Checkbox checked={field.required} disabled />
            </label>
            <label style={{ display: 'flex', alignItems: 'center', gap: 4, fontSize: 12, color: '#6B7280', cursor: 'default' }}>
              密钥 <Checkbox checked={field.secret} disabled />
            </label>
            <button type="button" className={styles.removeBtn} onClick={() => { handleRemoveField(i); }}>
              <TrashIcon size={13} />
            </button>
          </div>
        ))}
        <button type="button" className={styles.addBtn} onClick={handleAddField}>
          <PlusIcon size={14} /> 添加字段
        </button>

        {/* 动态属性（按 protocol + attr-rules 渲染） */}
        {activeRules.length > 0 && (
          <>
            <div className={styles.sectionTitle} style={{ marginTop: 20 }}>
              协议属性（{protocol}）
            </div>
            {activeRules.map((rule) => {
              const current = attributes[rule.key]
              const value = current.value
              if (rule.type === 'bool') {
                return (
                  <Form.Item key={rule.key} label={rule.label}>
                    <Checkbox
                      checked={value === 'true'}
                      onChange={(e) =>
                        { setAttributes((prev) => ({
                          ...prev,
                          [rule.key]: { type: 'bool', value: e.target.checked ? 'true' : 'false' },
                        })); }
                      }
                    >
                      {rule.labelEn}
                    </Checkbox>
                  </Form.Item>
                )
              }
              if (rule.enum && rule.enum.length > 0) {
                return (
                  <Form.Item key={rule.key} label={rule.label} required={rule.required}>
                    <Select
                      value={value}
                      placeholder={rule.labelEn}
                      options={rule.enum.map((v) => ({ label: v, value: v }))}
                      onChange={(v) =>
                        { setAttributes((prev) => ({ ...prev, [rule.key]: { type: 'string', value: v } })); }
                      }
                    />
                  </Form.Item>
                )
              }
              return (
                <Form.Item key={rule.key} label={rule.label} required={rule.required}>
                  <Input
                    value={value}
                    placeholder={rule.labelEn}
                    onChange={(e) =>
                      { setAttributes((prev) => ({
                        ...prev,
                        [rule.key]: { type: 'string', value: e.target.value },
                      })); }
                    }
                  />
                </Form.Item>
              )
            })}
          </>
        )}
      </Form>

      <div className={styles.modalFoot}>
        <Button
          icon={probing ? <Spin size="small" /> : <PlugIcon size={14} />}
          onClick={handleTestConnection}
          loading={probing}
        >
          测试连接
        </Button>
        <div className={styles.footRight}>
          <Button onClick={onClose}>取消</Button>
          <PrimaryButton onClick={handleSubmit} loading={submitting}>
            {isEdit ? '更新' : '创建'}
          </PrimaryButton>
        </div>
      </div>
    </Modal>
  )
}
