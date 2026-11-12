import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Alert,
  Button,
  Empty,
  Input,
  Modal,
  Result,
  Select,
  Space,
  Spin,
  Tag,
  Typography,
  Drawer,
  Popover,
  Tooltip,
  message
} from 'antd'
import type { ColumnsType } from 'antd/es/table'
import {
  ArrowsClockwiseIcon,
  PencilSimpleIcon,
  TrashIcon,
  UserCircleIcon,
  MagnifyingGlassIcon,
  SlidersHorizontalIcon,
  CaretDownIcon,
  PlusIcon
} from '@phosphor-icons/react'
import { isAxiosError } from 'axios'
import {
  memoryApi,
  memoryApplicationKey,
  memoryScopeKey,
  type MemoryFact,
  type MemoryApplication,
  type MemoryOperation,
  type MemorySubject
} from '@/api/memory'
import { parseApiError } from '@/api/client'
import NativeMemoryPanel from './NativeMemoryPanel'
import MemoryApplicationConfiguration from './MemoryApplicationConfiguration'
import CreateMemoryApplicationModal from './CreateMemoryApplicationModal'
import './memory-management.css'
import { useAuthStore } from '@/stores/auth'
import { useCanWrite } from '@/hooks/useCanWrite'
import { createStyles } from 'antd-style'
import { tokens as tk } from '@/styles/tokens'
import BorderedTable from '@/components/BorderedTable'
import PrimaryButton, {
  usePrimaryButtonStyle
} from '@/components/PrimaryButton'

const useManagementStyles = createStyles(({ css }) => ({
  page: css`
    animation: pageIn 0.35s ease;
    @media (prefers-reduced-motion: reduce) {
      animation: none;
    }
    @keyframes pageIn {
      from {
        opacity: 0;
        transform: translateY(6px);
      }
      to {
        opacity: 1;
        transform: translateY(0);
      }
    }
  `,
  pageHead: css`
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    margin-bottom: 24px;
    @media (max-width: 768px) {
      flex-direction: column;
      gap: 16px;
    }
  `,
  pageTitle: css`
    font-size: ${tk.text3xl};
    font-weight: 700;
    color: ${tk.text};
    letter-spacing: -0.03em;
    line-height: 1.15;
  `,
  pageSub: css`
    margin-top: 4px;
    font-size: ${tk.textBase};
    color: ${tk.textTertiary};
  `
}))

const panelStyle = {
  border: '1px solid var(--border)',
  borderRadius: tk.radiusSm,
  padding: 20,
  background: 'var(--card)',
  marginBottom: 20
}

function ErrorNotice({ error, retry }: { error: unknown; retry: () => void }) {
  const { t } = useTranslation()
  return (
    <Alert
      type="error"
      showIcon
      title={parseApiError(error)}
      action={<Button onClick={retry}>{t('memory.retry')}</Button>}
      style={{ marginBottom: 16 }}
    />
  )
}

function NoRecords({
  title,
  hint,
  compact = false
}: {
  title: string
  hint: string
  compact?: boolean
}) {
  return (
    <Empty
      image={Empty.PRESENTED_IMAGE_SIMPLE}
      description={
        <>
          <Typography.Paragraph
            style={{
              marginBottom: compact ? 4 : 6,
              fontSize: compact ? undefined : tk.textLg,
              fontWeight: 600,
              color: tk.text
            }}
          >
            {title}
          </Typography.Paragraph>
          <Typography.Text
            style={{ fontSize: tk.textSm, color: tk.textTertiary }}
          >
            {hint}
          </Typography.Text>
        </>
      }
      style={{ padding: compact ? '24px 8px' : '80px 0', margin: 0 }}
    />
  )
}

export default function MemoryManagementPage() {
  const { t } = useTranslation()
  const canManage = useCanWrite()
  const userId = useAuthStore((state) => state.user?.id)
  const role = useAuthStore((state) => state.user?.role)
  if (!canManage) return <Result status="403" title={t('memory.forbidden')} />
  // Account changes remount the sensitive workspace, including local mutations.
  return (
    <MemoryWorkspace
      key={JSON.stringify([userId, role])}
      principalId={userId ?? ''}
    />
  )
}

