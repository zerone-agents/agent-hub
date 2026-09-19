import { useState, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { Spin, Popconfirm, Tooltip } from 'antd'
import NameSearch from '@/components/NameSearch'
import { PlusIcon, PencilSimpleIcon, TrashIcon, PlugsConnectedIcon } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import PrimaryButton from '@/components/PrimaryButton'
import { useMcps, useDeleteMcp, useProbeMcp } from '@/queries/useMcps'
import { useCanWrite } from '@/hooks/useCanWrite'
import type { Mcp } from '@/api/mcps'
import { formatTime } from '@/utils/time'
import { tokens as tk } from '@/styles/tokens'
import EntityCard from '@/components/EntityCard'
import CardGrid from '@/components/CardGrid'
import McpForm from './McpForm'

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
  loadingWrap: css`display: flex; justify-content: center; padding: 80px 0;`,
  emptyState: css`text-align: center; padding: 80px 0;`,
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
  metaLine: css`
    font-family: ${tk.fontMono};
    font-size: 11px;
    color: ${tk.textMuted};
    word-break: break-all;
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
}))

const transportColor: Record<Mcp['transportType'], string> = {
  sse: 'rgba(251, 146, 60, 0.08)',
  http: 'rgba(5, 150, 105, 0.08)',
}

const transportTextColor: Record<Mcp['transportType'], string> = {
  sse: '#fb923c',
  http: 'var(--success)',
}

const transportLabel: Record<Mcp['transportType'], string> = {
  sse: 'SSE',
  http: 'HTTP',
}

function Badge({ label, color }: { label: string; color: string }) {
  return (
    <span style={{
      display: 'inline-block',
      padding: '1px 7px',
      borderRadius: '3px',
      fontSize: '10px',
      fontWeight: 600,
      letterSpacing: '0.02em',
      textTransform: 'uppercase',
      background: color,
      color: 'inherit',
    }}>
      {label}
    </span>
  )
}

function ToolTag({ name }: { name: string }) {
  return (
    <span style={{
      display: 'inline-block',
      padding: '2px 8px',
      borderRadius: 3,
      fontSize: 11,
      fontFamily: tk.fontMono,
      background: tk.inkSubtle,
      color: tk.textSecondary,
    }}>
      {name}
    </span>
  )
}

export default function McpListPage() {
  const { t } = useTranslation()
  const { styles } = useStyles()
  const { data: mcps = [], isLoading } = useMcps()
  const deleteMcp = useDeleteMcp()
  const canWrite = useCanWrite()
  const probeMcp = useProbeMcp()

  const [formOpen, setFormOpen] = useState(false)
  const [editingMcp, setEditingMcp] = useState<Mcp | null>(null)
  const [probingName, setProbingName] = useState<string | null>(null)

  // 搜索
  const [keywords, setKeywords] = useState('')

  // 按关键词过滤，结果按标识首字母排序
  const filteredMcps = useMemo(() => {
    let result: Mcp[]
    if (!keywords) {
      result = mcps
    } else {
      const kw = keywords.toLowerCase()
      result = mcps.filter((mcp) => {
        const fields = [mcp.title, mcp.name, mcp.description, mcp.url]
        return fields.some((f) => f?.toLowerCase().includes(kw))
      })
    }
    return result.sort((a, b) => a.name.localeCompare(b.name))
  }, [mcps, keywords])

  const showCreate = () => {
    setEditingMcp(null)
    setFormOpen(true)
  }

  const showEdit = (mcp: Mcp) => {
    setEditingMcp(mcp)
    setFormOpen(true)
  }

  const handleDelete = async (name: string) => {
    await deleteMcp.mutateAsync(name)
  }

  const handleProbe = async (mcp: Mcp) => {
    setProbingName(mcp.name)
    try {
      await probeMcp.mutateAsync({ name: mcp.name })
    } finally {
      setProbingName(null)
    }
  }

  return (
    <div className={styles.page}>
      <div className={styles.pageHead}>
        <div>
          <div className={styles.pageTitle}>{t('mcps.pageTitle')}</div>
          <div className={styles.pageSub}>{t('mcps.pageSub')}</div>
        </div>
        {canWrite && (
          <PrimaryButton icon={<PlusIcon size={16} weight="bold" />} onClick={showCreate}>
            {t('mcps.create')}
          </PrimaryButton>
        )}
      </div>

      <div className={styles.toolbar}>
          <NameSearch
            placeholder={t('mcps.searchPlaceholder')}
            onSearch={setKeywords}
            realtime
          />
      </div>

      {isLoading ? (
        <div className={styles.loadingWrap}>
          <Spin size="medium" />
        </div>
      ) : filteredMcps.length === 0 ? (
        <div className={styles.emptyState}>
          <div className={styles.emptyIcon}>
            <PlugsConnectedIcon size={48} weight="thin" color={tk.textMuted} />
          </div>
          <div className={styles.emptyTitle}>{keywords ? t('mcps.empty.noMatch') : t('mcps.empty.none')}</div>
          <div className={styles.emptyDesc}>{keywords ? t('mcps.empty.noMatchHint') : t('mcps.empty.noneHint')}</div>
        </div>
      ) : (
        <CardGrid>
          {filteredMcps.map((mcp) => (
            <EntityCard
              key={mcp.name}
              icon={(mcp.title || mcp.name)[0].toUpperCase()}
              title={mcp.title || mcp.name}
              subtitle={mcp.name}
              headerExtra={
                <div style={{ display: 'flex', gap: 6 }}>
                  <span style={{ color: transportTextColor[mcp.transportType] }}>
                    <Badge label={transportLabel[mcp.transportType]} color={transportColor[mcp.transportType]} />
                  </span>
                  {mcp.isBuiltin && (
                    <span style={{ color: '#3b82f6' }}>
                      <Badge label={t('mcps.builtinBadge')} color="rgba(59, 130, 246, 0.08)" />
                    </span>
                  )}
                </div>
              }
              description={mcp.description || t('mcps.noDescription')}
              bodyExtra={
                <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
                  <div className={styles.metaLine}>{mcp.url}</div>
                  <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                    <div style={{ fontSize: 13, color: 'var(--text-secondary)', fontWeight: 500 }}>
                      {mcp.isBuiltin
                        ? t('mcps.builtinToolsCount', { n: mcp.tools?.length ?? 0 })
                        : mcp.probeStatus === 'success'
                        ? t('mcps.probedToolsCount', { n: mcp.tools?.length ?? 0, time: formatTime(mcp.lastProbedAt) })
                        : mcp.probeStatus === 'failed'
                        ? t('mcps.probeFailed')
                        : t('mcps.notProbed')}
                    </div>
                    {mcp.tools && mcp.tools.length > 0 && (
                      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
                        {mcp.tools.slice(0, 4).map((tool) => (
                          <ToolTag key={tool.name} name={tool.name} />
                        ))}
                        {mcp.tools.length > 4 && (
                          <Tooltip title={mcp.tools.slice(4).map(t => t.name).join(', ')}>
                            <ToolTag name={`+${mcp.tools.length - 4}`} />
                          </Tooltip>
                        )}
                      </div>
                    )}
                  </div>
                </div>
              }
              footerLeft={formatTime(mcp.createdAt)}
              footerRight={
                canWrite ? (
                  <div style={{ display: 'flex', gap: 2 }}>
                    {!mcp.isBuiltin && (
                      <button
                        type="button"
                        className={styles.actBtn}
                        title={t('mcps.probe')}
                        disabled={probingName === mcp.name}
                        onClick={() => handleProbe(mcp)}
                      >
                        {probingName === mcp.name ? (
                          <Spin size="small" />
                        ) : (
                          <PlugsConnectedIcon size={14} />
                        )}
                      </button>
                    )}
                    <button
                      type="button"
                      className={styles.actBtn}
                      title={t('common.edit')}
                      onClick={() => { showEdit(mcp); }}
                    >
                      <PencilSimpleIcon size={14} />
                    </button>
                    {!mcp.isBuiltin && (
                      <Popconfirm
                        title={t('mcps.deleteConfirmTitle')}
                        description={t('mcps.deleteConfirm', { name: mcp.name })}
                        okText={t('common.delete')}
                        okButtonProps={{ danger: true }}
                        cancelText={t('common.cancel')}
                        onConfirm={() => handleDelete(mcp.name)}
                      >
                        <button
                          type="button"
                          className={`${styles.actBtn} ${styles.actBtnDanger}`}
                          title={t('common.delete')}
                        >
                          <TrashIcon size={14} />
                        </button>
                      </Popconfirm>
                    )}
                  </div>
                ) : null
              }
            />
          ))}
        </CardGrid>
      )}

      <McpForm
        open={formOpen}
        editingMcp={editingMcp}
        onClose={() => { setFormOpen(false); }}
      />
    </div>
  )
}
