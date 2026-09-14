import { Tooltip } from 'antd'
import { CaretDownIcon, CaretUpIcon, WarningCircleIcon } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import { tokens as t } from '@/styles/tokens'

const useStyles = createStyles(({ css }) => ({
  // 内嵌页眉的紧凑形态（原整行 bar 改为页眉内联胶囊）：宽度自适应、无底边框、
  // 透明背景 + hover 提示可点；详情面板由 AgentDetailBar 以页眉下缘浮层展开。
  bar: css`
    all: unset;
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 6px 10px;
    box-sizing: border-box;
    text-align: left;
    font-family: inherit;
    font-size: inherit;
    border-radius: var(--radius);
    cursor: pointer;
    flex-shrink: 1;
    min-width: 0;
    user-select: none;
    transition: background 0.15s;
    &:hover {
      background: ${t.surfaceHover};
    }
    &:focus-visible {
      outline: 2px solid ${t.ink};
      outline-offset: -2px;
    }
    @media (max-width: 768px) {
      gap: 6px;
      padding: 6px 8px;
    }
  `,
  name: css`
    font-size: 15px;
    font-weight: 600;
    color: ${t.text};
    display: inline-flex;
    align-items: center;
    gap: 6px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    min-width: 0;
  `,
  warning: css`
    color: ${t.danger};
    display: inline-flex;
    align-items: center;
  `,
  modelPill: css`
    background: ${t.inkLight};
    color: ${t.text};
    padding: 2px 8px;
    border-radius: 4px;
    font-size: 13px;
    font-weight: 500;
    white-space: nowrap;
    flex-shrink: 0;
  `,
  separator: css`
    color: ${t.textMuted};
    font-size: 13px;
  `,
  countWrapper: css`
    display: inline-flex;
    align-items: center;
    gap: 12px;
    @media (max-width: 768px) {
      display: none;
    }
  `,
  count: css`
    font-size: 13px;
    color: ${t.textTertiary};
    & > b {
      color: ${t.text};
      font-weight: 500;
      margin-left: 2px;
    }
  `,
  chevron: css`
    margin-left: 4px;
    color: ${t.textTertiary};
    display: inline-flex;
    align-items: center;
  `,
}))

export interface AgentDetailCounts {
  tools: number
  mcps: number
  skills: number
  subagents: number
  datasets: number
}

interface Props {
  name: string
  model: string
  status: 'ready' | 'unavailable'
  counts: AgentDetailCounts
  expanded: boolean
  onToggle: () => void
}

export default function AgentDetailSummary({
  name,
  model,
  status,
  counts,
  expanded,
  onToggle,
}: Props) {
  const { styles } = useStyles()
  const Chevron = expanded ? CaretUpIcon : CaretDownIcon

  const countEntries: [string, number][] = [
    ['Tools', counts.tools],
    ['MCP', counts.mcps],
    ['Skills', counts.skills],
    ['Subagents', counts.subagents],
    ['Datasets', counts.datasets],
  ]

  return (
    <button
      type="button"
      className={styles.bar}
      onClick={onToggle}
      aria-expanded={expanded}
    >
      <span className={styles.name}>
        {name}
        {status === 'unavailable' && (
          <Tooltip title="Agent 配置解析失败，可能无法调用">
            <span className={styles.warning} data-testid="status-warning">
              <WarningCircleIcon size={14} weight="fill" />
            </span>
          </Tooltip>
        )}
      </span>
      <span className={styles.modelPill}>{model}</span>
      {countEntries.map(([label, n]) => (
        <span key={label} className={styles.countWrapper}>
          <span className={styles.separator}>·</span>
          <span className={styles.count}>
            {label} <b>{n}</b>
          </span>
        </span>
      ))}
      <span className={styles.chevron}>
        <Chevron size={14} weight="fill" />
      </span>
    </button>
  )
}
