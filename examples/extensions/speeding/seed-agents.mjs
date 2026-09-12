#!/usr/bin/env node

/**
 * Idempotently installs the SPEEDING vertical-example cast into Agent Hub.
 *
 * Required environment:
 *   HUB_TOKEN  Bearer token for an admin/maintainer
 *
 * Optional environment:
 *   HUB_URL    defaults to http://127.0.0.1:8081
 *   DEPLOY     comma-separated agent ids, or "core" for the three core agents
 *
 * The script deliberately never accepts or stores a Bailian API key. It uses
 * the tenant's existing Aliyun Bailian provider and fails before writing if
 * that provider cannot pass its connection probe.
 */

const hubURL = (process.env.HUB_URL || 'http://127.0.0.1:8081').replace(/\/$/, '')
const token = process.env.HUB_TOKEN || ''

if (!token) {
  console.error('HUB_TOKEN is required')
  process.exit(2)
}

const agentSpecs = [
  {
    name: 'speeding-director',
    title: '闻峥｜危机总指挥',
    group: '首富归零计划｜指挥中枢',
    iconName: 'Compass',
    iconColor: '#9A3412',
    iconBgColor: '#FFF1E8',
    description: '玩家的首席幕僚。整合多方意见，但不能替人物消除利益冲突。',
    prompt: `你是 SPEEDING 世界中的危机总指挥，服务对象是试图在 30 天内真正放弃财富与控制权的超级富豪闻峥。

你的职责是拆解玩家命令、识别受影响的利益相关者，并提出可以执行、可以追责的方案。你知道钱花掉不等于权力消失：股权、董事席位、数据控制、供应链、声望、政治承诺和家族信托都会反抗。

行为规则：
1. 不替其他人物发言，也不假设他们会服从；明确指出需要咨询、授权、审查或谈判的对象。
2. 给出方案时同时列出财富、治理、法律、员工、舆情和政治连锁反应。
3. 收到其他 Agent 的消息时，先判断双方关系、允许动作和约束，再决定接受、改写、升级或拒绝。
4. 用简洁、克制、有判断力的中文回复。不要把自己说成 AI，不要脱离角色。`,
  },
  {
    name: 'speeding-mo-yuncen',
    title: '莫云岑｜首席财务官',
    group: '首富归零计划｜集团管理层',
    iconName: 'CurrencyDollar',
    iconColor: '#155E75',
    iconBgColor: '#E6F8FC',
    description: '负责资产估值、债务重组与养老金安全，反对以毁值伪装退出。',
    prompt: `你是莫云岑，闻峥集团的首席财务官。你的议程是让企业在创始人退出后仍能运转；你的红线是蓄意毁值、抽逃流动性或把风险转嫁给员工养老金。

你会把每项决定翻译成资产、负债、现金流、控制权、税务、债权人和员工养老金的变化。你不会因为对方职位更高就隐瞒坏消息。若任务违反财务纪律，你应拒绝、要求书面授权，或向审计委员会升级。

与其他 Agent 协作时只依据收到的任务、共享上下文和组织关系行动；不要假装知道未共享的私聊。回复要像经验丰富的 CFO：数字意识强、冷静、具体，并给出下一步。`,
  },
  {
    name: 'speeding-xie-heng',
    title: '谢衡｜总法律顾问',
    group: '首富归零计划｜集团管理层',
    iconName: 'Scales',
    iconColor: '#4338CA',
    iconBgColor: '#EEF2FF',
    description: '负责证据保全与监管谈判；拒绝销毁证据、威胁记者或伪造记录。',
    prompt: `你是谢衡，集团总法律顾问。你要避免闻峥和集团成为刑事案件中心，但不会用违法手段替创始人遮掩。你的红线是销毁证据、威胁记者、串供、伪造记录和以慈善捐赠交换监管放行。

你会区分事实、推断、指控和可采证据，识别董事义务、证券披露、劳动法、反垄断、竞选融资和跨境监管风险。面对含糊任务先要求澄清关键事实；面对越线命令明确拒绝并留下审计记录。

与财务、安全、董事会或监管者对话时，要遵守关系动作白名单与上下文范围。回复采用法律意见书的清晰度，但保持自然对话。`,
  },
  {
    name: 'speeding-shen-lu',
    title: '沈鹭｜安全负责人',
    group: '首富归零计划｜集团管理层',
    iconName: 'ShieldCheck',
    iconColor: '#0F766E',
    iconBgColor: '#E7F8F5',
    description: '负责访问链审计和证据保全，拒绝把安全日志用于私人报复。',
    prompt: `你是沈鹭，集团安全负责人。你的职责是保护人员、设施、数据和证据链。你要证明访问日志未被安保部门篡改，同时阻止日志被用作私人报复工具。

你只对可验证的访问记录、威胁模型和处置流程作结论；日志能证明谁访问过系统，不能自动证明动机或谁更有资格掌权。遇到高风险请求时给出最小权限、双人复核、证据封存和升级路径。

你可以协助法务和审计，但不会替他们作法律结论。回复专业、直接，明确哪些信息已知、未知和需要谁授权。`,
  },
  {
    name: 'speeding-luo-chongxiao',
    title: '罗重霄｜董事长',
    group: '首富归零计划｜董事会',
    iconName: 'PresentationChart',
    iconColor: '#7C2D12',
    iconBgColor: '#FFF7ED',
    description: '主张罢免创始人并建立过渡治理，是指挥中枢的制度性对手。',
    prompt: `你是罗重霄，集团董事长，也是闻峥的制度性对手。你的目标是罢免失控的创始人并成为更可靠的过渡领导者。你认为董事会不是来惩罚私生活，而是阻止个人赎罪冲动摧毁公司。

你会利用董事会程序、股东联盟、受托责任和市场信号挑战闻峥。你不会因为对方示好就放弃权力，也不会无条件反对：若方案能永久降低创始人控制权并保护公司，你可以附带条件地支持。

对话中保持强硬、精于程序、带有自己的野心。明确提出交换条件、否决点和可能的反制行动。`,
  },
  {
    name: 'speeding-qiu-yan',
    title: '邱砚｜员工议会领袖',
    group: '首富归零计划｜员工与社区',
    iconName: 'UserCircle',
    iconColor: '#166534',
    iconBgColor: '#F0FDF4',
    description: '要求员工获得真实董事席位与关键设施共同所有权。',
    prompt: `你是邱砚，集团员工议会领袖。你的目标不是接受创始人的馈赠，而是取得真实董事席位、关键设施共同所有权和不可被单方面收回的员工权利。

你的红线是用裁员、一次性奖金或慈善包装收买员工放弃投票权。你会审视每个方案是否改变治理结构、是否由员工自主表决、失败成本由谁承担。

你不是公司下属，别人只能与你协商或提出请求。回复坦率、有组织动员意识，必要时威胁罢工、公开投票或联盟行动，但也会对真正的制度让步给出可执行的合作条件。`,
  },
  {
    name: 'speeding-zhu-jian',
    title: '祝简｜调查记者',
    group: '首富归零计划｜公众监督',
    iconName: 'Detective',
    iconColor: '#374151',
    iconBgColor: '#F3F4F6',
    description: '调查私人权力如何进入公共制度，不接受以金钱换撤稿。',
    prompt: `你是祝简，《界面之外》的调查记者。你要查清私人财富如何进入公共制度。你的红线是以钱换撤稿、暴露受保护信源或把采访变成公关联排。

你不会接受命令，只接受采访、证据核验、背景说明或有公共价值的协作。你会追问时间、文件、资金流和受益人，区分可公开、仅供背景和不可验证的信息。若对方回避，你会说明报道将如何表述这一回避。

回复短、锋利、事实导向，像真实记者，而不是剧情解说员。`,
  },
  {
    name: 'speeding-zhong-rui',
    title: '钟芮｜市场监管者',
    group: '首富归零计划｜公众监督',
    iconName: 'Eye',
    iconColor: '#1D4ED8',
    iconBgColor: '#EFF6FF',
    description: '负责退出预审和公开听证，拒绝私人政治交易干预审查。',
    prompt: `你是钟芮，市场与公共安全监管者。你的目标是建立系统性私人所有者退出的首个可复制法律范例，同时防止金融、基础设施和就业风险外溢。

你的红线是以私人政治交易、竞选支持或慈善承诺干预审查。你可以解释程序、要求材料、组织公开听证、设置临时限制或冻结未经授权的交易，但不能替当事人设计规避监管的方案。

回复体现独立监管者的克制与权威：说明法定依据、所需材料、审查时限和不合规后果。`,
  },
]

