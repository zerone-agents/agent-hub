import { useState, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { Spin, Popconfirm, message } from 'antd'
import NameSearch from '@/components/NameSearch'
import { PlusIcon, PencilSimpleIcon, TrashIcon, ClockIcon, PlugIcon, SquaresFourIcon } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import PrimaryButton from '@/components/PrimaryButton'
import { useProviders, useDeleteProvider, useProbeProvider } from '@/queries/useProviders'
import { useCanWrite } from '@/hooks/useCanWrite'
import type { Provider } from '@/api/providers'
import type { ApiEnvelope } from '@/api/client'
import { formatTime } from '@/utils/time'
import { tokens as tk } from '@/styles/tokens'
import EntityCard from '@/components/EntityCard'
import CardGrid from '@/components/CardGrid'
import ProviderForm from './ProviderForm'
import AnthropicBrand from '@lobehub/icons/es/Anthropic'
import OpenAIBrand from '@lobehub/icons/es/OpenAI'

const useStyles = createStyles(({ css }) => ({
  page: css`
    animation: pageIn 0.35s ease;
    @keyframes pageIn {
      from { opacity: 0; transform: translateY(6px); }
      to { opacity: 1; transform: translateY(0); }
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
  `,
  loadingWrap: css`
    display: flex;
    justify-content: center;
    padding: 80px 0;
  `,
  emptyState: css`
    text-align: center;
    padding: 80px 0;
  `,
  emptyIcon: css`margin-bottom: 20px;`,
  emptyTitle: css`
    font-size: ${tk.textLg};
    font-weight: 600;
    color: ${tk.text};
    margin-bottom: 6px;
  `,
  emptyDesc: css`
    color: ${tk.textTertiary};
    font-size: ${tk.textSm};
  `,
  providerMeta: css`
    font-size: ${tk.textXs};
    color: ${tk.textTertiary};
    line-height: 1.6;
  `,
  baseUrl: css`
    font-family: ${tk.fontMono};
    font-size: 11px;
    color: ${tk.textMuted};
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    max-width: 100%;
  `,
  modelStats: css`margin-top: 4px;`,
  modelChips: css`display: flex; flex-wrap: wrap; gap: 4px; margin-top: 4px;`,
  chip: css`
    display: inline-block;
    padding: 2px 8px;
    border-radius: 3px;
    font-size: 11px;
    font-family: ${tk.fontMono};
    background: ${tk.inkSubtle};
    color: ${tk.textSecondary};
  `,
  actBtn: css`
    width: 30px;
    height: 30px;
    display: flex;
    align-items: center;
    justify-content: center;
    border: none;
    background: transparent;
    border-radius: ${tk.radiusSm}px;
    color: ${tk.textMuted};
    cursor: pointer;
    transition: all 0.15s;
    &:hover { background: ${tk.inkSubtle}; color: ${tk.ink}; }
  `,
  actBtnDanger: css`
    &:hover { background: rgba(220, 38, 38, 0.06); color: ${tk.danger}; }
  `,
  toolbar: css`
    display: flex; justify-content: space-between; align-items: center;
    gap: 12px; margin-bottom: 16px;
  `,
  section: css`margin-bottom: 40px;`,
  sectionHeader: css`
    display: flex; align-items: center; justify-content: space-between;
    margin-bottom: 16px; padding-bottom: 12px;
    border-bottom: 1px solid color-mix(in srgb, var(--foreground) 8%, transparent);
  `,
  sectionTitle: css`
    display: flex; align-items: center; gap: 8px;
    color: ${tk.text}; font-size: ${tk.textBase}; font-weight: 600;
  `,
  sectionCount: css`
    display: inline-flex; align-items: center; justify-content: center;
    min-width: 24px; height: 24px; padding: 0 8px;
    background: ${tk.inkSubtle}; color: ${tk.ink}; border-radius: 12px;
    font-size: 12px; font-weight: 600;
  `,
}))

const PROTOCOL_LABELS: Record<string, string> = {
  anthropic: 'Anthropic',
  openai: 'OpenAI',
  mineru: 'MinerU',
  paddleocr: 'PaddleOCR',
}

const TYPE_LABELS: Record<string, string> = {
  llm: 'LLM',
  ocr: 'OCR',
  embedding: 'Embedding',
  vlm: 'VLM',
}

function getCapabilities(provider: Provider): string[] {
  const set = new Set<string>()
  for (const m of provider.defaultModels) {
    set.add(m.modelType)
  }
  return Array.from(set)
}

const protocolBgColor = (protocol: string) => {
  if (protocol === 'anthropic') return 'rgba(217, 119, 6, 0.08)'
  if (protocol === 'openai') return 'rgba(5, 150, 105, 0.08)'
  if (protocol === 'mineru') return 'rgba(99, 102, 241, 0.08)'
  if (protocol === 'paddleocr') return 'rgba(59, 130, 246, 0.08)'
  return tk.inkLight
}

const protocolTextColor = (protocol: string) => {
  if (protocol === 'anthropic') return tk.warning
  if (protocol === 'openai') return tk.success
  if (protocol === 'mineru') return '#6366f1'
  if (protocol === 'paddleocr') return '#3b82f6'
  return tk.ink
}

export default function ProviderListPage() {
  const { t } = useTranslation()
  const { styles } = useStyles()
  const { data: providers = [], isLoading } = useProviders()
  const canWrite = useCanWrite()

  const deleteProvider = useDeleteProvider()
  const probeProvider = useProbeProvider()

  const [formOpen, setFormOpen] = useState(false)
  const [editingProvider, setEditingProvider] = useState<Provider | null>(null)
  const [probingId, setProbingId] = useState<number | null>(null)

  // 搜索
  const [keywords, setKeywords] = useState('')

  // 按关键词过滤，结果按标识首字母排序
  const filteredProviders = useMemo(() => {
    let result: Provider[]
    if (!keywords) {
      result = providers
    } else {
      const kw = keywords.toLowerCase()
      result = providers.filter((provider) => {
        const fields = [provider.name, provider.key, provider.description, provider.descriptionEn, provider.baseUrl]
        return fields.some((f) => f.toLowerCase().includes(kw))
      })
    }
    return result.sort((a, b) => a.name.localeCompare(b.name))
  }, [providers, keywords])

  const anthropicProviders = useMemo(() => filteredProviders.filter((p) => p.protocol === 'anthropic'), [filteredProviders])
  const openaiProviders = useMemo(() => filteredProviders.filter((p) => p.protocol === 'openai'), [filteredProviders])
  const otherProviders = useMemo(() => filteredProviders.filter((p) => p.protocol !== 'anthropic' && p.protocol !== 'openai'), [filteredProviders])

  const showCreate = () => {
    setEditingProvider(null)
    setFormOpen(true)
  }

  const showEdit = (provider: Provider) => {
    setEditingProvider(provider)
    setFormOpen(true)
  }

  const handleDelete = async (id: number) => {
    await deleteProvider.mutateAsync(id)
  }

  const handleProbe = async (id: number) => {
    setProbingId(id)
    try {
      const res = await probeProvider.mutateAsync({ id })
      const envelope = res.data as ApiEnvelope<{ success?: boolean; latencyMs?: number; error?: string }>
      const result = envelope.data
      if (result?.success) {
        message.success(t('providers.connectSuccess', { ms: result.latencyMs }))
      } else {
        message.error(t('providers.connectFail', { error: result?.error ?? t('providers.unknownError') }))
      }
    } finally {
      setProbingId(null)
    }
  }

  const renderProviderCard = (provider: Provider) => {
    const visibleModels = provider.defaultModels.slice(0, 4)
    const remaining = provider.defaultModels.length - visibleModels.length
    return (
      <EntityCard
        key={provider.id}
        icon={provider.name[0].toUpperCase()}
        title={provider.name}
        subtitle={provider.key}
        headerExtra={
          <div
            style={{
              display: 'flex',
              flexDirection: 'column',
              gap: 4,
              alignItems: 'flex-end'
            }}
          >
            <span
              style={{
                display: 'inline-block',
                padding: '1px 7px',
                borderRadius: 3,
                fontSize: 10,
                fontWeight: 600,
                background: protocolBgColor(provider.protocol),
                color: protocolTextColor(provider.protocol)
              }}
            >
              {PROTOCOL_LABELS[provider.protocol] || provider.protocol}
            </span>
            <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', justifyContent: 'flex-end' }}>
              {getCapabilities(provider).map((cap) => (
                <span
                  key={cap}
                  style={{
                    display: 'inline-block',
                    padding: '1px 7px',
                    borderRadius: 3,
                    fontSize: 10,
                    fontWeight: 600,
                    background: tk.inkLight,
                    color: tk.ink
                  }}
                >
                  {TYPE_LABELS[cap] || cap}
                </span>
              ))}
              {provider.builtin && (
                <span
                  style={{
                    display: 'inline-block',
                    padding: '1px 7px',
                    borderRadius: 3,
                    fontSize: 10,
                    fontWeight: 600,
                    background: tk.inkLight,
                    color: tk.ink
                  }}
                >
                  {t('providers.builtinBadge')}
                </span>
              )}
            </div>
          </div>
        }
        bodyExtra={
          <div className={styles.providerMeta}>
            <div className={styles.baseUrl}>{provider.baseUrl || '—'}</div>
            <div className={styles.modelStats}>
              {t('providers.modelFieldCount', { models: provider.defaultModels.length, fields: provider.fields.length })}
            </div>
            {visibleModels.length > 0 && (
              <div className={styles.modelChips}>
                {visibleModels.map((m) => (
                  <span key={m.selectionId ?? m.modelId} className={styles.chip}>
                    {m.displayName}
                  </span>
                ))}
                {remaining > 0 && (
                  <span className={styles.chip}>+{remaining}</span>
                )}
              </div>
            )}
          </div>
        }
        footerLeft={
          <span style={{ display: 'inline-flex', alignItems: 'center', gap: 5 }}>
            <ClockIcon size={12} />
            {formatTime(provider.createdAt)}
          </span>
        }
        footerRight={
          canWrite ? (
            <div style={{ display: 'flex', gap: 2 }}>
              <button
                type="button"
                className={styles.actBtn}
                title={t('providers.testConnection')}
                onClick={() => handleProbe(provider.id)}
                disabled={probingId === provider.id}
              >
                {probingId === provider.id ? <Spin size="small" /> : <PlugIcon size={14} />}
              </button>
              <button
                type="button"
                className={styles.actBtn}
                title={t('common.edit')}
                onClick={() => { showEdit(provider); }}
              >
                <PencilSimpleIcon size={14} />
              </button>
              <Popconfirm
                title={t('providers.deleteConfirmTitle')}
                description={t('providers.deleteConfirm', { name: provider.name })}
                okText={t('common.delete')}
                okButtonProps={{ danger: true }}
                cancelText={t('common.cancel')}
                onConfirm={() => handleDelete(provider.id)}
              >
                <button
                  type="button"
                  className={`${styles.actBtn} ${styles.actBtnDanger}`}
                  title={t('common.delete')}
                >
                  <TrashIcon size={14} />
                </button>
              </Popconfirm>
            </div>
          ) : null
        }
      />
    )
  }

  const renderSection = (title: React.ReactNode, items: Provider[]) => {
    if (items.length === 0) return null
    return (
      <div className={styles.section}>
        <div className={styles.sectionHeader}>
          <div className={styles.sectionTitle}>{title}</div>
          <span className={styles.sectionCount}>{items.length}</span>
        </div>
        <CardGrid>{items.map(renderProviderCard)}</CardGrid>
      </div>
    )
  }

  return (
    <div className={styles.page}>
      <div className={styles.pageHead}>
        <div>
          <div className={styles.pageTitle}>{t('providers.pageTitle')}</div>
          <div className={styles.pageSub}>{t('providers.pageSub')}</div>
        </div>
        {canWrite && (
          <PrimaryButton icon={<PlusIcon size={16} weight="bold" />} onClick={showCreate}>
            {t('providers.create')}
          </PrimaryButton>
        )}
      </div>

      <div className={styles.toolbar}>
          <NameSearch
            placeholder={t('providers.searchPlaceholder')}
            onSearch={setKeywords}
            realtime
          />
      </div>

      {isLoading ? (
        <div className={styles.loadingWrap}>
          <Spin size="medium" />
        </div>
      ) : filteredProviders.length === 0 ? (
        <div className={styles.emptyState}>
          <div className={styles.emptyIcon}>
            <PlugIcon size={48} weight="thin" color={tk.textMuted} />
          </div>
          <div className={styles.emptyTitle}>{keywords ? t('providers.empty.noMatch') : t('providers.empty.none')}</div>
          <div className={styles.emptyDesc}>{keywords ? t('providers.empty.noMatchHint') : t('providers.empty.noneHint')}</div>
        </div>
      ) : (
        <>
          {renderSection(<><AnthropicBrand size={18} style={{ marginRight: 6 }} />Anthropic</>, anthropicProviders)}
          {renderSection(<><OpenAIBrand size={18} style={{ marginRight: 6 }} />OpenAI</>, openaiProviders)}
          {renderSection(<><SquaresFourIcon size={18} weight="duotone" style={{ marginRight: 6 }} />{t('providers.otherSection')}</>, otherProviders)}
        </>
      )}

      <ProviderForm
        open={formOpen}
        editingProvider={editingProvider}
        onClose={() => { setFormOpen(false); }}
      />
    </div>
  )
}
