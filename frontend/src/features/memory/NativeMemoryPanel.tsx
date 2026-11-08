import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Alert,
  Button,
  Descriptions,
  Drawer,
  Empty,
  Input,
  Modal,
  Pagination,
  Select,
  Space,
  Spin,
  Switch,
  Segmented,
  InputNumber,
  Collapse,
  Tabs,
  Tag,
  Tooltip,
  Typography,
  message
} from 'antd'
import {
  ArrowSquareOutIcon,
  ArrowsClockwiseIcon,
  ChatCircleTextIcon,
  FunnelSimpleIcon,
  MagnifyingGlassIcon,
  CaretDownIcon,
  CaretRightIcon,
  InfoIcon
} from '@phosphor-icons/react'
import {
  memoryApi,
  memoryScopeKey,
  type MemoryApplication,
  type MemorySubject,
  type NativeMemoryMessage
} from '@/api/memory'
import { parseApiError } from '@/api/client'
import { tokens as tk } from '@/styles/tokens'
import PrimaryButton, {
  usePrimaryButtonStyle
} from '@/components/PrimaryButton'
import {
  memoryPreview,
  memoryTurnGroups,
  type MemoryTurnGroup
} from './nativeMemoryView'
import './memory-management.css'

interface StatusIntent { item: NativeMemoryMessage; enabled: boolean }

function NativeMemoryEmpty({
  title,
  description
}: {
  title: string
  description: string
}) {
  return (
    <Empty
      image={Empty.PRESENTED_IMAGE_SIMPLE}
      style={{ padding: '80px 0', margin: 0 }}
      description={
        <>
          <Typography.Paragraph
            style={{
              fontSize: tk.textLg,
              fontWeight: 600,
              color: tk.text,
              marginBottom: 6
            }}
          >
            {title}
          </Typography.Paragraph>
          <Typography.Text
            style={{ fontSize: tk.textSm, color: tk.textTertiary }}
          >
            {description}
          </Typography.Text>
        </>
      }
    />
  )
}

