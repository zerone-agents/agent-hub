import { useState, useMemo } from 'react'
import { Spin, Popconfirm, message } from 'antd'
import NameSearch from '@/components/NameSearch'
import { PlusIcon, PencilSimpleIcon, TrashIcon, ClockIcon, PlugIcon, SquaresFourIcon } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import PrimaryButton from '@/components/PrimaryButton'
import { useProviders, useDeleteProvider, useProbeProvider } from '@/queries/useProviders'
import type { Provider } from '@/api/providers'
import type { ApiEnvelope } from '@/api/client'
import { formatTime } from '@/utils/time'
import { tokens as t } from '@/styles/tokens'
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
    font-size: ${t.text3xl};
    font-weight: 700;
    color: ${t.text};
    letter-spacing: -0.03em;
    line-height: 1.15;
  `,
  pageSub: css`
    margin-top: 4px;
    font-size: ${t.textBase};
    color: ${t.textTertiary};
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
    font-size: ${t.textLg};
    font-weight: 600;
    color: ${t.text};
    margin-bottom: 6px;
  `,
  emptyDesc: css`
    color: ${t.textTertiary};
    font-size: ${t.textSm};
  `,
  providerMeta: css`
    font-size: ${t.textXs};
    color: ${t.textTertiary};
    line-height: 1.6;
  `,
  baseUrl: css`
    font-family: ${t.fontMono};
    font-size: 11px;
    color: ${t.textMuted};
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
    font-family: ${t.fontMono};
    background: ${t.inkSubtle};
    color: ${t.textSecondary};
  `,
  actBtn: css`
    width: 30px;
    height: 30px;
    display: flex;
    align-items: center;
    justify-content: center;
    border: none;
    background: transparent;
    border-radius: ${t.radiusSm}px;
    color: ${t.textMuted};
    cursor: pointer;
    transition: all 0.15s;
    &:hover { background: ${t.inkSubtle}; color: ${t.ink}; }
  `,
  actBtnDanger: css`
    &:hover { background: rgba(220, 38, 38, 0.06); color: ${t.danger}; }
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
    color: ${t.text}; font-size: ${t.textBase}; font-weight: 600;
  `,
  sectionCount: css`
    display: inline-flex; align-items: center; justify-content: center;
    min-width: 24px; height: 24px; padding: 0 8px;
    background: ${t.inkSubtle}; color: ${t.ink}; border-radius: 12px;
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
  return t.inkLight
}

const protocolTextColor = (protocol: string) => {
  if (protocol === 'anthropic') return t.warning
  if (protocol === 'openai') return t.success
  if (protocol === 'mineru') return '#6366f1'
  if (protocol === 'paddleocr') return '#3b82f6'
  return t.ink
}

export default function ProviderListPage() {
  const { styles } = useStyles()
  const { data: providers = [], isLoading } = useProviders()

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
        message.success(`连接成功 · ${result.latencyMs}ms`)
      } else {
        message.error(`连接失败 · ${result?.error ?? '未知错误'}`)
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
                    background: t.inkLight,
                    color: t.ink
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
                    background: t.inkLight,
                    color: t.ink
                  }}
                >
                  内置
                </span>
              )}
            </div>
          </div>
        }
        bodyExtra={
          <div className={styles.providerMeta}>
            <div className={styles.baseUrl}>{provider.baseUrl || '—'}</div>
            <div className={styles.modelStats}>
              {provider.defaultModels.length} 个模型 · {provider.fields.length} 个表单字段
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
          <div style={{ display: 'flex', gap: 2 }}>
            <button
              type="button"
              className={styles.actBtn}
              title="测试连接"
              onClick={() => handleProbe(provider.id)}
              disabled={probingId === provider.id}
            >
              {probingId === provider.id ? <Spin size="small" /> : <PlugIcon size={14} />}
            </button>
            <button
              type="button"
              className={styles.actBtn}
              title="编辑"
              onClick={() => { showEdit(provider); }}
            >
              <PencilSimpleIcon size={14} />
            </button>
            <Popconfirm
              title="确认删除？"
              description={`删除 "${provider.name}"？此操作不可撤销。`}
              okText="删除"
              okButtonProps={{ danger: true }}
              cancelText="取消"
              onConfirm={() => handleDelete(provider.id)}
            >
              <button
                type="button"
                className={`${styles.actBtn} ${styles.actBtnDanger}`}
                title="删除"
              >
                <TrashIcon size={14} />
              </button>
            </Popconfirm>
          </div>
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
          <div className={styles.pageTitle}>模型管理</div>
          <div className={styles.pageSub}>管理 Vendor Preset 配置和模型列表</div>
        </div>
        <PrimaryButton icon={<PlusIcon size={16} weight="bold" />} onClick={showCreate}>
          新建 Provider
        </PrimaryButton>
      </div>

      <div className={styles.toolbar}>
          <NameSearch
            placeholder="搜索 Provider 名称"
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
            <PlugIcon size={48} weight="thin" color={t.textMuted} />
          </div>
          <div className={styles.emptyTitle}>{keywords ? '未找到匹配的 Provider' : '暂无 Provider'}</div>
          <div className={styles.emptyDesc}>{keywords ? '请尝试其他关键词' : '添加您的第一个 Provider 配置'}</div>
        </div>
      ) : (
        <>
          {renderSection(<><AnthropicBrand size={18} style={{ marginRight: 6 }} />Anthropic</>, anthropicProviders)}
          {renderSection(<><OpenAIBrand size={18} style={{ marginRight: 6 }} />OpenAI</>, openaiProviders)}
          {renderSection(<><SquaresFourIcon size={18} weight="duotone" style={{ marginRight: 6 }} />其他</>, otherProviders)}
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
