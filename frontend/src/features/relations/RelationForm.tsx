import { useEffect } from 'react'
import { Alert, Button, Form, Input, Modal, Radio, Select, Switch } from 'antd'
import { ArrowRightIcon, XIcon } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import type {
  AgentRelation,
  AgentRelationCreatePayload,
  AgentRelationUpdatePayload,
  ContextPolicy,
  DeliveryPolicy,
  RelationAction,
  RelationStance,
  RelationType,
} from '@/api/agent-relations'
import type { Agent } from '@/api/agents'
import PrimaryButton from '@/components/PrimaryButton'
import {
  ACTIONS,
  CONTEXT_POLICIES,
  DEFAULT_ACTIONS,
  DELIVERY_POLICIES,
  RELATION_TYPES,
  STANCES,
} from './relationOptions'
import { useCreateAgentRelation, useUpdateAgentRelation } from '@/queries/useAgentRelations'
import { tokens as t } from '@/styles/tokens'

const useStyles = createStyles(({ css }) => ({
  head: css`
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 20px;
    padding: 20px 24px 16px;
    border-bottom: 1px solid color-mix(in srgb, var(--foreground) 7%, transparent);
  `,
  title: css`
    color: ${t.text};
    font-size: ${t.textLg};
    font-weight: 650;
    letter-spacing: -0.02em;
  `,
  subtitle: css`
    margin-top: 4px;
    color: ${t.textTertiary};
    font-size: ${t.textSm};
  `,
  closeButton: css`
    display: grid;
    width: 32px;
    height: 32px;
    flex: 0 0 auto;
    place-items: center;
    border: 0;
    border-radius: ${t.radiusSm}px;
    background: ${t.inkSubtle};
    color: ${t.textTertiary};
    cursor: pointer;
    &:hover {
      color: ${t.text};
      background: ${t.inkLight};
    }
    &:focus-visible {
      outline: 2px solid ${t.ink};
      outline-offset: 2px;
    }
  `,
  body: css`
    max-height: min(68vh, 720px);
    overflow-y: auto;
    padding: 20px 24px 4px;
  `,
  sectionTitle: css`
    margin: 2px 0 14px;
    color: ${t.textMuted};
    font-size: ${t.textXs};
    font-weight: 700;
    letter-spacing: 0.08em;
    text-transform: uppercase;
  `,
  grid: css`
    display: grid;
    grid-template-columns: 1fr 1fr;
    column-gap: 16px;
    @media (max-width: 640px) {
      grid-template-columns: 1fr;
    }
  `,
  asymmetryHint: css`
    grid-column: 2;
    margin-top: -17px;
    margin-bottom: 14px;
    color: ${t.textMuted};
    font-size: ${t.textXs};
    line-height: 1.5;
    @media (max-width: 640px) {
      grid-column: 1;
    }
  `,
  directionRow: css`
    display: grid;
    grid-template-columns: minmax(0, 1fr) 28px minmax(0, 1fr);
    align-items: center;
    gap: 8px;
  `,
  arrow: css`
    display: grid;
    place-items: center;
    color: ${t.textMuted};
  `,
  lockedEdge: css`
    display: grid;
    grid-template-columns: minmax(0, 1fr) 28px minmax(0, 1fr);
    align-items: center;
    gap: 8px;
    margin-bottom: 18px;
    padding: 12px;
    border: 1px solid color-mix(in srgb, var(--foreground) 9%, transparent);
    border-radius: ${t.radius}px;
    background: ${t.surfaceHover};
    color: ${t.textSecondary};
    font-size: ${t.textSm};
    font-weight: 600;
    & > span {
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
  `,
  foot: css`
    display: flex;
    justify-content: flex-end;
    gap: 10px;
    padding: 14px 24px;
    border-top: 1px solid color-mix(in srgb, var(--foreground) 7%, transparent);
  `,
}))

interface RelationFormProps {
  open: boolean
  editingRelation: AgentRelation | null
  agents: Agent[]
  onClose: () => void
}

interface FormValues {
  sourceAgentId: number | null
  targetAgentId: number | null
  direction: 'one_way' | 'two_way'
  scope: string
  relationType: RelationType
  stance: RelationStance
  allowedActions: RelationAction[]
  contextPolicy: ContextPolicy
  deliveryPolicy: DeliveryPolicy
  constraint: string
  enabled: boolean
}

function agentTitle(agent: Agent): string {
  return agent.config.title?.zh ?? agent.config.title?.en ?? agent.name
}

