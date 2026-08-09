import { useState, useMemo } from 'react'
import { Spin, Popconfirm, Tooltip } from 'antd'
import NameSearch from '@/components/NameSearch'
import { PlusIcon, PencilSimpleIcon, TrashIcon, PlugsConnectedIcon } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import PrimaryButton from '@/components/PrimaryButton'
import { useMcps, useDeleteMcp, useProbeMcp } from '@/queries/useMcps'
import type { Mcp } from '@/api/mcps'
import { formatTime } from '@/utils/time'
import { tokens as t } from '@/styles/tokens'
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
  loadingWrap: css`display: flex; justify-content: center; padding: 80px 0;`,
  emptyState: css`text-align: center; padding: 80px 0;`,
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
  metaLine: css`
    font-family: ${t.fontMono};
    font-size: 11px;
    color: ${t.textMuted};
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
      fontFamily: t.fontMono,
      background: t.inkSubtle,
      color: t.textSecondary,
    }}>
      {name}
    </span>
  )
}

export default function McpListPage() {
  const { styles } = useStyles()
  const { data: mcps = [], isLoading } = useMcps()
  const deleteMcp = useDeleteMcp()
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
          <div className={styles.pageTitle}>MCP 配置</div>
          <div className={styles.pageSub}>管理外部 MCP 服务器配置，供 Agent 绑定使用</div>
        </div>
        <PrimaryButton icon={<PlusIcon size={16} weight="bold" />} onClick={showCreate}>
          新建 MCP
        </PrimaryButton>
      </div>

      <div className={styles.toolbar}>
          <NameSearch
            placeholder="搜索 MCP 名称"
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
            <PlugsConnectedIcon size={48} weight="thin" color={t.textMuted} />
          </div>
          <div className={styles.emptyTitle}>{keywords ? '未找到匹配的 MCP' : '暂无 MCP 配置'}</div>
          <div className={styles.emptyDesc}>{keywords ? '请尝试其他关键词' : '添加您的第一个 MCP 服务器以开始使用'}</div>
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
                      <Badge label="内置" color="rgba(59, 130, 246, 0.08)" />
                    </span>
                  )}
                </div>
              }
              description={mcp.description || '暂无描述'}
              bodyExtra={
                <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
                  <div className={styles.metaLine}>{mcp.url}</div>
                  <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                    <div style={{ fontSize: 13, color: 'var(--text-secondary)', fontWeight: 500 }}>
                      {mcp.isBuiltin
                        ? `${mcp.tools?.length ?? 0} 个内置 tools`
                        : mcp.probeStatus === 'success'
                        ? `${mcp.tools?.length ?? 0} 个 tools · 上次探测 ${formatTime(mcp.lastProbedAt)}`
                        : mcp.probeStatus === 'failed'
                        ? '探测失败'
                        : '未探测'}
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
                <div style={{ display: 'flex', gap: 2 }}>
                  {!mcp.isBuiltin && (
                    <button
                      type="button"
                      className={styles.actBtn}
                      title="探测"
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
                    title="编辑"
                    onClick={() => { showEdit(mcp); }}
                  >
                    <PencilSimpleIcon size={14} />
                  </button>
                  {!mcp.isBuiltin && (
                    <Popconfirm
                      title="确认删除？"
                      description={`删除 "${mcp.name}"？所有 Agent 的绑定关系将一并清除。`}
                      okText="删除"
                      okButtonProps={{ danger: true }}
                      cancelText="取消"
                      onConfirm={() => handleDelete(mcp.name)}
                    >
                      <button
                        type="button"
                        className={`${styles.actBtn} ${styles.actBtnDanger}`}
                        title="删除"
                      >
                        <TrashIcon size={14} />
                      </button>
                    </Popconfirm>
                  )}
                </div>
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