export default function NativeMemoryPanel({
  subject,
  queryPrefix,
  application,
  onSelectAccount
}: {
  subject: MemorySubject
  queryPrefix: readonly string[]
  application?: MemoryApplication
  onSelectAccount?: () => void
}) {
  const { t } = useTranslation()
  const qc = useQueryClient()
  const primaryStyle = usePrimaryButtonStyle()
  const [query, setQuery] = useState('')
  const [keyword, setKeyword] = useState('')
  const [type, setType] = useState('all')
  const [status, setStatus] = useState('all')
  const [tab, setTab] = useState('records')
  const [page, setPage] = useState(1)
  const [expandedGroups, setExpandedGroups] = useState<Record<string, boolean>>(
    {}
  )
  const [recallMode, setRecallMode] = useState<'supply' | 'native'>('supply')
  const [recallLimit, setRecallLimit] = useState(5)
  const [selected, setSelected] = useState<NativeMemoryMessage>()
  const [intent, setIntent] = useState<StatusIntent>()
  const active = subject.state === 'active'
  const scopeKey = memoryScopeKey(subject.scope)
  const queryKey = [...queryPrefix, 'native', scopeKey]
  const records = useQuery({
    queryKey,
    queryFn: ({ signal }) => memoryApi.nativeMessages(subject.scope, signal),
    enabled: active,
    gcTime: 0
  })
  const recall = useMutation({
    mutationFn: async (request: {
      question: string
      mode: 'supply' | 'native'
      limit: number
    }) => {
      if (request.mode === 'native') {
        const result = await memoryApi.nativeSearch({
          scope: subject.scope,
          query: request.question,
          limit: request.limit
        })
        return {
          ...result,
          recentSourceIncluded: false,
          budgetLimited: false,
          mode: request.mode
        }
      }
      const result = await memoryApi.nativeContext({
        scope: subject.scope,
        query: request.question,
        limit: request.limit
      })
      return { ...result, mode: request.mode }
    },
    retry: false,
    gcTime: 0
  })
  const currentRecall =
    recall.variables?.question === query &&
    recall.variables.mode === recallMode &&
    recall.variables.limit === recallLimit
  const toggle = useMutation({
    mutationFn: (request: StatusIntent) => {
      if (typeof request.item.nativeStatus !== 'boolean')
        throw new Error(t('memory.unsupported'))
      return memoryApi.setNativeStatus(request.item.id, {
        scope: subject.scope,
        enabled: request.enabled,
        expectedEnabled: request.item.nativeStatus
      })
    },
    retry: false,
    gcTime: 0,
    onSuccess: () => {
      setIntent(undefined)
      void qc.invalidateQueries({ queryKey })
      void message.success(t('memory.nativeStatusSaved'))
    },
    onError: () => {
      void qc.invalidateQueries({ queryKey })
    }
  })
  const all = useMemo(() => records.data ?? [], [records.data])
  const groups = useMemo(() => memoryTurnGroups(all), [all])
  const rawCount = all.filter((item) => item.type === 'raw').length
  const derivedCount = all.length - rawCount
  const failedCount = all.filter(
    (item) => item.type === 'raw' && item.extractionStatus === 'failed'
  ).length
  const selectedItem = all.find((item) => item.id === selected?.id) ?? selected
  const selectedSource =
    selectedItem?.type === 'raw'
      ? selectedItem
      : all.find((item) => item.id === selectedItem?.sourceId)
  const selectedChildren = all.filter(
    (item) => item.type !== 'raw' && item.sourceId === selectedSource?.id
  )
  const matches = (item: NativeMemoryMessage) => {
    if (type !== 'all' && item.type !== type) return false
    const source =
      item.type === 'raw' ? item : all.find((row) => row.id === item.sourceId)
    if (
      (status === 'enabled' && !item.status) ||
      (status === 'disabled' && item.status)
    )
      return false
    if (status === 'failed' && source?.extractionStatus !== 'failed')
      return false
    if (status === 'completed' && source?.extractionStatus !== 'completed')
      return false
    if (
      status === 'pending' &&
      ![
        'pending',
        'received',
        'extracting',
        'unknown',
        'derivedAvailable'
      ].includes(source?.extractionStatus ?? '')
    )
      return false
    return (
      !keyword.trim() ||
      (item.content + ' ' + item.id + ' ' + item.sourceId)
        .toLowerCase()
        .includes(keyword.trim().toLowerCase())
    )
  }
  const filtered = groups
    .map((group) => ({ ...group, messages: group.messages.filter(matches) }))
    .filter((group) => group.messages.length)
  const visibleCount = filtered.reduce(
    (sum, group) => sum + group.messages.length,
    0
  )
  const currentPage = Math.min(
    page,
    Math.max(1, Math.ceil(filtered.length / 10))
  )
  const open = (item: NativeMemoryMessage) => {
    setSelected(item)
  }
  const resetPage = () => {
    setPage(1)
  }
  const changeAllowed = (item: NativeMemoryMessage) =>
    item.statusChangeAllowed &&
    (item.nativeStatus === true || item.parentStatus !== false)
  const switchFor = (item: NativeMemoryMessage, inDrawer = false) =>
    application?.nativeStatusSupported &&
    typeof item.nativeStatus === 'boolean' ? (
      <Tooltip
        title={
          !item.nativeStatus && item.parentStatus === false
            ? t('memory.parentDisabledHint')
            : t('memory.statusSwitchHint')
        }
      >
        <Switch
          size="small"
          checked={item.nativeStatus}
          disabled={!changeAllowed(item) || toggle.isPending}
          aria-label={t(
            inDrawer ? 'memory.nativeDetailSwitch' : 'memory.nativeSwitch',
            { id: item.id }
          )}
          onChange={(enabled) => {
            toggle.reset()
            setIntent({ item, enabled })
          }}
        />
      </Tooltip>
    ) : null
  const sourceState = (source?: NativeMemoryMessage) =>
    source?.extractionStatus ? (
      <Tag
        color={
          source.extractionStatus === 'failed'
            ? 'red'
            : source.extractionStatus === 'completed'
              ? 'green'
              : 'gold'
        }
      >
        {t(
          'memory.extractionStates.' + source.extractionStatus,
          source.extractionStatus
        )}
      </Tag>
    ) : null
  const statusLabel = (item: NativeMemoryMessage) => (
    <Tooltip
      title={
        item.nativeStatus && item.parentStatus === false
          ? t('memory.parentDisabledHint')
          : undefined
      }
    >
      <Tag color={item.status ? 'green' : 'default'}>
        {t(item.status ? 'memory.nativeActive' : 'memory.nativeDisabled')}
      </Tag>
    </Tooltip>
  )
  const row = (item: NativeMemoryMessage, rank?: number) => (
    <div className="memory-record-row" key={item.id}>
      <div className="memory-record-kind">
        <Tag color={item.type === 'raw' ? 'default' : 'blue'}>
          {t('memory.nativeTypes.' + item.type, item.type)}
        </Tag>
      </div>
      <div className="memory-record-main">
        <button
          className="memory-record-open"
          type="button"
          aria-label={t('memory.openMemoryContent', { id: item.id })}
          onClick={() => {
            open(item)
          }}
        >
          <span className="memory-record-preview">
            {memoryPreview(item.content)}
          </span>
        </button>
        <div className="memory-record-meta">
          {rank !== undefined && <span>#{rank + 1}</span>}
          <span>
            {t('memory.nativeRecord')} #{item.id}
          </span>
          {item.type !== 'raw' && (
            <Button
              size="small"
              type="link"
              onClick={() => {
                const source = all.find((source) => source.id === item.sourceId)
                if (source) open(source)
              }}
              disabled={!all.some((source) => source.id === item.sourceId)}
            >
              {t('memory.viewSource')} #{item.sourceId}
            </Button>
          )}
          {!item.status && <span>{t('memory.notSupplied')}</span>}
        </div>
      </div>
      <div className="memory-record-actions">
        {statusLabel(item)}
        {switchFor(item)}
        <Button
          size="small"
          type="text"
          aria-label={t('memory.openMemoryDetail', { id: item.id })}
          icon={<ArrowSquareOutIcon size={15} />}
          onClick={() => {
            open(item)
          }}
        />
      </div>
    </div>
  )
  const groupExpanded = (group: MemoryTurnGroup) =>
    expandedGroups[group.id] ??
    (keyword.trim() !== '' ||
      type !== 'all' ||
      status !== 'all' ||
      groups.length <= 3 ||
      group.id === filtered[0]?.id)
  const allVisibleExpanded = filtered.every(groupExpanded)
  const groupCard = (group: MemoryTurnGroup) => {
    const isExpanded = groupExpanded(group)
    return (
      <section
        className="memory-turn"
        key={group.id}
        aria-label={t('memory.sourceTurn', { id: group.id })}
      >
        <div className="memory-turn-header">
          <button
            className="memory-group-toggle"
            type="button"
            aria-expanded={isExpanded}
            aria-controls={'memory-source-' + group.id}
            onClick={() => {
              setExpandedGroups({ ...expandedGroups, [group.id]: !isExpanded })
            }}
          >
            {isExpanded ? (
              <CaretDownIcon size={13} />
            ) : (
              <CaretRightIcon size={13} />
            )}
            <ChatCircleTextIcon size={16} />
            <span>{t('memory.sourceTurn', { id: group.id })}</span>
            <span className="memory-turn-time">
              {group.source?.createdAt ??
                group.source?.validAt ??
                t('memory.timeNotProvided')}
            </span>
          </button>
          <Space size={4} wrap>
            {sourceState(group.source)}
            <span className="memory-turn-time">
              {t('memory.derivedCount', { count: group.children.length })}
            </span>
            {group.source && (
              <Button
                size="small"
                type="text"
                onClick={() => {
                  if (group.source) open(group.source)
                }}
              >
                {t('memory.viewConversation')}
              </Button>
            )}
          </Space>
        </div>
        {isExpanded && (
          <div id={'memory-source-' + group.id}>
            {group.source?.extractionStatus === 'failed' && (
              <div className="memory-record-task-note">
                {t(
                  group.children.length
                    ? 'memory.failedWithDerivedHint'
                    : 'memory.failedWithoutDerivedHint'
                )}
              </div>
            )}
            {group.messages.map((item) => row(item))}
          </div>
        )}
      </section>
    )
  }
  const filterOptions = [
    'all',
    'raw',
    'semantic',
    'episodic',
    'procedural'
  ].map((value) => ({
    value,
    label: t(
      value === 'all' ? 'memory.allMemoryTypes' : 'memory.nativeTypes.' + value
    )
  }))
  const statusOptions = [
    'all',
    'enabled',
    'disabled',
    'completed',
    'pending',
    'failed'
  ].map((value) => ({ value, label: t('memory.recordFilters.' + value) }))
  const scopeSummary =
    t('memory.currentScopeOnly') +
    ' · ' +
    (records.isPending || records.isError
      ? '—'
      : all.length === 0
        ? t('memory.noNativeMessages')
        : [
            `${t('memory.statTurns')} ${rawCount}`,
            `${t('memory.statDerived')} ${derivedCount}`,
            `${t('memory.statSupplied')} ${all.filter((item) => item.status).length}`,
            `${t('memory.statFailed')} ${failedCount}`
          ].join(' · '))
  const recordFilters = (
    <div className="memory-filter-bar">
      <Input
        className="memory-filter-search"
        prefix={
          <MagnifyingGlassIcon size={14} color="var(--text-muted, #999)" />
        }
        aria-label={t('memory.searchMemoryRecords')}
        placeholder={t('memory.searchMemoryPlaceholder')}
        allowClear
        value={keyword}
        onChange={(event) => {
          setKeyword(event.target.value)
          resetPage()
        }}
      />
      <Select
        aria-label={t('memory.filterMemoryType')}
        className="memory-filter-select"
        value={type}
        options={filterOptions}
        onChange={(value) => {
          setType(value)
          resetPage()
        }}
      />
      <Select
        aria-label={t('memory.filterMemoryStatus')}
        className="memory-filter-select"
        value={status}
        options={statusOptions}
        onChange={(value) => {
          setStatus(value)
          resetPage()
        }}
      />
      {(keyword || type !== 'all' || status !== 'all') && (
        <Tooltip title={t('memory.clearFilters')}>
          <Button
            type="text"
            aria-label={t('memory.clearFilters')}
            icon={<FunnelSimpleIcon size={15} />}
            onClick={() => {
              setKeyword('')
              setType('all')
              setStatus('all')
              resetPage()
            }}
          />
        </Tooltip>
      )}
      <Tooltip title={t('memory.nativeRefreshExtraction')}>
        <Button
          type="text"
          aria-label={t('memory.nativeRefreshExtraction')}
          icon={<ArrowsClockwiseIcon size={15} />}
          loading={records.isFetching}
          onClick={() => {
            void records.refetch()
          }}
        />
      </Tooltip>
      {filtered.length > 3 && (
        <Button
          type="text"
          onClick={() => {
            setExpandedGroups({
              ...expandedGroups,
              ...Object.fromEntries(
                filtered.map((group) => [group.id, !allVisibleExpanded])
              )
            })
          }}
        >
          {t(
            allVisibleExpanded
              ? 'memory.collapseAllTurns'
              : 'memory.expandAllTurns'
          )}
        </Button>
      )}
      <Tooltip title={scopeSummary + ' ' + t('memory.modelDerivedHint')}>
        <span
          className="memory-scope-note"
          role="region"
          aria-label={t('memory.currentScopeStatistics')}
        >
          <span className="memory-sr-only">{scopeSummary}</span>
          <InfoIcon size={15} />
        </span>
      </Tooltip>
    </div>
  )
  const recordsView = (
    <div className="memory-records-pane">
      <div className="memory-turn-scroll">
        {records.isPending ? (
          <div className="memory-loading" role="status">
            <Spin aria-label={t('common.loading')} />
          </div>
        ) : records.isError ? (
          <Alert
            type="error"
            showIcon
            title={parseApiError(records.error)}
            action={
              <Button
                onClick={() => {
                  void records.refetch()
                }}
              >
                {t('memory.retry')}
              </Button>
            }
          />
        ) : !filtered.length ? (
          <div className="memory-account-empty">
            <NativeMemoryEmpty
              title={t(
                all.length
                  ? 'memory.noMatchingNativeTitle'
                  : 'memory.noNativeTitle'
              )}
              description={
                all.length
                  ? t('memory.noMatchingMemories')
                  : t('memory.noNativeMessages')
              }
            />
            {!all.length && onSelectAccount && (
              <Button onClick={onSelectAccount}>
                {t('memory.switchAccount')}
              </Button>
            )}
          </div>
        ) : (
          <div className="memory-turn-list">
            {filtered
              .slice((currentPage - 1) * 10, currentPage * 10)
              .map(groupCard)}
          </div>
        )}
      </div>
      {filtered.length > 10 && (
        <Pagination
          size="small"
          style={{ marginTop: 8 }}
          current={currentPage}
          pageSize={10}
          total={filtered.length}
          showSizeChanger={false}
          onChange={setPage}
        />
      )}
    </div>
  )
  const recallView = (
    <section
      className="memory-recall-pane"
      aria-label={t('memory.nativeRecallTitle')}
    >
      <p className="memory-native-help">{t('memory.recallDiagnosticHint')}</p>
      {application?.nativeDiagnosticsSupported && (
        <Space style={{ marginBottom: 16 }} wrap>
          <Segmented
            aria-label={t('memory.recallMode')}
            value={recallMode}
            options={[
              { value: 'supply', label: t('memory.chatSupplyMode') },
              { value: 'native', label: t('memory.nativeSearchMode') }
            ]}
            onChange={(value) => {
              setRecallMode(value as 'supply' | 'native')
              recall.reset()
            }}
          />
          <label>
            {t('memory.recallLimit')}{' '}
            <InputNumber
              aria-label={t('memory.recallLimit')}
              min={1}
              max={20}
              value={recallLimit}
              onChange={(value) => {
                if (value !== null) {
                  setRecallLimit(value)
                  recall.reset()
                }
              }}
            />
          </label>
        </Space>
      )}
      {recallMode === 'native' && (
        <Collapse
          size="small"
          ghost
          style={{ marginBottom: 12 }}
          items={[
            {
              key: 'parameters',
              label: t('memory.nativeParameterDisclosure'),
              children: (
                <div className="memory-native-help">
                  {t('memory.nativeParameterHint')}
                </div>
              )
            }
          ]}
        />
      )}

      <form
        className="memory-recall-form"
        onSubmit={(event) => {
          event.preventDefault()
          if (query.trim() && !recall.isPending)
            recall.mutate({
              question: query,
              mode: recallMode,
              limit: recallLimit
            })
        }}
      >
        <Input
          aria-label={t('memory.nativeRecallTitle')}
          placeholder={t('memory.recallPlaceholder')}
          value={query}
          onChange={(event) => {
            setQuery(event.target.value)
            recall.reset()
          }}
          maxLength={2000}
        />
        <PrimaryButton
          htmlType="submit"
          disabled={!query.trim()}
          loading={recall.isPending}
        >
          {t('memory.recallAction')}
        </PrimaryButton>
      </form>
      {recall.isError && currentRecall && (
        <Alert type="error" showIcon title={parseApiError(recall.error)} />
      )}
      {currentRecall && recall.data && (
        <>
          <Space wrap size={4}>
            <Tag>
              {t('memory.recalledCount', {
                count: recall.data.messages.length
              })}
            </Tag>
            {recall.data.recentSourceIncluded && (
              <Tag color="blue">{t('memory.recentSourceIncluded')}</Tag>
            )}
            {recall.data.budgetLimited && (
              <Tag color="gold">{t('memory.contextBudgetReached')}</Tag>
            )}
          </Space>
          {recall.data.budgetLimited && (
            <Alert
              style={{ marginTop: 12 }}
              type="info"
              showIcon
              title={t('memory.nativeBudgetLimited')}
            />
          )}
          <div className="memory-recall-results memory-turn">
            {recall.data.messages.length ? (
              recall.data.messages.map((item, index) => row(item, index))
            ) : (
              <NativeMemoryEmpty
                title={t('memory.noRecallNativeTitle')}
                description={t('memory.recallEmpty')}
              />
            )}
          </div>
        </>
      )}
    </section>
  )
  if (!active)
    return (
      <Alert
        type="info"
        showIcon
        title={t('memory.inactive')}
        description={t('memory.inactiveHint')}
      />
    )
  return (
    <div className="memory-native-workspace">
      <Tabs
        className="memory-native-tabs"
        activeKey={tab}
        onChange={setTab}
        destroyOnHidden
        tabBarExtraContent={tab === 'records' ? recordFilters : null}
        items={[
          {
            key: 'records',
            label:
              t('memory.recordsTab') +
              (!records.isPending && !records.isError && all.length
                ? ` · ${visibleCount}`
                : ''),
            children: recordsView
          },
          { key: 'recall', label: t('memory.recallTab'), children: recallView }
        ]}
      />
      <Drawer
        title={t('memory.memoryDetail')}
        open={!!selectedItem}
        onClose={() => {
          setSelected(undefined)
        }}
        size={680}
        destroyOnHidden
        extra={selectedItem && switchFor(selectedItem, true)}
      >
        {selectedItem && (
          <>
            <Space size={4} wrap>
              <Tag color={selectedItem.type === 'raw' ? 'default' : 'blue'}>
                {t('memory.nativeTypes.' + selectedItem.type)}
              </Tag>
              {statusLabel(selectedItem)}
              {sourceState(selectedSource)}
            </Space>
            <Descriptions
              style={{ marginTop: 18 }}
              column={1}
              size="small"
              items={[
                {
                  key: 'id',
                  label: t('memory.nativeRecord'),
                  children: '#' + selectedItem.id
                },
                {
                  key: 'source',
                  label: t('memory.nativeSource'),
                  children:
                    selectedItem.type === 'raw' ? (
                      t('memory.thisIsSource')
                    ) : (
                      <Button
                        type="link"
                        size="small"
                        disabled={!selectedSource}
                        onClick={() => {
                          if (selectedSource) open(selectedSource)
                        }}
                      >
                        {t('memory.viewSource')} #{selectedItem.sourceId}
                      </Button>
                    )
                },
                {
                  key: 'status',
                  label: t('memory.nativeOwnStatus'),
                  children:
                    typeof selectedItem.nativeStatus !== 'boolean'
                      ? t('memory.recordStatusUnknown')
                      : t(
                          selectedItem.nativeStatus
                            ? 'memory.nativeSwitchOn'
                            : 'memory.nativeSwitchOff'
                        )
                },
                {
                  key: 'effective',
                  label: t('memory.effectiveSupplyStatus'),
                  children: t(
                    selectedItem.status
                      ? 'memory.currentlySupplied'
                      : 'memory.notSupplied'
                  )
                },
                {
                  key: 'created',
                  label: t('memory.recordCreatedAt'),
                  children: selectedItem.createdAt || '—'
                },
                {
                  key: 'start',
                  label: t('memory.nativeValidAt'),
                  children: selectedItem.validAt ?? '—'
                },
                {
                  key: 'end',
                  label: t('memory.nativeInvalidAt'),
                  children: selectedItem.invalidAt ?? '—'
                }
              ]}
            />
            {selectedItem.parentStatus === false && (
              <Alert
                style={{ marginTop: 12 }}
                type="info"
                showIcon
                title={t('memory.parentDisabledHint')}
              />
            )}
            {selectedSource?.extractionStatus === 'failed' && (
              <Alert
                style={{ marginTop: 12 }}
                type="warning"
                showIcon
                title={t(
                  selectedChildren.length
                    ? 'memory.failedWithDerivedHint'
                    : 'memory.failedWithoutDerivedHint'
                )}
                description={selectedSource.extractionError}
              />
            )}
            <Typography.Title level={5} style={{ marginTop: 24 }}>
              {t(
                selectedItem.type === 'raw'
                  ? 'memory.fullConversation'
                  : 'memory.extractedContent'
              )}
            </Typography.Title>
            {selectedItem.type !== 'raw' && (
              <div className="memory-native-help">
                {t('memory.modelDerivedHint')}
              </div>
            )}
            <pre className="memory-source-content">{selectedItem.content}</pre>
            <Typography.Title level={5}>
              {t('memory.relatedDerivedTitle', {
                count: selectedChildren.length
              })}
            </Typography.Title>
            <div className="memory-related-list">
              {selectedChildren.map((item) => (
                <button
                  type="button"
                  key={item.id}
                  className="memory-related-button"
                  aria-label={t('memory.openRelatedMemory', { id: item.id })}
                  onClick={() => {
                    open(item)
                  }}
                >
                  <Tag>{t('memory.nativeTypes.' + item.type)}</Tag>
                  <span className="memory-record-preview">{item.content}</span>
                </button>
              ))}
            </div>
            <div className="memory-native-help">
              {t('memory.nativeTimeMetadataHint')}
            </div>
          </>
        )}
      </Drawer>
      <Modal
        title={t(
          intent?.enabled
            ? 'memory.enableMemoryTitle'
            : 'memory.disableMemoryTitle'
        )}
        open={!!intent}
        onCancel={() => {
          if (!toggle.isPending) {
            setIntent(undefined)
            toggle.reset()
          }
        }}
        okText={t(
          intent?.enabled ? 'memory.enableMemory' : 'memory.disableMemory'
        )}
        cancelText={t('memory.cancel')}
        okButtonProps={{
          className: primaryStyle.root,
          disabled: toggle.isError
        }}
        confirmLoading={toggle.isPending}
        onOk={() => {
          if (intent) toggle.mutate(intent)
        }}
        destroyOnHidden
      >
        <p>
          {t(
            intent?.item.type === 'raw'
              ? intent.enabled
                ? 'memory.enableSourceExplanation'
                : 'memory.disableSourceExplanation'
              : intent?.enabled
                ? 'memory.enableDerivedExplanation'
                : 'memory.disableDerivedExplanation'
          )}
        </p>
        <p className="memory-native-help">
          {t('memory.statusSupplyScopeHint')}
        </p>
        <div className="memory-source-content">
          {intent && memoryPreview(intent.item.content)}
        </div>
        {toggle.isError && (
          <Alert
            type="error"
            showIcon
            title={parseApiError(toggle.error)}
            description={t('memory.statusChangeUnconfirmed')}
          />
        )}
      </Modal>
    </div>
  )
}
