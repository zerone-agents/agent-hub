// H7.5 预算管理页：预算列表 + 用量/限额进度条（>80% 黄、>100% 红）+
// 预算 CRUD + 告警规则配置 + 触发记录。
import { useState } from 'react'
import {
  Alert,
  Card,
  Form,
  Input,
  InputNumber,
  Modal,
  Popconfirm,
  Progress,
  Select,
  Space,
  Table,
  Tabs,
  Tag,
  message
} from 'antd'
import { PlusIcon, TrashIcon } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import dayjs from 'dayjs'
import PrimaryButton from '@/components/PrimaryButton'
import { parseApiError } from '@/api/client'
import type { UsageAlertRule, UsageBudget } from '@/api/usage'
import {
  useDeleteUsageAlert,
  useDeleteUsageBudget,
  useUpsertUsageAlert,
  useUpsertUsageBudget,
  useUsageAlertEvents,
  useUsageAlerts,
  useUsageBudgets
} from '@/queries/useUsage'
import { tokens as t } from '@/styles/tokens'

const useStyles = createStyles(({ css }) => ({
  page: css`
    width: 100%;
    max-width: 1200px;
    margin: 0 auto;
  `,
  header: css`
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 24px;
    margin-bottom: 20px;
  `,
  title: css`
    margin: 0;
    color: ${t.text};
    font-size: ${t.text2xl};
    font-weight: 650;
  `,
  subtitle: css`
    margin: 5px 0 0;
    color: ${t.textTertiary};
    font-size: ${t.textBase};
  `,
  section: css`
    margin-bottom: 16px;
  `,
  kindName: css`
    color: ${t.textSecondary};
    font-size: ${t.textSm};
  `
}))

const BUDGET_KINDS = [
  { value: 'model_call', label: '模型调用（次数）' },
  { value: 'extension_call', label: '扩展调用（次数）' },
  { value: 'storage', label: '存储（字节）' }
]

const RULE_LABELS: Record<string, string> = {
  budget_pct: '预算用量百分比',
  error_rate_spike: '错误率突增',
  health_red: '系统健康转红'
}

function budgetProgress(b: UsageBudget) {
  const pct = Math.round(b.usedPct * 10) / 10
  const strokeColor = b.usedPct > 100 ? 'var(--destructive)' : b.usedPct > 80 ? 'var(--warning)' : 'var(--primary)'
  return (
    <div style={{ minWidth: 220 }}>
      <Progress
        percent={pct}
        size="small"
        strokeColor={strokeColor}
        status={b.usedPct > 100 ? 'exception' : 'normal'}
      />
      <span style={{ color: t.textTertiary, fontSize: t.textXs }}>
        {b.usedValue.toLocaleString()} / {b.limitValue.toLocaleString()}
      </span>
    </div>
  )
}

