import { useMemo, useState } from 'react'
import { Alert, Button, Empty, Popconfirm, Spin, Tag, Tooltip } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import {
  ArrowRightIcon,
  ArrowsLeftRightIcon,
  PencilSimpleIcon,
  PlusIcon,
  TrashIcon
} from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import { useNavigate } from 'react-router'
import type { AgentRelation, RelationAction } from '@/api/agent-relations'
import type { Agent } from '@/api/agents'
import BorderedTable from '@/components/BorderedTable'
import NameSearch from '@/components/NameSearch'
import PrimaryButton from '@/components/PrimaryButton'
import StatusBadge from '@/components/StatusBadge'
import { useCanWrite } from '@/hooks/useCanWrite'
import { useAgents } from '@/queries/useAgents'
import {
  useAgentRelations,
  useDeleteAgentRelation
} from '@/queries/useAgentRelations'
import { tokens as t } from '@/styles/tokens'
import RelationForm from './RelationForm'
import {
  ACTIONS,
  CONTEXT_POLICIES,
  DELIVERY_POLICIES,
  RELATION_TYPES,
  STANCES,
  optionLabel
} from './relationOptions'

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
    align-items: flex-start;
    justify-content: space-between;
    gap: 20px;
    margin-bottom: 20px;
    @media (max-width: 768px) { flex-direction: column; gap: 14px; }
  `,
  pageTitle: css`
    color: ${t.text};
    font-size: ${t.text3xl};
    font-weight: 700;
    line-height: 1.15;
    letter-spacing: -0.03em;
  `,
  pageSub: css`
    max-width: 720px;
    margin-top: 5px;
    color: ${t.textTertiary};
    font-size: ${t.textBase};
    line-height: 1.6;
  `,
  modelStrip: css`
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    margin-bottom: 20px;
    overflow: hidden;
    border: 1px solid color-mix(in srgb, var(--foreground) 8%, transparent);
    border-radius: ${t.radius}px;
    background: ${t.surface};
    box-shadow: ${t.elevation1};
    @media (max-width: 720px) { grid-template-columns: 1fr; }
  `,
  modelItem: css`
    display: flex;
    align-items: center;
    gap: 11px;
    min-width: 0;
    padding: 14px 16px;
    border-right: 1px solid color-mix(in srgb, var(--foreground) 7%, transparent);
    &:last-child { border-right: 0; }
    @media (max-width: 720px) {
      border-right: 0;
      border-bottom: 1px solid color-mix(in srgb, var(--foreground) 7%, transparent);
      &:last-child { border-bottom: 0; }
    }
  `,
  modelIndex: css`
    display: grid;
    width: 25px;
    height: 25px;
    flex: 0 0 auto;
    place-items: center;
    border-radius: 50%;
    background: ${t.inkSubtle};
    color: ${t.ink};
    font-family: ${t.fontMono};
    font-size: ${t.textXs};
    font-weight: 700;
  `,
  modelCopy: css`
    min-width: 0;
    color: ${t.textSecondary};
    font-size: ${t.textSm};
    line-height: 1.45;
    strong { color: ${t.text}; font-weight: 650; }
  `,
  toolbar: css`
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    margin-bottom: 16px;
  `,
  loading: css`
    display: flex;
    justify-content: center;
    padding: 80px 0;
  `,
  empty: css`
    display: flex;
    flex-direction: column;
    align-items: center;
    padding: 64px 24px;
  `,
  edge: css`
    display: grid;
    grid-template-columns: minmax(100px, 1fr) 24px minmax(100px, 1fr);
    align-items: center;
    gap: 7px;
  `,
  agentNode: css`
    min-width: 0;
  `,
  agentTitle: css`
    overflow: hidden;
    color: ${t.text};
    font-size: ${t.textSm};
    font-weight: 650;
    text-overflow: ellipsis;
    white-space: nowrap;
  `,
  agentName: css`
    overflow: hidden;
    margin-top: 1px;
    color: ${t.textMuted};
    font-family: ${t.fontMono};
    font-size: 11px;
    text-overflow: ellipsis;
    white-space: nowrap;
  `,
  edgeArrow: css`
    display: grid;
    place-items: center;
    color: ${t.ink};
  `,
  policy: css`
    color: ${t.textSecondary};
    font-size: ${t.textSm};
    line-height: 1.55;
  `,
  constraint: css`
    display: block;
    max-width: 240px;
    overflow: hidden;
    color: ${t.textTertiary};
    text-overflow: ellipsis;
    white-space: nowrap;
  `,
  actions: css`
    display: flex;
    gap: 2px;
  `,
  actionButton: css`
    display: grid;
    width: 30px;
    height: 30px;
    place-items: center;
    border: 0;
    border-radius: ${t.radiusSm}px;
    background: transparent;
    color: ${t.textMuted};
    cursor: pointer;
    &:hover { color: ${t.ink}; background: ${t.inkSubtle}; }
    &:focus-visible { outline: 2px solid ${t.ink}; outline-offset: 1px; }
  `,
  danger: css`
    &:hover { color: ${t.danger}; background: color-mix(in srgb, ${t.danger} 8%, transparent); }
  `
}))

function getAgentTitle(agents: Agent[], id: number, fallback: string): string {
  const agent = agents.find((item) => item.id === id)
  return agent?.config.title?.zh ?? agent?.config.title?.en ?? fallback
}

function actionLabels(actions: RelationAction[]): string[] {
  return actions.map((action) => optionLabel(ACTIONS, action))
}

export default function RelationListPage() {
  const { styles } = useStyles()
  const { data: relations = [], isLoading, isError, refetch } = useAgentRelations()
  const { data: agents = [], isLoading: agentsLoading } = useAgents()
  const deleteRelation = useDeleteAgentRelation()
  const navigate = useNavigate()
  const canWrite = useCanWrite()
  const [keywords, setKeywords] = useState('')
  const [formOpen, setFormOpen] = useState(false)
  const [editingRelation, setEditingRelation] = useState<AgentRelation | null>(null)

  const filteredRelations = useMemo(() => {
    const query = keywords.trim().toLowerCase()
    if (!query) return relations
    return relations.filter((relation) => [
      relation.sourceAgentName,
      relation.targetAgentName,
      relation.scope,
      optionLabel(RELATION_TYPES, relation.relationType),
      optionLabel(STANCES, relation.stance),
      ...actionLabels(relation.allowedActions),
      relation.constraint
    ].some((value) => value.toLowerCase().includes(query)))
  }, [keywords, relations])

  const columns: ColumnsType<AgentRelation> = [
    {
      title: '有向关系',
      key: 'edge',
      width: 330,
      render: (_, relation) => (
        <div className={styles.edge}>
          <div className={styles.agentNode}>
            <div className={styles.agentTitle}>{getAgentTitle(agents, relation.sourceAgentId, relation.sourceAgentName)}</div>
            <div className={styles.agentName}>{relation.sourceAgentName}</div>
          </div>
          <span className={styles.edgeArrow}><ArrowRightIcon size={16} weight="bold" /></span>
          <div className={styles.agentNode}>
            <div className={styles.agentTitle}>{getAgentTitle(agents, relation.targetAgentId, relation.targetAgentName)}</div>
            <div className={styles.agentName}>{relation.targetAgentName}</div>
          </div>
        </div>
      )
    },
    {
      title: '关系 / 立场',
      key: 'semantics',
      width: 170,
      render: (_, relation) => {
        const stance = STANCES.find((item) => item.value === relation.stance)
        return (
          <div>
            <div style={{ color: t.text, fontWeight: 600 }}>{optionLabel(RELATION_TYPES, relation.relationType)}</div>
            <Tag color={stance?.color} style={{ marginTop: 5 }}>{stance?.label ?? relation.stance}</Tag>
          </div>
        )
      }
    },
    {
      title: '允许动作',
      key: 'allowedActions',
      width: 220,
      render: (_, relation) => (
        <div>{actionLabels(relation.allowedActions).map((label) => <Tag key={label}>{label}</Tag>)}</div>
      )
    },
    {
      title: '消息策略',
      key: 'policy',
      width: 160,
      render: (_, relation) => (
        <div className={styles.policy}>
          <div>{optionLabel(CONTEXT_POLICIES, relation.contextPolicy)}</div>
          <div>{optionLabel(DELIVERY_POLICIES, relation.deliveryPolicy)}</div>
        </div>
      )
    },
    {
      title: '范围',
      dataIndex: 'scope',
      key: 'scope',
      width: 110,
      render: (scope: string) => <Tag>{scope}</Tag>
    },
    {
      title: '约束',
      dataIndex: 'constraint',
      key: 'constraint',
      width: 220,
      render: (constraint: string) => constraint ? (
        <Tooltip title={constraint}><span className={styles.constraint}>{constraint}</span></Tooltip>
      ) : <span style={{ color: t.textMuted }}>—</span>
    },
    {
      title: '状态',
      key: 'enabled',
      width: 86,
      render: (_, relation) => <StatusBadge enabled={relation.enabled} />
    },
    {
      title: '操作',
      key: 'actions',
      width: 94,
      fixed: 'right',
      render: (_, relation) => canWrite ? (
        <div className={styles.actions}>
          <button
            type="button"
            className={styles.actionButton}
            title="编辑"
            onClick={() => {
              setEditingRelation(relation)
              setFormOpen(true)
            }}
          >
            <PencilSimpleIcon size={15} />
          </button>
          <Popconfirm
            title="删除这条有向关系？"
            description="反向关系不会被一并删除。"
            okText="删除"
            okButtonProps={{ danger: true }}
            cancelText="取消"
            onConfirm={() => { deleteRelation.mutate(relation.id) }}
          >
            <button type="button" className={`${styles.actionButton} ${styles.danger}`} title="删除">
              <TrashIcon size={15} />
            </button>
          </Popconfirm>
        </div>
      ) : null
    }
  ]

  const openCreate = () => {
    setEditingRelation(null)
    setFormOpen(true)
  }

  return (
    <div className={styles.page}>
      <div className={styles.pageHead}>
        <div>
          <div className={styles.pageTitle}>组织关系</div>
          <div className={styles.pageSub}>
            Agent 创建完成后，在这里定义谁可以联系谁、以什么身份协作，以及消息能携带多少上下文。
          </div>
        </div>
        {canWrite && (
          <Tooltip title={agents.length < 2 ? '至少需要先创建两个 Agent' : undefined}>
            <span>
              <PrimaryButton
                icon={<PlusIcon size={16} weight="bold" />}
                onClick={agents.length < 2 ? () => { void navigate('/agents') } : openCreate}
              >
                {agents.length < 2 ? '先创建 Agent' : '新建关系'}
              </PrimaryButton>
            </span>
          </Tooltip>
        )}
      </div>

      <div className={styles.modelStrip} aria-label="关系模型说明">
        <div className={styles.modelItem}>
          <span className={styles.modelIndex}>01</span>
          <div className={styles.modelCopy}><strong>独立资源</strong><br />不写进 Agent 自身配置</div>
        </div>
        <div className={styles.modelItem}>
          <span className={styles.modelIndex}>02</span>
          <div className={styles.modelCopy}><strong>有向边</strong><br />A → B 与 B → A 分别维护</div>
        </div>
        <div className={styles.modelItem}>
          <span className={styles.modelIndex}>03</span>
          <div className={styles.modelCopy}><strong>可执行约定</strong><br />MCP 后续按动作白名单通信</div>
        </div>
      </div>

      <div className={styles.toolbar}>
        <NameSearch placeholder="搜索 Agent、关系、动作或范围" realtime onSearch={setKeywords} />
        <Tag icon={<ArrowsLeftRightIcon size={13} />}>共 {filteredRelations.length} 条有向边</Tag>
      </div>

      {isError ? (
        <Alert
          type="error"
          showIcon
          title="组织关系加载失败"
          action={<Button type="link" size="small" onClick={() => { void refetch() }}>重新加载</Button>}
        />
      ) : isLoading || agentsLoading ? (
        <div className={styles.loading}><Spin size="medium" /></div>
      ) : filteredRelations.length === 0 ? (
        <div className={styles.empty}>
          <Empty description={keywords ? '没有匹配的关系' : '还没有组织关系'} />
          {canWrite && !keywords && agents.length >= 2 && (
            <PrimaryButton style={{ marginTop: 14 }} onClick={openCreate}>创建第一条关系</PrimaryButton>
          )}
          {canWrite && !keywords && agents.length < 2 && (
            <PrimaryButton style={{ marginTop: 14 }} onClick={() => { void navigate('/agents') }}>
              去 Agent 管理
            </PrimaryButton>
          )}
        </div>
      ) : (
        <BorderedTable<AgentRelation>
          columns={columns}
          dataSource={filteredRelations}
          rowKey="id"
          size="middle"
          scroll={{ x: 1390 }}
          pagination={{
            pageSize: 10,
            showSizeChanger: true,
            showTotal: (total) => `共 ${total} 条`
          }}
        />
      )}

      <RelationForm
        open={formOpen}
        editingRelation={editingRelation}
        agents={agents}
        onClose={() => { setFormOpen(false) }}
      />
    </div>
  )
}
