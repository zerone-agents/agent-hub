import { useState, useMemo } from 'react'
import { Spin, Popconfirm } from 'antd'
import NameSearch from '@/components/NameSearch'
import { Plus, PencilSimple, Trash, Wrench } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import PrimaryButton from '@/components/PrimaryButton'
import { useTools, useDeleteTool } from '@/queries/useTools'
import type { Tool } from '@/api/tools'
import { formatTime } from '@/utils/time'
import { tokens as t } from '@/styles/tokens'
import EntityCard from '@/components/EntityCard'
import CardGrid from '@/components/CardGrid'
import ToolForm from './ToolForm'

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
    font-size: ${t.textLg};
    font-weight: 600;
    color: ${t.text};
    margin-bottom: 6px;
  `,
  emptyDesc: css`
    color: ${t.textTertiary};
    font-size: ${t.textSm};
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

export default function ToolListPage() {
  const { styles } = useStyles()
  const { data: tools = [], isLoading } = useTools()
  const deleteTool = useDeleteTool()

  const [formOpen, setFormOpen] = useState(false)
  const [editingTool, setEditingTool] = useState<Tool | null>(null)

  // 搜索
  const [keywords, setKeywords] = useState('')

  // 按关键词过滤，结果按标识首字母排序
  const filteredTools = useMemo(() => {
    let result: Tool[]
    if (!keywords) {
      result = tools
    } else {
      const kw = keywords.toLowerCase()
      result = tools.filter((tool) => {
        const fields = [tool.title, tool.name, tool.description]
        return fields.some((f) => f.toLowerCase().includes(kw))
      })
    }
    return result.sort((a, b) => a.name.localeCompare(b.name))
  }, [tools, keywords])

  const showCreate = () => {
    setEditingTool(null)
    setFormOpen(true)
  }

  const showEdit = (tool: Tool) => {
    setEditingTool(tool)
    setFormOpen(true)
  }

  const handleDelete = async (name: string) => {
    await deleteTool.mutateAsync(name)
  }

  return (
    <div className={styles.page}>
      <div className={styles.pageHead}>
        <div>
          <div className={styles.pageTitle}>工具管理</div>
          <div className={styles.pageSub}>管理所有可用的 AI 工具定义</div>
        </div>
        <PrimaryButton icon={<Plus size={16} weight="bold" />} onClick={showCreate}>
          新建工具
        </PrimaryButton>
      </div>

      <div className={styles.toolbar}>
          <NameSearch
            placeholder="搜索工具名称"
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
            <Wrench size={48} weight="thin" color={t.textMuted} />
          </div>
          <div className={styles.emptyTitle}>{keywords ? '未找到匹配的工具' : '暂无工具'}</div>
          <div className={styles.emptyDesc}>{keywords ? '请尝试其他关键词' : '创建您的第一个工具以开始使用'}</div>
        </div>
      ) : (
        <CardGrid>
          {filteredTools.map((tool) => (
            <EntityCard
              key={tool.name}
              icon={tool.name[0].toUpperCase()}
              title={tool.title || tool.name}
              subtitle={tool.name}
              headerExtra={
                tool.isDefault ? (
                  <span
                    style={{
                      display: 'inline-block',
                      padding: '1px 7px',
                      borderRadius: 3,
                      fontSize: 10,
                      fontWeight: 600,
                      background: 'color-mix(in srgb, var(--foreground) 8%, transparent)',
                      color: '#1a3a5c'
                    }}
                  >
                    默认
                  </span>
                ) : null
              }
              description={tool.description || '暂无描述'}
              footerLeft={formatTime(tool.createdAt)}
              footerRight={
                <div style={{ display: 'flex', gap: 2 }}>
                  <button
                    type="button"
                    className={styles.actBtn}
                    title="编辑"
                    onClick={() => { showEdit(tool); }}
                  >
                    <PencilSimple size={14} />
                  </button>
                  <Popconfirm
                    title="确认删除？"
                    description={`删除 "${tool.name}"？此操作不可撤销。`}
                    okText="删除"
                    okButtonProps={{ danger: true }}
                    cancelText="取消"
                    onConfirm={() => handleDelete(tool.name)}
                  >
                    <button
                      type="button"
                      className={`${styles.actBtn} ${styles.actBtnDanger}`}
                      title="删除"
                    >
                      <Trash size={14} />
                    </button>
                  </Popconfirm>
                </div>
              }
            />
          ))}
        </CardGrid>
      )}

      <ToolForm
        open={formOpen}
        editingTool={editingTool}
        onClose={() => { setFormOpen(false); }}
      />
    </div>
  )
}
