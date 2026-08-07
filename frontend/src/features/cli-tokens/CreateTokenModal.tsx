import { useState } from 'react'
import { Modal, Form, Input, Select, Alert, Button } from 'antd'
import { CheckOutlined } from '@ant-design/icons'
import { useCLITokens, useIssueCLIToken } from '@/queries/useCLITokens'
import { identifierFormRules } from '@/utils/identifier'
import PrimaryButton from '@/components/PrimaryButton'

const TTL_OPTIONS = [
  { label: '30 天', value: 30 },
  { label: '90 天', value: 90 },
  { label: '180 天', value: 180 },
  { label: '365 天', value: 365 }
]

interface Props {
  open: boolean
  onClose: () => void
}

export default function CreateTokenModal({ open, onClose }: Props) {
  const [form] = Form.useForm()
  const { data: existingTokens = [] } = useCLITokens()
  const issueToken = useIssueCLIToken()
  const [issuedToken, setIssuedToken] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)

  const existingNames = new Set(existingTokens.map((t) => t.name))

  const nameRules = [
    ...identifierFormRules('Token 名称'),
    {
      validator: (_: unknown, value: string) => {
        if (value && existingNames.has(value)) {
          return Promise.reject(new Error('该名称已存在，请使用其他名称'))
        }
        return Promise.resolve()
      }
    }
  ]

  const handleSubmit = async () => {
    try {
      const { name, ttlDays } = await form.validateFields() as { name: string; ttlDays: number }
      const result = await issueToken.mutateAsync({ name, ttlDays })
      setIssuedToken(result.token ?? '')
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
      title={issuedToken ? 'Token 已创建' : '创建 CLI Token'}
      open={open}
      onCancel={handleCancel}
      width={680}
      footer={
        issuedToken ? (
          <PrimaryButton onClick={handleDone}>我已保存</PrimaryButton>
        ) : (
          <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 10 }}>
            <Button onClick={handleCancel}>取消</Button>
            <PrimaryButton onClick={handleSubmit} loading={issueToken.isPending}>创建</PrimaryButton>
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
            message="请立即保存此 Token"
            description="关闭此窗口后将无法再次查看。请将 Token 安全存储。"
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
                <CheckOutlined /> 已复制
              </span>
            ) : (
              <Button
                type="link"
                size="small"
                onClick={() => {
                  void navigator.clipboard.writeText(issuedToken).then(() => { setCopied(true); })
                }}
              >
                复制
              </Button>
            )}
          </div>
        </div>
      ) : (
        <Form form={form} layout="vertical" initialValues={{ ttlDays: 90 }}>
          <Form.Item name="name" label="名称" rules={nameRules}>
            <Input placeholder="例如：my-macbook" />
          </Form.Item>
          <Form.Item name="ttlDays" label="有效期">
            <Select options={TTL_OPTIONS} />
          </Form.Item>
        </Form>
      )}
    </Modal>
  )
}
