import { Select } from 'antd'
import { useNavigate } from 'react-router'
import { usePublicAgents } from '@/queries/useAgents'
import type { Agent } from '@/api/agents'

// label 展示 title（取法与 AgentDetailBar 同源；兜底 name）
function agentTitleOf(a: Agent): string {
  return a.config.title?.zh ?? a.config.title?.en ?? a.name
}

/** 聊天页顶部的 Agent 切换器（spec 6.2）：切换 = URL 段替换 + 整页重挂载。 */
export default function AgentSwitcher({ current }: { current: string }) {
  const navigate = useNavigate()
  const { data: agents } = usePublicAgents()

  return (
    <Select
      size="small"
      showSearch={{ optionFilterProp: 'label' }}
      value={current}
      placeholder="切换 Agent"
      style={{ minWidth: 180 }}
      options={(agents ?? []).map((a) => ({
        value: a.name,
        label: agentTitleOf(a)
      }))}
      onChange={(name) => {
        if (name !== current) {
          void Promise.resolve(navigate(`/agents/${encodeURIComponent(name)}/chat`, { replace: true }))
        }
      }}
    />
  )
}
