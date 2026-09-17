import { useState, useMemo } from 'react'
import { Spin, Popconfirm, Tooltip } from 'antd'
import NameSearch from '@/components/NameSearch'
import type { ColumnsType } from 'antd/es/table'
import { PlusIcon, PencilSimpleIcon, TrashIcon } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import PrimaryButton from '@/components/PrimaryButton'
import StatusBadge from '@/components/StatusBadge'
import type { Scene } from '@/api/scenes'
import type { Agent } from '@/api/agents'
import { useScenes, useDeleteScene } from '@/queries/useScenes'
import { useAgents } from '@/queries/useAgents'
import { useTranslation } from 'react-i18next'
import { useCanWrite } from '@/hooks/useCanWrite'
import { formatTime } from '@/utils/time'
import { tokens as tk } from '@/styles/tokens'
import BorderedTable from '@/components/BorderedTable'
import SceneForm from './SceneForm'

const useStyles = createStyles(({ css }) => ({
  page: css`
    animation: pageIn 0.35s ease;
    @keyframes pageIn {
      from { opacity: 0; transform: translateY(6px); }
      to { opacity: 1; transform: translateY(0); }
    }
  `,
  pageHead: css`
    display: flex; justify-content: space-between; align-items: flex-start;
    margin-bottom: 24px;
    @media (max-width: 768px) { flex-direction: column; gap: 16px; }
  `,
  pageTitle: css`
    font-size: ${tk.text3xl}; font-weight: 700; color: ${tk.text};
    letter-spacing: -0.03em; line-height: 1.15;
  `,
  pageSub: css`
    margin-top: 4px; font-size: ${tk.textBase}; color: ${tk.textTertiary};
  `,
  loadingWrap: css`
    display: flex; justify-content: center; padding: 80px 0;
  `,
  actBtn: css`
    width: 30px; height: 30px; display: flex; align-items: center; justify-content: center;
    border: none; background: transparent; border-radius: ${tk.radiusSm}px;
    color: ${tk.textMuted}; cursor: pointer; transition: all 0.15s;
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

function getAgentTitle(agents: Agent[], agentName: string): string {
  const agent = agents.find((a) => a.name === agentName)
  return agent ? (agent.config.title?.zh ?? agent.config.title?.en ?? agent.name) : agentName
}

export default function SceneListPage() {
  const { styles } = useStyles()
  const { t } = useTranslation()
  const { data: scenes = [], isLoading } = useScenes()
  const { data: agents = [] } = useAgents()
  const deleteScene = useDeleteScene()
  const canWrite = useCanWrite()

  const [formOpen, setFormOpen] = useState(false)
  const [editingScene, setEditingScene] = useState<Scene | null>(null)

  // 搜索
  const [keywords, setKeywords] = useState('')

  // 按关键词过滤，结果按标识首字母排序
  const filteredScenes = useMemo(() => {
    let result: Scene[]
    if (!keywords) {
      result = scenes
    } else {
      const kw = keywords.toLowerCase()
      result = scenes.filter((scene) => {
        const fields = [scene.title, scene.titleEn, scene.name, scene.agent, scene.prompt, scene.promptEn]
        return fields.some((f) => f.toLowerCase().includes(kw))
      })
    }
    return result.sort((a, b) => a.name.localeCompare(b.name))
  }, [scenes, keywords])

  const columns: ColumnsType<Scene> = [
    { title: t('scenes.columns.name'), dataIndex: 'name', key: 'name', width: 160 },
    {
      title: t('scenes.columns.title'),
      key: 'title',
      width: 180,
      render: (_, record) => record.title || record.titleEn || record.name
    },
    {
      title: t('scenes.columns.agent'),
      key: 'agent',
      width: 180,
      render: (_, record) => getAgentTitle(agents, record.agent)
    },
    {
      title: t('scenes.columns.prompt'),
      key: 'prompt',
      ellipsis: true,
      render: (_, record) => (
        <Tooltip title={record.prompt} placement="topLeft">
          <span style={{ color: tk.textTertiary }}>{record.prompt || '-'}</span>
        </Tooltip>
      )
    },
    {
      title: t('scenes.columns.status'),
      key: 'enabled',
      width: 90,
      render: (_, record) => <StatusBadge enabled={record.enabled} />
    },
    {
      title: t('scenes.columns.createdAt'),
      key: 'createdAt',
      width: 160,
      render: (_, record) => formatTime(record.createdAt)
    },
    {
      title: t('scenes.columns.actions'),
      key: 'action',
      width: 100,
      fixed: 'right',
      render: (_, record) => (
        <div style={{ display: 'flex', gap: 2 }}>
          {canWrite && (
            <>
              <button
                type="button"
                className={styles.actBtn}
                title={t('common.edit')}
                onClick={() => {
                  setEditingScene(record)
                  setFormOpen(true)
                }}
              >
                <PencilSimpleIcon size={14} />
              </button>
              <Popconfirm
                title={t('scenes.deleteConfirmTitle')}
                description={t('scenes.deleteConfirm', { name: record.name })}
                okText={t('common.delete')}
                okButtonProps={{ danger: true }}
                cancelText={t('common.cancel')}
                onConfirm={() => { deleteScene.mutate(record.name); }}
              >
                <button type="button" className={`${styles.actBtn} ${styles.actBtnDanger}`} title={t('common.delete')}>
                  <TrashIcon size={14} />
                </button>
              </Popconfirm>
            </>
          )}
        </div>
      )
    }
  ]

  return (
    <div className={styles.page}>
      <div className={styles.pageHead}>
        <div>
          <div className={styles.pageTitle}>{t('scenes.pageTitle')}</div>
          <div className={styles.pageSub}>{t('scenes.pageSub')}</div>
        </div>
        {canWrite && (
          <PrimaryButton
            icon={<PlusIcon size={16} weight="bold" />}
            onClick={() => {
              setEditingScene(null)
              setFormOpen(true)
            }}
          >
            {t('scenes.create')}
          </PrimaryButton>
        )}
      </div>

      <div className={styles.toolbar}>
          <NameSearch
            placeholder={t('scenes.searchPlaceholder')}
            onSearch={setKeywords}
            realtime
          />
      </div>

      {isLoading ? (
        <div className={styles.loadingWrap}>
          <Spin size="medium" />
        </div>
      ) : (
        <BorderedTable<Scene>
          columns={columns}
          dataSource={filteredScenes}
          rowKey="name"
          size="middle"
          scroll={{ x: 960 }}
          pagination={{
            pageSize: 10,
            showSizeChanger: true,
            showTotal: (total) => t('common.totalItems', { total })
          }}
        />
      )}

      <SceneForm
        open={formOpen}
        editingScene={editingScene}
        onClose={() => { setFormOpen(false); }}
      />
    </div>
  )
}
