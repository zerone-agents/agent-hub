import { useState, useMemo } from 'react'
import { Button, Spin, Modal, Select, Empty, Input, AutoComplete, Tag, message } from 'antd'
import NameSearch from '@/components/NameSearch'
import { PlusIcon, SquaresFourIcon, PlugIcon } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import PrimaryButton from '@/components/PrimaryButton'
import type { Agent } from '@/api/agents'
import {
  useAgents, useDeleteAgent, useUpdateAgent,
  useUpdateSubagents, useUpdateAgentTools, useUpdateAgentSkills,
  useProbeAgent,
} from '@/queries/useAgents'
import { useTools } from '@/queries/useTools'
import { useSkills } from '@/queries/useSkills'
import { useProviders } from '@/queries/useProviders'
import { useMcps, useUpdateAgentMcps } from '@/queries/useMcps'
import { agentApi } from '@/api/agents'
import type { ApiEnvelope } from '@/api/client'
import { tokens as t } from '@/styles/tokens'
import AgentCard from './AgentCard'
import AgentForm from './AgentForm'
import DeployModal from './DeployModal'
import AgentKnowledgeModal from './AgentKnowledgeModal'
import CardGrid from '@/components/CardGrid'

const useStyles = createStyles(({ css }) => ({
  page: css`
    animation: pageIn 0.35s ease;
    @keyframes pageIn { from { opacity: 0; transform: translateY(6px); } to { opacity: 1; transform: translateY(0); } }
  `,
  pageHead: css`
    display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 24px;
    @media (max-width: 768px) { flex-direction: column; gap: 16px; }
  `,
  pageTitle: css`font-size: ${t.text3xl}; font-weight: 700; color: ${t.text}; letter-spacing: -0.03em; line-height: 1.15;`,
  pageSub: css`margin-top: 4px; font-size: ${t.textBase}; color: ${t.textTertiary};`,
  loadingWrap: css`display: flex; justify-content: center; padding: 80px 0;`,
  emptyState: css`text-align: center; padding: 80px 0;`,
  emptyTitle: css`font-size: ${t.textLg}; font-weight: 600; color: ${t.text}; margin-bottom: 6px;`,
  emptyDesc: css`color: ${t.textTertiary}; font-size: ${t.textSm};`,
  modalFoot: css`
    display: flex; justify-content: space-between; align-items: center; gap: 10px;
    padding: 14px 24px; border-top: 1px solid color-mix(in srgb, var(--foreground) 5%, transparent);
  `,
  footRight: css`display: flex; gap: 10px;`,
  sectionTitle: css`
    font-size: 11px; font-weight: 600; color: var(--text-muted);
    text-transform: uppercase; letter-spacing: 0.05em; margin-bottom: 14px;
  `,
  // Group section styles (reused from skills page)
  section: css`margin-bottom: 40px;`,
  sectionHeader: css`
    display: flex; align-items: center; justify-content: space-between;
    margin-bottom: 16px; padding-bottom: 12px; border-bottom: 1px solid color-mix(in srgb, var(--foreground) 8%, transparent);
  `,
  sectionGroupTitle: css`display: flex; align-items: center; gap: 8px; color: ${t.text}; font-size: ${t.textBase}; font-weight: 600;`,
  sectionCount: css`
    display: inline-flex; align-items: center; justify-content: center;
    min-width: 24px; height: 24px; padding: 0 8px;
    background: ${t.inkSubtle}; color: ${t.ink}; border-radius: 12px;
    font-size: 12px; font-weight: 600;
  `,
  toolbar: css`
    display: flex; justify-content: space-between; align-items: center;
    gap: 12px; margin-bottom: 16px;
  `,
}))

