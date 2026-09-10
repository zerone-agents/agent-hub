import { useEffect } from 'react'
import { Button, Form, Input, Modal, Switch } from 'antd'
import { XIcon } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import PrimaryButton from '@/components/PrimaryButton'
import type { Personality } from '@/api/personalities'
import {
  useCreatePersonality,
  useUpdatePersonality,
} from '@/queries/usePersonalities'
import { identifierFormRules } from '@/utils/identifier'

const useStyles = createStyles(({ css }) => ({
  head: css`
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 18px 24px;
    border-bottom: 1px solid var(--border);
  `,
  title: css`
    font-size: 18px;
    font-weight: 650;
    color: var(--text);
    letter-spacing: -0.02em;
  `,
  close: css`
    width: 32px;
    height: 32px;
    display: grid;
    place-items: center;
    border: 0;
    border-radius: 6px;
    color: var(--text-tertiary);
    background: var(--ink-subtle);
    cursor: pointer;
    &:hover {
      color: var(--text);
      background: var(--ink-light);
    }
    &:focus-visible {
      outline: 2px solid var(--primary);
      outline-offset: 2px;
    }
  `,
  body: css`
    padding: 20px 24px 6px;
    max-height: 68vh;
    overflow-y: auto;
  `,
  foot: css`
    display: flex;
    justify-content: flex-end;
    gap: 10px;
    padding: 14px 24px;
    border-top: 1px solid var(--border);
  `,
  prompt: css`
    font-family:
      ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace !important;
    line-height: 1.7 !important;
  `,
  hint: css`
    margin: -10px 0 18px;
    padding-left: 11px;
    border-left: 2px solid color-mix(in srgb, var(--primary) 44%, transparent);
    color: var(--text-muted);
    font-size: 12px;
    line-height: 1.6;
  `,
}))

interface FormValues {
  name: string
  title: string
  description: string
  prompt: string
  changeNote: string
  enabled: boolean
}

interface PersonalityFormProps {
  open: boolean
  editing: Personality | null
  onClose: () => void
}

export default function PersonalityForm({
  open,
  editing,
  onClose,
}: PersonalityFormProps) {
  const { styles } = useStyles()
  const [form] = Form.useForm<FormValues>()
  const createPersonality = useCreatePersonality()
  const updatePersonality = useUpdatePersonality()
  const submitting = createPersonality.isPending || updatePersonality.isPending

  useEffect(() => {
    if (!open) return
    if (editing) {
      form.setFieldsValue({
        name: editing.name,
        title: editing.title,
        description: editing.description,
        prompt: editing.prompt,
        changeNote: '',
        enabled: editing.enabled,
      })
    } else {
      form.resetFields()
      form.setFieldsValue({ enabled: true })
    }
  }, [editing, form, open])

  const handleSubmit = async () => {
    const values = await form.validateFields()
    if (editing) {
      await updatePersonality.mutateAsync({
        name: editing.name,
        data: {
          title: values.title,
          description: values.description,
          prompt: values.prompt,
          enabled: values.enabled,
          changeNote: values.changeNote,
        },
      })
    } else {
      await createPersonality.mutateAsync({
        name: values.name,
        title: values.title,
        description: values.description,
        prompt: values.prompt,
        enabled: values.enabled,
      })
    }
    onClose()
  }

  return (
    <Modal
      open={open}
      footer={null}
      closable={false}
      width={760}
      onCancel={onClose}
      destroyOnHidden
      styles={{ body: { padding: 0 } }}
    >
      <div className={styles.head}>
        <div className={styles.title}>
          {editing ? `编辑人格 · v${editing.currentVersion}` : '新建人格'}
        </div>
        <button
          type="button"
          aria-label="关闭"
          className={styles.close}
          onClick={onClose}
        >
          <XIcon size={18} />
        </button>
      </div>
      <Form
        form={form}
        layout="vertical"
        requiredMark={false}
        className={styles.body}
      >
        <Form.Item
          label="人格标识"
          name="name"
          rules={identifierFormRules('人格标识')}
        >
          <Input
            disabled={Boolean(editing)}
            placeholder="例如: crisis-negotiator"
          />
        </Form.Item>
        <Form.Item
          label="人格名称"
          name="title"
          rules={[{ required: true, message: '请输入人格名称' }, { max: 128 }]}
        >
          <Input placeholder="例如：危机谈判者" />
        </Form.Item>
        <Form.Item
          label="一句话说明"
          name="description"
          rules={[{ max: 2000 }]}
        >
          <Input.TextArea rows={2} placeholder="这个人通常如何判断与行动" />
        </Form.Item>
        <Form.Item
          label="人格原稿"
          name="prompt"
          rules={[
            { required: true, message: '请输入人格提示词' },
            { max: 40000 },
          ]}
        >
          <Input.TextArea
            className={styles.prompt}
            rows={14}
            showCount
            maxLength={40000}
            placeholder={
              '写清楚这个人的价值排序、决策方式、沟通习惯、越级条件与盲点。\n\n不要只写“专业、负责”，要写他在利益冲突和证据不完整时会怎么选。'
            }
          />
        </Form.Item>
        <div className={styles.hint}>
          人格可以影响判断和表达，但不会获得新的工具、数据、通信或组织权限。
        </div>
        {editing && (
          <Form.Item
            label="版本说明"
            name="changeNote"
            tooltip="人格原稿发生变化时，将自动发布一个不可变的新版本"
          >
            <Input
              placeholder={`例如：补充越级汇报条件（将发布 v${editing.currentVersion + 1}）`}
              maxLength={255}
            />
          </Form.Item>
        )}
        <Form.Item
          label="允许 Agent 选用"
          name="enabled"
          valuePropName="checked"
        >
          <Switch />
        </Form.Item>
      </Form>
      <div className={styles.foot}>
        <Button onClick={onClose}>取消</Button>
        <PrimaryButton loading={submitting} onClick={handleSubmit}>
          {editing ? '保存并发布' : '创建人格'}
        </PrimaryButton>
      </div>
    </Modal>
  )
}
