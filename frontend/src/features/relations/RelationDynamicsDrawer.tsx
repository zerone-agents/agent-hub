import { useEffect } from 'react'
import { Drawer, Empty, Form, Input, Segmented, Select, Spin, Tag, Timeline } from 'antd'
import { createStyles } from 'antd-style'
import dayjs from 'dayjs'
import type { AgentRelation, RecordAgentRelationEventPayload, RelationEventVisibility } from '@/api/agent-relations'
import PrimaryButton from '@/components/PrimaryButton'
import { useAgentRelationEvents, useRecordAgentRelationEvent } from '@/queries/useAgentRelations'
import { tokens as t } from '@/styles/tokens'
import { RELATION_EVENTS, STANCES } from './relationOptions'

const useStyles = createStyles(({ css }) => ({
  scoreCard: css`
    display: grid;
    grid-template-columns: auto 1fr;
    gap: 14px;
    align-items: center;
    margin-bottom: 20px;
    padding: 16px;
    border: 1px solid color-mix(in srgb, var(--foreground) 8%, transparent);
    border-radius: ${t.radius}px;
    background: ${t.surfaceHover};
  `,
  score: css`
    min-width: 64px;
    color: ${t.text};
    font-family: ${t.fontMono};
    font-size: 30px;
    font-weight: 750;
    line-height: 1;
  `,
  edge: css`
    color: ${t.text};
    font-size: ${t.textSm};
    font-weight: 650;
  `,
  hint: css`
    margin-top: 4px;
    color: ${t.textMuted};
    font-size: ${t.textXs};
    line-height: 1.45;
  `,
  section: css`
    margin: 20px 0 12px;
    color: ${t.textMuted};
    font-size: ${t.textXs};
    font-weight: 700;
    letter-spacing: 0.08em;
    text-transform: uppercase;
  `,
  eventTitle: css`
    display: flex;
    align-items: center;
    gap: 8px;
    color: ${t.text};
    font-size: ${t.textSm};
    font-weight: 650;
  `,
  eventMeta: css`
    margin-top: 3px;
    color: ${t.textMuted};
    font-family: ${t.fontMono};
    font-size: 11px;
  `,
  eventReason: css`
    margin-top: 6px;
    color: ${t.textSecondary};
    font-size: ${t.textSm};
    line-height: 1.55;
  `,
  form: css`
    padding: 14px;
    border: 1px solid color-mix(in srgb, var(--foreground) 8%, transparent);
    border-radius: ${t.radius}px;
    background: ${t.surface};
  `,
}))

interface RelationDynamicsDrawerProps {
  open: boolean
  relation: AgentRelation | null
  canWrite: boolean
  onClose: () => void
}

type EventFormValues = RecordAgentRelationEventPayload

export default function RelationDynamicsDrawer({ open, relation, canWrite, onClose }: RelationDynamicsDrawerProps) {
  const { styles } = useStyles()
  const [form] = Form.useForm<EventFormValues>()
  const { data: events = [], isLoading } = useAgentRelationEvents(open ? (relation?.id ?? null) : null)
  const recordEvent = useRecordAgentRelationEvent()

  useEffect(() => {
    if (!open) return
    form.setFieldsValue({
      eventType: 'task_completed',
      severity: 1,
      reason: '',
      visibility: 'private',
    })
  }, [form, open, relation?.id])

  if (!relation) return null
  const stance = STANCES.find((item) => item.value === relation.stance)

  const submit = async () => {
    const values = await form.validateFields()
    await recordEvent.mutateAsync({
      id: relation.id,
      data: {
        ...values,
        sourceKind: 'admin',
        reason: values.reason.trim(),
      },
    })
    form.setFieldValue('reason', '')
  }

  return (
    <Drawer open={open} onClose={onClose} size={520} title="关系动态" destroyOnHidden>
      <div className={styles.scoreCard}>
        <div className={styles.score}>
          {relation.relationshipScore > 0 ? '+' : ''}
          {relation.relationshipScore}
        </div>
        <div>
          <div className={styles.edge}>
            {relation.sourceAgentName} → {relation.targetAgentName}
          </div>
          <div style={{ marginTop: 5 }}>
            <Tag color={stance?.color}>{stance?.label ?? relation.stance}</Tag>
          </div>
          <div className={styles.hint}>这是发起方对接收方的单向看法，反向关系独立变化。</div>
        </div>
      </div>

      {canWrite && (
        <>
          <div className={styles.section}>记录测试事件</div>
          <Form form={form} layout="vertical" requiredMark={false} className={styles.form}>
            <Form.Item label="发生了什么" name="eventType" rules={[{ required: true }]}>
              <Select
                options={RELATION_EVENTS.map((event) => ({
                  value: event.value,
                  label: `${event.label}（基础 ${event.delta > 0 ? '+' : ''}${event.delta}）`,
                }))}
              />
            </Form.Item>
            <Form.Item label="严重程度" name="severity" rules={[{ required: true }]}>
              <Segmented
                block
                options={[
                  { value: 1, label: '轻微 ×1' },
                  { value: 2, label: '明显 ×1.5' },
                  { value: 3, label: '严重 ×2' },
                ]}
              />
            </Form.Item>
            <Form.Item
              label="事件事实"
              name="reason"
              rules={[
                {
                  required: true,
                  whitespace: true,
                  message: '请写明导致变化的事实',
                },
                { max: 2000 },
              ]}
            >
              <Input.TextArea
                rows={3}
                showCount
                maxLength={2000}
                placeholder="例如：董事会上把共同成果全部归到自己名下。"
              />
            </Form.Item>
            <Form.Item label="谁知道这件事" name="visibility" rules={[{ required: true }]}>
              <Select
                options={[
                  {
                    value: 'private' satisfies RelationEventVisibility,
                    label: '仅发起方感知',
                  },
                  {
                    value: 'participants' satisfies RelationEventVisibility,
                    label: '双方知道',
                  },
                  {
                    value: 'public' satisfies RelationEventVisibility,
                    label: '公开事件',
                  },
                ]}
              />
            </Form.Item>
            <PrimaryButton loading={recordEvent.isPending} onClick={() => void submit()}>
              应用事件
            </PrimaryButton>
          </Form>
        </>
      )}

      <div className={styles.section}>变化记录</div>
      {isLoading ? (
        <div style={{ display: 'grid', placeItems: 'center', padding: 40 }}>
          <Spin />
        </div>
      ) : events.length === 0 ? (
        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="还没有关系事件" />
      ) : (
        <Timeline
          items={events.map((event) => {
            const definition = RELATION_EVENTS.find((item) => item.value === event.eventType)
            const eventLabel =
              event.eventType === 'admin_stance_reset' ? '管理员重设立场' : (definition?.label ?? event.eventType)
            return {
              color: event.delta >= 0 ? 'green' : 'red',
              children: (
                <div>
                  <div className={styles.eventTitle}>
                    <span>{eventLabel}</span>
                    <Tag color={event.delta >= 0 ? 'green' : 'red'}>
                      {event.delta > 0 ? '+' : ''}
                      {event.delta}
                    </Tag>
                  </div>
                  <div className={styles.eventMeta}>
                    {event.scoreBefore} → {event.scoreAfter} · {dayjs(event.occurredAt).format('YYYY-MM-DD HH:mm')}
                  </div>
                  {event.reason && <div className={styles.eventReason}>{event.reason}</div>}
                </div>
              ),
            }
          })}
        />
      )}
    </Drawer>
  )
}