function MemoryWorkspace({ principalId }: { principalId: string }) {
  const { styles } = useManagementStyles()
  const { t } = useTranslation()
  const qc = useQueryClient()
  // A new visit never reuses sensitive data from an earlier authenticated
  // workspace, even when an identity provider reuses the same user ID.
  const [workspaceId] = useState(() => crypto.randomUUID())
  const queryPrefix = ['admin', 'memory', principalId, workspaceId] as const
  const [applicationChoice, setApplicationChoice] = useState<string>()
  const [subjectChoice, setSubjectChoice] = useState<string>()
  const [createOpen, setCreateOpen] = useState(false)
  const applications = useQuery({
    queryKey: [...queryPrefix, 'applications'],
    queryFn: ({ signal }) => memoryApi.applications(signal),
    retry: (failureCount, error) =>
      !(
        isAxiosError<{ code?: string } | undefined>(error) &&
        ['memory_disabled', 'memory_not_configured'].includes(
          error.response?.data?.code ?? ''
        )
      ) && failureCount < 1,
    gcTime: 0
  })
  const application =
    applications.data?.find(
      (app) => memoryApplicationKey(app) === applicationChoice
    ) ??
    applications.data?.find((app) => app.automaticExtractionSupported) ??
    applications.data?.at(0)
  const applicationKey = application ? memoryApplicationKey(application) : ''
  const provisioning =
    application?.status === 'unknown' || application?.status === 'provisioning'
  const subjects = useQuery({
    queryKey: [...queryPrefix, 'subjects', applicationKey],
    queryFn: ({ signal }) => {
      if (!application) throw new Error('Memory application is required')
      return memoryApi.subjects(
        {
          tenantId: application.tenantId,
          applicationId: application.applicationId
        },
        signal
      )
    },
    enabled: !!application && !provisioning,
    gcTime: 0
  })
  const entries = subjects.data?.subjects ?? []
  const subject =
    entries.find((entry) => memoryScopeKey(entry.scope) === subjectChoice) ??
    entries.at(0)
  const [subjectSearch, setSubjectSearch] = useState('')
  const [configOpen, setConfigOpen] = useState(false)
  const visibleSubjects = entries.filter((entry) =>
    (entry.scope.userId + ' ' + entry.scope.accountLifecycleId)
      .toLowerCase()
      .includes(subjectSearch.trim().toLowerCase())
  )
  const refresh = () => {
    void qc.invalidateQueries({ queryKey: queryPrefix })
  }
  const checkProvisioning = useMutation({
    mutationFn: () =>
      memoryApi.applicationProvisioning(application?.applicationId ?? ''),
    onSuccess: refresh
  })
  const configuration = application ? (
    <MemoryApplicationConfiguration
      application={application}
      queryPrefix={queryPrefix}
    />
  ) : null

  const [subjectOpen, setSubjectOpen] = useState(false)
  const accountPicker = (
    <div className="memory-account-menu">
      <div className="memory-subjects-header">
        <div className="memory-subjects-title">
          <span>{t('memory.subjectsTitle')}</span>
          <Tag>{entries.length}</Tag>
        </div>
        <Input
          aria-label={t('memory.searchSubjects')}
          placeholder={t('memory.searchSubjectsPlaceholder')}
          prefix={
            <MagnifyingGlassIcon size={14} color="var(--text-muted, #999)" />
          }
          allowClear
          value={subjectSearch}
          onChange={(event) => {
            setSubjectSearch(event.target.value)
          }}
        />
      </div>
      <div className="memory-subject-list">
        {subjects.isError ? (
          <ErrorNotice
            error={subjects.error}
            retry={() => {
              void subjects.refetch()
            }}
          />
        ) : subjects.isPending ? (
          <Spin aria-label={t('common.loading')} />
        ) : !entries.length ? (
          <NoRecords
            compact
            title={t('memory.noSubjects')}
            hint={t('memory.noSubjectsHint')}
          />
        ) : !visibleSubjects.length ? (
          <Empty
            image={Empty.PRESENTED_IMAGE_SIMPLE}
            description={t('memory.noMatchingSubjects')}
          />
        ) : (
          visibleSubjects.map((entry) => (
            <button
              key={memoryScopeKey(entry.scope)}
              className="memory-subject-button"
              type="button"
              aria-pressed={
                !!subject &&
                memoryScopeKey(subject.scope) === memoryScopeKey(entry.scope)
              }
              aria-label={t('memory.subjectOption', {
                user: entry.scope.userId,
                lifecycle: entry.scope.accountLifecycleId
              })}
              onClick={() => {
                setSubjectChoice(memoryScopeKey(entry.scope))
                setSubjectOpen(false)
              }}
            >
              <UserCircleIcon size={22} className="memory-subject-icon" />
              <span className="memory-subject-identity">
                <span
                  className="memory-subject-user"
                  title={entry.scope.userId}
                >
                  {entry.scope.userId}
                </span>
                <span
                  className="memory-subject-life"
                  title={entry.scope.accountLifecycleId}
                >
                  {entry.scope.accountLifecycleId}
                </span>
                <span className="memory-subject-state">
                  {t('memory.states.' + entry.state, entry.state)}
                </span>
              </span>
            </button>
          ))
        )}
      </div>
    </div>
  )
  const toolbar = (
    <div className="memory-toolbar">
      <Select
        aria-label={t('memory.application')}
        placeholder={t('memory.selectApplication')}
        className="memory-toolbar-application"
        value={applicationKey || undefined}
        options={applications.data?.map((app) => ({
          value: memoryApplicationKey(app),
          label: t('memory.applicationOption', {
            application: app.name
              ? `${app.name} (${app.applicationId})`
              : app.applicationId,
            tenant: app.tenantId
          })
        }))}
        onChange={(value) => {
          checkProvisioning.reset()
          setApplicationChoice(value)
          setSubjectChoice(undefined)
          setSubjectSearch('')
          setSubjectOpen(false)
          setConfigOpen(false)
        }}
      />
      <Popover
        open={subjectOpen}
        onOpenChange={setSubjectOpen}
        content={accountPicker}
        trigger="click"
        placement="bottomLeft"
        arrow={false}
      >
        <Tooltip
          title={
            subject
              ? subject.scope.userId + ' · ' + subject.scope.accountLifecycleId
              : undefined
          }
        >
          <Button
            className="memory-toolbar-account"
            disabled={!application || provisioning}
            aria-label={t('memory.chooseAccount')}
            icon={<UserCircleIcon size={18} />}
          >
            <span>{subject?.scope.userId ?? t('memory.selectSubject')}</span>
            <CaretDownIcon size={13} />
          </Button>
        </Tooltip>
      </Popover>
      {application && (
        <Tooltip
          title={t(
            provisioning
              ? 'memory.creationUnconfirmed'
              : application.automaticExtractionSupported
                ? 'memory.managementAutoHint'
                : 'memory.managementLedgerHint'
          )}
        >
          <Tag
            className="memory-toolbar-mode"
            color={
              application.automaticExtractionSupported ? 'blue' : 'default'
            }
          >
            {t(
              provisioning
                ? 'memory.creationPending'
                : application.automaticExtractionSupported
                  ? 'memory.autoMode'
                  : 'memory.ledgerMode'
            )}
          </Tag>
        </Tooltip>
      )}
      <Tooltip
        title={application?.memoryTypes
          ?.map((type) => t('memory.nativeTypes.' + type, type))
          .join(' / ')}
      >
        <span className="memory-toolbar-types">
          {application?.memoryTypes?.length
            ? t('memory.memoryTypeCount', {
                count: application.memoryTypes.length
              })
            : ''}
        </span>
      </Tooltip>
      <Tooltip title={t('memory.refresh')}>
        <Button
          type="text"
          aria-label={t('memory.refresh')}
          icon={<ArrowsClockwiseIcon size={17} />}
          onClick={refresh}
        />
      </Tooltip>
      <Tooltip title={t('memory.applicationConfiguration')}>
        <Button
          type="text"
          aria-label={t('memory.applicationConfiguration')}
          disabled={!application || provisioning}
          icon={<SlidersHorizontalIcon size={17} />}
          onClick={() => {
            setConfigOpen(true)
          }}
        />
      </Tooltip>
      <PrimaryButton
        icon={<PlusIcon size={16} />}
        disabled={!applications.isSuccess}
        onClick={() => {
          setCreateOpen(true)
        }}
      >
        {t('memory.createApplication')}
      </PrimaryButton>
    </div>
  )
  return (
    <div className={styles.page + ' memory-management-shell'}>
      <div className={styles.pageHead + ' memory-page-head'}>
        <div>
          <div className={styles.pageTitle}>{t('memory.title')}</div>
          <div className={styles.pageSub}>{t('memory.managementSubtitle')}</div>
        </div>
        {toolbar}
      </div>
      {applications.isError ? (
        <ErrorNotice
          error={applications.error}
          retry={() => {
            void applications.refetch()
          }}
        />
      ) : applications.isPending ? (
        <div className="memory-loading" role="status">
          <Spin aria-label={t('common.loading')} />
        </div>
      ) : !applications.data.length ? (
        <NoRecords
          title={t('memory.noApplications')}
          hint={t('memory.noApplicationsHint')}
        />
      ) : provisioning ? (
        <Alert
          type="warning"
          showIcon
          title={t('memory.creationPending')}
          description={
            checkProvisioning.isError
              ? parseApiError(checkProvisioning.error)
              : t('memory.creationUnconfirmed')
          }
          action={
            <Button
              loading={checkProvisioning.isPending}
              onClick={() => {
                checkProvisioning.mutate()
              }}
            >
              {t('memory.checkCreation')}
            </Button>
          }
        />
      ) : (
        <>
          <div
            className={
              'memory-management-content' +
              (application?.automaticExtractionSupported
                ? ''
                : ' memory-legacy-content')
            }
          >
            {!!subjects.data?.missingScopeCount && (
              <Alert
                type="info"
                showIcon
                title={t('memory.missingScopes', {
                  count: subjects.data.missingScopeCount
                })}
                style={{ marginBottom: 8 }}
              />
            )}
            {subjects.data?.limited && (
              <Alert
                type="warning"
                showIcon
                title={t('memory.limitedSubjects', {
                  limit: subjects.data.limit
                })}
                style={{ marginBottom: 8 }}
              />
            )}
            {subject && application ? (
              application.automaticExtractionSupported ? (
                <NativeMemoryPanel
                  key={memoryScopeKey(subject.scope)}
                  subject={subject}
                  queryPrefix={queryPrefix}
                  application={application}
                  onSelectAccount={() => {
                    setSubjectOpen(true)
                  }}
                />
              ) : (
                <SubjectMemory
                  key={memoryScopeKey(subject.scope)}
                  subject={subject}
                  queryPrefix={queryPrefix}
                  capabilities={application}
                />
              )
            ) : subjects.isPending ? (
              <div className="memory-loading" role="status">
                <Spin aria-label={t('common.loading')} />
              </div>
            ) : (
              <div className="memory-account-empty">
                <NoRecords
                  title={t(
                    entries.length
                      ? 'memory.selectSubject'
                      : 'memory.noSubjects'
                  )}
                  hint={t(
                    entries.length
                      ? 'memory.selectSubjectHint'
                      : 'memory.noSubjectsHint'
                  )}
                />
                <Button
                  onClick={() => {
                    setSubjectOpen(true)
                  }}
                >
                  {t('memory.switchAccount')}
                </Button>
              </div>
            )}
          </div>
          <Drawer
            title={t('memory.applicationConfiguration')}
            open={configOpen}
            onClose={() => {
              setConfigOpen(false)
            }}
            size={720}
            destroyOnHidden
          >
            {configuration}
          </Drawer>
        </>
      )}
      {createOpen && (
        <CreateMemoryApplicationModal
          queryPrefix={queryPrefix}
          onChanged={(app) => {
            setApplicationChoice(memoryApplicationKey(app))
            setSubjectChoice(undefined)
            refresh()
          }}
          onClose={() => {
            setCreateOpen(false)
          }}
        />
      )}
    </div>
  )
}

