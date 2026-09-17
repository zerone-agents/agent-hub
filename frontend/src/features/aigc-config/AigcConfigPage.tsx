import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import { Button, Form, Input, Popconfirm, Spin, Tag, Typography } from 'antd'
import { createStyles } from 'antd-style'
import { PlusIcon } from '@phosphor-icons/react'
import PrimaryButton from '@/components/PrimaryButton'
import {
  useAigcConfig,
  useSaveAigcConfig,
  useRotateAigcKey,
  useClearAigcConfig
} from '@/queries/useAigcConfig'
import { tokens as tk } from '@/styles/tokens'

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
    font-size: ${tk.text3xl};
    font-weight: 700;
    color: ${tk.text};
    letter-spacing: -0.03em;
  `,
  pageSub: css`
    margin-top: 4px;
    margin-bottom: 32px;
    font-size: ${tk.textBase};
    color: ${tk.textTertiary};
  `,
  statusCard: css`
    margin-top: 32px;
    padding: 20px;
    border: 1px solid color-mix(in srgb, var(--foreground) 8%, transparent);
    border-radius: ${tk.radiusSm}px;
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
    font-size: ${tk.textSm};
    color: ${tk.textTertiary};
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
  const { t } = useTranslation()
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
      <div className={styles.pageTitle}>{t('aigcConfig.pageTitle')}</div>
      <div className={styles.pageSub}>
        {t('aigcConfig.pageSub')}
      </div>

      <Form
        form={form}
        layout="vertical"
        onFinish={(values) => save.mutateAsync(values)}
      >
        <Form.Item
          name="uscc"
          label={t('aigcConfig.usccLabel')}
          rules={[
            { required: true, message: t('aigcConfig.usccRequired') },
            { pattern: USCC_PATTERN, message: t('aigcConfig.usccPattern') }
          ]}
        >
          <Input placeholder={t('aigcConfig.usccPlaceholder')} maxLength={18} />
        </Form.Item>
        <Form.Item
          name="companyName"
          label={t('aigcConfig.companyLabel')}
          rules={[{ required: true, whitespace: true, message: t('aigcConfig.companyRequired') }]}
        >
          <Input placeholder={t('aigcConfig.companyPlaceholder')} />
        </Form.Item>
        <PrimaryButton icon={<PlusIcon size={16} weight="bold" />} htmlType="submit" loading={save.isPending}>
          {t('aigcConfig.save')}
        </PrimaryButton>
      </Form>

      {data?.configured && (
        <div className={styles.statusCard}>
          <div className={styles.statusRow}>
            <span className={styles.statusLabel}>{t('aigcConfig.producerCode')}</span>
            <Typography.Text copyable code>
              {data.contentProducer}
            </Typography.Text>
          </div>
          <div className={styles.statusRow}>
            <span className={styles.statusLabel}>{t('aigcConfig.signingKey')}</span>
            {data.signingKeyConfigured ? (
              <Tag color="green">{t('aigcConfig.keyConfigured')}</Tag>
            ) : (
              <Tag>{t('aigcConfig.keyMissing')}</Tag>
            )}
          </div>
          <div className={styles.statusRow}>
            <span className={styles.statusLabel}>{t('aigcConfig.modelCodes')}</span>
            <span>
              {t('aigcConfig.modelCodesHint1')}
              <Link to="/providers">{t('aigcConfig.providersLink')}</Link>
              {t('aigcConfig.modelCodesHint2')}
            </span>
          </div>
          <div className={styles.actions}>
            <Popconfirm
              title={t('aigcConfig.regenerateTitle')}
              description={t('aigcConfig.regenerateDesc')}
              okText={t('aigcConfig.regenerate')}
              okButtonProps={{ danger: true }}
              cancelText={t('common.cancel')}
              onConfirm={() => { rotate.mutate(); }}
            >
              <Button danger>{t('aigcConfig.regenerateKey')}</Button>
            </Popconfirm>
            <Popconfirm
              title={t('aigcConfig.clearTitle')}
              description={t('aigcConfig.clearDesc')}
              okText={t('aigcConfig.clear')}
              okButtonProps={{ danger: true }}
              cancelText={t('common.cancel')}
              onConfirm={() => { clear.mutate(); }}
            >
              <Button danger type="text">
                {t('aigcConfig.clearConfig')}
              </Button>
            </Popconfirm>
          </div>
        </div>
      )}
    </div>
  )
}
