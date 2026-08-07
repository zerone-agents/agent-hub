import { useEffect } from 'react'
import { Modal, Form, Input, Switch, Button } from 'antd'
import { X } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import PrimaryButton from '@/components/PrimaryButton'
import type { Tool } from '@/api/tools'
import { useCreateTool, useUpdateTool } from '@/queries/useTools'
import { identifierFormRules } from '@/utils/identifier'

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
    &:hover {
      background: var(--ink-light);
      color: var(--text);
    }
  `,
  modalBody: css`
    padding: 20px 24px 8px;
    max-height: 60vh;
    overflow-y: auto;
  `,
  sectionTitle: css`
    font-size: 11px;
    font-weight: 600;
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.05em;
    margin-bottom: 14px;
  `,
  toggleRow: css`
    display: flex; align-items: center; gap: 12px; padding: 12px 0;
    cursor: pointer;
    &:hover { background: var(--ink-subtle); border-radius: 4px; padding: 12px 8px; margin: 0 -8px; }
  `,
  toggleText: css`display: flex; flex-direction: column;`,
  toggleName: css`font-size: 13px; font-weight: 500; color: var(--text);`,
  toggleDesc: css`font-size: 11px; color: var(--text-muted);`,
  modalFoot: css`
    display: flex;
    justify-content: flex-end;
    gap: 10px;
    padding: 14px 24px;
    border-top: 1px solid color-mix(in srgb, var(--foreground) 5%, transparent);
  `
}))

interface ToolFormProps {
  open: boolean
  editingTool: Tool | null
  onClose: () => void
}

interface FormValues {
  name: string
  title: string
  description: string
  isDefault: boolean
}

export default function ToolForm({ open, editingTool, onClose }: ToolFormProps) {
  const { styles } = useStyles()
  const [form] = Form.useForm<FormValues>()
  const createTool = useCreateTool()
  const updateTool = useUpdateTool()
  const submitting = createTool.isPending || updateTool.isPending

  useEffect(() => {
    if (open) {
      if (editingTool) {
        form.setFieldsValue({
          name: editingTool.name,
          title: editingTool.title || '',
          description: editingTool.description || '',
          isDefault: editingTool.isDefault || false
        })
      } else {
        form.resetFields()
      }
    }
  }, [open, editingTool, form])

  const handleSubmit = async () => {
    const values = await form.validateFields()
    if (editingTool) {
      await updateTool.mutateAsync({
        name: editingTool.name,
        data: { title: values.title, description: values.description, isDefault: values.isDefault }
      })
    } else {
      await createTool.mutateAsync({
        name: values.name,
        title: values.title,
        description: values.description,
        isDefault: values.isDefault
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
      width={560}
      styles={{ body: { padding: 0 } }}
      title={null}
      destroyOnHidden
    >
      <div className={styles.modalHead}>
        <div className={styles.modalTitle}>{editingTool ? '编辑工具' : '新建工具'}</div>
        <button type="button" className={styles.modalClose} onClick={onClose}>
          <X size={18} />
        </button>
      </div>

      <Form form={form} layout="vertical" className={styles.modalBody} requiredMark={false}>
        <div className={styles.sectionTitle}>基本信息</div>
        <Form.Item
          label="工具标识"
          name="name"
          rules={identifierFormRules('工具标识')}
        >
          <Input placeholder="e.g. Read" disabled={!!editingTool} />
        </Form.Item>

        <div className={styles.sectionTitle}>显示设置</div>
        <Form.Item label="中文名称" name="title">
          <Input placeholder="中文名称" />
        </Form.Item>
        <Form.Item label="功能描述" name="description">
          <Input.TextArea placeholder="描述此工具的功能用途" rows={3} />
        </Form.Item>

        <div className={styles.sectionTitle}>默认设置</div>
        <div className={styles.toggleRow} onClick={() => { form.setFieldValue('isDefault', !form.getFieldValue('isDefault')); }}>
          <Form.Item name="isDefault" valuePropName="checked" noStyle>
            <Switch
              size="small"
              onClick={(_checked, ev) => { ev.stopPropagation(); }}
            />
          </Form.Item>
          <div className={styles.toggleText}>
            <span className={styles.toggleName}>默认工具</span>
            <span className={styles.toggleDesc}>自动添加到所有当前及未来的 Agent，且不可取消选择</span>
          </div>
        </div>
      </Form>

      <div className={styles.modalFoot}>
        <Button onClick={onClose}>取消</Button>
        <PrimaryButton onClick={handleSubmit} loading={submitting}>
          {editingTool ? '更新' : '创建'}
        </PrimaryButton>
      </div>
    </Modal>
  )
}
