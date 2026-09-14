import { useState } from 'react'
import { createStyles } from 'antd-style'
import { useAgentDetail } from '@/queries/useAgentDetail'
import { usePublicAgents } from '@/queries/useAgents'
import { useAuthMode } from '@/features/login/useAuthMode'
import { useUserInfo } from '@/queries/useUserInfo'
import { isGuestUser } from '@/lib/auth-guest'
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
  const { data: mode } = useAuthMode()
  const { data: user } = useUserInfo()
  const guest = isGuestUser(user, mode?.mode)
  // guest 不发 admin detail 请求（403 注定失败）；formal 现状不变。
  const { data, isLoading, isError } = useAgentDetail(agentName, { enabled: !guest })
  const agents = usePublicAgents()
  const [expanded, setExpanded] = useState(false)

  const hubAgent = agents.data?.find((a) => a.name === agentName)
  const displayName = hubAgent?.config.title?.zh ?? hubAgent?.config.title?.en ?? agentName

  if (guest) {
    // guest 降级：公开列表数据（view=chat 的完整 AgentDTO）渲染 Summary；
    // 无 transport 级详情（Grid 不可用），counts 取绑定名单长度。
    if (!hubAgent) return null
    const guestCounts: AgentDetailCounts = {
      tools: hubAgent.tools?.length ?? 0,
      mcps: hubAgent.mcps?.length ?? 0,
      skills: hubAgent.skills?.length ?? 0,
      subagents: hubAgent.subagents?.length ?? 0,
      datasets: hubAgent.datasets?.length ?? 0,
    }
    return (
      <div className={styles.wrapper}>
        <AgentDetailSummary
          name={displayName}
          model={hubAgent.config.modelId ?? '—'}
          status="ready"
          counts={guestCounts}
          expanded={false}
          onToggle={() => { /* guest 无可展开详情 */ }}
        />
      </div>
    )
  }

  // Silent hide on loading/error/success-no-data. Chat flow continues
  // independently — this panel is non-blocking decoration.
  if (isLoading || isError || !data) return null

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
          disallowedTools={data.disallowedTools}
          mcpServers={data.mcpServers}
          subagents={data.subagents}
          datasets={data.datasets}
          availableSkills={data.availableSkills}
          maxTurns={data.maxTurns}
          maxSessionQueries={data.maxSessionQueries}
        />
      )}
    </div>
  )
}
