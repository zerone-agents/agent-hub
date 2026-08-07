import { Tag, Tooltip } from 'antd'
import { createStyles } from 'antd-style'
import type { AgentDetail } from '@/api/agents'
import { tokens as t } from '@/styles/tokens'
import McpServerTooltip from './McpServerTooltip'

const useStyles = createStyles(({ css }) => ({
  grid: css`
    padding: 16px 20px;
    display: flex;
    flex-direction: column;
    gap: 12px;
    border-bottom: 1px solid ${t.inkLighter};
  `,
  row: css`
    display: flex;
    align-items: flex-start;
    gap: 12px;
  `,
  rowLabel: css`
    flex: 0 0 100px;
    color: ${t.textTertiary};
    font-size: 13px;
    line-height: 24px;
  `,
  rowTags: css`
    flex: 1;
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    align-items: center;
  `,
  subagentTag: css`
    cursor: help;
    &:hover {
      background: ${t.inkSubtle};
    }
  `,
}))

type Props = Pick<
  AgentDetail,
  'allowedTools' | 'mcpServers' | 'subagents' | 'datasets' | 'availableSkills' | 'maxTurns' | 'maxSessionTurns'
>

export default function AgentDetailGrid(props: Props) {
  const { styles } = useStyles()

  const allowedTools = props.allowedTools
  const mcpServers = props.mcpServers
  const subagents = props.subagents
  const datasets = props.datasets
  const availableSkills = props.availableSkills

  const hasTools = !!allowedTools && allowedTools.length > 0
  const hasMcps = !!mcpServers && Object.keys(mcpServers).length > 0
  const hasSubagents = !!subagents && Object.keys(subagents).length > 0
  const hasDatasets = !!datasets && Object.keys(datasets).length > 0
  const hasSkills = !!availableSkills && availableSkills.length > 0
  // Limits row keys off maxSessionTurns (the optional one). maxTurns always
  // has a runtime default, so rendering on maxTurns alone would force the
  // grid to never return null for bare agents and break the existing
  // hide-when-empty behavior.
  const hasLimits = props.maxSessionTurns !== undefined

  if (!hasTools && !hasMcps && !hasSubagents && !hasDatasets && !hasSkills && !hasLimits) {
    return null
  }

  return (
    <div className={styles.grid}>
      {hasLimits && (
        <div className={styles.row}>
          <div className={styles.rowLabel} id="agent-detail-limits-label">Limits</div>
          <div className={styles.rowTags} role="group" aria-labelledby="agent-detail-limits-label">
            <Tag>maxTurns: {props.maxTurns}</Tag>
            <Tag>maxSessionTurns: {props.maxSessionTurns}</Tag>
          </div>
        </div>
      )}
      {hasTools && (
        <div className={styles.row}>
          <div className={styles.rowLabel} id="agent-detail-tools-label">Tools</div>
          <div className={styles.rowTags} role="group" aria-labelledby="agent-detail-tools-label">
            {allowedTools.map((tool) => (
              <Tag key={tool}>{tool}</Tag>
            ))}
          </div>
        </div>
      )}
      {hasMcps && (
        <div className={styles.row}>
          <div className={styles.rowLabel} id="agent-detail-mcp-label">MCP</div>
          <div className={styles.rowTags} role="group" aria-labelledby="agent-detail-mcp-label">
            {Object.entries(mcpServers).map(([name, server]) => (
              <McpServerTooltip key={name} name={name} server={server} />
            ))}
          </div>
        </div>
      )}
      {hasSubagents && (
        <div className={styles.row}>
          <div className={styles.rowLabel} id="agent-detail-subagents-label">Subagents</div>
          <div className={styles.rowTags} role="group" aria-labelledby="agent-detail-subagents-label">
            {Object.entries(subagents).map(([name, info]) =>
              info.description ? (
                <Tooltip key={name} title={info.description} placement="bottom">
                  <Tag className={styles.subagentTag} tabIndex={0}>
                    {name}
                  </Tag>
                </Tooltip>
              ) : (
                <Tag key={name}>{name}</Tag>
              )
            )}
          </div>
        </div>
      )}
      {hasDatasets && (
        <div className={styles.row}>
          <div className={styles.rowLabel} id="agent-detail-datasets-label">Datasets</div>
          <div className={styles.rowTags} role="group" aria-labelledby="agent-detail-datasets-label">
            {Object.values(datasets).map((desc) => (
              <Tooltip key={desc} title={desc}>
                <Tag>{desc.length > 10 ? `${desc.slice(0, 10)}…` : desc}</Tag>
              </Tooltip>
            ))}
          </div>
        </div>
      )}
      {hasSkills && (
        <div className={styles.row}>
          <div className={styles.rowLabel} id="agent-detail-skills-label">Skills</div>
          <div className={styles.rowTags} role="group" aria-labelledby="agent-detail-skills-label">
            {availableSkills.map((skill) => (
              <Tooltip key={skill.name} title={skill.description}>
                <Tag className={styles.subagentTag} tabIndex={0}>
                  {skill.name}
                </Tag>
              </Tooltip>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
