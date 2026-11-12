import { useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  Alert,
  Button,
  Form,
  Input,
  Modal,
  Select,
  Space,
  Typography
} from 'antd'
import { useTranslation } from 'react-i18next'
import { isAxiosError } from 'axios'
import {
  memoryApi,
  type CreateMemoryApplication,
  type MemoryApplication,
  type MemoryApplicationModel,
  type MemoryApplicationReceipt
} from '@/api/memory'
import { parseApiError } from '@/api/client'
import PrimaryButton from '@/components/PrimaryButton'

type Fields = Omit<CreateMemoryApplication, 'operationId'>

export default function CreateMemoryApplicationModal({
  queryPrefix,
  onChanged,
  onClose
}: {
  queryPrefix: readonly string[]
  onChanged: (application: MemoryApplication) => void
  onClose: () => void
}) {
  const { t } = useTranslation()
  const [form] = Form.useForm<Fields>()
  const [request, setRequest] = useState<CreateMemoryApplication>()
  const [receipt, setReceipt] = useState<MemoryApplicationReceipt>()
  const [error, setError] = useState<unknown>()
  const [busy, setBusy] = useState(false)
  const submitting = useRef(false)
  const models = useQuery({
    queryKey: [...queryPrefix, 'application-models'],
    queryFn: ({ signal }) => memoryApi.applicationModels(signal),
    retry: false,
    gcTime: 0
  })
  const complete = receipt?.application.status === 'active'
  const submit = async (fields?: Fields) => {
    if (submitting.current) return
    const input =
      request ?? (fields && { ...fields, operationId: crypto.randomUUID() })
    if (!input) return
    submitting.current = true
    setRequest(input)
    setBusy(true)
    setError(undefined)
    try {
      const result = await memoryApi.createApplication(input)
      setReceipt(result)
      onChanged(result.application)
    } catch (cause) {
      setError(cause)
      if (
        isAxiosError(cause) &&
        [400, 409].includes(cause.response?.status ?? 0)
      ) {
        setRequest(undefined)
      }
    } finally {
      submitting.current = false
      setBusy(false)
    }
  }
  const options = (rows: MemoryApplicationModel[] = []) =>
    rows.map((model) => ({
      value: model.fullId,
      label: `${model.name} · ${model.instanceName ?? model.factory} (${model.fullId})`
    }))
  return (
    <Modal
      open
      title={t('memory.createApplication')}
      onCancel={onClose}
      closable={!busy}
      mask={{ closable: false }}
      keyboard={!busy}
      width={560}
      footer={
        <Space>
          <Button disabled={busy} onClick={onClose}>
            {t(receipt ? 'memory.creationDone' : 'common.cancel')}
          </Button>
          {!complete && (
            <PrimaryButton
              aria-label={t(
                request ? 'memory.checkCreation' : 'memory.createApplication'
              )}
              loading={busy}
              disabled={
                !request &&
                (!models.data?.chat.length || !models.data.embedding.length)
              }
              onClick={() => {
                if (request) void submit()
                else form.submit()
              }}
            >
              {t(request ? 'memory.checkCreation' : 'memory.createApplication')}
            </PrimaryButton>
          )}
        </Space>
      }
    >
      {models.isError && !request && (
        <Alert
          type="error"
          showIcon
          title={parseApiError(models.error)}
          action={
            <Button
              onClick={() => {
                void models.refetch()
              }}
            >
              {t('memory.retry')}
            </Button>
          }
        />
      )}
      {models.data &&
        (!models.data.chat.length || !models.data.embedding.length) &&
        !request && (
          <Alert
            type="warning"
            showIcon
            title={t('memory.noCreationModels')}
            action={
              <Button
                onClick={() => {
                  void models.refetch()
                }}
              >
                {t('memory.refresh')}
              </Button>
            }
          />
        )}
      {error != null && (
        <Alert
          type="error"
          showIcon
          title={parseApiError(error)}
          description={request ? t('memory.creationUnconfirmed') : undefined}
        />
      )}
      {receipt ? (
        <Space orientation="vertical" size={16} style={{ width: '100%' }}>
          <Alert
            showIcon
            type={complete ? 'success' : 'warning'}
            title={t(
              complete ? 'memory.applicationCreated' : 'memory.creationPending'
            )}
            description={
              complete
                ? receipt.application.name
                : t('memory.creationUnconfirmed')
            }
          />
          <Typography.Text>
            {t('memory.application')}: {receipt.application.applicationId}
          </Typography.Text>
          <Typography.Text>
            {t('memory.applicationTenant')}: {receipt.application.tenantId}
          </Typography.Text>
          <div>
            <Typography.Paragraph>
              {t('memory.applicationTokenHint')}
            </Typography.Paragraph>
            <Input.Password
              aria-label={t('memory.applicationToken')}
              value={receipt.applicationToken}
              readOnly
            />
            <Typography.Paragraph
              copyable={{ text: receipt.applicationToken }}
              style={{ marginTop: 8 }}
            >
              {t('memory.copyApplicationToken')}
            </Typography.Paragraph>
          </div>
        </Space>
      ) : (
        <Form
          form={form}
          layout="vertical"
          disabled={!!request}
          onFinish={(fields) => {
            void submit(fields)
          }}
          style={{ marginTop: 16 }}
        >
          <Typography.Paragraph type="secondary">
            {t('memory.createApplicationHint')}
          </Typography.Paragraph>
          <Form.Item
            name="name"
            label={t('memory.applicationName')}
            rules={[{ required: true, whitespace: true }]}
          >
            <Input maxLength={64} />
          </Form.Item>
          <Form.Item
            name="applicationId"
            label={t('memory.applicationId')}
            extra={t('memory.applicationIdHint')}
            rules={[
              { required: true },
              {
                pattern: /^[A-Za-z0-9_.:-]{1,128}$/,
                message: t('memory.applicationIdHint')
              }
            ]}
          >
            <Input maxLength={128} />
          </Form.Item>
          <Form.Item
            name="chatModel"
            label={t('memory.extractionModel')}
            rules={[{ required: true }]}
          >
            <Select
              showSearch={{ optionFilterProp: 'label' }}
              loading={models.isPending}
              options={options(models.data?.chat)}
            />
          </Form.Item>
          <Form.Item
            name="embeddingModel"
            label={t('memory.embeddingModel')}
            rules={[{ required: true }]}
          >
            <Select
              showSearch={{ optionFilterProp: 'label' }}
              loading={models.isPending}
              options={options(models.data?.embedding)}
            />
          </Form.Item>
          <Form.Item
            name="description"
            label={t('memory.configurationDescription')}
          >
            <Input maxLength={1024} />
          </Form.Item>
        </Form>
      )}
    </Modal>
  )
}
