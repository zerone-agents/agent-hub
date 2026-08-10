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
import { formatTime } from '@/utils/time'
import { tokens as t } from '@/styles/tokens'
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
    font-size: ${t.text3xl}; font-weight: 700; color: ${t.text};
    letter-spacing: -0.03em; line-height: 1.15;
  `,
  pageSub: css`
    margin-top: 4px; font-size: ${t.textBase}; color: ${t.textTertiary};
  `,
  loadingWrap: css`
    display: flex; justify-content: center; padding: 80px 0;
  `,
  actBtn: css`
    width: 30px; height: 30px; display: flex; align-items: center; justify-content: center;
    border: none; background: transparent; border-radius: ${t.radiusSm}px;
    color: ${t.textMuted}; cursor: pointer; transition: all 0.15s;
    &:hover { background: ${t.inkSubtle}; color: ${t.ink}; }
  `,
  actBtnDanger: css`
    &:hover { background: rgba(220, 38, 38, 0.06); color: ${t.danger}; }
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
  const { data: scenes = [], isLoading } = useScenes()
  const { data: agents = [] } = useAgents()
  const deleteScene = useDeleteScene()

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
    { title: '场景标识', dataIndex: 'name', key: 'name', width: 160 },
    {
      title: '场景名称',
      key: 'title',
      width: 180,
      render: (_, record) => record.title || record.titleEn || record.name
    },
    {
      title: '关联 Agent',
      key: 'agent',
      width: 180,
      render: (_, record) => getAgentTitle(agents, record.agent)
    },
    {
      title: '提示词',
      key: 'prompt',
      ellipsis: true,
      render: (_, record) => (
        <Tooltip title={record.prompt} placement="topLeft">
          <span style={{ color: t.textTertiary }}>{record.prompt || '-'}</span>
        </Tooltip>
      )
    },
    {
      title: '状态',
      key: 'enabled',
      width: 90,
      render: (_, record) => <StatusBadge enabled={record.enabled} />
    },
    {
      title: '创建时间',
      key: 'createdAt',
      width: 160,
      render: (_, record) => formatTime(record.createdAt)
    },
    {
      title: '操作',
      key: 'action',
      width: 100,
      fixed: 'right',
      render: (_, record) => (
        <div style={{ display: 'flex', gap: 2 }}>
          <button
            type="button"
            className={styles.actBtn}
            title="编辑"
            onClick={() => {
              setEditingScene(record)
              setFormOpen(true)
            }}
          >
            <PencilSimpleIcon size={14} />
          </button>
          <Popconfirm
            title="确认删除？"
            description={`删除 "${record.name}"？此操作不可撤销。`}
            okText="删除"
            okButtonProps={{ danger: true }}
            cancelText="取消"
            onConfirm={() => { deleteScene.mutate(record.name); }}
          >
            <button type="button" className={`${styles.actBtn} ${styles.actBtnDanger}`} title="删除">
              <TrashIcon size={14} />
            </button>
          </Popconfirm>
        </div>
      )
    }
  ]

  return (
    <div className={styles.page}>
      <div className={styles.pageHead}>
        <div>
          <div className={styles.pageTitle}>场景管理</div>
          <div className={styles.pageSub}>管理 Agent 场景配置，组合 Agent 与提示词预设</div>
        </div>
        <PrimaryButton
          icon={<PlusIcon size={16} weight="bold" />}
          onClick={() => {
            setEditingScene(null)
            setFormOpen(true)
          }}
        >
          新建场景
        </PrimaryButton>
      </div>

      <div className={styles.toolbar}>
          <NameSearch
            placeholder="搜索场景名称"
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
            showTotal: (total) => `共 ${total} 条`
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
