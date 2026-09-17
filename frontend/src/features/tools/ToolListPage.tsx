import { useState, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { Spin, Popconfirm, message } from 'antd'
import NameSearch from '@/components/NameSearch'
import {
  PencilSimpleIcon, TrashIcon, WrenchIcon, DownloadSimpleIcon,
  UploadSimpleIcon, WarningCircleIcon
} from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import PrimaryButton from '@/components/PrimaryButton'
import { useTools, useDeleteTool } from '@/queries/useTools'
import { useCanWrite } from '@/hooks/useCanWrite'
import { toolApi, type Tool } from '@/api/tools'
import { parseApiError, type ApiEnvelope } from '@/api/client'
import { formatTime } from '@/utils/time'
import { tokens as tk } from '@/styles/tokens'
import EntityCard from '@/components/EntityCard'
import CardGrid from '@/components/CardGrid'
import ToolForm, { type ToolFormMode } from './ToolForm'

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
  emptyIcon: css`
    margin-bottom: 20px;
  `,
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
  section: css`
    margin-bottom: 40px;
  `,
  sectionHeader: css`
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 16px;
    padding-bottom: 12px;
    border-bottom: 1px solid color-mix(in srgb, var(--foreground) 8%, transparent);
  `,
  sectionTitle: css`
    display: flex;
    align-items: center;
    gap: 8px;
    color: ${tk.text};
    font-size: ${tk.textBase};
    font-weight: 600;
  `,
  sectionCount: css`
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 24px;
    height: 24px;
    padding: 0 8px;
    background: ${tk.inkSubtle};
    color: ${tk.ink};
    border-radius: 12px;
    font-size: 12px;
    font-weight: 600;
  `,
  artifactBadge: css`
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding: 1px 7px;
    border-radius: 3px;
    font-size: 10px;
    font-weight: 600;
    background: rgba(5, 150, 105, 0.08);
    color: #059669;
  `,
  artifactBadgeMissing: css`
    background: rgba(217, 119, 6, 0.1);
    color: #b45309;
  `,
  fileMeta: css`
    font-size: 11px;
    color: ${tk.textMuted};
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

const formatFileSize = (bytes?: number): string => {
  if (!bytes || bytes <= 0) return '-'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`
}