export default function AgentListPage() {
  const { styles } = useStyles()
  const { data: agents = [], isLoading } = useAgents()
  const { data: tools = [] } = useTools()
  const { data: skills = [] } = useSkills()
  const { data: providers = [] } = useProviders('chat')
  const { data: mcps = [] } = useMcps()
  const updateAgent = useUpdateAgent()
  const deleteAgent = useDeleteAgent()
  const updateSubagents = useUpdateSubagents()
  const updateAgentTools = useUpdateAgentTools()
  const updateAgentSkills = useUpdateAgentSkills()
  const updateAgentMcps = useUpdateAgentMcps()
  const probeAgent = useProbeAgent()

  // Main form
  const [formOpen, setFormOpen] = useState(false)
  const [editingAgent, setEditingAgent] = useState<Agent | null>(null)

  // Sub-resource modals
  const [currentName, setCurrentName] = useState('')
  const [currentAgent, setCurrentAgent] = useState<Agent | null>(null)
  const [subagentOpen, setSubagentOpen] = useState(false)
  const [toolOpen, setToolOpen] = useState(false)
  const [skillOpen, setSkillOpen] = useState(false)
  const [mcpOpen, setMcpOpen] = useState(false)
  const [modelOpen, setModelOpen] = useState(false)
  const [selectedSubagents, setSelectedSubagents] = useState<string[]>([])
  const [selectedTools, setSelectedTools] = useState<string[]>([])
  const [selectedSkills, setSelectedSkills] = useState<string[]>([])
  const [selectedMcps, setSelectedMcps] = useState<string[]>([])
  const [selectedProviderId, setSelectedProviderId] = useState<number | null>(null)
  const [selectedModelId, setSelectedModelId] = useState<string>('')
  const [selectedSelectionId, setSelectedSelectionId] = useState<string>('')
  const [fieldValues, setFieldValues] = useState<Record<string, string>>({})
  const [initialApiKey, setInitialApiKey] = useState<string>('') // masked api_key from backend
  const [testPassed, setTestPassed] = useState(false)
  const [testing, setTesting] = useState(false)
  const [saving, setSaving] = useState(false)
  const [modelDropdownOpen, setModelDropdownOpen] = useState(false)

  // DeployModal state
  const [deployOpen, setDeployOpen] = useState(false)
  const [deployAgent, setDeployAgent] = useState<Agent | null>(null)

  // Knowledge modal state
  const [knowledgeOpen, setKnowledgeOpen] = useState(false)
  const [knowledgeAgent, setKnowledgeAgent] = useState<Agent | null>(null)

  // 搜索
  const [keywords, setKeywords] = useState('')

  // 按关键词过滤
  const filteredAgents = useMemo(() => {
    if (!keywords) return agents
    const kw = keywords.toLowerCase()
    return agents.filter((agent) => {
      const fields = [
        agent.name,
        agent.config.title?.zh,
        agent.config.title?.en,
        agent.config.description?.zh,
        agent.config.description?.en,
        agent.group
      ]
      return fields.some((f) => f?.toLowerCase().includes(kw))
    })
  }, [agents, keywords])

  // 按 group 分组，组内按 name 排序
  const groupedAgents = useMemo(() => {
    const grouped = filteredAgents.reduce<Record<string, Agent[] | undefined>>((acc, agent) => {
      const group = agent.group ?? '默认分组'
      acc[group] ??= []
      acc[group].push(agent)
      return acc
    }, {})
    // 组内按标识首字母排序
    Object.values(grouped).forEach((list) => (list ?? []).sort((a, b) => a.name.localeCompare(b.name)))
    return grouped
  }, [filteredAgents])

  // 排序：默认分组放最后
  const sortedGroups = useMemo(() => {
    return Object.keys(groupedAgents).sort((a, b) => {
      if (a === '默认分组') return 1
      if (b === '默认分组') return -1
      return a.localeCompare(b)
    })
  }, [groupedAgents])

  const showDeploy = (agent: Agent) => {
    setDeployAgent(agent)
    setDeployOpen(true)
  }

  const showEdit = (agent: Agent) => { setEditingAgent(agent); setFormOpen(true) }
  const showCreate = () => { setEditingAgent(null); setFormOpen(true) }

  const handleEditSubagents = (agent: Agent) => {
    setCurrentName(agent.name)
    setSelectedSubagents([...(agent.subagents ?? [])])
    setSubagentOpen(true)
  }

  const defaultToolNames = useMemo(() =>
    new Set(tools.filter((t) => t.isDefault).map((t) => t.name)),
  [tools])

  const handleEditTools = async (agent: Agent) => {
    setCurrentName(agent.name)
    let names = agent.tools ?? []
    try {
      const res = await agentApi.getTools(agent.name)
      const body = res.data as { success: boolean; data?: string[] }
      if (body.success) names = body.data ?? []
    } catch { /* use fallback */ }
    const merged = Array.from(new Set([...names, ...defaultToolNames]))
    setSelectedTools(merged)
    setToolOpen(true)
  }

  const handleEditSkills = async (agent: Agent) => {
    setCurrentName(agent.name)
    let names = agent.skills ?? []
    try {
      const res = await agentApi.getSkills(agent.name)
      const body = res.data as { success: boolean; data?: string[] }
      if (body.success) names = body.data ?? []
    } catch { /* use fallback */ }
    setSelectedSkills([...names])
    setSkillOpen(true)
  }

  const handleEditMcps = async (agent: Agent) => {
    setCurrentName(agent.name)
    let names = agent.mcps ?? []
    try {
      const res = await agentApi.getMcps(agent.name)
      const body = res.data as { success: boolean; data?: string[] }
      if (body.success) names = body.data ?? []
    } catch { /* use fallback */ }
    setSelectedMcps([...names])
    setMcpOpen(true)
  }

  const handleEditKnowledge = (agent: Agent) => {
    setKnowledgeAgent(agent)
    setKnowledgeOpen(true)
  }

  const handleEditModel = (agent: Agent) => {
    setCurrentName(agent.name)
    setCurrentAgent(agent)
    const pid = agent.config.providerId ?? null
    const mid = agent.config.modelId ?? ''
    const sid = agent.config.modelSelectionId ?? ''
    setSelectedProviderId(pid)
    setSelectedModelId(mid)
    setSelectedSelectionId(sid)
    setTestPassed(false)
    setTesting(false)
    setSaving(false)

    // Initialize fieldValues: prefer agent's existing overrides, fallback to Provider values
    const initial: Record<string, string> = {}
    const provider = providers.find(p => p.id === pid)
    if (provider) {
      for (const f of provider.fields) {
        const override = agent.config.fieldOverrides?.[f.key]
        if (override !== undefined) {
          initial[f.key] = override
        } else if (f.key === 'name') {
          initial[f.key] = provider.name
        } else if (f.key === 'base_url') {
          initial[f.key] = provider.baseUrl
        }
      }
    }
    // api_key comes back masked from the backend; show it so the user knows
    // a key is configured, but skip it on save/test unless changed.
    const maskedApiKey = agent.config.fieldOverrides?.api_key
    setInitialApiKey(maskedApiKey ?? '')
    if (maskedApiKey) {
      initial.api_key = maskedApiKey
    }
    setFieldValues(initial)
    setModelOpen(true)
  }

  const updateField = (key: string, value: string) => {
    setFieldValues(prev => ({ ...prev, [key]: value }))
    setTestPassed(false)
  }

  const handleProviderChange = (value: number | undefined) => {
    setTestPassed(false)

    if (!value) {
      setSelectedProviderId(null)
      setSelectedModelId('')
      setFieldValues({})
      setInitialApiKey('')
      return
    }

    const newProvider = providers.find(p => p.id === value)

    // Re-fill fields from new Provider defaults (api_key left empty)
    const initial: Record<string, string> = {}
    if (newProvider) {
      for (const f of newProvider.fields) {
        if (f.key === 'api_key') continue
        if (f.key === 'name') initial[f.key] = newProvider.name
        else if (f.key === 'base_url') initial[f.key] = newProvider.baseUrl
      }
    }

    setInitialApiKey('')
    setSelectedProviderId(value)
    setSelectedModelId('')
    setSelectedSelectionId('')
    setFieldValues(initial)
  }

  // AutoComplete 选项的 value 用 selectionId（区分同 modelId 的多条 catalog 记录，
  // 如 kimi-cn 的 k3 256K 与 k3 1M）。当用户从下拉选中一项时，同步设置 modelId
  // 和 selectionId；用户自定义输入（无匹配建议）时，selectionId 置空，只存 modelId
  // 兼容运行时直接传任意模型 ID 的场景。
  const handleModelChange = (value: string) => {
    setTestPassed(false)
    const hit = modelSuggestions.find((m) => m.value === value)
    if (hit) {
      setSelectedModelId(hit.modelId)
      setSelectedSelectionId(hit.selectionId ?? '')
    } else {
      setSelectedModelId(value)
      setSelectedSelectionId('')
    }
    if (!value) {
      setModelDropdownOpen(false)
    }
  }

  const canTest = !!selectedProviderId && !!selectedModelId && !testing

  const handleTest = async () => {
    if (!selectedProviderId || !selectedModelId || !currentName) return

    setTesting(true)
    try {
      // Send the current form values; backend resolves masked values against
      // the Agent's fieldOverrides or the Provider's stored key.
      const res = await probeAgent.mutateAsync({
        name: currentName,
        data: {
          providerId: selectedProviderId,
          apiKey: fieldValues.api_key || '',
          baseUrl: fieldValues.base_url || '',
        },
      })
      const result = (res.data as ApiEnvelope<{ success?: boolean; latencyMs?: number; error?: string }>).data
      if (result?.success) {
        message.success(`连接成功 · ${result.latencyMs}ms`)
        setTestPassed(true)
      } else {
        message.error(`连接失败 · ${result?.error ?? '未知错误'}`)
        setTestPassed(false)
      }
    } catch {
      setTestPassed(false)
    } finally {
      setTesting(false)
    }
  }

  const canConfirm = testPassed && !!selectedProviderId && !!selectedModelId && !saving

  const handleSave = async () => {
    if (!currentAgent) return
    setSaving(true)
    try {
      // Strip api_key if it is the same masked value from the backend;
      // the backend uses the Provider's stored key when no override is present.
      const overrides = { ...fieldValues }
      if (overrides.api_key === initialApiKey) {
        delete overrides.api_key
      }
      await updateAgent.mutateAsync({
        name: currentName,
        data: {
          config: {
            ...currentAgent.config,
            providerId: selectedProviderId,
            modelId: selectedModelId,
            modelSelectionId: selectedSelectionId,
            fieldOverrides: overrides,
          }
        }
      })
      setModelOpen(false)
    } catch (err: unknown) {
      const errMsg = err instanceof Error ? err.message : '未知错误'
      message.error(`保存失败 · ${errMsg}`)
    } finally {
      setSaving(false)
    }
  }

  // Options for selects
  const subagentOptions = agents
    .filter((a) => a.name !== currentName)
    .map((a) => ({ value: a.name, label: a.config.title?.zh ?? a.config.title?.en ?? a.name }))

  const toolOptions = tools.map((tl) => ({ value: tl.name, label: tl.name, disabled: defaultToolNames.has(tl.name) }))

  const mcpOptions = mcps.map((m) => ({
    value: m.name,
    label: m.title || m.name,
  }))

  const skillOptions = (() => {
    const expert = skills.filter((s) => s.type === 'expert').map((s) => ({ value: s.name, label: s.name }))
    const community = skills.filter((s) => s.type === 'community').map((s) => ({ value: s.name, label: s.name }))
    const groups: { label: string; options: { value: string; label: string }[] }[] = []
    if (expert.length) groups.push({ label: '专家技能', options: expert })
    if (community.length) groups.push({ label: '社区技能', options: community })
    return groups
  })()

  // Provider options for the first Select
  const providerOptions = providers.map(p => ({ value: p.id, label: p.name }))

  // Protocol → display metadata. Used by the provider dropdown's optionRender
  // to append a colored protocol tag after each provider name.
  const protocolMeta: Partial<Record<string, { label: string; color: string }>> = {
    anthropic: { label: 'Anthropic', color: 'orange' },
    openai: { label: 'OpenAI', color: 'emerald' },
    mineru: { label: 'MinerU', color: 'geekblue' },
    paddleocr: { label: 'PaddleOCR', color: 'purple' },
  }
  const renderProtocolTag = (protocol: string) => {
    const meta = protocolMeta[protocol]
    if (!meta) return null
    return (
      <Tag
        color={meta.color}
        style={{ marginLeft: 8, marginRight: 0, fontSize: 11, lineHeight: '18px', padding: '0 6px' }}
      >
        {meta.label}
      </Tag>
    )
  }

  // Model suggestions from the selected Provider's defaultModels.
  // Only LLM and VLM models are eligible for agent binding (defense-in-depth
  // alongside the backend validator); embedding/OCR models are excluded.
  // option.value 用 selectionId（区分同 modelId 的多条 catalog 记录）。
  const modelSuggestions = (() => {
    const provider = providers.find(p => p.id === selectedProviderId)
    if (!provider) return []
    return provider.defaultModels
      .filter((m) => m.modelType === 'llm' || m.modelType === 'vlm')
      .map(m => ({
        value: m.selectionId ?? m.modelId,
        selectionId: m.selectionId,
        modelId: m.modelId,
        label: `${m.displayName || m.modelId} (${m.modelId})`,
        display: m.displayName || m.modelId,
      }))
  })()

  // 当前选中的建议项：优先用 selectionId 命中（新绑定），否则用 modelId 命中
  // （兼容只有 modelId 的存量 agent 与自定义输入）。
  const selectedModelSuggestion = useMemo(
    () => modelSuggestions.find(m => (selectedSelectionId ? m.selectionId === selectedSelectionId : m.value === selectedModelId)),
    [modelSuggestions, selectedSelectionId, selectedModelId],
  )

  // Reverse-lookup map: 主键 `providerId::selectionId`，次键 `providerId::modelId`
  // （兼容存量 agent 没存 selectionId 的情况；同 modelId 多条记录时取最后一条）。
  const modelDisplayNameMap = useMemo(() => {
    const bySelection = new Map<string, string>()
    const byModel = new Map<string, string>()
    for (const p of providers) {
      for (const mo of p.defaultModels) {
        if (mo.selectionId) {
          bySelection.set(`${p.id}::${mo.selectionId}`, mo.displayName)
        }
        byModel.set(`${p.id}::${mo.modelId}`, mo.displayName)
      }
    }
    return { bySelection, byModel }
  }, [providers])

  const getModelDisplayName = (agent: Agent): string => {
    const pid = agent.config.providerId
    const mid = agent.config.modelId ?? ''
    const sid = agent.config.modelSelectionId ?? ''
    if (!pid || !mid) return ''
    if (sid) {
      const name = modelDisplayNameMap.bySelection.get(`${pid}::${sid}`)
      if (name) return name
    }
    return modelDisplayNameMap.byModel.get(`${pid}::${mid}`) ?? mid
  }

  return (
    <div className={styles.page}>
      <div className={styles.pageHead}>
        <div>
          <div className={styles.pageTitle}>Agent 管理</div>
          <div className={styles.pageSub}>管理您的 AI Agent 配置</div>
        </div>
        <PrimaryButton icon={<PlusIcon size={16} weight="bold" />} onClick={showCreate}>
          新建代理
        </PrimaryButton>
      </div>

      <div className={styles.toolbar}>
          <NameSearch
            placeholder="搜索代理名称"
            onSearch={setKeywords}
            realtime
          />
      </div>

      {isLoading ? (
        <div className={styles.loadingWrap}><Spin size="medium" /></div>
      ) : agents.length === 0 ? (
        <div className={styles.emptyState}>
          <div style={{ marginBottom: 20 }}><SquaresFourIcon size={48} weight="thin" color={t.textMuted} /></div>
          <div className={styles.emptyTitle}>暂无代理</div>
          <div className={styles.emptyDesc}>创建您的第一个代理以开始使用</div>
        </div>
      ) : (
        sortedGroups.map(group => (
          <div key={group} className={styles.section}>
            <div className={styles.sectionHeader}>
              <div className={styles.sectionGroupTitle}>
                <span>{group}</span>
                <span className={styles.sectionCount}>{(groupedAgents[group] ?? []).length}</span>
              </div>
            </div>
            <CardGrid>
              {(groupedAgents[group] ?? []).map((agent) => (
                <AgentCard
                  key={agent.name}
                  agent={agent}
                  modelDisplayName={getModelDisplayName(agent)}
                  onEdit={showEdit}
                  onDelete={(name) => { deleteAgent.mutate(name); }}
                  onEditSubagents={handleEditSubagents}
                  onEditTools={handleEditTools}
                  onEditSkills={handleEditSkills}
                  onEditMcps={handleEditMcps}
                  onEditModel={handleEditModel}
                  onDeploy={showDeploy}
                  onEditKnowledge={handleEditKnowledge}
                />
              ))}
            </CardGrid>
          </div>
        ))
      )}

      <AgentForm open={formOpen} editingAgent={editingAgent} onClose={() => { setFormOpen(false); }} />

      {deployAgent && (
        <DeployModal
          agent={deployAgent}
          providers={providers}
          open={deployOpen}
          onClose={() => {
            setDeployOpen(false)
            setDeployAgent(null)
          }}
        />
      )}

      {knowledgeOpen && knowledgeAgent && (
        <AgentKnowledgeModal
          open={knowledgeOpen}
          agent={knowledgeAgent}
          onClose={() => {
            setKnowledgeOpen(false)
            setKnowledgeAgent(null)
          }}
        />
      )}

      {/* Sub-agents modal */}
      <Modal
        title="管理子代理"
        open={subagentOpen}
        onCancel={() => { setSubagentOpen(false); }}
        width={480}
        footer={
          <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 10 }}>
            <Button onClick={() => { setSubagentOpen(false); }}>取消</Button>
            <PrimaryButton
              loading={updateSubagents.isPending}
              onClick={async () => {
                await updateSubagents.mutateAsync({ name: currentName, subagents: selectedSubagents })
                setSubagentOpen(false)
              }}
            >
              确认
            </PrimaryButton>
          </div>
        }
      >
        <p style={{ marginBottom: 14, fontSize: 13, color: 'var(--text-secondary)' }}>选择此代理可调用的子代理：</p>
        <Select
          mode="multiple"
          style={{ width: '100%' }}
          placeholder="选择子代理"
          options={subagentOptions}
          size="large"
          value={selectedSubagents}
          onChange={setSelectedSubagents}
        />
      </Modal>

      {/* Tools modal */}
      <Modal
        title="管理工具"
        open={toolOpen}
        onCancel={() => { setToolOpen(false); }}
        width={480}
        footer={
          <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 10 }}>
            <Button onClick={() => { setToolOpen(false); }}>取消</Button>
            <PrimaryButton
              loading={updateAgentTools.isPending}
              onClick={async () => {
                const merged = Array.from(new Set([...selectedTools, ...defaultToolNames]))
                await updateAgentTools.mutateAsync({ name: currentName, toolNames: merged })
                setToolOpen(false)
              }}
            >
              确认
            </PrimaryButton>
          </div>
        }
      >
        <p style={{ marginBottom: 14, fontSize: 13, color: 'var(--text-secondary)' }}>选择此代理可使用的工具：</p>
        <Select
          mode="multiple"
          style={{ width: '100%' }}
          placeholder="选择工具"
          options={toolOptions}
          size="large"
          value={selectedTools}
          onChange={setSelectedTools}
        />
      </Modal>

      {/* Skills modal */}
      <Modal
        title="管理技能"
        open={skillOpen}
        onCancel={() => { setSkillOpen(false); }}
        width={480}
        footer={
          <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 10 }}>
            <Button onClick={() => { setSkillOpen(false); }}>取消</Button>
            <PrimaryButton
              loading={updateAgentSkills.isPending}
              onClick={async () => {
                await updateAgentSkills.mutateAsync({ name: currentName, skillNames: selectedSkills })
                setSkillOpen(false)
              }}
            >
              确认
            </PrimaryButton>
          </div>
        }
      >
        <p style={{ marginBottom: 14, fontSize: 13, color: 'var(--text-secondary)' }}>
          选择此代理的技能（专家和社区技能均可选）：
        </p>
        <Select
          mode="multiple"
          style={{ width: '100%' }}
          placeholder="选择技能"
          options={skillOptions}
          size="large"
          value={selectedSkills}
          onChange={setSelectedSkills}
        />
      </Modal>

      {/* MCPs modal */}
      <Modal
        title="管理 MCP"
        open={mcpOpen}
        onCancel={() => { setMcpOpen(false); }}
        width={480}
        footer={
          <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 10 }}>
            <Button onClick={() => { setMcpOpen(false); }}>取消</Button>
            <PrimaryButton
              loading={updateAgentMcps.isPending}
              onClick={async () => {
                await updateAgentMcps.mutateAsync({ agentName: currentName, mcpNames: selectedMcps })
                setMcpOpen(false)
              }}
            >
              确认
            </PrimaryButton>
          </div>
        }
      >
        <p style={{ marginBottom: 14, fontSize: 13, color: 'var(--text-secondary)' }}>
          选择此代理可使用的 MCP 服务器（在 MCP 配置页面管理可用列表）：
        </p>
        {mcps.length === 0 ? (
          <Empty description="请先在 MCP 配置页面添加服务器" />
        ) : (
          <Select
            mode="multiple"
            style={{ width: '100%' }}
            placeholder="选择 MCP"
            options={mcpOptions}
            size="large"
            value={selectedMcps}
            onChange={setSelectedMcps}
          />
        )}
      </Modal>

      {/* Model modal */}
      <Modal
        title="设置模型"
        open={modelOpen}
        onCancel={() => { setModelOpen(false); }}
        footer={
          <div className={styles.modalFoot}>
            <Button onClick={() => { setModelOpen(false); }}>取消</Button>
            <div className={styles.footRight}>
              <Button onClick={handleTest} disabled={!canTest} loading={testing}>
                <PlugIcon size={14} /> 测试
              </Button>
              <PrimaryButton onClick={handleSave} disabled={!canConfirm} loading={saving}>
                确认
              </PrimaryButton>
            </div>
          </div>
        }
        width={720}
        destroyOnHidden
      >
        {providers.length === 0 ? (
          <Empty description="请先在模型管理添加 Provider" />
        ) : (
          <>
            {/* Provider/Model offline warning */}
            {(() => {
              const pid = currentAgent?.config.providerId
              const mid = currentAgent?.config.modelId
              const sid = currentAgent?.config.modelSelectionId
              if (!pid || !mid) return null
              // 优先用 selectionId 命中（精确），否则用 modelId 兜底（兼容存量）
              const hit = sid
                ? modelDisplayNameMap.bySelection.has(`${pid}::${sid}`)
                : modelDisplayNameMap.byModel.has(`${pid}::${mid}`)
              if (!hit) {
                return (
                  <p style={{ marginBottom: 14, fontSize: 13, color: '#dc2626' }}>
                    ⚠️ 原 Provider 或模型已下线，请重新选择
                  </p>
                )
              }
              return null
            })()}

            {/* Provider Select */}
            <div style={{ marginBottom: 20 }}>
              <label style={{ display: 'block', marginBottom: 6, fontSize: 13, color: 'var(--text-secondary)' }}>
                选择供应商
              </label>
              <Select
                style={{ width: '100%' }}
                size="large"
                placeholder="选择供应商"
                allowClear
                value={selectedProviderId ?? undefined}
                onChange={handleProviderChange}
                options={providerOptions}
                optionRender={(option) => {
                  const provider = providers.find(p => p.id === option.value)
                  return (
                    <span style={{ display: 'inline-flex', alignItems: 'center' }}>
                      {option.label}
                      {provider && renderProtocolTag(provider.protocol)}
                    </span>
                  )
                }}
              />
            </div>

            {/* Model AutoComplete */}
            {selectedProviderId && (
              <div style={{ marginBottom: 20 }}>
                <label style={{ display: 'block', marginBottom: 6, fontSize: 13, color: 'var(--text-secondary)' }}>
                  选择或输入模型
                </label>
                <AutoComplete
                  style={{ width: '100%' }}
                  size="large"
                  placeholder="选择模型或输入自定义模型 ID"
                  value={modelDropdownOpen || !selectedModelSuggestion ? selectedModelId : selectedModelSuggestion.display}
                  onChange={(value) => {
                    handleModelChange(value)
                    setModelDropdownOpen(false)
                  }}
                  options={modelSuggestions}
                  allowClear
                  showSearch={{
                    filterOption: (input, option) => {
                      const value = (option?.value ?? '').toLowerCase()
                      const label = (option?.label ?? '').toLowerCase()
                      const query = input.toLowerCase()
                      return value.includes(query) || label.includes(query)
                    },
                    onSearch: (value) => {
                      const query = value.toLowerCase()
                      const hasMatch = modelSuggestions.some(m =>
                        m.value.toLowerCase().includes(query) ||
                        m.label.toLowerCase().includes(query)
                      )
                      setModelDropdownOpen(hasMatch)
                    },
                  }}
                  open={modelDropdownOpen}
                  onBlur={() => { setModelDropdownOpen(false); }}
                  onFocus={() => { setModelDropdownOpen(modelSuggestions.length > 0); }}
                  onSelect={() => { setModelDropdownOpen(false); }}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') {
                      setModelDropdownOpen(false)
                    }
                  }}
                />
              </div>
            )}

            {/* Dynamic fields based on Provider.fields */}
            {(() => {
              const selectedProvider = providers.find(p => p.id === selectedProviderId)
              if (!selectedProvider) return null

              if (selectedProvider.fields.length === 0) {
                return (
                  <p style={{ fontSize: 13, color: 'var(--text-muted)' }}>
                    该 Provider 无需额外连接参数
                  </p>
                )
              }

              return (
                <>
                  <div className={styles.sectionTitle}>连接参数</div>
                  {selectedProvider.fields.map((field) => (
                    <div key={field.key} style={{ marginBottom: 16 }}>
                      <label style={{ display: 'block', marginBottom: 6, fontSize: 13, color: 'var(--text-secondary)' }}>
                        {field.label}
                        {field.required && <span style={{ color: '#dc2626', marginLeft: 4 }}>*</span>}
                      </label>
                {field.type === 'password' ? (
                  <Input
                    value={fieldValues[field.key] ?? ''}
                    onChange={(e) => { updateField(field.key, e.target.value); }}
                    placeholder={`输入${field.label}`}
                  />
                ) : field.type === 'select' ? (
                  <Select
                    value={fieldValues[field.key] ?? undefined}
                    onChange={(v) => { updateField(field.key, v); }}
                    placeholder={`选择${field.label}`}
                    style={{ width: '100%' }}
                    options={[]}
                  />
                ) : (
                  <Input
                    value={fieldValues[field.key] ?? ''}
                    onChange={(e) => { updateField(field.key, e.target.value); }}
                    placeholder={`输入${field.label}`}
                  />
                )}
                    </div>
                  ))}
                </>
              )
            })()}
          </>
        )}
      </Modal>
    </div>
  )
}