export default function UsageBudgetsPage() {
  const { styles } = useStyles()
  const budgets = useUsageBudgets()
  const upsertBudget = useUpsertUsageBudget()
  const deleteBudget = useDeleteUsageBudget()
  const alerts = useUsageAlerts()
  const upsertAlert = useUpsertUsageAlert()
  const deleteAlert = useDeleteUsageAlert()
  const events = useUsageAlertEvents(50)

  const [budgetModal, setBudgetModal] = useState(false)
  const [alertModal, setAlertModal] = useState(false)
  const [form] = Form.useForm()
  const [alertForm] = Form.useForm()

  const submitBudget = async () => {
    const values = await form.validateFields()
    try {
      await upsertBudget.mutateAsync(values)
      message.success('预算已保存')
      setBudgetModal(false)
      form.resetFields()
    } catch (e) {
      message.error(parseApiError(e))
    }
  }

  const submitAlert = async () => {
    const values = await alertForm.validateFields()
    try {
      await upsertAlert.mutateAsync(values)
      message.success('告警规则已保存')
      setAlertModal(false)
      alertForm.resetFields()
    } catch (e) {
      message.error(parseApiError(e))
    }
  }

  return (
    <div className={styles.page}>
      <div className={styles.header}>
        <div>
          <h1 className={styles.title}>预算与告警</h1>
          <p className={styles.subtitle}>软预算：超限不阻断，只标记并告警 · 进度超 80% 变黄、超 100% 变红</p>
        </div>
        <Space>
          <PrimaryButton icon={<PlusIcon size={16} />} onClick={() => { setAlertModal(true); }}>
            新增告警规则
          </PrimaryButton>
          <PrimaryButton icon={<PlusIcon size={16} />} onClick={() => { setBudgetModal(true); }}>
            新增预算
          </PrimaryButton>
        </Space>
      </div>

      <Tabs
        items={[
          {
            key: 'budgets',
            label: '预算',
            children: (
              <Card className={styles.section}>
                <Table<UsageBudget>
                  rowKey="id"
                  loading={budgets.isLoading}
                  dataSource={budgets.data?.items ?? []}
                  pagination={false}
                  locale={{ emptyText: '尚未配置预算' }}
                  columns={[
                    { title: '维度', dataIndex: 'kind', width: 160, render: (v: string) => BUDGET_KINDS.find((k) => k.value === v)?.label ?? v },
                    { title: '周期', dataIndex: 'period', width: 100, render: (v: string) => (v === 'monthly' ? '每月' : '每天') },
                    {
                      title: '用量 / 限额',
                      key: 'progress',
                      render: (_, b) => budgetProgress(b)
                    },
                    { title: '告警阈值', dataIndex: 'alertThresholdPct', width: 100, render: (v: number) => `${v}%` },
                    {
                      title: '操作',
                      key: 'actions',
                      width: 90,
                      render: (_, b) => (
                        <Popconfirm
                          title="删除该预算？"
                          onConfirm={async () => {
                            try {
                              await deleteBudget.mutateAsync(b.id)
                              message.success('已删除')
                            } catch (e) {
                              message.error(parseApiError(e))
                            }
                          }}
                        >
                          <Tag color="red" style={{ cursor: 'pointer' }}>
                            <TrashIcon size={13} /> 删除
                          </Tag>
                        </Popconfirm>
                      )
                    }
                  ]}
                />
              </Card>
            )
          },
          {
            key: 'alerts',
            label: '告警规则',
            children: (
              <Card className={styles.section}>
                <Table<UsageAlertRule>
                  rowKey="id"
                  loading={alerts.isLoading}
                  dataSource={alerts.data?.items ?? []}
                  pagination={false}
                  locale={{ emptyText: '尚未配置告警规则' }}
                  columns={[
                    { title: '规则', dataIndex: 'rule', render: (v: string) => RULE_LABELS[v] ?? v },
                    { title: '通道', dataIndex: 'channel', width: 100, render: (v: string) => (v === 'webhook' ? 'Webhook' : '日志') },
                    { title: '阈值', dataIndex: 'threshold', width: 100 },
                    {
                      title: '最近触发',
                      dataIndex: 'lastFiredAt',
                      width: 200,
                      render: (v?: string) => (v ? dayjs(v).format('YYYY-MM-DD HH:mm:ss') : '—')
                    },
                    {
                      title: '操作',
                      key: 'actions',
                      width: 90,
                      render: (_, a) => (
                        <Popconfirm
                          title="删除该规则？"
                          onConfirm={async () => {
                            try {
                              await deleteAlert.mutateAsync(a.id)
                              message.success('已删除')
                            } catch (e) {
                              message.error(parseApiError(e))
                            }
                          }}
                        >
                          <Tag color="red" style={{ cursor: 'pointer' }}>
                            <TrashIcon size={13} /> 删除
                          </Tag>
                        </Popconfirm>
                      )
                    }
                  ]}
                />
              </Card>
            )
          },
          {
            key: 'events',
            label: '触发记录',
            children: (
              <Card className={styles.section}>
                <Table
                  rowKey="id"
                  size="small"
                  loading={events.isLoading}
                  dataSource={events.data?.items ?? []}
                  pagination={{ pageSize: 15 }}
                  locale={{ emptyText: '暂无触发记录' }}
                  columns={[
                    { title: '时间', dataIndex: 'createdAt', width: 200, render: (v: string) => dayjs(v).format('YYYY-MM-DD HH:mm:ss') },
                    { title: '规则', dataIndex: 'rule', width: 160, render: (v: string) => RULE_LABELS[v] ?? v },
                    { title: '内容', dataIndex: 'message' }
                  ]}
                />
              </Card>
            )
          }
        ]}
      />

      <Modal title="新增 / 更新预算" open={budgetModal} onOk={submitBudget} onCancel={() => { setBudgetModal(false); }} confirmLoading={upsertBudget.isPending}>
        <Alert type="info" showIcon message="同维度 + 同周期的预算已存在时将覆盖更新" style={{ marginBottom: 16 }} />
        <Form form={form} layout="vertical">
          <Form.Item name="kind" label="预算维度" rules={[{ required: true, message: '请选择维度' }]}>
            <Select options={BUDGET_KINDS} placeholder="选择维度" />
          </Form.Item>
          <Form.Item name="period" label="周期" rules={[{ required: true, message: '请选择周期' }]}>
            <Select
              options={[
                { value: 'daily', label: '每天' },
                { value: 'monthly', label: '每月' }
              ]}
              placeholder="选择周期"
            />
          </Form.Item>
          <Form.Item name="limitValue" label="限额（正整数）" rules={[{ required: true, message: '请输入限额' }]}>
            <InputNumber min={1} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="alertThresholdPct" label="告警阈值（%）" initialValue={80} rules={[{ required: true, message: '请输入阈值' }]}>
            <InputNumber min={0} max={100} style={{ width: '100%' }} />
          </Form.Item>
        </Form>
      </Modal>

      <Modal title="新增 / 更新告警规则" open={alertModal} onOk={submitAlert} onCancel={() => { setAlertModal(false); }} confirmLoading={upsertAlert.isPending}>
        <Form form={alertForm} layout="vertical">
          <Form.Item name="rule" label="规则" rules={[{ required: true, message: '请选择规则' }]}>
            <Select
              options={[
                { value: 'budget_pct', label: '预算用量百分比' },
                { value: 'error_rate_spike', label: '错误率突增' },
                { value: 'health_red', label: '系统健康转红' }
              ]}
              placeholder="选择规则"
            />
          </Form.Item>
          <Form.Item name="channel" label="通道" initialValue="log" rules={[{ required: true }]}>
            <Select
              options={[
                { value: 'log', label: '日志' },
                { value: 'webhook', label: 'Webhook' }
              ]}
            />
          </Form.Item>
          <Form.Item noStyle shouldUpdate={(a, b) => a.channel !== b.channel}>
            {({ getFieldValue }) =>
              getFieldValue('channel') === 'webhook' ? (
                <Form.Item name="webhookUrl" label="Webhook URL" rules={[{ required: true, message: '请输入 Webhook URL' }]}>
                  <Input placeholder="https://..." />
                </Form.Item>
              ) : null
            }
          </Form.Item>
          <Form.Item name="threshold" label="阈值（0-100）" initialValue={80} rules={[{ required: true, message: '请输入阈值' }]}>
            <InputNumber min={0} max={100} style={{ width: '100%' }} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}