export default function ToolListPage() {
  const { t } = useTranslation()
  const { styles } = useStyles()
  const { data: tools = [], isLoading } = useTools()
  const deleteTool = useDeleteTool()
  const canWrite = useCanWrite()

  const [formOpen, setFormOpen] = useState(false)
  const [formMode, setFormMode] = useState<ToolFormMode>('create')
  const [editingTool, setEditingTool] = useState<Tool | null>(null)

  // 搜索
  const [keywords, setKeywords] = useState('')

  // 按关键词过滤（builtin/custom 两区共用），结果按标识排序
  const filteredTools = useMemo(() => {
    let result: Tool[]
    if (!keywords) {
      result = tools
    } else {
      const kw = keywords.toLowerCase()
      result = tools.filter((tool) => {
        const fields = [tool.title, tool.name, tool.description, tool.descriptionEn ?? '']
        return fields.some((f) => f.toLowerCase().includes(kw))
      })
    }
    return result.sort((a, b) => a.name.localeCompare(b.name))
  }, [tools, keywords])

  const builtinList = filteredTools.filter((tl) => tl.source === 'builtin')
  const customList = filteredTools.filter((tl) => tl.source !== 'builtin')

  const openForm = (mode: ToolFormMode, tool: Tool | null) => {
    setFormMode(mode)
    setEditingTool(tool)
    setFormOpen(true)
  }

  const handleDownload = async (tool: Tool) => {
    try {
      const res = await toolApi.download(tool.name)
      const body = res.data as ApiEnvelope<{ url?: string }>
      if (body.success && body.data?.url) {
        window.open(body.data.url, '_blank')
      }
    } catch (err) {
      message.error(parseApiError(err))
    }
  }

  const renderSection = (title: string, list: Tool[], children: React.ReactNode) =>
    list.length > 0 ? (
      <div className={styles.section}>
        <div className={styles.sectionHeader}>
          <span className={styles.sectionTitle}>{title}</span>
          <span className={styles.sectionCount}>{list.length}</span>
        </div>
        {children}
      </div>
    ) : null

  const renderCustomCard = (tool: Tool) => {
    const missing = tool.artifactStatus === 'missing'
    return (
      <EntityCard
        key={tool.name}
        icon={tool.name[0].toUpperCase()}
        title={tool.title || tool.name}
        subtitle={tool.name}
        headerExtra={
          missing ? (
            <span className={`${styles.artifactBadge} ${styles.artifactBadgeMissing}`}>
              <WarningCircleIcon size={11} weight="fill" /> {t('tools.artifactMissing')}
            </span>
          ) : (
            <span className={styles.artifactBadge}>{t('tools.artifactUploaded')}</span>
          )
        }
        description={
          // eslint-disable-next-line @typescript-eslint/prefer-nullish-coalescing -- D2: empty EN string must also fall back to CN, `??` would pass it through
          tool.descriptionEn || tool.description || t('tools.noDescription')
        }
        bodyExtra={
          <div className={styles.fileMeta}>
            {tool.fileName ?? '-'} · {formatFileSize(tool.fileSize)}
            {tool.fileHash ? ` · ${tool.fileHash.slice(0, 8)}` : ''}
          </div>
        }
        footerLeft={formatTime(tool.createdAt)}
        footerRight={
          canWrite ? (
            <div style={{ display: 'flex', gap: 2 }}>
              {!missing && (
                <button
                  type="button"
                  className={styles.actBtn}
                  title={t('common.download')}
                  onClick={() => { void handleDownload(tool) }}
                >
                  <DownloadSimpleIcon size={14} />
                </button>
              )}
              {!missing && (
                <button
                  type="button"
                  className={styles.actBtn}
                  title={t('common.edit')}
                  onClick={() => { openForm('edit', tool) }}
                >
                  <PencilSimpleIcon size={14} />
                </button>
              )}
              <button
                type="button"
                className={styles.actBtn}
                title={missing ? t('tools.uploadFile') : t('tools.replaceFile')}
                onClick={() => { openForm('upload', tool) }}
              >
                <UploadSimpleIcon size={14} />
              </button>
              <Popconfirm
                title={t('tools.deleteConfirmTitle')}
                description={t('tools.deleteConfirm', { name: tool.name })}
                okText={t('common.delete')}
                okButtonProps={{ danger: true }}
                cancelText={t('common.cancel')}
                onConfirm={() => { deleteTool.mutate(tool.name) }}
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

  return (
    <div className={styles.page}>
      <div className={styles.pageHead}>
        <div>
          <div className={styles.pageTitle}>{t('tools.pageTitle')}</div>
          <div className={styles.pageSub}>{t('tools.pageSub')}</div>
        </div>
        {canWrite && (
          <PrimaryButton
            icon={<UploadSimpleIcon size={16} weight="bold" />}
            onClick={() => { openForm('create', null) }}
          >
            {t('tools.upload')}
          </PrimaryButton>
        )}
      </div>

      <div className={styles.toolbar}>
        <NameSearch
          placeholder={t('tools.searchPlaceholder')}
          onSearch={setKeywords}
          realtime
        />
      </div>

      {isLoading ? (
        <div className={styles.loadingWrap}>
          <Spin size="medium" />
        </div>
      ) : filteredTools.length === 0 ? (
        <div className={styles.emptyState}>
          <div className={styles.emptyIcon}>
            <WrenchIcon size={48} weight="thin" color={tk.textMuted} />
          </div>
          <div className={styles.emptyTitle}>{keywords ? t('tools.empty.noMatch') : t('tools.empty.none')}</div>
          <div className={styles.emptyDesc}>{keywords ? t('tools.empty.noMatchHint') : t('tools.empty.noneHint')}</div>
        </div>
      ) : (
        <>
          {renderSection(t('tools.builtinSection'), builtinList,
            <CardGrid>
              {builtinList.map((tool) => (
                <EntityCard
                  key={tool.name}
                  icon={tool.name[0].toUpperCase()}
                  title={tool.title || tool.name}
                  subtitle={tool.name}
                  headerExtra={tool.isDefault ? <span className={styles.artifactBadge}>{t('tools.defaultBadge')}</span> : null}
                  description={
                    // eslint-disable-next-line @typescript-eslint/prefer-nullish-coalescing -- D2: empty EN string must also fall back to CN, `??` would pass it through
                    tool.descriptionEn || tool.description || t('tools.noDescription')
                  }
                  footerLeft={formatTime(tool.createdAt)}
                />
              ))}
            </CardGrid>
          )}
          {renderSection(t('tools.customSection'), customList,
            <CardGrid>
              {customList.map(renderCustomCard)}
            </CardGrid>
          )}
        </>
      )}

      <ToolForm
        open={formOpen}
        mode={formMode}
        editingTool={editingTool}
        onClose={() => { setFormOpen(false) }}
      />
    </div>
  )
}
