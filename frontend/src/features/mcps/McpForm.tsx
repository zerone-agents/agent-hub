import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Modal, Form, Input, Select, Spin, InputNumber, List, Button } from 'antd'
import { XIcon, PlusIcon, TrashIcon, PlugsConnectedIcon } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import PrimaryButton from '@/components/PrimaryButton'
import type { Mcp, McpTransportType, McpProbeResult } from '@/api/mcps'
import { useCreateMcp, useUpdateMcp, useMcp, useProbeMcp } from '@/queries/useMcps'
import { tokens as tk } from '@/styles/tokens'

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
    justify-content: flex-end;
    gap: 10px;
    padding: 14px 24px;
    border-top: 1px solid color-mix(in srgb, var(--foreground) 5%, transparent);
  `,
  // 动态字段编辑器
  editorRow: css`
    display: grid;
    gap: 8px;
    margin-bottom: 8px;
    align-items: center;
  `,
  kvRow: css`grid-template-columns: 140px 1fr 28px;`,
  editorHeader: css`
    display: grid;
    gap: 8px;
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
    border-radius: ${tk.radiusSm}px;
    background: transparent;
    color: var(--text-tertiary);
    font-size: ${tk.textSm};
    cursor: pointer;
    transition: all 0.15s;
    &:hover { border-color: ${tk.ink}; color: ${tk.ink}; }
  `,
  removeBtn: css`
    width: 28px;
    height: 28px;
    display: flex;
    align-items: center;
    justify-content: center;
    border: none;
    background: transparent;
    border-radius: ${tk.radiusSm}px;
    color: var(--text-muted);
    cursor: pointer;
    transition: all 0.15s;
    &:hover { background: rgba(220, 38, 38, 0.06); color: ${tk.danger}; }
  `,
  toolsBox: css`
    margin-top: 16px;
    padding: 12px;
    border-radius: 6px;
    background: var(--bg-secondary, #fafafa);
    border: 1px solid color-mix(in srgb, var(--foreground) 6%, transparent);
  `,
  toolsTitle: css`
    font-size: 11px;
    font-weight: 600;
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.05em;
    margin-bottom: 8px;
  `,
  toolsEmpty: css`
    font-size: 13px;
    color: var(--text-muted);
  `,
  hint: css`
    font-size: 11px;
    color: var(--text-muted);
    margin-top: -8px;
    margin-bottom: 12px;
  `
}))

interface McpFormProps {
  open: boolean
  editingMcp: Mcp | null
  onClose: () => void
}

interface FormValues {
  name: string
  title: string
  description: string
  transportType: McpTransportType
  url: string
  retryMaxRetries: number | null
  retryTimeoutMs: number | null
}

interface KvPair {
  key: string
  value: string
}

export default function McpForm({ open, editingMcp, onClose }: McpFormProps) {
  const { t } = useTranslation()
  const { styles } = useStyles()
  const [form] = Form.useForm<FormValues>()
  const createMcp = useCreateMcp()
  const updateMcp = useUpdateMcp()
  const probeMcp = useProbeMcp()

  // 编辑时拉取详情（含解密后的 headers）
  const { data: detail, isLoading: detailLoading } = useMcp(open && editingMcp ? editingMcp.name : null)

  // 动态字段：脱离 antd Form，用 useState 管理
  const [headerPairs, setHeaderPairs] = useState<KvPair[]>([])

  // 探测相关
  const [probedResult, setProbedResult] = useState<McpProbeResult | null>(null)
  const [probing, setProbing] = useState(false)

  const submitting = createMcp.isPending || updateMcp.isPending
  const isEdit = !!editingMcp

  const transportType = Form.useWatch('transportType', form)

  useEffect(() => {
    if (!open) return
    if (editingMcp) {
      // 详情加载完成后再填表
      if (detail) {
        form.setFieldsValue({
          name: detail.name,
          title: detail.title,
          description: detail.description,
          transportType: detail.transportType,
          url: detail.url ?? '',
          retryMaxRetries: detail.retryMaxRetries ?? null,
          retryTimeoutMs: detail.retryTimeoutMs ?? null,
        })
        // eslint-disable-next-line react-hooks/set-state-in-effect -- sync dynamic-field state with loaded detail; coupled to the antd form.setFieldsValue above
        setHeaderPairs(Object.entries(detail.headers).map(([k, v]) => ({ key: k, value: v })))
        // 编辑模式：用后端存储的 tools 初始化显示
        if (detail.tools && detail.tools.length > 0) {
          setProbedResult({
            status: detail.probeStatus === 'success' ? 'success' : detail.probeStatus === 'failed' ? 'failed' : 'unsupported',
            tools: detail.tools,
          })
        } else {
          setProbedResult(null)
        }
      }
    } else {
      form.resetFields()
      form.setFieldsValue({ transportType: 'sse', retryMaxRetries: null, retryTimeoutMs: null })
       
      setHeaderPairs([])
      setProbedResult(null)
    }
  }, [open, editingMcp, detail, form])

  // ── headers 编辑器 ──
  const handleAddHeader = () => { setHeaderPairs([...headerPairs, { key: '', value: '' }]); }
  const handleRemoveHeader = (i: number) => { setHeaderPairs(headerPairs.filter((_, idx) => idx !== i)); }
  const handleHeaderChange = (i: number, field: keyof KvPair, v: string) =>
    { setHeaderPairs(headerPairs.map((p, idx) => (idx === i ? { ...p, [field]: v } : p))); }

  const pairsToMap = (pairs: KvPair[]): Record<string, string> => {
    const out: Record<string, string> = {}
    for (const p of pairs) {
      if (p.key.trim()) out[p.key.trim()] = p.value
    }
    return out
  }

  // ── 探测 ──
  const handleProbe = async () => {
    try {
      const values = await form.validateFields(['transportType', 'url'])
      setProbing(true)
      setProbedResult(null)
      const config = {
        transportType: values.transportType,
        url: values.url,
        headers: pairsToMap(headerPairs),
      }
    const result = await probeMcp.mutateAsync({ config })
    setProbedResult(result)
    } catch {
      setProbedResult(null)
    } finally {
      setProbing(false)
    }
  }

  const handleSubmit = async () => {
    const values = await form.validateFields()
    const payload: Record<string, unknown> = {
      title: values.title,
      description: values.description,
      transportType: values.transportType,
      url: values.url,
      retryMaxRetries: values.retryMaxRetries ?? null,
      retryTimeoutMs: values.retryTimeoutMs ?? null,
      headers: pairsToMap(headerPairs),
    }
    if (probedResult?.status === 'success') {
      payload.tools = probedResult.tools
    }

    if (editingMcp) {
      await updateMcp.mutateAsync({ name: editingMcp.name, data: payload })
    } else {
      await createMcp.mutateAsync({ ...payload, name: values.name })
    }
    onClose()
  }

  return (
    <Modal
      open={open}
      onCancel={onClose}
      footer={null}
      closable={false}
      width={680}
      styles={{ body: { padding: 0 } }}
      title={null}
      destroyOnHidden
    >
      <div className={styles.modalHead}>
        <div className={styles.modalTitle}>{isEdit ? t('mcps.form.editTitle') : t('mcps.create')}</div>
        <button type="button" className={styles.modalClose} onClick={onClose}>
          <XIcon size={18} />
        </button>
      </div>

      <Form form={form} layout="vertical" className={styles.modalBody} requiredMark={false}>
        {isEdit && detailLoading ? (
          <div style={{ display: 'flex', justifyContent: 'center', padding: '40px 0' }}>
            <Spin size="medium" />
          </div>
        ) : (
          <>
            {/* 基本信息 */}
            <div className={styles.sectionTitle}>{t('mcps.form.basicSection')}</div>
            <Form.Item label={t('mcps.form.nameLabel')} name="name" rules={[{ required: true, message: t('mcps.form.nameRequired') }]}>
              <Input placeholder="e.g. filesystem" disabled={isEdit} />
            </Form.Item>
            <Form.Item label={t('mcps.form.titleLabel')} name="title" rules={[{ required: true, message: t('mcps.form.titleRequired') }]}>
              <Input placeholder={t('mcps.form.titlePlaceholder')} />
            </Form.Item>
            <Form.Item label={t('mcps.form.descLabel')} name="description">
              <Input.TextArea placeholder={t('mcps.form.descPlaceholder')} rows={2} />
            </Form.Item>

            {/* 传输配置 */}
            <div className={styles.sectionTitle} style={{ marginTop: 20 }}>{t('mcps.form.transportSection')}</div>
            <Form.Item label={t('mcps.form.typeLabel')} name="transportType" rules={[{ required: true }]}>
              <Select
                options={[
                  { label: 'SSE', value: 'sse' },
                  { label: 'HTTP', value: 'http' },
                ]}
              />
            </Form.Item>

            <>
              <Form.Item
                label="URL"
                name="url"
                rules={[{ required: true, message: t('mcps.form.urlRequired') }]}
              >
                <Input placeholder={transportType === 'sse' ? 'https://mcp.example.com/sse' : 'https://mcp.example.com/mcp'} />
              </Form.Item>

              <div className={styles.sectionTitle} style={{ marginTop: 20 }}>{t('mcps.form.headersSection')}</div>
              <div className={styles.hint}>{t('mcps.form.headersHint')}</div>
              {editingMcp?.isBuiltin && (
                <div className={styles.hint}>{t('mcps.form.builtinVarHint')}</div>
              )}
              {headerPairs.length > 0 && (
                <div className={`${styles.editorHeader} ${styles.kvRow}`}>
                  <span>Key</span>
                  <span>Value</span>
                  <span />
                </div>
              )}
              {headerPairs.map((p, i) => (
                <div key={i} className={`${styles.editorRow} ${styles.kvRow}`}>
                  <Input
                    size="small"
                    placeholder="Authorization"
                    value={p.key}
                    onChange={(e) => { handleHeaderChange(i, 'key', e.target.value); }}
                  />
                  <Input
                    size="small"
                    placeholder="Bearer xxxx"
                    value={p.value}
                    onChange={(e) => { handleHeaderChange(i, 'value', e.target.value); }}
                  />
                  <button type="button" className={styles.removeBtn} onClick={() => { handleRemoveHeader(i); }}>
                    <TrashIcon size={13} />
                  </button>
                </div>
              ))}
              <button type="button" className={styles.addBtn} onClick={handleAddHeader}>
                <PlusIcon size={14} /> {t('mcps.form.addHeader')}
              </button>
            </>

            {/* 重试策略 */}
            <div className={styles.sectionTitle} style={{ marginTop: 20 }}>{t('mcps.form.retrySection')}</div>
            <div className={styles.hint}>{t('mcps.form.retryHint')}</div>
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
              <Form.Item label={t('mcps.form.retryMaxLabel')} name="retryMaxRetries">
                <InputNumber min={0} max={10} placeholder={t('mcps.form.retryMaxPlaceholder')} style={{ width: '100%' }} />
              </Form.Item>
              <Form.Item label={t('mcps.form.timeoutLabel')} name="retryTimeoutMs">
                <InputNumber min={1000} max={120000} step={1000} placeholder={t('mcps.form.timeoutPlaceholder')} style={{ width: '100%' }} />
              </Form.Item>
            </div>

            {/* 探测连接 */}
            <div style={{ marginTop: 8, marginBottom: 16 }}>
              <Button
                icon={probing ? <Spin size="small" /> : <PlugsConnectedIcon size={15} />}
                onClick={handleProbe}
                loading={probing}
              >
                {t('mcps.form.probeSection')}
              </Button>
              {probedResult?.status === 'success' && (
                <span style={{ marginLeft: 8, fontSize: 12, color: tk.success }}>
                  {t('mcps.form.probeSuccess')}{probedResult.tools ? t('mcps.form.foundTools', { n: probedResult.tools.length }) : ''}
                </span>
              )}
              {probedResult?.status === 'failed' && (
                <span style={{ marginLeft: 8, fontSize: 12, color: tk.danger }}>
                  ✗ {probedResult.error ?? t('mcps.form.probeErrorFallback')}
                </span>
              )}
            </div>

            {/* Tools 列表 */}
            <div className={styles.toolsBox}>
              <div className={styles.toolsTitle}>Tools</div>
              {probedResult?.status === 'success' && probedResult.tools && probedResult.tools.length > 0 ? (
                <List
                  size="small"
                  dataSource={probedResult.tools}
                  renderItem={(tool) => (
                    <List.Item>
                      <div>
                        <div style={{ fontWeight: 500, fontFamily: 'monospace', fontSize: 13 }}>{tool.name}</div>
                        <div style={{ fontSize: 12, color: 'var(--text-secondary)' }}>{tool.description}</div>
                      </div>
                    </List.Item>
                  )}
                />
              ) : (
                <div className={styles.toolsEmpty}>
                  {t('mcps.form.toolsListHint')}
                </div>
              )}
            </div>
          </>
        )}
      </Form>

      <div className={styles.modalFoot}>
        <Button onClick={onClose}>{t('common.cancel')}</Button>
        <PrimaryButton onClick={handleSubmit} loading={submitting} disabled={isEdit && detailLoading}>
          {isEdit ? t('scenes.update') : t('scenes.createSubmit')}
        </PrimaryButton>
      </div>
    </Modal>
  )
}
