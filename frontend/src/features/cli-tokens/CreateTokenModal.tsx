import { useState } from 'react'
import { Modal, Form, Input, Select, Alert, Button, message } from 'antd'
import { CheckOutlined } from '@ant-design/icons'
import { useTranslation } from 'react-i18next'
import { useCLITokens, useIssueCLIToken } from '@/queries/useCLITokens'
import { identifierFormRules } from '@/utils/identifier'
import { copyOrManual } from '@/utils/clipboard'
import PrimaryButton from '@/components/PrimaryButton'

// 有效期档位（天）。label 由组件内 t('cliTokens.ttlDays') 插值生成——
// 模块级常量拿不到 hook 的 t，保持 value-only。
const TTL_OPTIONS = [30, 90, 180, 365]

interface Props {
  open: boolean
  onClose: () => void
}

export default function CreateTokenModal({ open, onClose }: Props) {
  const { t } = useTranslation()
  const [form] = Form.useForm()
  const { data: existingTokens = [] } = useCLITokens()
  const issueToken = useIssueCLIToken()
  const [issuedToken, setIssuedToken] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)

  const existingNames = new Set(existingTokens.map((item) => item.name))

  const nameRules = [
    ...identifierFormRules(t('cliTokens.nameRuleLabel')),
    {
      validator: (_: unknown, value: string) => {
        if (value && existingNames.has(value)) {
          return Promise.reject(new Error(t('cliTokens.nameTaken')))
        }
        return Promise.resolve()
      }
    }
  ]

  const handleSubmit = async () => {
    try {
      const { name, ttlDays } = await form.validateFields() as { name: string; ttlDays: number }
      const result = await issueToken.mutateAsync({ name, ttlDays })
      setIssuedToken(result.token)
      form.resetFields()
      setCopied(false)
    } catch {
      // validation error or mutation error — handled by hook
    }
  }

  const handleDone = () => {
    setIssuedToken(null)
    setCopied(false)
    onClose()
  }

  const handleCancel = () => {
    if (issuedToken) {
      setIssuedToken(null)
      setCopied(false)
      onClose()
      return
    }
    form.resetFields()
    onClose()
  }

  return (
    <Modal
      title={issuedToken ? t('cliTokens.createdTitle') : t('cliTokens.create')}
      open={open}
      onCancel={handleCancel}
      width={680}
      footer={
        issuedToken ? (
          <PrimaryButton onClick={handleDone}>{t('cliTokens.saved')}</PrimaryButton>
        ) : (
          <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 10 }}>
            <Button onClick={handleCancel}>{t('common.cancel')}</Button>
            <PrimaryButton onClick={handleSubmit} loading={issueToken.isPending}>{t('cliTokens.createSubmit')}</PrimaryButton>
          </div>
        )
      }
      destroyOnHidden
    >
      {issuedToken ? (
        <div>
          <Alert
            type="warning"
            showIcon
            title={t('cliTokens.saveNowTitle')}
            description={t('cliTokens.saveNowDesc')}
            style={{ marginBottom: 16 }}
          />
          <div
            style={{
              background: '#f5f5f5',
              padding: '8px 12px',
              borderRadius: '4px',
              fontFamily: 'monospace',
              fontSize: '12px',
              wordBreak: 'break-all',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
              gap: '8px'
            }}
          >
            <code style={{ flex: 1, fontSize: '12px' }}>{issuedToken}</code>
            {copied ? (
              <span style={{ color: '#52c41a', fontSize: '12px', display: 'inline-flex', alignItems: 'center', gap: '4px', height: '27px', lineHeight: 1 }}>
                <CheckOutlined /> {t('cliTokens.copied')}
              </span>
            ) : (
              <Button
                type="link"
                size="small"
                onClick={() => {
                  void copyOrManual(issuedToken).then((result) => {
                    if (result === 'copied') setCopied(true)
                    else if (result === 'failed') message.error(t('cliTokens.copyFailed'))
                  })
                }}
              >
                {t('cliTokens.copy')}
              </Button>
            )}
          </div>
        </div>
      ) : (
        <Form form={form} layout="vertical" initialValues={{ ttlDays: 90 }}>
          <Form.Item name="name" label={t('cliTokens.nameLabel')} rules={nameRules}>
            <Input placeholder={t('cliTokens.namePlaceholder')} />
          </Form.Item>
          <Form.Item name="ttlDays" label={t('cliTokens.ttlLabel')}>
            <Select options={TTL_OPTIONS.map((days) => ({ label: t('cliTokens.ttlDays', { days }), value: days }))} />
          </Form.Item>
        </Form>
      )}
    </Modal>
  )
}
