import { Alert, Button, Spin, Tabs, Tag } from 'antd'
import { useTranslation } from 'react-i18next'
import { ArrowLeftIcon } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import { useNavigate, useParams, useLocation, Outlet } from 'react-router'
import { useKnowledgeDetail } from '@/queries/useKnowledge'
import { tokens as tk } from '@/styles/tokens'

const useStyles = createStyles(({ css }) => ({
  page: css`
    animation: pageIn 0.35s ease;
    @media (prefers-reduced-motion: reduce) { animation: none; }
    @keyframes pageIn {
      from { opacity: 0; transform: translateY(6px); }
      to { opacity: 1; transform: translateY(0); }
    }
  `,
  back: css`
    display: inline-flex; align-items: center; gap: 6px; margin-bottom: 12px;
    font-size: ${tk.textSm}; color: ${tk.textTertiary}; background: none; border: none;
    cursor: pointer; padding: 0;
    &:hover { color: ${tk.ink}; }
  `,
  head: css`
    display: flex; align-items: baseline; gap: 12px; flex-wrap: wrap; margin-bottom: 4px;
  `,
  title: css`
    font-size: ${tk.text2xl}; font-weight: 700; color: ${tk.text}; letter-spacing: -0.02em;
    margin: 0; overflow-wrap: anywhere;
  `,
  desc: css`
    margin: 4px 0 12px; font-size: ${tk.textSm}; color: ${tk.textTertiary};
  `,
  loadingWrap: css`
    display: flex; justify-content: center; padding: 80px 0;
  `
}))

function activeTabFromPath(pathname: string): string {
  if (pathname.includes('/retrieval')) return 'retrieval'
  if (pathname.includes('/ingestions')) return 'ingestions'
  if (pathname.includes('/settings')) return 'settings'
  return 'documents'
}

export default function KnowledgeDetailPage() {
  const { t } = useTranslation()
  const { styles } = useStyles()
  const navigate = useNavigate()
  const location = useLocation()
  const { id = '' } = useParams()
  const { data: dataset, isLoading, isError, refetch } = useKnowledgeDetail(id)

  const activeKey = activeTabFromPath(location.pathname)

  const tabs = [
    { key: 'documents', label: 'knowledge.tabs.documents' },
    { key: 'retrieval', label: 'knowledge.tabs.retrieval' },
    { key: 'ingestions', label: 'knowledge.tabs.ingestions' },
    { key: 'settings', label: 'knowledge.tabs.settings' }
  ]

  return (
    <div className={styles.page}>
      <button type="button" className={styles.back} onClick={async () => { await navigate('/knowledge'); }}>
        <ArrowLeftIcon size={14} />
        {t('knowledge.backToList')}
      </button>

      {dataset ? (
        <>
          {isError ? <Alert type="warning" showIcon title={t('knowledge.states.detailStale')} description={t('knowledge.states.detailStaleHint')} action={<Button onClick={() => { void refetch() }}>{t('knowledge.states.retry')}</Button>} style={{ marginBottom: 16 }} /> : null}
          <div className={styles.head}>
            <h1 className={styles.title}>{dataset.name || t('knowledge.list.unnamed')}</h1>
            <Tag>{t('knowledge.docCountTag', { n: dataset.doc_num })}</Tag>
            <Tag>{t('knowledge.chunkCountTag', { n: dataset.chunk_num })}</Tag>
          </div>
          {dataset.description ? <div className={styles.desc}>{dataset.description}</div> : null}

          <Tabs
            activeKey={activeKey}
            items={tabs.map(tab => ({ ...tab, label: t(tab.label) }))}
            onChange={async (key) => { await navigate(`/knowledge/${id}/${key}`); }}
          />

          <Outlet />
        </>
      ) : isError ? (
        <Alert type="error" showIcon title={t('knowledge.states.detailFailed')} description={t('knowledge.states.detailFailedHint')} action={<Button onClick={() => { void refetch() }}>{t('knowledge.states.retry')}</Button>} />
      ) : isLoading ? (
        <div className={styles.loadingWrap} role="status" aria-label={t('common.loading')}><Spin /></div>
      ) : (
        <Alert type="warning" showIcon title={t('knowledge.states.notFound')} action={<Button onClick={() => { void refetch() }}>{t('knowledge.states.retry')}</Button>} />
      )}
    </div>
  )
}
