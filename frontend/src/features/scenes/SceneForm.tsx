import { useEffect } from 'react'
import { Modal, Form, Input, Select, Switch, Tooltip, Button } from 'antd'
import { X } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import PrimaryButton from '@/components/PrimaryButton'
import type { Scene } from '@/api/scenes'
import { useAgents } from '@/queries/useAgents'
import { useCreateScene, useUpdateScene } from '@/queries/useScenes'
import { identifierFormRules } from '@/utils/identifier'

const useStyles = createStyles(({ css }) => ({
  head: css`
    display: flex; justify-content: space-between; align-items: center;
    padding: 18px 24px; border-bottom: 1px solid color-mix(in srgb, var(--foreground) 5%, transparent);
  `,
  title: css`
    font-size: 18px; font-weight: 600; color: var(--text); letter-spacing: -0.02em;
  `,
  closeBtn: css`
    width: 32px; height: 32px; display: flex; align-items: center; justify-content: center;
    border: none; background: var(--ink-subtle); border-radius: 4px;
    color: var(--text-tertiary); cursor: pointer; transition: all 0.15s;
    &:hover { background: var(--ink-light); color: var(--text); }
  `,
  body: css`
    padding: 20px 24px 8px; max-height: 60vh; overflow-y: auto;
  `,
  section: css`
    font-size: 11px; font-weight: 600; color: var(--text-muted);
    text-transform: uppercase; letter-spacing: 0.05em; margin-bottom: 14px;
  `,
  foot: css`
    display: flex; justify-content: flex-end; gap: 10px;
    padding: 14px 24px; border-top: 1px solid color-mix(in srgb, var(--foreground) 5%, transparent);
  `
}))

interface SceneFormProps {
  open: boolean
  editingScene: Scene | null
  onClose: () => void
}

interface FormValues {
  name: string
  agentId: number | null
  title: string
  titleEn: string
  prompt: string
  promptEn: string
  enabled: boolean
}

export default function SceneForm({ open, editingScene, onClose }: SceneFormProps) {
  const { styles } = useStyles()
  const [form] = Form.useForm<FormValues>()
  const { data: agents = [] } = useAgents()
  const createScene = useCreateScene()
  const updateScene = useUpdateScene()
  const submitting = createScene.isPending || updateScene.isPending

  useEffect(() => {
    if (open) {
      if (editingScene) {
        const matchedAgent = agents.find((a) => a.name === editingScene.agent)
        form.setFieldsValue({
          name: editingScene.name,
          agentId: matchedAgent?.id ?? null,
          title: editingScene.title || '',
          titleEn: editingScene.titleEn || '',
          prompt: editingScene.prompt || '',
          promptEn: editingScene.promptEn || '',
          enabled: editingScene.enabled
        })
      } else {
        form.resetFields()
        form.setFieldsValue({ enabled: true })
      }
    }
  }, [open, editingScene, form, agents])

  const handleSubmit = async () => {
    const values = await form.validateFields()
    if (editingScene) {
      await updateScene.mutateAsync({
        name: editingScene.name,
        data: {
          agentId: values.agentId ?? undefined,
          title: values.title,
          titleEn: values.titleEn,
          prompt: values.prompt,
          promptEn: values.promptEn,
          enabled: values.enabled
        }
      })
    } else {
      await createScene.mutateAsync({
        name: values.name,
        agentId: values.agentId!,
        title: values.title,
        titleEn: values.titleEn,
        prompt: values.prompt,
        promptEn: values.promptEn
      })
    }
    onClose()
  }

  const agentOptions = agents.map((a) => ({
    label: a.config?.title?.zh || a.config?.title?.en || a.name,
    value: a.id
  }))

  return (
    <Modal
      open={open}
      onCancel={onClose}
      footer={null}
      closable={false}
      width={600}
      styles={{ body: { padding: 0 } }}
      title={null}
      destroyOnHidden
    >
      <div className={styles.head}>
        <div className={styles.title}>{editingScene ? '编辑场景' : '新建场景'}</div>
        <button type="button" className={styles.closeBtn} onClick={onClose}>
          <X size={18} />
        </button>
      </div>

      <Form form={form} layout="vertical" className={styles.body} requiredMark={false}>
        <div className={styles.section}>基本信息</div>
        <Form.Item label="场景标识" name="name" rules={identifierFormRules('场景标识')}>
          <Input placeholder="e.g. default-scene" disabled={!!editingScene} />
        </Form.Item>
        <Form.Item label="关联 Agent" name="agentId" rules={[{ required: true, message: '请选择关联 Agent' }]}>
          <Select
            placeholder="选择关联的 Agent"
            showSearch
            optionFilterProp="label"
            options={agentOptions}
          />
        </Form.Item>

        <Form.Item label="场景名称" name="title">
          <Input placeholder="场景展示名称" />
        </Form.Item>
        <Form.Item label="Scene Name (EN)" name="titleEn">
          <Input placeholder="Scene display name" />
        </Form.Item>

        <div className={styles.section} style={{ marginTop: 20 }}>提示词配置</div>
        <Form.Item label="提示词" name="prompt">
          <Input.TextArea placeholder="输入该场景的提示词，定义 Agent 的行为和角色" rows={4} />
        </Form.Item>
        <Form.Item label="Prompt (EN)" name="promptEn">
          <Input.TextArea placeholder="Define the agent's behavior and role" rows={4} />
        </Form.Item>
        <Form.Item label="启用状态" name="enabled" valuePropName="checked">
          <Switch />
        </Form.Item>
      </Form>

      <div className={styles.foot}>
        <Button onClick={onClose}>取消</Button>
        <PrimaryButton onClick={handleSubmit} loading={submitting}>
          {editingScene ? '更新' : '创建'}
        </PrimaryButton>
      </div>
    </Modal>
  )
}

// Keep Tooltip import for potential future use (prompt preview in table)
void Tooltip