interface ActionDialog {
  action: 'edit' | 'delete' | 'revoke'
  fact: MemoryFact
  content: string
  operationId: string
}

function operationId() {
  return 'admin-memory-' + crypto.randomUUID()
}

function SubjectMemory({
  subject,
  queryPrefix,
  capabilities
}: {
  subject: MemorySubject
  queryPrefix: readonly string[]
  capabilities: MemoryApplication
}) {
  const { t, i18n } = useTranslation()
  const qc = useQueryClient()
  const primaryStyle = usePrimaryButtonStyle()
  const scope = subject.scope
  const key = memoryScopeKey(scope)
  const active = subject.state === 'active'
  const mounted = useRef(true)
  const recallEpoch = useRef(0)
  useEffect(() => {
    mounted.current = true
    return () => {
      mounted.current = false
    }
  }, [])
  const [dialog, setDialog] = useState<ActionDialog | null>(null)
  const [receipt, setReceipt] = useState<{
    operationId: string
    operation?: MemoryOperation
    startedAt: number
  } | null>(null)
  const [query, setQuery] = useState('')
  const [mutationError, setMutationError] = useState<string>()
  const [recallResult, setRecallResult] = useState<Awaited<
    ReturnType<typeof memoryApi.context>
  > | null>(null)
  const facts = useQuery({
    queryKey: [...queryPrefix, 'facts', key],
    queryFn: ({ signal }) => memoryApi.facts(scope, signal),
    enabled: active,
    refetchInterval: (state) =>
      state.state.data?.some((fact) => fact.syncStatus !== 'confirmed')
        ? 1500
        : false,
    gcTime: 0
  })
  const operation = useQuery({
    queryKey: [...queryPrefix, 'operation', key, receipt?.operationId],
    queryFn: ({ signal }) => {
      if (!receipt) throw new Error('Memory operation is required')
      return memoryApi.operation(scope, receipt.operationId, signal)
    },
    enabled: !!receipt,
    gcTime: 0,
    refetchInterval: (state) => {
      if (!receipt || Date.now() - receipt.startedAt > 30_000) return false
      return state.state.data?.status === 'completed' ? false : 1000
    }
  })
  const recall = useMutation({
    mutationFn: async ({
      question,
      epoch
    }: {
      question: string
      epoch: number
    }) => ({
      context: await memoryApi.context({ scope, query: question }),
      epoch
    }),
    onSuccess: (result) => {
      if (mounted.current && result.epoch === recallEpoch.current)
        setRecallResult(result.context)
    },
    onError: (_error, request) => {
      if (mounted.current && request.epoch === recallEpoch.current)
        setRecallResult(null)
    }
  })
  const action = useMutation({
    mutationFn: async (request: ActionDialog) => {
      if (
        (request.action === 'edit' &&
          capabilities.correctionSupported !== true) ||
        (request.action === 'delete' &&
          capabilities.physicalDeleteSupported !== true) ||
        (request.action === 'revoke' &&
          capabilities.consentRevokeSupported !== true)
      ) {
        throw new Error(t('memory.unsupported'))
      }
      const common = {
        scope,
        operationId: request.operationId,
        expectedVersion: request.fact.version
      }
      if (request.action === 'edit')
        return {
          action: request.action,
          fact: await memoryApi.update(request.fact.factId, {
            ...common,
            content: request.content
          })
        }
      if (request.action === 'delete')
        return {
          action: request.action,
          operation: await memoryApi.delete(request.fact.factId, common)
        }
      return {
        action: request.action,
        operation: await memoryApi.revoke({
          scope,
          operationId: request.operationId,
          consentRef: request.fact.consentRef ?? ''
        })
      }
    },
    retry: false,
    onMutate: () => {
      recallEpoch.current++
      setRecallResult(null)
      recall.reset()
    },
    onSuccess: (result) => {
      void qc.invalidateQueries({ queryKey: [...queryPrefix, 'facts', key] })
      if (!mounted.current) return
      setDialog(null)
      setMutationError(undefined)
      setRecallResult(null)
      recall.reset()
      if ('operation' in result && result.operation)
        setReceipt({
          operationId: result.operation.operationId,
          operation: result.operation,
          startedAt: Date.now()
        })
      void message.success(
        t(
          result.action === 'edit'
            ? 'memory.saved'
            : result.action === 'delete'
              ? 'memory.deleted'
              : 'memory.revoked'
        )
      )
    },
    onError: (error, request) => {
      void qc.invalidateQueries({ queryKey: [...queryPrefix, 'facts', key] })
      if (!mounted.current) return
      setMutationError(parseApiError(error))
      setRecallResult(null)
      recall.reset()
      // A lost response is not a cleanup receipt. Recover only by reading the
      // original operation ID; retries keep that same idempotency key.
      if (
        request.action !== 'edit' &&
        isAxiosError(error) &&
        (!error.response || error.response.status >= 500)
      ) {
        setReceipt({ operationId: request.operationId, startedAt: Date.now() })
      }
    }
  })
  const open = (kind: ActionDialog['action'], fact: MemoryFact) => {
    setMutationError(undefined)
    setDialog({
      action: kind,
      fact,
      content: fact.content,
      operationId: operationId()
    })
  }
  const columns: ColumnsType<MemoryFact> = [
    {
      title: t('memory.columns.content'),
      dataIndex: 'content',
      key: 'content',
      render: (content: string, fact) => (
        <div style={{ maxWidth: 600 }}>
          <div
            style={{
              whiteSpace: 'pre-wrap',
              overflowWrap: 'anywhere',
              marginBottom: 6
            }}
          >
            {content}
          </div>
          <Tag>{t('memory.categories.' + fact.category, fact.category)}</Tag>
        </div>
      )
    },
    {
      title: t('memory.columns.source'),
      dataIndex: 'source',
      key: 'source',
      width: 140,
      render: (source?: string) => (source ? <Tag>{source}</Tag> : '—')
    },
    {
      title: t('memory.columns.version'),
      dataIndex: 'version',
      key: 'version',
      width: 80,
      render: (version: number) => `v${version}`
    },
    {
      title: t('memory.columns.sync'),
      dataIndex: 'syncStatus',
      key: 'sync',
      width: 112,
      render: (status: string) => (
        <Tag color={status === 'confirmed' ? 'green' : 'gold'}>
          {t('memory.sync.' + status, status)}
        </Tag>
      )
    },
    {
      title: t('memory.columns.updated'),
      dataIndex: 'updatedAt',
      key: 'updatedAt',
      width: 170,
      render: (value: string) =>
        new Date(value).toLocaleString(
          i18n.language === 'en' ? 'en-US' : 'zh-CN'
        )
    },
    {
      title: t('memory.columns.actions'),
      key: 'actions',
      width: 240,
      render: (_, fact) => (
        <Space size={0} wrap>
          <Button
            type="text"
            icon={<PencilSimpleIcon size={14} />}
            disabled={
              !active ||
              capabilities.correctionSupported !== true ||
              action.isPending ||
              !Number.isSafeInteger(fact.version)
            }
            onClick={() => {
              open('edit', fact)
            }}
          >
            {t('memory.edit')}
          </Button>
          <Button
            type="text"
            danger
            icon={<TrashIcon size={14} />}
            disabled={
              !active ||
              capabilities.physicalDeleteSupported !== true ||
              action.isPending ||
              !Number.isSafeInteger(fact.version)
            }
            onClick={() => {
              open('delete', fact)
            }}
          >
            {t('memory.delete')}
          </Button>
          {!!fact.consentRef && (
            <Button
              type="link"
              disabled={
                !active ||
                capabilities.consentRevokeSupported !== true ||
                action.isPending
              }
              onClick={() => {
                open('revoke', fact)
              }}
            >
              {t('memory.revoke')}
            </Button>
          )}
        </Space>
      )
    }
  ]
  const currentReceipt = operation.data ?? receipt?.operation
  const length = Array.from(dialog?.content ?? '').length
  const invalidContent =
    dialog?.action === 'edit' && (!dialog.content.trim() || length > 200)
  const unsupportedDialog =
    !!dialog &&
    ((dialog.action === 'edit' && capabilities.correctionSupported !== true) ||
      (dialog.action === 'delete' &&
        capabilities.physicalDeleteSupported !== true) ||
      (dialog.action === 'revoke' &&
        capabilities.consentRevokeSupported !== true))

  return (
    <>
      <Space style={{ marginBottom: 16 }} wrap>
        <Tag color={active ? 'green' : 'default'}>
          {t('memory.states.' + subject.state, subject.state)}
        </Tag>
        <Typography.Text type="secondary">{scope.userId}</Typography.Text>
      </Space>
      {!active ? (
        <Alert
          type="info"
          showIcon
          title={t('memory.inactive')}
          description={t('memory.inactiveHint')}
        />
      ) : (
        <>
          <section
            aria-label={t('memory.factsTitle')}
            style={{ marginBottom: 24 }}
          >
            <Typography.Title level={4} style={{ marginTop: 0 }}>
              {t('memory.factsTitle')}{' '}
              <Typography.Text
                type="secondary"
                style={{ fontSize: 14, fontWeight: 400 }}
              >
                {t('memory.factCount', { count: facts.data?.length ?? 0 })}
              </Typography.Text>
            </Typography.Title>
            {facts.isError ? (
              <ErrorNotice
                error={facts.error}
                retry={() => {
                  void facts.refetch()
                }}
              />
            ) : (
              <BorderedTable<MemoryFact>
                rowKey="factId"
                columns={columns}
                dataSource={facts.data ?? []}
                loading={facts.isPending}
                scroll={{ x: 900 }}
                pagination={{
                  pageSize: 20,
                  hideOnSinglePage: true,
                  showSizeChanger: false
                }}
                locale={{
                  emptyText: (
                    <NoRecords
                      title={t('memory.noFacts')}
                      hint={t('memory.noFactsHint')}
                    />
                  )
                }}
              />
            )}
          </section>
          {receipt && (
            <section style={panelStyle} aria-label={t('memory.receiptTitle')}>
              <Typography.Title level={5} style={{ marginTop: 0 }}>
                {t('memory.receiptTitle')}
              </Typography.Title>
              {currentReceipt ? (
                <>
                  <Space wrap style={{ marginBottom: 8 }}>
                    <Tag
                      color={currentReceipt.stoppedSupply ? 'green' : 'gold'}
                    >
                      {t(
                        currentReceipt.stoppedSupply
                          ? 'memory.supplyStopped'
                          : 'memory.processing'
                      )}
                    </Tag>
                    <Tag
                      color={currentReceipt.physicalCleanup ? 'green' : 'gold'}
                    >
                      {t(
                        currentReceipt.physicalCleanup
                          ? 'memory.physicalDone'
                          : 'memory.physicalPending'
                      )}
                    </Tag>
                  </Space>
                  <Typography.Paragraph style={{ marginBottom: 8 }}>
                    {t('memory.cleanupScope')}：
                    {t(
                      'memory.cleanupScopes.' + currentReceipt.cleanupScope,
                      t('memory.cleanupUnspecified')
                    )}
                  </Typography.Paragraph>
                  <Typography.Paragraph type="secondary">
                    {t(
                      currentReceipt.ledgerRetained
                        ? 'memory.ledgerRetained'
                        : 'memory.ledgerCleared'
                    )}
                  </Typography.Paragraph>
                  {!currentReceipt.physicalCleanup && (
                    <Typography.Paragraph
                      type="secondary"
                      style={{ marginBottom: 0, marginTop: 8 }}
                    >
                      {t('memory.receiptHint')}
                    </Typography.Paragraph>
                  )}
                </>
              ) : (
                <Alert
                  type="warning"
                  showIcon
                  title={t('memory.outcomeUnknown')}
                  style={{ marginBottom: 12 }}
                />
              )}
              <Space wrap style={{ marginTop: 8 }}>
                <Typography.Text type="secondary">
                  {t('memory.receiptId')}: {receipt.operationId}
                </Typography.Text>
                <Button
                  size="small"
                  loading={operation.isFetching}
                  onClick={() => {
                    void operation.refetch()
                  }}
                >
                  {t('memory.refresh')}
                </Button>
              </Space>
              {operation.isError && (
                <div style={{ marginTop: 12 }}>
                  <ErrorNotice
                    error={operation.error}
                    retry={() => {
                      void operation.refetch()
                    }}
                  />
                </div>
              )}
            </section>
          )}
          <section style={panelStyle} aria-label={t('memory.recallTitle')}>
            <Typography.Title level={5} style={{ marginTop: 0 }}>
              {t('memory.recallTitle')}
            </Typography.Title>
            <Typography.Paragraph type="secondary">
              {t('memory.recallHint')}
            </Typography.Paragraph>
            <form
              onSubmit={(event) => {
                event.preventDefault()
                if (query.trim() && !recall.isPending && !action.isPending) {
                  setRecallResult(null)
                  recall.mutate({
                    question: query,
                    epoch: recallEpoch.current
                  })
                }
              }}
            >
              <div style={{ display: 'flex', gap: 10, flexWrap: 'wrap' }}>
                <Input
                  aria-label={t('memory.recallTitle')}
                  style={{ flex: '1 1 240px' }}
                  placeholder={t('memory.recallPlaceholder')}
                  value={query}
                  maxLength={2000}
                  onChange={(event) => {
                    setQuery(event.target.value)
                  }}
                />
                <PrimaryButton
                  htmlType="submit"
                  loading={recall.isPending}
                  disabled={!query.trim() || action.isPending}
                >
                  {t('memory.recallAction')}
                </PrimaryButton>
              </div>
            </form>
            {recall.isError && (
              <div style={{ marginTop: 16 }}>
                <ErrorNotice
                  error={recall.error}
                  retry={() => {
                    recall.mutate({
                      question: query,
                      epoch: recallEpoch.current
                    })
                  }}
                />
              </div>
            )}
            {recallResult ? (
              <div style={{ marginTop: 16 }}>
                <Space wrap>
                  <Tag
                    color={recallResult.status === 'normal' ? 'green' : 'gold'}
                  >
                    {t(
                      'memory.recallStates.' + recallResult.status,
                      recallResult.status
                    )}
                  </Tag>
                  <Typography.Text type="secondary">
                    {t('memory.contextVersion', {
                      version: recallResult.contextVersion
                    })}
                  </Typography.Text>
                </Space>
                {recallResult.status === 'degraded' && (
                  <Alert
                    style={{ marginTop: 12 }}
                    type="warning"
                    showIcon
                    title={t('memory.recallDegraded')}
                  />
                )}
                {recallResult.status === 'stale' && (
                  <Alert
                    style={{ marginTop: 12 }}
                    type="info"
                    showIcon
                    title={t('memory.recallStale')}
                  />
                )}
                {recallResult.facts.length ? (
                  <ul style={{ paddingLeft: 22, marginBottom: 0 }}>
                    {recallResult.facts.map((fact) => (
                      <li
                        key={fact.factId}
                        style={{ marginTop: 12, whiteSpace: 'pre-wrap' }}
                      >
                        {fact.content}{' '}
                        <Typography.Text type="secondary">
                          v{fact.version}
                        </Typography.Text>
                      </li>
                    ))}
                  </ul>
                ) : (
                  <Typography.Paragraph
                    type="secondary"
                    style={{ marginTop: 16, marginBottom: 0 }}
                  >
                    {t('memory.recallEmpty')}
                  </Typography.Paragraph>
                )}
                {recallResult.profile && (
                  <div style={{ marginTop: 16 }}>
                    <Typography.Text strong>
                      {t('memory.profile')}
                    </Typography.Text>
                    <Typography.Paragraph style={{ whiteSpace: 'pre-wrap' }}>
                      {recallResult.profile}
                    </Typography.Paragraph>
                  </div>
                )}
              </div>
            ) : (
              !recall.isPending && (
                <Typography.Paragraph
                  type="secondary"
                  style={{ marginTop: 16, marginBottom: 0 }}
                >
                  {t('memory.recallWaiting')}
                </Typography.Paragraph>
              )
            )}
          </section>
        </>
      )}
      <Modal
        open={!!dialog}
        title={t(
          dialog?.action === 'edit'
            ? 'memory.editTitle'
            : dialog?.action === 'delete'
              ? 'memory.deleteTitle'
              : 'memory.revokeTitle'
        )}
        okText={t(
          dialog?.action === 'edit'
            ? 'memory.save'
            : dialog?.action === 'delete'
              ? 'memory.delete'
              : 'memory.revoke'
        )}
        cancelText={t('memory.cancel')}
        okButtonProps={{
          className: primaryStyle.root,
          danger: dialog?.action !== 'edit',
          loading: action.isPending,
          disabled: invalidContent || unsupportedDialog
        }}
        cancelButtonProps={{ disabled: action.isPending }}
        closable={!action.isPending}
        mask={{ closable: !action.isPending }}
        onCancel={() => {
          if (!action.isPending) setDialog(null)
        }}
        onOk={() => {
          if (dialog && !invalidContent && !unsupportedDialog)
            action.mutate(dialog)
        }}
        destroyOnHidden
      >
        {dialog && (
          <>
            <Typography.Paragraph type="secondary">
              {t(
                dialog.action === 'edit'
                  ? 'memory.editHint'
                  : dialog.action === 'delete'
                    ? 'memory.deleteHint'
                    : 'memory.revokeHint'
              )}
            </Typography.Paragraph>
            {dialog.action === 'edit' ? (
              <>
                <Input.TextArea
                  aria-label={t('memory.editLabel')}
                  rows={4}
                  value={dialog.content}
                  status={invalidContent ? 'error' : undefined}
                  disabled={action.isPending}
                  onChange={(event) => {
                    setDialog({
                      ...dialog,
                      content: event.target.value,
                      operationId: operationId()
                    })
                    setMutationError(undefined)
                  }}
                />
                <Typography.Paragraph
                  type={invalidContent ? 'danger' : 'secondary'}
                  style={{ textAlign: 'right', marginTop: 8 }}
                >
                  {t('memory.editCount', { count: length })}
                </Typography.Paragraph>
                {invalidContent && (
                  <Typography.Text type="danger">
                    {t('memory.editLength')}
                  </Typography.Text>
                )}
              </>
            ) : (
              <Typography.Paragraph style={{ whiteSpace: 'pre-wrap' }}>
                {dialog.fact.content}
              </Typography.Paragraph>
            )}
            {dialog.action === 'revoke' && (
              <Typography.Paragraph>
                {t('memory.consentRef')}:{' '}
                <Typography.Text code>{dialog.fact.consentRef}</Typography.Text>
              </Typography.Paragraph>
            )}
            {mutationError && (
              <Alert
                type="error"
                showIcon
                title={mutationError}
                style={{ marginTop: 16 }}
              />
            )}
          </>
        )}
      </Modal>
    </>
  )
}
