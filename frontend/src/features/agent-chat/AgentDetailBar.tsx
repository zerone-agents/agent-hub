import { useState } from 'react'
import { createStyles } from 'antd-style'
import { useAgentDetail } from '@/queries/useAgentDetail'
import { useAgents } from '@/queries/useAgents'
import { tokens as t } from '@/styles/tokens'
import AgentDetailSummary, { type AgentDetailCounts } from './AgentDetailSummary'
import AgentDetailGrid from './AgentDetailGrid'

const useStyles = createStyles(({ css }) => ({
  wrapper: css`
    background: ${t.surface};
    flex-shrink: 0;
  `,
}))

interface Props {
  agentName: string
}

export default function AgentDetailBar({ agentName }: Props) {
  const { styles } = useStyles()
  const { data, isLoading, isError } = useAgentDetail(agentName)
  const agents = useAgents()
  const [expanded, setExpanded] = useState(false)

  // Silent hide on loading/error/success-no-data. Chat flow continues
  // independently — this panel is non-blocking decoration.
  if (isLoading || isError || !data) return null

  // Prefer the human-readable title from the hub agent record; fall back
  // to the technical identifier when the record/title is unavailable.
  const hubAgent = agents.data?.find((a) => a.name === agentName)
  const displayName = hubAgent?.config.title?.zh ?? hubAgent?.config.title?.en ?? data.name

  const counts: AgentDetailCounts = {
    tools: data.allowedTools?.length ?? 0,
    mcps: data.mcpServers ? Object.keys(data.mcpServers).length : 0,
    skills: data.availableSkills?.length ?? 0,
    subagents: data.subagents ? Object.keys(data.subagents).length : 0,
    datasets: data.datasets ? Object.keys(data.datasets).length : 0,
  }

  return (
    <div className={styles.wrapper}>
      <AgentDetailSummary
        name={displayName}
        model={data.model}
        status={data.status}
        counts={counts}
        expanded={expanded}
        onToggle={() => { setExpanded(!expanded); }}
      />
      {expanded && (
        <AgentDetailGrid
          allowedTools={data.allowedTools}
          mcpServers={data.mcpServers}
          subagents={data.subagents}
          datasets={data.datasets}
          availableSkills={data.availableSkills}
          maxTurns={data.maxTurns}
          maxSessionTurns={data.maxSessionTurns}
        />
      )}
    </div>
  )
}