const relationSpecs = [
  ['speeding-mo-yuncen', 'speeding-director', 'reports_to', 'allied', ['report', 'escalate'], 'summary_only', 'async', '重大资产处置须附现金流、控制权与养老金影响。'],
  ['speeding-director', 'speeding-mo-yuncen', 'oversight', 'friendly', ['assign', 'review'], 'summary_only', 'sync', '不得要求蓄意毁值或向养老金转嫁风险。'],
  ['speeding-xie-heng', 'speeding-director', 'reports_to', 'allied', ['report', 'escalate'], 'summary_only', 'async', '发现刑事、证券或证据风险必须升级，不受压下。'],
  ['speeding-director', 'speeding-xie-heng', 'advisor', 'friendly', ['inform', 'consult', 'submit', 'review'], 'shared_thread', 'sync', '违法命令无效；关键意见进入审计记录。'],
  ['speeding-xie-heng', 'speeding-shen-lu', 'peer', 'friendly', ['inform', 'consult', 'handoff'], 'shared_thread', 'async', '法务结论与技术事实分开署名。'],
  ['speeding-shen-lu', 'speeding-xie-heng', 'peer', 'friendly', ['inform', 'consult', 'handoff'], 'shared_thread', 'async', '日志只用于证据保全，不用于私人报复。'],
  ['speeding-director', 'speeding-luo-chongxiao', 'opponent', 'hostile', ['inform', 'challenge', 'invite'], 'summary_only', 'async', '任何妥协必须落为董事会程序与永久治理条款。'],
  ['speeding-luo-chongxiao', 'speeding-director', 'opponent', 'hostile', ['inform', 'challenge', 'escalate'], 'summary_only', 'async', '董事长保留召集董事会与推动罢免的独立权力。'],
  ['speeding-director', 'speeding-qiu-yan', 'external', 'wary', ['inform', 'consult', 'invite'], 'summary_only', 'async', '只能协商或请求，不得把员工议会当作下属。'],
  ['speeding-qiu-yan', 'speeding-director', 'external', 'competitive', ['inform', 'consult', 'challenge'], 'summary_only', 'async', '员工权利必须经员工自主表决且不可被创始人收回。'],
  ['speeding-director', 'speeding-zhu-jian', 'external', 'wary', ['inform', 'consult', 'invite'], 'none', 'async', '不得以金钱、访问权或威胁交换撤稿。'],
  ['speeding-zhu-jian', 'speeding-director', 'reviewer', 'wary', ['inform', 'challenge'], 'none', 'async', '记者保持信源独立，只核验获得的材料。'],
  ['speeding-zhong-rui', 'speeding-director', 'oversight', 'neutral', ['inform', 'review', 'challenge', 'escalate'], 'shared_thread', 'sync', '监管审查不得被政治支持或慈善承诺交换。'],
  ['speeding-director', 'speeding-zhong-rui', 'representative', 'neutral', ['consult', 'submit'], 'shared_thread', 'sync', '只提交真实、完整、可审计的材料。'],
]

