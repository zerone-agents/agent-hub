import { useEffect } from 'react'
import { Link } from 'react-router-dom'
import { Button, Form, Input, Popconfirm, Spin, Tag, Typography } from 'antd'
import { createStyles } from 'antd-style'
import { Plus } from '@phosphor-icons/react'
import PrimaryButton from '@/components/PrimaryButton'
import {
  useAigcConfig,
  useSaveAigcConfig,
  useRotateAigcKey,
  useClearAigcConfig
} from '@/queries/useAigcConfig'
import { tokens as t } from '@/styles/tokens'

const USCC_PATTERN = /^[0-9A-HJ-NPQRTUWXY]{18}$/

const useStyles = createStyles(({ css }) => ({
  page: css`
    max-width: 640px;
    margin: 0 auto;
    padding: 32px 24px;
    animation: pageIn 0.35s ease;
    @keyframes pageIn {
      from { opacity: 0; transform: translateY(6px); }
      to { opacity: 1; transform: translateY(0); }
    }
  `,
  pageTitle: css`
    font-size: ${t.text3xl};
    font-weight: 700;
    color: ${t.text};
    letter-spacing: -0.03em;
  `,
  pageSub: css`
    margin-top: 4px;
    margin-bottom: 32px;
    font-size: ${t.textBase};
    color: ${t.textTertiary};
  `,
  statusCard: css`
    margin-top: 32px;
    padding: 20px;
    border: 1px solid color-mix(in srgb, var(--foreground) 8%, transparent);
    border-radius: ${t.radiusSm}px;
    display: flex;
    flex-direction: column;
    gap: 12px;
  `,
  statusRow: css`
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
  `,
  statusLabel: css`
    font-size: ${t.textSm};
    color: ${t.textTertiary};
    min-width: 112px;
  `,
  actions: css`
    display: flex;
    gap: 12px;
    margin-top: 8px;
  `,
  loadingWrap: css`
    display: flex;
    justify-content: center;
    padding: 80px 0;
  `
}))

export default function AigcConfigPage() {
  const { styles } = useStyles()
  const { data, isLoading } = useAigcConfig()
  const save = useSaveAigcConfig()
  const rotate = useRotateAigcKey()
  const clear = useClearAigcConfig()
  const [form] = Form.useForm<{ uscc: string; companyName: string }>()

  useEffect(() => {
    if (data?.configured) {
      form.setFieldsValue({ uscc: data.uscc, companyName: data.companyName })
    }
  }, [data, form])

  if (isLoading) {
    return (
      <div className={styles.loadingWrap}>
        <Spin size="large" />
      </div>
    )
  }

  return (
    <div className={styles.page}>
      <div className={styles.pageTitle}>AIGC 标识配置</div>
      <div className={styles.pageSub}>
        依据 GB 45438-2025，配置后部署 Agent 将自动携带 AI 生成内容标识；签名密钥由后端自动生成并保管。
      </div>

      <Form
        form={form}
        layout="vertical"
        onFinish={(values) => save.mutateAsync(values)}
      >
        <Form.Item
          name="uscc"
          label="统一社会信用代码"
          rules={[
            { required: true, message: '请输入 18 位统一社会信用代码' },
            { pattern: USCC_PATTERN, message: '须为 18 位数字与大写字母（不含 I/O/S/V/Z）' }
          ]}
        >
          <Input placeholder="18 位统一社会信用代码" maxLength={18} />
        </Form.Item>
        <Form.Item
          name="companyName"
          label="公司完整名称"
          rules={[{ required: true, whitespace: true, message: '请输入公司完整名称' }]}
        >
          <Input placeholder="与营业执照一致的公司全称" />
        </Form.Item>
        <PrimaryButton icon={<Plus size={16} weight="bold" />} htmlType="submit" loading={save.isPending}>
          保存配置
        </PrimaryButton>
      </Form>

      {data?.configured && (
        <div className={styles.statusCard}>
          <div className={styles.statusRow}>
            <span className={styles.statusLabel}>服务提供者编码</span>
            <Typography.Text copyable code>
              {data.contentProducer}
            </Typography.Text>
          </div>
          <div className={styles.statusRow}>
            <span className={styles.statusLabel}>签名密钥</span>
            {data.signingKeyConfigured ? (
              <Tag color="green">已配置（由后端保管）</Tag>
            ) : (
              <Tag>未配置</Tag>
            )}
          </div>
          <div className={styles.statusRow}>
            <span className={styles.statusLabel}>模型 AIGC 码</span>
            <span>
              模型 AIGC 码在
              <Link to="/providers">模型管理</Link>
              中按模型自动分配
            </span>
          </div>
          <div className={styles.actions}>
            <Popconfirm
              title="确认重新生成签名密钥？"
              description="重新生成后，历史内容的签名将无法用新密钥验签。"
              okText="重新生成"
              okButtonProps={{ danger: true }}
              cancelText="取消"
              onConfirm={() => { rotate.mutate(); }}
            >
              <Button danger>重新生成密钥</Button>
            </Popconfirm>
            <Popconfirm
              title="确认清除配置？"
              description="清除后部署 Agent 将不再携带 AIGC 标识。"
              okText="清除"
              okButtonProps={{ danger: true }}
              cancelText="取消"
              onConfirm={() => { clear.mutate(); }}
            >
              <Button danger type="text">
                清除配置
              </Button>
            </Popconfirm>
          </div>
        </div>
      )}
    </div>
  )
}
