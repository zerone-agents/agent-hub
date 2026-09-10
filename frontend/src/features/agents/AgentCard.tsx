import { createElement } from 'react'
import { PencilSimpleIcon, TrashIcon, DiamondsFourIcon, WrenchIcon, StarIcon, CpuIcon, PlusIcon, PlugsConnectedIcon, RocketIcon, BooksIcon } from '@phosphor-icons/react'
import { Popconfirm, Tag, Tooltip, Checkbox } from 'antd'
import { createStyles } from 'antd-style'
import type { Agent } from '@/api/agents'
import EntityCard from '@/components/EntityCard'
import { getIconComponent } from '@/utils/icons'
import { formatTime } from '@/utils/time'
import { tokens as t } from '@/styles/tokens'

const useStyles = createStyles(({ css }) => ({
  iconImg: css`
    width: 100%;
    height: 100%;
    object-fit: cover;
    border-radius: ${t.radiusSm}px;
  `,
  stats: css`
    display: flex;
    gap: 12px;
    flex-grow: 1;
    align-content: flex-start;
    flex-wrap: wrap;
  `,
  statLink: css`
    display: inline-flex;
    align-items: center;
    gap: 4px;
    font-size: 11px;
    color: ${t.textTertiary};
    cursor: pointer;
    transition: color 0.15s;
    &:hover { color: ${t.ink}; }
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
  selectableWrap: css`
    position: relative;
    border-radius: ${t.radius}px;
    border: 2px solid transparent;
    cursor: pointer;
    transition: border-color 0.15s;
    &:hover { border-color: color-mix(in srgb, var(--primary) 34%, transparent); }
  `,
  selectableSelected: css`
    border-color: var(--primary);
  `,
  cardCheckbox: css`
    position: absolute;
    top: 8px;
    left: 8px;
    z-index: 1;
  `,
  statsDisabled: css`
    pointer-events: none;
    opacity: 0.55;
  `
}))

interface AgentCardProps {
  agent: Agent
  modelDisplayName: string
  /** member 无管理台写权限：隐藏编辑/删除按钮（部署按钮只读打开弹窗，全角色可见）。 */
  canWrite: boolean
  onEdit: (agent: Agent) => void
  onDelete: (name: string) => void
  onEditSubagents: (agent: Agent) => void
  onEditTools: (agent: Agent) => void
  onEditSkills: (agent: Agent) => void
  onEditMcps: (agent: Agent) => void
  onEditModel: (agent: Agent) => void
  onDeploy: (agent: Agent) => void
  onEditKnowledge: (agent: Agent) => void
  /** 批量选择模式（#141）：显示复选框、隐藏单卡操作、stats 链接禁用、整卡点击切换 */
  selectionMode?: boolean
  selected?: boolean
  onToggleSelect?: (name: string) => void
}

export default function AgentCard({
  agent, modelDisplayName, canWrite, onEdit, onDelete,
  onEditSubagents, onEditTools, onEditSkills, onEditMcps, onEditModel, onDeploy, onEditKnowledge,
  selectionMode = false, selected = false, onToggleSelect,
}: AgentCardProps) {
  const { styles } = useStyles()

  const IconCmp = agent.config.iconName ? getIconComponent(agent.config.iconName) : null
  const iconColor = agent.config.iconColor ?? '#6B7280'
  // Render via createElement rather than JSX (<IconCmp />) so the linter
  // doesn't mistake IconCmp for a component defined during render —
  // getIconComponent returns a stable component reference (icon from catalog).
  const icon = IconCmp ? (
    createElement(IconCmp, { size: 20, weight: 'duotone', color: iconColor })
  ) : agent.config.icon ? (
    <img src={agent.config.icon} alt={agent.name} className={styles.iconImg} />
  ) : (
    agent.name[0].toUpperCase()
  )

  const defaultBadgeStyle: React.CSSProperties = {
    display: 'inline-block',
    padding: '1px 7px',
    borderRadius: '3px',
    fontSize: '10px',
    fontWeight: 600,
    letterSpacing: '0.02em',
    textTransform: 'uppercase',
    background: 'color-mix(in srgb, var(--foreground) 8%, transparent)',
    color: t.ink
  }

  const platformBadgeStyle: React.CSSProperties = {
    ...defaultBadgeStyle,
    background: 'color-mix(in srgb, var(--primary) 10%, transparent)',
    color: 'var(--primary)'
  }

  const hasPending = agent.pendingArtifactUpdates != null &&
    (agent.pendingArtifactUpdates.tools.length > 0 || agent.pendingArtifactUpdates.skills.length > 0)

  const card = (
    <EntityCard
      icon={icon}
      title={agent.config.title?.zh ?? agent.config.title?.en ?? agent.name}
      subtitle={agent.name}
      headerExtra={
        <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-end', gap: 4 }}>
          {hasPending && (
            <Tooltip title="工具/技能已更新，重新部署后生效">
              <Tag color="orange" data-testid="pending-badge">待更新</Tag>
            </Tooltip>
          )}
          {agent.isDefault && <span style={defaultBadgeStyle}>默认</span>}
          {agent.desktopEnabled && <span style={platformBadgeStyle}>桌面端</span>}
          {agent.mobileEnabled && <span style={platformBadgeStyle}>手机端</span>}
        </div>
      }
      description={agent.config.description?.zh ?? agent.config.description?.en ?? '暂无描述'}
      bodyExtra={
        <div className={selectionMode ? `${styles.stats} ${styles.statsDisabled}` : styles.stats}>
          <span className={styles.statLink} onClick={() => { onEditSubagents(agent); }}>
            <DiamondsFourIcon size={12} />
            {agent.subagents?.length ?? 0} 子代理
          </span>
          <span className={styles.statLink} onClick={() => { onEditTools(agent); }}>
            <WrenchIcon size={12} />
            {agent.tools?.length ?? 0} 工具
          </span>
          <span className={styles.statLink} onClick={() => { onEditSkills(agent); }}>
            <StarIcon size={12} />
            {agent.skills?.length ?? 0} 技能
          </span>
          <span className={styles.statLink} onClick={() => { onEditMcps(agent); }}>
            <PlugsConnectedIcon size={12} />
            {agent.mcps?.length ?? 0} MCP
          </span>
          <span className={styles.statLink} onClick={() => { onEditKnowledge(agent); }}>
            <BooksIcon size={12} />
            {agent.datasets?.length ?? 0} 知识库
          </span>
          <span className={styles.statLink} onClick={() => { onEditModel(agent); }}>
            <CpuIcon size={12} />
            {!modelDisplayName && <PlusIcon size={10} />}
            {modelDisplayName || '未选模型'}
          </span>
        </div>
      }
      footerLeft={formatTime(agent.createdAt)}
      footerRight={selectionMode ? undefined : (
        <>
          <button type="button" className={styles.actBtn} title="部署" onClick={() => { onDeploy(agent); }}>
            <RocketIcon size={14} />
          </button>
          {canWrite && (
            <>
              <button type="button" className={styles.actBtn} title="编辑" onClick={() => { onEdit(agent); }}>
                <PencilSimpleIcon size={14} />
              </button>
              <Popconfirm
                title="确认删除？"
                description={`删除 "${agent.name}"？此操作不可撤销。`}
                okText="删除"
                okButtonProps={{ danger: true }}
                cancelText="取消"
                onConfirm={() => { onDelete(agent.name); }}
              >
                <button type="button" className={`${styles.actBtn} ${styles.actBtnDanger}`} title="删除">
                  <TrashIcon size={14} />
                </button>
              </Popconfirm>
            </>
          )}
        </>
      )}
    />
  )

  if (!selectionMode) return card

  return (
    <div
      className={`${styles.selectableWrap}${selected ? ` ${styles.selectableSelected}` : ''}`}
      aria-label={`选择 ${agent.name}`}
      tabIndex={0}
      onClick={() => { onToggleSelect?.(agent.name); }}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault()
          onToggleSelect?.(agent.name)
        }
      }}
    >
      <span
        className={styles.cardCheckbox}
        onClick={(e) => { e.stopPropagation(); onToggleSelect?.(agent.name); }}
      >
        <Checkbox checked={selected} />
      </span>
      {card}
    </div>
  )
}