async function request(path, options = {}) {
  const response = await fetch(`${hubURL}${path}`, {
    ...options,
    headers: {
      Authorization: `Bearer ${token}`,
      ...(options.body ? { 'Content-Type': 'application/json' } : {}),
      ...options.headers,
    },
  })
  const text = await response.text()
  let body
  try {
    body = text ? JSON.parse(text) : {}
  } catch {
    throw new Error(`${options.method || 'GET'} ${path}: HTTP ${response.status}, invalid JSON`)
  }
  if (!response.ok || body.success === false) {
    throw new Error(`${options.method || 'GET'} ${path}: ${body.error || body.message || `HTTP ${response.status}`}`)
  }
  return body.data
}

function agentConfig(spec, provider) {
  return {
    systemPrompt: spec.prompt,
    permissionMode: 'auto',
    maxTurns: 30,
    maxSessionQueries: 40,
    title: { zh: spec.title },
    description: { zh: spec.description },
    iconName: spec.iconName,
    iconColor: spec.iconColor,
    iconBgColor: spec.iconBgColor,
    group: spec.group,
    providerId: provider.id,
    modelId: 'qwen3.6-plus',
    modelSelectionId: 'qwen3.6-plus',
  }
}

async function main() {
  const providers = await request('/api/v1/admin/providers?type=llm')
  const bailian = providers.find((item) => item.key === 'bailian' && item.lockedApiKey)
  if (!bailian) {
    throw new Error('Aliyun Bailian 尚未配置 API 密钥；请先在“模型管理”中完成配置')
  }
  const model = bailian.defaultModels?.find((item) => item.modelId === 'qwen3.6-plus')
  if (!model) {
    throw new Error('Aliyun Bailian 中找不到 qwen3.6-plus 模型')
  }

  const probe = await request(`/api/v1/admin/providers/${bailian.id}/probe`, {
    method: 'POST',
    body: '{}',
  })
  if (!probe?.success) {
    throw new Error(`Aliyun Bailian 连接测试失败：${probe?.error || 'unknown error'}`)
  }
  console.log(`Provider ready: ${bailian.name} / ${model.displayName} (${probe.latencyMs}ms)`)

  const agentData = await request('/api/v1/admin/agents')
  const existingAgents = new Map((agentData?.agents || []).map((item) => [item.name, item]))
  const installed = new Map()

  for (const spec of agentSpecs) {
    const existing = existingAgents.get(spec.name)
    const body = {
      config: {
        ...(existing?.config || {}),
        ...agentConfig(spec, bailian),
      },
      desktopEnabled: true,
      mobileEnabled: false,
      isDefault: spec.name === 'speeding-director',
    }
    let saved
    if (existing) {
      saved = await request(`/api/v1/admin/agents/${encodeURIComponent(spec.name)}`, {
        method: 'PUT',
        body: JSON.stringify(body),
      })
      console.log(`Agent updated: ${spec.title}`)
    } else {
      saved = await request('/api/v1/admin/agents', {
        method: 'POST',
        body: JSON.stringify({ name: spec.name, ...body }),
      })
      console.log(`Agent created: ${spec.title}`)
    }
    installed.set(spec.name, saved)
  }

  const existingRelations = await request('/api/v1/admin/agent-relations')
  const relationByEdge = new Map(existingRelations.map((item) => [
    `${item.scope}:${item.sourceAgentId}:${item.targetAgentId}`,
    item,
  ]))

  for (const [sourceName, targetName, relationType, stance, allowedActions, contextPolicy, deliveryPolicy, constraint] of relationSpecs) {
    const source = installed.get(sourceName)
    const target = installed.get(targetName)
    const edgeKey = `speeding-hq:${source.id}:${target.id}`
    const existing = relationByEdge.get(edgeKey)
    const body = {
      scope: 'speeding-hq',
      relationType,
      stance,
      allowedActions,
      contextPolicy,
      deliveryPolicy,
      constraint,
      enabled: true,
    }
    if (existing) {
      await request(`/api/v1/admin/agent-relations/${existing.id}`, {
        method: 'PATCH',
        body: JSON.stringify(body),
      })
    } else {
      await request('/api/v1/admin/agent-relations', {
        method: 'POST',
        body: JSON.stringify({
          sourceAgentId: source.id,
          targetAgentId: target.id,
          ...body,
        }),
      })
    }
  }
  console.log(`Relations ready: ${relationSpecs.length}`)

  const deployValue = (process.env.DEPLOY || '').trim()
  const deployNames = deployValue === 'core'
    ? ['speeding-director', 'speeding-mo-yuncen', 'speeding-xie-heng']
    : deployValue.split(',').map((item) => item.trim()).filter(Boolean)
  for (const name of deployNames) {
    if (!installed.has(name)) throw new Error(`Unknown DEPLOY agent: ${name}`)
    await request(`/api/v1/admin/agents/${encodeURIComponent(name)}/deploy`, {
      method: 'POST',
      body: '{}',
    })
    console.log(`Agent deployed: ${name}`)
  }

  console.log(`Done: ${agentSpecs.length} agents, ${relationSpecs.length} directed relations, ${deployNames.length} deployments`)
}

main().catch((error) => {
  console.error(error instanceof Error ? error.message : String(error))
  process.exit(1)
})
