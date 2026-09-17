import { useState, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { Spin, Input, Pagination, Popconfirm } from 'antd'
import { MagnifyingGlassIcon, TrashIcon, ChatCircleDotsIcon } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import type { ChatSession } from '@/api/chat'
import { useChatSessions, useDeleteChatSession } from '@/queries/useChat'
import { useProviders } from '@/queries/useProviders'
import { formatTime } from '@/utils/time'
import { tokens as tk } from '@/styles/tokens'

const useStyles = createStyles(({ css }) => ({
  sidebar: css`
    width: 320px; border-right: 1px solid color-mix(in srgb, var(--foreground) 6%, transparent);
    display: flex; flex-direction: column; flex-shrink: 0;
    height: 100%;
    @media (max-width: 768px) {
      width: 100%;
      max-height: none;
      border-right: none;
      border-bottom: none;
    }
  `,
  sidebarHiddenMobile: css`
    @media (max-width: 768px) {
      display: none;
    }
  `,
  head: css`
    padding: 16px 20px 8px;
  `,
  title: css`font-size: 14px; font-weight: 600; color: ${tk.text}; margin: 0 0 12px;`,
  search: css`margin-bottom: 8px;`,
  list: css`
    flex: 1; overflow-y: auto; padding: 4px 8px;
    display: flex; flex-direction: column; gap: 4px;
    min-height: 0;
  `,
  loading: css`display: flex; justify-content: center; padding: 40px 0;`,
  empty: css`
    display: flex; flex-direction: column; align-items: center; gap: 8px;
    padding: 40px 0; color: ${tk.textMuted}; font-size: 12px;
  `,
  item: css`
    display: flex; align-items: flex-start; gap: 8px;
    padding: 10px 12px; border-radius: ${tk.radiusSm}px;
    cursor: pointer; transition: background 0.12s;
    &:hover { background: ${tk.surfaceHover}; }
  `,
  itemActive: css`background: ${tk.surfaceHover};`,
  body: css`flex: 1; min-width: 0;`,
  itemTitle: css`
    font-size: 13px; font-weight: 500; color: ${tk.text};
    white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  `,
  meta: css`
    display: flex; align-items: center; gap: 4px; margin-top: 2px;
    font-size: 11px; color: ${tk.textMuted};
  `,
  dot: css`opacity: 0.5;`,
  delBtn: css`
    flex-shrink: 0; width: 24px; height: 24px;
    display: flex; align-items: center; justify-content: center;
    border: none; background: transparent; border-radius: 3px;
    color: ${tk.textMuted}; cursor: pointer; transition: all 0.15s;
    &:hover { background: rgba(220, 38, 38, 0.06); color: ${tk.danger}; }
  `,
  pagination: css`
    display: flex; justify-content: center; padding: 8px 0 12px;
  `
}))

interface SessionListPanelProps {
  selectedId: string | null
  onSelect: (session: ChatSession) => void
  hideOnMobile?: boolean
}

export default function SessionListPanel({ selectedId, onSelect, hideOnMobile }: SessionListPanelProps) {
  const { t } = useTranslation()
  const { styles } = useStyles()
  const { data, isLoading, page, pageSize, setPage } = useChatSessions()
  const deleteSession = useDeleteChatSession()
  const [search, setSearch] = useState('')

  const sessions = useMemo(() => data?.items ?? [], [data?.items])
  const total = data?.total ?? 0

  // 解析 (provider_id, model_selection_id) → catalog displayName。
  // 历史快照场景下 provider 可能已删 / selectionId 已不在 catalog，此时 fallback 到 session.model。
  const { data: providers = [] } = useProviders()
  const modelDisplayNameMap = useMemo(() => {
    const m = new Map<string, string>()
    for (const p of providers) {
      for (const mo of p.defaultModels) {
        if (mo.selectionId) {
          m.set(`${p.id}::${mo.selectionId}`, mo.displayName || mo.modelId)
        }
      }
    }
    return m
  }, [providers])

  const resolveModelLabel = (s: ChatSession): string => {
    if (s.model_selection_id && s.provider_id) {
      const hit = modelDisplayNameMap.get(`${s.provider_id}::${s.model_selection_id}`)
      if (hit) return hit
    }
    return s.model || '-'
  }

  const filtered = useMemo(() => {
    if (!search) return sessions
    const q = search.toLowerCase()
    return sessions.filter((s) =>
      (s.title || '').toLowerCase().includes(q) ||
      (s.model || '').toLowerCase().includes(q) ||
      (s.agent_id || '').toLowerCase().includes(q) ||
      (s.user_id || '').toLowerCase().includes(q) ||
      (s.display_name ?? '').toLowerCase().includes(q) ||
      (s.user_name ?? '').toLowerCase().includes(q)
    )
  }, [sessions, search])

  return (
    <div className={`${styles.sidebar} ${hideOnMobile ? styles.sidebarHiddenMobile : ''}`}>
      <div className={styles.head}>
        <h2 className={styles.title}>{t('chat.sessions')}</h2>
        <Input
          className={styles.search}
          placeholder={t('chat.searchPlaceholder')}
          allowClear
          size="small"
          prefix={<MagnifyingGlassIcon size={14} color={tk.textMuted} />}
          value={search}
          onChange={(e) => { setSearch(e.target.value); }}
        />
      </div>

      <div className={styles.list}>
        {isLoading ? (
          <div className={styles.loading}><Spin size="small" /></div>
        ) : filtered.length === 0 ? (
          <div className={styles.empty}>
            <ChatCircleDotsIcon size={32} weight="thin" color={tk.textMuted} />
            <span>{search ? t('chat.noMatch') : t('chat.empty')}</span>
          </div>
        ) : (
          filtered.map((session) => (
            <div
              key={session.id}
              className={`${styles.item} ${selectedId === session.id ? styles.itemActive : ''}`}
              onClick={() => { onSelect(session); }}
            >
              <div className={styles.body}>
                <div className={styles.itemTitle}>{session.title || t('chat.untitled')}</div>
                <div className={styles.meta}>
                  <span>{session.display_name ?? session.user_name ?? (session.user_id.slice(0, 8) || '-')}</span>
                  <span className={styles.dot}>·</span>
                  <span>{resolveModelLabel(session)}</span>
                  <span className={styles.dot}>·</span>
                  <span>{formatTime(session.updated_at)}</span>
                </div>
              </div>
              <Popconfirm
                title={t('chat.deleteConfirmTitle')}
                description={t('chat.deleteConfirmDesc')}
                okText={t('common.delete')}
                okButtonProps={{ danger: true }}
                cancelText={t('common.cancel')}
                onConfirm={(e) => {
                  e?.stopPropagation()
                  deleteSession.mutate(session.id)
                }}
              >
                <button
                  type="button"
                  className={styles.delBtn}
                  title={t('common.delete')}
                  onClick={(e) => { e.stopPropagation(); }}
                >
                  <TrashIcon size={13} />
                </button>
              </Popconfirm>
            </div>
          ))
        )}
      </div>

      {total > pageSize && (
        <div className={styles.pagination}>
          <Pagination
            current={page}
            total={total}
            pageSize={pageSize}
            size="small"
            simple
            onChange={(p) => { setPage(p); }}
          />
        </div>
      )}
    </div>
  )
}
