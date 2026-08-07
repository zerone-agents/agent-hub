import { createElement } from 'react'
import { PencilSimple, Trash, DiamondsFour, Wrench, Star, Cpu, Plus, PlugsConnected, Rocket, Books } from '@phosphor-icons/react'
import { Popconfirm } from 'antd'
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
  `
}))

interface AgentCardProps {
  agent: Agent
  modelDisplayName: string
  onEdit: (agent: Agent) => void
  onDelete: (name: string) => void
  onEditSubagents: (agent: Agent) => void
  onEditTools: (agent: Agent) => void
  onEditSkills: (agent: Agent) => void
  onEditMcps: (agent: Agent) => void
  onEditModel: (agent: Agent) => void
  onDeploy: (agent: Agent) => void
  onEditKnowledge: (agent: Agent) => void
}

export default function AgentCard({
  agent, modelDisplayName, onEdit, onDelete,
  onEditSubagents, onEditTools, onEditSkills, onEditMcps, onEditModel, onDeploy, onEditKnowledge
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

  return (
    <EntityCard
      icon={icon}
      title={agent.config.title?.zh ?? agent.config.title?.en ?? agent.name}
      subtitle={agent.name}
      headerExtra={
        <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-end', gap: 4 }}>
          {agent.isDefault && <span style={defaultBadgeStyle}>默认</span>}
          {agent.desktopEnabled && <span style={platformBadgeStyle}>桌面端</span>}
          {agent.mobileEnabled && <span style={platformBadgeStyle}>手机端</span>}
        </div>
      }
      description={agent.config.description?.zh ?? agent.config.description?.en ?? '暂无描述'}
      bodyExtra={
        <div className={styles.stats}>
          <span className={styles.statLink} onClick={() => { onEditSubagents(agent); }}>
            <DiamondsFour size={12} />
            {agent.subagents?.length ?? 0} 子代理
          </span>
          <span className={styles.statLink} onClick={() => { onEditTools(agent); }}>
            <Wrench size={12} />
            {agent.tools?.length ?? 0} 工具
          </span>
          <span className={styles.statLink} onClick={() => { onEditSkills(agent); }}>
            <Star size={12} />
            {agent.skills?.length ?? 0} 技能
          </span>
          <span className={styles.statLink} onClick={() => { onEditMcps(agent); }}>
            <PlugsConnected size={12} />
            {agent.mcps?.length ?? 0} MCP
          </span>
          <span className={styles.statLink} onClick={() => { onEditKnowledge(agent); }}>
            <Books size={12} />
            {agent.datasets?.length ?? 0} 知识库
          </span>
          <span className={styles.statLink} onClick={() => { onEditModel(agent); }}>
            <Cpu size={12} />
            {!modelDisplayName && <Plus size={10} />}
            {modelDisplayName || '未选模型'}
          </span>
        </div>
      }
      footerLeft={formatTime(agent.createdAt)}
      footerRight={
        <>
          <button type="button" className={styles.actBtn} title="部署" onClick={() => { onDeploy(agent); }}>
            <Rocket size={14} />
          </button>
          <button type="button" className={styles.actBtn} title="编辑" onClick={() => { onEdit(agent); }}>
            <PencilSimple size={14} />
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
              <Trash size={14} />
            </button>
          </Popconfirm>
        </>
      }
    />
  )
}