export default function RelationForm({ open, editingRelation, agents, onClose }: RelationFormProps) {
  const { styles } = useStyles()
  const [form] = Form.useForm<FormValues>()
  const sourceAgentId = Form.useWatch('sourceAgentId', form)
  const direction = Form.useWatch('direction', form)
  const relationType = Form.useWatch('relationType', form)
  const createRelation = useCreateAgentRelation()
  const updateRelation = useUpdateAgentRelation()
  const submitting = createRelation.isPending || updateRelation.isPending

  useEffect(() => {
    if (!open) return
    if (editingRelation) {
      form.setFieldsValue({
        sourceAgentId: editingRelation.sourceAgentId,
        targetAgentId: editingRelation.targetAgentId,
        direction: 'one_way',
        scope: editingRelation.scope,
        relationType: editingRelation.relationType,
        stance: editingRelation.stance,
        allowedActions: editingRelation.allowedActions,
        contextPolicy: editingRelation.contextPolicy,
        deliveryPolicy: editingRelation.deliveryPolicy,
        constraint: editingRelation.constraint,
        enabled: editingRelation.enabled,
      })
      return
    }
    form.resetFields()
    form.setFieldsValue({
      sourceAgentId: null,
      targetAgentId: null,
      direction: 'one_way',
      scope: 'global',
      relationType: 'peer',
      stance: 'neutral',
      allowedActions: DEFAULT_ACTIONS.peer,
      contextPolicy: 'summary_only',
      deliveryPolicy: 'async',
      constraint: '',
      enabled: true,
    })
  }, [editingRelation, form, open])

  const options = agents.map((agent) => ({
    label: `${agentTitle(agent)} · ${agent.name}`,
    value: agent.id,
  }))
  const targetOptions = options.filter((option) => option.value !== sourceAgentId)
  const supportsBidirectional = relationType === 'peer' || relationType === 'opponent' || relationType === 'external'

  const handleSubmit = async () => {
    const values = await form.validateFields()
    if (editingRelation) {
      const payload: AgentRelationUpdatePayload = {
        scope: values.scope.trim(),
        relationType: values.relationType,
        allowedActions: values.allowedActions,
        contextPolicy: values.contextPolicy,
        deliveryPolicy: values.deliveryPolicy,
        constraint: values.constraint.trim(),
        enabled: values.enabled,
      }
      if (values.stance !== editingRelation.stance) {
        payload.stance = values.stance
      }
      await updateRelation.mutateAsync({
        id: editingRelation.id,
        data: payload,
      })
    } else {
      if (values.sourceAgentId === null || values.targetAgentId === null) return
      const payload: AgentRelationCreatePayload = {
        sourceAgentId: values.sourceAgentId,
        targetAgentId: values.targetAgentId,
        scope: values.scope.trim(),
        relationType: values.relationType,
        stance: values.stance,
        allowedActions: values.allowedActions,
        contextPolicy: values.contextPolicy,
        deliveryPolicy: values.deliveryPolicy,
        constraint: values.constraint.trim(),
        enabled: values.enabled,
        bidirectional: values.direction === 'two_way',
      }
      await createRelation.mutateAsync(payload)
    }
    onClose()
  }

  return (
    <Modal
      open={open}
      onCancel={onClose}
      footer={null}
      closable={false}
      width={720}
      styles={{ body: { padding: 0 } }}
      destroyOnHidden
    >
      <div className={styles.head}>
        <div>
          <div className={styles.title}>{editingRelation ? '编辑有向关系' : '新建 Agent 关系'}</div>
          <div className={styles.subtitle}>
            {editingRelation
              ? '关系两端不可更换；需要换人时请新建一条关系。'
              : '先确定关系方向，再约定这条边允许发生什么。'}
          </div>
        </div>
        <button type="button" className={styles.closeButton} aria-label="关闭" onClick={onClose}>
          <XIcon size={18} />
        </button>
      </div>

      <Form form={form} layout="vertical" requiredMark={false} className={styles.body}>
        <div className={styles.sectionTitle}>关系两端</div>
        {editingRelation ? (
          <div className={styles.lockedEdge}>
            <span>{editingRelation.sourceAgentName}</span>
            <span className={styles.arrow}>
              <ArrowRightIcon size={16} />
            </span>
            <span>{editingRelation.targetAgentName}</span>
          </div>
        ) : (
          <>
            <div className={styles.directionRow}>
              <Form.Item
                label="发起方 Agent"
                name="sourceAgentId"
                rules={[{ required: true, message: '请选择发起方 Agent' }]}
              >
                <Select placeholder="谁发起沟通" showSearch={{ optionFilterProp: 'label' }} options={options} />
              </Form.Item>
              <span className={styles.arrow}>
                <ArrowRightIcon size={18} />
              </span>
              <Form.Item
                label="接收方 Agent"
                name="targetAgentId"
                dependencies={['sourceAgentId']}
                rules={[{ required: true, message: '请选择接收方 Agent' }]}
              >
                <Select placeholder="消息发给谁" showSearch={{ optionFilterProp: 'label' }} options={targetOptions} />
              </Form.Item>
            </div>
            <Form.Item label="方向" name="direction">
              <Radio.Group optionType="button" buttonStyle="solid">
                <Radio.Button value="one_way">单向 A → B</Radio.Button>
                <Radio.Button value="two_way" disabled={!supportsBidirectional}>
                  双向 A ⇄ B
                </Radio.Button>
              </Radio.Group>
            </Form.Item>
            {direction === 'two_way' && (
              <Alert
                type="info"
                showIcon
                title="将创建 A → B 与 B → A 两条独立关系，创建后可分别修改。"
                style={{ marginBottom: 18 }}
              />
            )}
          </>
        )}

        <div className={styles.sectionTitle}>协作约定</div>
        <div className={styles.grid}>
          <Form.Item
            label="关系范围"
            name="scope"
            tooltip="同一对 Agent 可在不同组织或项目范围内拥有不同关系"
            rules={[
              { required: true, message: '请输入关系范围' },
              {
                pattern: /^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,63}$/,
                message: '使用字母、数字、点、横线、下划线或冒号',
              },
            ]}
          >
            <Input placeholder="global" />
          </Form.Item>
          <Form.Item label="结构关系" name="relationType" rules={[{ required: true }]}>
            <Select
              options={RELATION_TYPES.map((item) => ({
                value: item.value,
                label: item.label,
                title: item.description,
              }))}
              onChange={(value: RelationType) => {
                if (!editingRelation) {
                  form.setFieldValue('allowedActions', DEFAULT_ACTIONS[value])
                  if (value !== 'peer' && value !== 'opponent' && value !== 'external') {
                    form.setFieldValue('direction', 'one_way')
                  }
                }
              }}
              optionRender={(option) => (
                <div>
                  <div>{option.label}</div>
                  <div style={{ color: t.textMuted, fontSize: t.textXs }}>{option.data.title}</div>
                </div>
              )}
            />
          </Form.Item>
          {!editingRelation && !supportsBidirectional && (
            <div className={styles.asymmetryHint}>
              非对称关系需要分别配置两个方向，避免把“下属”和“负责人”等语义错误镜像。
            </div>
          )}
          <Form.Item
            label={editingRelation ? '重设立场' : '初始立场'}
            name="stance"
            tooltip={
              editingRelation ? '修改后会把动态关系分值重设到该立场的起始值' : '创建后，事件将继续改变分值与当前立场'
            }
            rules={[{ required: true }]}
          >
            <Select options={STANCES.map(({ value, label }) => ({ value, label }))} />
          </Form.Item>
          <Form.Item label="投递方式" name="deliveryPolicy" rules={[{ required: true }]}>
            <Select options={DELIVERY_POLICIES} />
          </Form.Item>
        </div>
        <Form.Item
          label="允许动作"
          name="allowedActions"
          tooltip="MCP 执行通信时只能调用这里明确允许的动作"
          rules={[
            {
              required: true,
              type: 'array',
              min: 1,
              message: '请至少选择一个允许动作',
            },
          ]}
        >
          <Select mode="multiple" placeholder="选择该方向允许的动作" options={ACTIONS} />
        </Form.Item>
        <Form.Item label="上下文共享" name="contextPolicy" rules={[{ required: true }]}>
          <Radio.Group options={CONTEXT_POLICIES} />
        </Form.Item>
        <Form.Item
          label="补充约束"
          name="constraint"
          extra="例如：涉及公开声明时必须先提交法务 Agent 复核。"
          rules={[{ max: 2000, message: '最多 2000 个字符' }]}
        >
          <Input.TextArea rows={3} showCount maxLength={2000} placeholder="写下这条关系特有的边界和升级条件" />
        </Form.Item>
        <Form.Item label="启用关系" name="enabled" valuePropName="checked">
          <Switch />
        </Form.Item>
      </Form>

      <div className={styles.foot}>
        <Button onClick={onClose}>取消</Button>
        <PrimaryButton loading={submitting} onClick={() => void handleSubmit()}>
          {editingRelation ? '保存关系' : '创建关系'}
        </PrimaryButton>
      </div>
    </Modal>
  )
}
