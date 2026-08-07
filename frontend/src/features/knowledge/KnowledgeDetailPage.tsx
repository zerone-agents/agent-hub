import { Spin, Tabs, Tag } from 'antd'
import { ArrowLeftIcon } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import { useNavigate, useParams, useLocation, Outlet } from 'react-router-dom'
import { useKnowledgeDetail } from '@/queries/useKnowledge'
import { tokens as t } from '@/styles/tokens'

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
    font-size: ${t.textSm}; color: ${t.textTertiary}; background: none; border: none;
    cursor: pointer; padding: 0;
    &:hover { color: ${t.ink}; }
  `,
  head: css`
    display: flex; align-items: baseline; gap: 12px; flex-wrap: wrap; margin-bottom: 4px;
  `,
  title: css`
    font-size: ${t.text2xl}; font-weight: 700; color: ${t.text}; letter-spacing: -0.02em;
  `,
  desc: css`
    margin: 4px 0 12px; font-size: ${t.textSm}; color: ${t.textTertiary};
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
  const { styles } = useStyles()
  const navigate = useNavigate()
  const location = useLocation()
  const { id = '' } = useParams()
  const { data: dataset, isLoading } = useKnowledgeDetail(id)

  const activeKey = activeTabFromPath(location.pathname)

  const tabs = [
    { key: 'documents', label: '文档管理' },
    { key: 'retrieval', label: '检索测试' },
    { key: 'settings', label: '基础设置' }
  ]

  return (
    <div className={styles.page}>
      <button type="button" className={styles.back} onClick={() => { navigate('/knowledge'); }}>
        <ArrowLeftIcon size={14} />
        返回知识库列表
      </button>

      {/* eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- react-query data typed as T | undefined but lint infers dataset as always falsy */}
      {isLoading && !dataset ? (
        <div className={styles.loadingWrap}>
          <Spin />
        </div>
      ) : (
        <>
          <div className={styles.head}>
            <span className={styles.title}>{dataset?.name ?? '知识库'}</span>
            <Tag>文档 {dataset?.doc_num ?? 0}</Tag>
            <Tag>分块 {dataset?.chunk_num ?? 0}</Tag>
          </div>
          {dataset?.description ? <div className={styles.desc}>{dataset.description}</div> : null}

          <Tabs
            activeKey={activeKey}
            items={tabs}
            onChange={(key) => { navigate(`/knowledge/${id}/${key}`); }}
          />

          <Outlet />
        </>
      )}
    </div>
  )
}
