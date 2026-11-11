import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Alert, Button, Descriptions, Space, Spin, Tag } from 'antd'
import {
  memoryApi,
  memoryApplicationKey,
  type MemoryApplication
} from '@/api/memory'
import { parseApiError } from '@/api/client'
import { formatMemoryCapacity } from './nativeMemoryView'

export default function MemoryApplicationConfiguration({
  application,
  queryPrefix
}: {
  application: MemoryApplication
  queryPrefix: readonly string[]
}) {
  const { t } = useTranslation()
  const config = useQuery({
    queryKey: [
      ...queryPrefix,
      'native-config',
      memoryApplicationKey(application)
    ],
    queryFn: ({ signal }) =>
      memoryApi.nativeConfig(
        {
          tenantId: application.tenantId,
          applicationId: application.applicationId
        },
        signal
      ),
    enabled: application.nativeConfigurationSupported === true,
    gcTime: 0
  })
  const value = config.data
  return (
    <div>
      <div className="memory-native-help">
        {t('memory.configurationReadOnlyHint')}
      </div>
      <div className="memory-config-grid">
        <section className="memory-config-section">
          <h4>{t('memory.applicationContract')}</h4>
          <Descriptions
            column={1}
            size="small"
            items={[
              {
                key: 'app',
                label: t('memory.application'),
                children: application.applicationId
              },
              {
                key: 'policy',
                label: t('memory.policyVersion'),
                children: application.policyVersion
              },
              {
                key: 'mode',
                label: t('memory.memoryMode'),
                children: t(
                  application.automaticExtractionSupported
                    ? 'memory.autoMode'
                    : 'memory.ledgerMode'
                )
              },
              {
                key: 'types',
                label: t('memory.enabledMemoryTypes'),
                children: (
                  <Space size={[4, 4]} wrap>
                    {application.memoryTypes?.map((type) => (
                      <Tag key={type}>
                        {t('memory.nativeTypes.' + type, type)}
                      </Tag>
                    ))}
                  </Space>
                )
              },
              {
                key: 'context',
                label: t('memory.contextBudget'),
                children: application.nativeMaxContextChars
                  ? t('memory.contextBudgetValue', {
                      chars: application.nativeMaxContextChars,
                      count: application.nativeDefaultMessages
                    })
                  : '—'
              },
              {
                key: 'management',
                label: t('memory.managementCapabilities'),
                children: t(
                  application.nativeStatusSupported
                    ? application.nativeForgetSupported
                      ? 'memory.statusManagementWithForget'
                      : 'memory.statusManagementEnabled'
                    : 'memory.statusManagementReadOnly'
                )
              }
            ]}
          />
        </section>
        <section className="memory-config-section">
          <h4>{t('memory.nativeConfiguration')}</h4>
          {!application.nativeConfigurationSupported ? (
            <div className="memory-native-help">
              {t('memory.configurationUnavailable')}
            </div>
          ) : config.isPending ? (
            <Spin />
          ) : config.isError ? (
            <Alert
              type="error"
              showIcon
              title={parseApiError(config.error)}
              action={
                <Button
                  onClick={() => {
                    void config.refetch()
                  }}
                >
                  {t('memory.retry')}
                </Button>
              }
            />
          ) : (
            <Descriptions
              column={1}
              size="small"
              items={[
                {
                  key: 'name',
                  label: t('memory.bankName'),
                  children: value?.name ?? '—'
                },
                {
                  key: 'description',
                  label: t('memory.configurationDescription'),
                  children:
                    value?.description === ''
                      ? '—'
                      : (value?.description ?? '—')
                },
                {
                  key: 'chat',
                  label: t('memory.chatModelReference'),
                  children: value?.chatModel ?? '—'
                },
                {
                  key: 'embedding',
                  label: t('memory.embeddingModelReference'),
                  children: value?.embeddingModel ?? '—'
                },
                {
                  key: 'storage',
                  label: t('memory.storageBackend'),
                  children: value?.storageType ?? '—'
                },
                {
                  key: 'capacity',
                  label: t('memory.capacityLimit'),
                  children:
                    value?.capacityBytes === undefined
                      ? '—'
                      : formatMemoryCapacity(value.capacityBytes)
                },
                {
                  key: 'permissions',
                  label: t('memory.nativePermissions'),
                  children: value?.permissions ?? '—'
                },
                {
                  key: 'temperature',
                  label: t('memory.extractionTemperature'),
                  children: value?.temperature ?? '—'
                },
                {
                  key: 'policy',
                  label: t('memory.capacityPolicy'),
                  children: value?.forgettingPolicy ?? '—'
                },
                {
                  key: 'created',
                  label: t('memory.configurationCreatedAt'),
                  children: value?.createdAt ?? '—'
                }
              ]}
            />
          )}
        </section>
      </div>
      <div className="memory-native-help">
        {t('memory.capacityAndPolicyHint')}
      </div>
    </div>
  )
}
