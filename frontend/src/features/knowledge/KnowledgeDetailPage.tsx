import { Spin, Tabs, Tag } from 'antd'
import { useTranslation } from 'react-i18next'
import { ArrowLeftIcon } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import { useNavigate, useParams, useLocation, Outlet } from 'react-router'
import { useKnowledgeDetail } from '@/queries/useKnowledge'
import { tokens as tk } from '@/styles/tokens'

const useStyles = createStyles(({ css }) => ({
  page: css`
    animation: pageIn 0.35s ease;
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
  if (pathname.includes('/settings')) return 'settings'
  return 'documents'
}

export default function KnowledgeDetailPage() {
  const { t } = useTranslation()
  const { styles } = useStyles()
  const navigate = useNavigate()
  const location = useLocation()
  const { id = '' } = useParams()
  const { data: dataset, isLoading } = useKnowledgeDetail(id)

  const activeKey = activeTabFromPath(location.pathname)

  const tabs = [
    { key: 'documents', label: 'knowledge.tabs.documents' },
    { key: 'retrieval', label: 'knowledge.tabs.retrieval' },
    { key: 'settings', label: 'knowledge.tabs.settings' }
  ]

  return (
    <div className={styles.page}>
      <button type="button" className={styles.back} onClick={async () => { await navigate('/knowledge'); }}>
        <ArrowLeftIcon size={14} />
        {t('knowledge.backToList')}
      </button>

      {/* eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- react-query data typed as T | undefined but lint infers dataset as always falsy */}
      {isLoading && !dataset ? (
        <div className={styles.loadingWrap}>
          <Spin />
        </div>
      ) : (
        <>
          <div className={styles.head}>
            <span className={styles.title}>{dataset?.name ?? t('knowledge.kbFallback')}</span>
            <Tag>{t('knowledge.docCountTag', { n: dataset?.doc_num ?? 0 })}</Tag>
            <Tag>{t('knowledge.chunkCountTag', { n: dataset?.chunk_num ?? 0 })}</Tag>
          </div>
          {dataset?.description ? <div className={styles.desc}>{dataset.description}</div> : null}

          <Tabs
            activeKey={activeKey}
            items={tabs}
            onChange={async (key) => { await navigate(`/knowledge/${id}/${key}`); }}
          />

          <Outlet />
        </>
      )}
    </div>
  )
}
