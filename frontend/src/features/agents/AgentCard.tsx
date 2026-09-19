import { createElement } from 'react'
import { useTranslation } from 'react-i18next'
import { PencilSimpleIcon, TrashIcon, DiamondsFourIcon, WrenchIcon, StarIcon, CpuIcon, PlusIcon, PlugsConnectedIcon, RocketIcon, BooksIcon, ShareNetworkIcon } from '@phosphor-icons/react'
import { Popconfirm, Tag, Tooltip, Checkbox } from 'antd'
import { createStyles } from 'antd-style'
import type { Agent } from '@/api/agents'
import EntityCard from '@/components/EntityCard'
import { hasPendingArtifactUpdates } from './pendingArtifactUpdates'
import { getIconComponent } from '@/utils/icons'
import { formatTime } from '@/utils/time'
import { tokens as tk } from '@/styles/tokens'

const useStyles = createStyles(({ css }) => ({
  iconImg: css`
    width: 100%;
    height: 100%;
    object-fit: cover;
    border-radius: ${tk.radiusSm}px;
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
    color: ${tk.textTertiary};
    cursor: pointer;
    transition: color 0.15s;
    &:hover { color: ${tk.ink}; }
  `,
  actBtn: css`
    width: 30px;
    height: 30px;
    display: flex;
    align-items: center;
    justify-content: center;
    border: none;
    background: transparent;
    border-radius: ${tk.radiusSm}px;
    color: ${tk.textMuted};
    cursor: pointer;
    transition: all 0.15s;
    &:hover { background: ${tk.inkSubtle}; color: ${tk.ink}; }
  `,
  actBtnDanger: css`
    &:hover { background: rgba(220, 38, 38, 0.06); color: ${tk.danger}; }
  `,
  selectableWrap: css`
    position: relative;
    border-radius: ${tk.radius}px;
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
  onViewRelations?: (agent: Agent) => void
  /** 批量选择模式（#141）：显示复选框、隐藏单卡操作、stats 链接禁用、整卡点击切换 */
  selectionMode?: boolean
  selected?: boolean
  onToggleSelect?: (name: string) => void
}

export default function AgentCard({
  agent, modelDisplayName, canWrite, onEdit, onDelete,
  onEditSubagents, onEditTools, onEditSkills, onEditMcps, onEditModel, onDeploy, onEditKnowledge,
  onViewRelations, selectionMode = false, selected = false, onToggleSelect,
}: AgentCardProps) {
  const { t } = useTranslation()
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
    color: tk.ink
  }

  const platformBadgeStyle: React.CSSProperties = {
    ...defaultBadgeStyle,
    background: 'color-mix(in srgb, var(--primary) 10%, transparent)',
    color: 'var(--primary)'
  }

  const hasPending = hasPendingArtifactUpdates(agent)

  const card = (
    <EntityCard
      icon={icon}
      title={agent.config.title?.zh ?? agent.config.title?.en ?? agent.name}
      subtitle={agent.name}
      headerExtra={
        <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-end', gap: 4 }}>
          {hasPending && (
            <Tooltip title={t('agents.card.pendingTooltip')}>
              <Tag color="orange" data-testid="pending-badge">{t('agents.card.pending')}</Tag>
            </Tooltip>
          )}
          {agent.isDefault && <span style={defaultBadgeStyle}>{t('agents.card.defaultBadge')}</span>}
          {agent.desktopEnabled && <span style={platformBadgeStyle}>{t('agents.card.desktop')}</span>}
          {agent.mobileEnabled && <span style={platformBadgeStyle}>{t('agents.card.mobile')}</span>}
        </div>
      }
      description={agent.config.description?.zh ?? agent.config.description?.en ?? t('agents.card.noDesc')}
      bodyExtra={
        <div className={selectionMode ? `${styles.stats} ${styles.statsDisabled}` : styles.stats}>
          <span className={styles.statLink} onClick={() => { onEditSubagents(agent); }}>
            <DiamondsFourIcon size={12} />
            {t('agents.card.subagentCount', { n: agent.subagents?.length ?? 0 })}
          </span>
          <span className={styles.statLink} onClick={() => { onEditTools(agent); }}>
            <WrenchIcon size={12} />
            {t('agents.card.toolCount', { n: agent.tools?.length ?? 0 })}
          </span>
          <span className={styles.statLink} onClick={() => { onEditSkills(agent); }}>
            <StarIcon size={12} />
            {t('agents.card.skillCount', { n: agent.skills?.length ?? 0 })}
          </span>
          <span className={styles.statLink} onClick={() => { onEditMcps(agent); }}>
            <PlugsConnectedIcon size={12} />
            {agent.mcps?.length ?? 0} MCP
          </span>
          <span className={styles.statLink} onClick={() => { onEditKnowledge(agent); }}>
            <BooksIcon size={12} />
            {t('agents.card.kbCount', { n: agent.datasets?.length ?? 0 })}
          </span>
          <span className={styles.statLink} onClick={() => { onEditModel(agent); }}>
            <CpuIcon size={12} />
            {!modelDisplayName && <PlusIcon size={10} />}
            {modelDisplayName || t('agents.card.noModel')}
          </span>
        </div>
      }
      footerLeft={formatTime(agent.createdAt)}
      footerRight={selectionMode ? undefined : (
        <>
          <button type="button" className={styles.actBtn} title={t('agents.card.relations', { defaultValue: '关系拓扑' })} onClick={() => { onViewRelations?.(agent); }}>
            <ShareNetworkIcon size={14} />
          </button>
          <button type="button" className={styles.actBtn} title={t('agents.card.deploy')} onClick={() => { onDeploy(agent); }}>
            <RocketIcon size={14} />
          </button>
          {canWrite && (
            <>
              <button type="button" className={styles.actBtn} title={t('common.edit')} onClick={() => { onEdit(agent); }}>
                <PencilSimpleIcon size={14} />
              </button>
              <Popconfirm
                title={t('scenes.deleteConfirmTitle')}
                description={t('scenes.deleteConfirm', { name: agent.name })}
                okText={t('common.delete')}
                okButtonProps={{ danger: true }}
                cancelText={t('common.cancel')}
                onConfirm={() => { onDelete(agent.name); }}
              >
                <button type="button" className={`${styles.actBtn} ${styles.actBtnDanger}`} title={t('common.delete')}>
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
      aria-label={t('agents.card.selectAria', { name: agent.name })}
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
