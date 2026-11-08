export const memoryZh = {
  noNativeTitle: '暂无对话记忆',
  noMatchingNativeTitle: '没有匹配的记忆',
  noRecallNativeTitle: '本轮未召回记忆',
  expandAllTurns: '展开全部',
  collapseAllTurns: '收起全部',
  chooseAccount: '选择账户',
  switchAccount: '切换账户查看记忆',
  memoryTypeCount: '{{count}} 类',
  recallMode: '诊断方式',
  chatSupplyMode: '聊天供给',
  nativeSearchMode: '原生检索',
  recallLimit: '返回条数',
  nativeParameterDisclosure: '原生请求参数说明',
  nativeParameterHint:
    '本次原生检索默认请求相似度阈值 0.2、关键词权重 0.7。参数接收不等于当前引擎执行，本页不提供这两个参数的调节，也不修改默认召回策略。排序序号不代表相似度得分。',
  configurationSummary:
    '模型引用 {{chat}} · 向量引用 {{embedding}} · 容量上限 {{capacity}}',
  openMemoryContent: '打开记忆 {{id}} 内容',
  recordStatusUnknown: '状态未提供',
  statusSupplyScopeHint:
    '仅影响之后的检索供给，不暂停异步抽取，也不会撤回已发送或在途的回复。',
  recordFilters: {
    all: '全部状态',
    enabled: '可供给',
    disabled: '不供给',
    completed: '提取完成',
    pending: '提取待确认',
    failed: '提取失败'
  },
  managementSubtitle: '管理对话记忆、来源与检索供给。',
  autoMode: '聊天自动提取',
  ledgerMode: '已审核事实',
  managementAutoHint:
    '对话自动形成记忆。原文与模型派生内容分别管理，遗忘和删除暂不开放。',
  managementLedgerHint: '查看已审核事实、版本和同步状态，按应用能力管理。',
  applicationConfiguration: '应用配置',
  subjectsTitle: '用户与账户',
  searchSubjects: '搜索用户或账户生命周期',
  searchSubjectsPlaceholder: '搜索用户或账户',
  noMatchingSubjects: '没有匹配的账户',
  selectSubjectHint: '从左侧选择账户，查看该范围内的记忆。',
  configurationReadOnlyHint:
    '这里只读展示当前应用的接入配置，不在此更改模型、提取提示词或底层记忆库。',
  applicationContract: '应用与接入',
  policyVersion: '策略版本',
  memoryMode: '记忆模式',
  enabledMemoryTypes: '启用类型',
  contextBudget: '本轮召回预算',
  contextBudgetValue: '正文最多 {{chars}} 字，默认 {{count}} 条',
  managementCapabilities: '管理能力',
  statusManagementEnabled: '可启用 / 停用，遗忘删除未开放',
  statusManagementReadOnly: '按当前应用能力管理',
  nativeConfiguration: '原生记忆配置',
  configurationUnavailable: '此应用尚未提供原生配置读回。',
  bankName: '记忆库名称',
  configurationDescription: '说明',
  chatModelReference: '提取模型引用',
  embeddingModelReference: '向量模型引用',
  storageBackend: '存储后端',
  capacityLimit: '容量上限',
  nativePermissions: '原生可见范围',
  extractionTemperature: '提取温度',
  capacityPolicy: '原生容量策略',
  configurationCreatedAt: '创建时间',
  capacityAndPolicyHint:
    '容量是配置上限，不代表已用空间。原生容量策略仅展示配置，不表示本次开放主动遗忘或删除。',
  currentScopeOnly: '仅统计当前账户范围',
  currentScopeStatistics: '当前账户记忆统计',
  currentLifecycle: '账户生命周期：',
  statTurns: '原文轮次',
  statDerived: '派生记忆',
  statSupplied: '当前可供给',
  statFailed: '提取失败任务',
  recordsTab: '记忆记录',
  recallTab: '召回诊断',
  searchMemoryRecords: '搜索记忆记录',
  searchMemoryPlaceholder: '搜索正文或记录编号',
  filterMemoryType: '筛选记忆类型',
  filterMemoryStatus: '筛选记忆状态',
  allMemoryTypes: '全部类型',
  clearFilters: '清除筛选',
  filteredRecords: '{{turns}} 轮对话 · {{records}} 条记录',
  noMatchingMemories: '没有匹配的记忆，试试调整筛选。',
  modelDerivedHint: '派生内容由模型提取，不等于已核验事实；判断时请结合原文。',
  sourceTurn: '原文 #{{id}}',
  timeNotProvided: '时间未提供',
  derivedCount: '{{count}} 条派生',
  viewConversation: '查看原文',
  viewSource: '追溯原文',
  openMemoryDetail: '查看记忆 {{id}} 详情',
  openRelatedMemory: '查看关联记忆 {{id}}',
  notSupplied: '当前不供给',
  failedWithDerivedHint:
    '提取任务失败，但已有派生内容保留。请结合原文核对，失败不代表这些内容已删除。',
  failedWithoutDerivedHint: '提取任务失败，尚未读到派生内容。原文仍保留。',
  recallDiagnosticHint:
    '用实际问题检查本轮会供给哪些记忆。最近原文优先补入，检索排序不代表事实新旧顺序。',
  recalledCount: '本轮 {{count}} 条',
  recentSourceIncluded: '已补入最近原文',
  contextBudgetReached: '已触及上下文预算',
  memoryDetail: '记忆详情',
  nativeOwnStatus: '原生开关',
  nativeSwitchOn: '启用',
  nativeSwitchOff: '停用',
  effectiveSupplyStatus: '实际供给状态',
  currentlySupplied: '允许本轮检索供给',
  recordCreatedAt: '记录创建时间',
  thisIsSource: '本条即原文',
  fullConversation: '完整对话原文',
  extractedContent: '模型提取内容',
  relatedDerivedTitle: '同源派生记忆 · {{count}} 条',
  nativeTimeMetadataHint:
    '关联开始、结束时间属于原生事件或资料时间，不统一当成访问有效期；已结束经历与未来计划仍可用于回忆。',
  nativeSwitch: '切换记忆 {{id}} 启用状态',
  nativeDetailSwitch: '在详情中切换记忆 {{id}} 启用状态',
  parentDisabledHint: '来源原文已停用，此派生内容当前不供给；需先恢复原文。',
  statusSwitchHint: '管理检索供给，不会删除原文或派生内容。',
  nativeStatusSaved: '供给状态已保存，正在刷新关联记录。',
  enableMemoryTitle: '恢复这条记忆的供给？',
  disableMemoryTitle: '停用这条记忆的供给？',
  enableMemory: '恢复供给',
  disableMemory: '停用供给',
  enableSourceExplanation:
    '恢复原文后，仅恢复未被单独停用的同源派生供给。原文与派生内容仍保留。',
  disableSourceExplanation:
    '停用原文会阻止其自身及同源派生内容供给。数据继续保留，这不是遗忘或删除。',
  enableDerivedExplanation:
    '恢复本条派生内容的供给，需要来源原文仍处于启用状态。不会恢复其他单独停用的记忆。',
  disableDerivedExplanation:
    '仅停用本条派生内容的供给，不影响原文和同源其他记录，也不会删除数据。',
  statusChangeUnconfirmed:
    '操作结果未确认或状态已变化，请刷新后核对，避免直接重复切换。',

  title: '记忆管理',
  subtitle: '查看用户记忆，并按当前应用能力处理纠正与管理。',
  autoTitle: '原生记忆 · 聊天自动提取',
  autoScope:
    '真实对话完成后自动提交原文，异步提取语义、情节及流程记忆。提取状态与来源可查看；本期不开放遗忘、删除、授权撤回及注销清理。',
  nativeMessagesTitle: '对话形成的记忆',
  nativeRefreshExtraction: '刷新提取状态',
  nativeExtractionHint:
    '已形成派生记忆不等于后台任务已完成，可手动刷新查看最新状态。',
  nativeMessagesHint:
    '无需手动添加。原始对话和模型派生内容分别展示，派生内容应结合来源与时间理解。纠正也通过对话提交，不承诺自动消除所有历史冲突。',
  nativeRecallTitle: '检索原生记忆',
  noNativeMessages: '暂无记忆，完成聊天后可刷新查看自动提取进度。',
  nativeRecord: '记录',
  nativeSource: '源原文',
  nativeActive: '可供给',
  nativeDisabled: '不供给',
  nativeValidAt: '关联开始时间',
  nativeInvalidAt: '关联结束时间',
  nativeBudgetLimited: '部分记忆超过本轮上下文预算，未供给完整正文。',
  nativeColumns: { type: '记忆类型', source: '来源 / 提取状态', time: '时间' },
  nativeTypes: {
    raw: '原始对话',
    semantic: '语义记忆',
    episodic: '情节记忆',
    procedural: '流程记忆'
  },
  extractionStates: {
    received: '已受理',
    pending: '等待提取',
    extracting: '提取中',
    completed: '提取完成',
    failed: '提取失败',
    unknown: '状态未确认',
    derivedAvailable: '已形成派生记忆'
  },
  nativeTitle: '原生记忆 · 原话模式',
  nativeScope:
    '保存审核后的原话并检索。本期支持查看和纠正，暂不开放遗忘、删除、授权撤回及注销清理。纠正会停用旧版本并保留其原文。',
  unsupported: '当前应用暂不支持此操作，请查看本期能力说明。',
  forbidden: '仅管理员和维护者可访问记忆管理',
  application: '应用',
  applicationOption: '{{application}} · {{tenant}}',
  subject: '用户 / 账户生命周期',
  subjectOption: '{{user}} · {{lifecycle}}',
  selectApplication: '选择应用',
  selectSubject: '选择用户和账户',
  refresh: '刷新',
  retry: '重试',
  synthetic: '合成数据',
  noApplications: '尚未配置记忆应用',
  noApplicationsHint: '接入记忆服务后，应用会显示在这里。',
  noSubjects: '该应用暂无可展示的用户账户',
  noSubjectsHint: '用户经业务对话入口提交记忆后，可在这里查看对应账户。',
  missingScopes:
    '{{count}} 个旧账户缺少可展示的用户身份，暂未列出。请联系管理员补齐账户映射。',
  limitedSubjects: '当前最多显示 {{limit}} 个账户，应用中还有未列出的账户。',
  noFacts: '该账户暂无有效记忆',
  noFactsHint: '仅显示当前账户范围内有效并可供给的记忆。',
  inactive: '该账户生命周期已停止供给记忆',
  inactiveHint: '请选择有效账户查看或管理记忆。注销清理进度以操作回执为准。',
  states: { active: '有效账户', frozen: '已冻结', cleaned: '已清理' },
  factsTitle: '有效记忆',
  factCount: '{{count}} 条',
  columns: {
    content: '记忆内容',
    version: '版本',
    sync: '同步状态',
    updated: '更新时间',
    actions: '操作'
  },
  categories: {
    event: '事件',
    person: '人物',
    preference: '偏好',
    situation: '处境'
  },
  sync: { confirmed: '已同步', pending: '同步中', unknown: '待核对' },
  edit: '编辑',
  delete: '删除',
  revoke: '撤回授权',
  editTitle: '编辑记忆',
  editHint: '保存后使用新版本，旧内容将立即停止供给。',
  editLabel: '记忆内容',
  editCount: '{{count}} / 200 字',
  editLength: '请输入 1 至 200 个字符。',
  save: '保存修改',
  cancel: '取消',
  deleteTitle: '删除这条记忆？',
  deleteHint:
    '删除后立即停止供给该事实，检索副本随后清理。事实及证据的保留状态会显示在回执中。',
  revokeTitle: '撤回这项授权？',
  revokeHint: '这项授权覆盖的记忆将停止供给，相关检索副本随后清理。',
  consentRef: '授权引用',
  saved: '记忆已更新',
  deleted: '已停止供给这条记忆',
  revoked: '已撤回授权',
  receiptTitle: '操作回执',
  receiptId: '操作编号',
  supplyStopped: '已停止供给',
  processing: '处理中',
  physicalDone: '清理已完成',
  physicalPending: '清理待完成',
  cleanupScope: '清理范围',
  cleanupScopes: {
    retrievalReplica: '检索副本',
    hubAndRetrievalReplica: '本服务记录与检索副本'
  },
  cleanupUnspecified: '以服务回执为准',
  ledgerRetained: '事实与证据记录仍保留，按保留策略管理。',
  ledgerCleared: '本服务的事实与证据记录已清理。',
  receiptHint: '回执会短时自动刷新；也可手动查询最新进度。',
  outcomeUnknown:
    '尚未确认操作结果，正在按原操作编号查询。停止供给与清理状态以服务回执为准。',
  recallTitle: '检索验证',
  recallHint:
    '输入问题，查看该账户当前可供给的记忆。实际聊天的回复与历史由业务对话入口编排。',
  recallPlaceholder: '例如：这位用户喜欢什么？',
  recallAction: '检索记忆',
  recallEmpty: '本次没有可供给的记忆',
  recallWaiting: '输入问题后查看真实检索结果。',
  contextVersion: '上下文版本 {{version}}',
  recallStates: {
    normal: '读取成功',
    degraded: '已降级',
    stale: '上下文已变化'
  },
  recallDegraded: '记忆暂不可用，本次返回空结果。稍后可重新检索。',
  recallStale: '检索期间记忆发生了变化，请重新检索。',
  profile: '用户概况'
}

export const memoryEn = {
  noNativeTitle: 'No conversation memories',
  noMatchingNativeTitle: 'No matching memories',
  noRecallNativeTitle: 'No memories recalled',
  expandAllTurns: 'Expand all',
  collapseAllTurns: 'Collapse all',
  chooseAccount: 'Choose account',
  switchAccount: 'Switch account to inspect memories',
  memoryTypeCount: '{{count}} types',
  recallMode: 'Diagnostic mode',
  chatSupplyMode: 'Chat supply',
  nativeSearchMode: 'Native retrieval',
  recallLimit: 'Result limit',
  nativeParameterDisclosure: 'Native request parameters',
  nativeParameterHint:
    'Native retrieval requests default to similarity threshold 0.2 and keyword weight 0.7. Acceptance does not prove execution by the current engine. This view does not expose those controls or change default recall policy. Rank is not a similarity score.',
  configurationSummary:
    'Model {{chat}} · Embedding {{embedding}} · Capacity limit {{capacity}}',
  openMemoryContent: 'Open memory {{id}} content',
  recordStatusUnknown: 'Status unavailable',
  statusSupplyScopeHint:
    'Affects subsequent retrieval only. It does not pause extraction or retract sent or in-flight replies.',
  recordFilters: {
    all: 'All states',
    enabled: 'Available',
    disabled: 'Not supplied',
    completed: 'Extraction completed',
    pending: 'Extraction unconfirmed',
    failed: 'Extraction failed'
  },
  managementSubtitle: 'Inspect memories, provenance and retrieval supply.',
  autoMode: 'Automatic chat extraction',
  ledgerMode: 'Reviewed facts',
  managementAutoHint:
    'Conversations form memories automatically. Originals and model-derived content are managed separately. Forgetting and deletion are unavailable.',
  managementLedgerHint:
    'Review approved facts, versions and synchronization within the application capabilities.',
  applicationConfiguration: 'Application settings',
  subjectsTitle: 'Users and accounts',
  searchSubjects: 'Search users or account lifecycles',
  searchSubjectsPlaceholder: 'Search users or accounts',
  noMatchingSubjects: 'No matching accounts',
  selectSubjectHint: 'Select an account on the left to inspect its memories.',
  configurationReadOnlyHint:
    'This view reads the application configuration. Models, extraction prompts and underlying memory banks are not edited here.',
  applicationContract: 'Application integration',
  policyVersion: 'Policy version',
  memoryMode: 'Memory mode',
  enabledMemoryTypes: 'Enabled types',
  contextBudget: 'Recall budget',
  contextBudgetValue:
    'Up to {{chars}} characters, {{count}} records by default',
  managementCapabilities: 'Management capabilities',
  statusManagementEnabled:
    'Enable / disable; forgetting and deletion unavailable',
  statusManagementReadOnly: 'Governed by application capabilities',
  nativeConfiguration: 'Native memory configuration',
  configurationUnavailable:
    'Native configuration readback is unavailable for this application.',
  bankName: 'Memory bank name',
  configurationDescription: 'Description',
  chatModelReference: 'Extraction model reference',
  embeddingModelReference: 'Embedding model reference',
  storageBackend: 'Storage backend',
  capacityLimit: 'Capacity limit',
  nativePermissions: 'Native visibility',
  extractionTemperature: 'Extraction temperature',
  capacityPolicy: 'Native capacity policy',
  configurationCreatedAt: 'Created at',
  capacityAndPolicyHint:
    'Capacity is a configured limit, not measured usage. Showing the native policy does not make active forgetting or deletion available.',
  currentScopeOnly: 'Current account scope only',
  currentScopeStatistics: 'Current account memory statistics',
  currentLifecycle: 'Account lifecycle: ',
  statTurns: 'Original turns',
  statDerived: 'Derived memories',
  statSupplied: 'Available for supply',
  statFailed: 'Failed extraction tasks',
  recordsTab: 'Memory records',
  recallTab: 'Recall diagnostics',
  searchMemoryRecords: 'Search memory records',
  searchMemoryPlaceholder: 'Search text or record ID',
  filterMemoryType: 'Filter memory type',
  filterMemoryStatus: 'Filter memory status',
  allMemoryTypes: 'All types',
  clearFilters: 'Clear filters',
  filteredRecords: '{{turns}} turns · {{records}} records',
  noMatchingMemories: 'No matching memories. Try adjusting the filters.',
  modelDerivedHint:
    'Derived content is model output, not a verified fact. Check the original conversation.',
  sourceTurn: 'Original #{{id}}',
  timeNotProvided: 'Time unavailable',
  derivedCount: '{{count}} derived',
  viewConversation: 'View original',
  viewSource: 'Trace original',
  openMemoryDetail: 'View memory {{id}} details',
  openRelatedMemory: 'View related memory {{id}}',
  notSupplied: 'Not supplied',
  failedWithDerivedHint:
    'Extraction failed, but existing derived content is retained. Check the original; failure does not mean deletion.',
  failedWithoutDerivedHint:
    'Extraction failed and no derived content has been read back. The original is retained.',
  recallDiagnosticHint:
    'Inspect the memories supplied for an actual question. Recent originals are included first; retrieval rank is not a chronology of facts.',
  recalledCount: '{{count}} supplied records',
  recentSourceIncluded: 'Recent originals included',
  contextBudgetReached: 'Context budget reached',
  memoryDetail: 'Memory details',
  nativeOwnStatus: 'Native switch',
  nativeSwitchOn: 'Enabled',
  nativeSwitchOff: 'Disabled',
  effectiveSupplyStatus: 'Effective supply',
  currentlySupplied: 'Available for retrieval supply',
  recordCreatedAt: 'Record created at',
  thisIsSource: 'This is the original',
  fullConversation: 'Complete original conversation',
  extractedContent: 'Model-derived content',
  relatedDerivedTitle: 'Related derived memories · {{count}}',
  nativeTimeMetadataHint:
    'Associated start and end times describe native events or data, not a universal access-expiry rule. Past events and future plans remain memorable.',
  nativeSwitch: 'Toggle memory {{id}} status',
  nativeDetailSwitch: 'Toggle memory {{id}} status in details',
  parentDisabledHint:
    'The original is disabled, so this derived record is not supplied. Restore the original first.',
  statusSwitchHint:
    'Controls retrieval supply without deleting original or derived content.',
  nativeStatusSaved: 'Supply state saved. Refreshing related records.',
  enableMemoryTitle: 'Restore this memory for supply?',
  disableMemoryTitle: 'Disable this memory for supply?',
  enableMemory: 'Restore supply',
  disableMemory: 'Disable supply',
  enableSourceExplanation:
    'Restoring the original restores only derived records that were not individually disabled. All content remains retained.',
  disableSourceExplanation:
    'Disabling the original blocks supply of it and its derived records. Content is retained; this is not forgetting or deletion.',
  enableDerivedExplanation:
    'Restore this derived record only while its original is enabled. Other individually disabled records remain disabled.',
  disableDerivedExplanation:
    'Disable only this derived record, leaving its original and sibling records unchanged. No data is deleted.',
  statusChangeUnconfirmed:
    'The result is unconfirmed or the state has changed. Refresh and inspect before switching again.',

  title: 'Memory',
  subtitle:
    'Review user memories and manage them within the application’s supported capabilities.',
  autoTitle: 'Native memory · Automatic chat extraction',
  autoScope:
    'Completed conversations are submitted automatically for asynchronous semantic, episodic and procedural extraction. Inspect extraction status and provenance here. Forgetting, deletion, consent withdrawal and account cleanup are unavailable in this phase.',
  nativeMessagesTitle: 'Memories from conversations',
  nativeRefreshExtraction: 'Refresh extraction status',
  nativeExtractionHint:
    'Derived records do not prove the background task has completed. Refresh to inspect its latest state.',
  nativeMessagesHint:
    'No manual entry is required. Original conversations and model-derived content are shown separately with their sources and times. Submit corrections through conversation; automatic removal of every historical conflict is not guaranteed.',
  nativeRecallTitle: 'Retrieve native memories',
  noNativeMessages:
    'No memories yet. Complete a conversation and refresh to inspect extraction.',
  nativeRecord: 'Record',
  nativeSource: 'Source',
  nativeActive: 'Available',
  nativeDisabled: 'Not supplied',
  nativeValidAt: 'Associated start',
  nativeInvalidAt: 'Associated end',
  nativeBudgetLimited:
    'Some records exceeded the context budget and were not supplied.',
  nativeColumns: {
    type: 'Memory type',
    source: 'Source / extraction',
    time: 'Time'
  },
  nativeTypes: {
    raw: 'Original conversation',
    semantic: 'Semantic',
    episodic: 'Episodic',
    procedural: 'Procedural'
  },
  extractionStates: {
    received: 'Received',
    pending: 'Pending',
    extracting: 'Extracting',
    completed: 'Completed',
    failed: 'Failed',
    unknown: 'Unconfirmed',
    derivedAvailable: 'Derived memories available'
  },
  nativeTitle: 'Native memory · Original statements',
  nativeScope:
    'Store and retrieve approved original statements. Viewing and correction are available in this phase; forgetting, deletion, consent withdrawal and account cleanup are unavailable. Corrections deactivate previous versions and retain their original content.',
  unsupported:
    'This application does not support the operation in the current phase.',
  forbidden: 'Memory management is available to admins and maintainers',
  application: 'Application',
  applicationOption: '{{application}} · {{tenant}}',
  subject: 'User / account lifecycle',
  subjectOption: '{{user}} · {{lifecycle}}',
  selectApplication: 'Select an application',
  selectSubject: 'Select a user and account',
  refresh: 'Refresh',
  retry: 'Retry',
  synthetic: 'Synthetic data',
  noApplications: 'No memory applications configured',
  noApplicationsHint:
    'Applications appear here after memory service integration.',
  noSubjects: 'This application has no displayable user accounts',
  noSubjectsHint:
    'Accounts appear here after memories are submitted through the business conversation flow.',
  missingScopes:
    '{{count}} older accounts have no displayable user identity and are excluded. Ask an administrator to complete their account mapping.',
  limitedSubjects:
    'Only the first {{limit}} accounts are shown. This application contains additional accounts.',
  noFacts: 'This account has no active memories',
  noFactsHint:
    'Only active memories available within the selected account scope are listed.',
  inactive: 'This account lifecycle no longer supplies memories',
  inactiveHint:
    'Select an active account to view or manage memories. Check the operation receipt for account cleanup progress.',
  states: { active: 'Active account', frozen: 'Frozen', cleaned: 'Cleaned' },
  factsTitle: 'Active memories',
  factCount: '{{count}} memories',
  columns: {
    content: 'Memory',
    version: 'Version',
    sync: 'Sync status',
    updated: 'Updated',
    actions: 'Actions'
  },
  categories: {
    event: 'Event',
    person: 'Person',
    preference: 'Preference',
    situation: 'Situation'
  },
  sync: {
    confirmed: 'Synced',
    pending: 'Syncing',
    unknown: 'Awaiting confirmation'
  },
  edit: 'Edit',
  delete: 'Delete',
  revoke: 'Withdraw consent',
  editTitle: 'Edit memory',
  editHint:
    'Saving creates a new version and immediately stops supplying the previous content.',
  editLabel: 'Memory content',
  editCount: '{{count}} / 200 characters',
  editLength: 'Enter between 1 and 200 characters.',
  save: 'Save changes',
  cancel: 'Cancel',
  deleteTitle: 'Delete this memory?',
  deleteHint:
    'The fact stops being supplied immediately. Its retrieval copy is then removed. The receipt shows whether facts and evidence are retained.',
  revokeTitle: 'Withdraw this consent?',
  revokeHint:
    'Memories covered by this consent stop being supplied, followed by removal of their retrieval copies.',
  consentRef: 'Consent reference',
  saved: 'Memory updated',
  deleted: 'Memory supply stopped',
  revoked: 'Consent withdrawn',
  receiptTitle: 'Operation receipt',
  receiptId: 'Operation ID',
  supplyStopped: 'Supply stopped',
  processing: 'Processing',
  physicalDone: 'Cleanup completed',
  physicalPending: 'Cleanup pending',
  cleanupScope: 'Cleanup scope',
  cleanupScopes: {
    retrievalReplica: 'Retrieval copy',
    hubAndRetrievalReplica: 'Service records and retrieval copy'
  },
  cleanupUnspecified: 'As reported by the service',
  ledgerRetained:
    'Facts and evidence remain stored under the retention policy.',
  ledgerCleared:
    'Facts and evidence held by this service have been cleaned up.',
  receiptHint:
    'The receipt refreshes automatically for a short period. You can also request the latest status.',
  outcomeUnknown:
    'The operation outcome is unconfirmed. Its original ID is being checked. Supply and cleanup status require a service receipt.',
  recallTitle: 'Test retrieval',
  recallHint:
    'Enter a question to inspect memories currently available for this account. Replies and history in an actual chat are coordinated by the business conversation flow.',
  recallPlaceholder: 'For example: What does this user like?',
  recallAction: 'Retrieve memories',
  recallEmpty: 'No memories available for this request',
  recallWaiting: 'Enter a question to view actual retrieval results.',
  contextVersion: 'Context version {{version}}',
  recallStates: {
    normal: 'Retrieved',
    degraded: 'Degraded',
    stale: 'Context changed'
  },
  recallDegraded:
    'Memory is temporarily unavailable and this request returned no facts. Try again later.',
  recallStale: 'Memories changed during retrieval. Run the query again.',
  profile: 'User profile'
} satisfies typeof memoryZh

export const memoryErrorsZh = {
  MEMORY_OPERATION_UNSUPPORTED:
    '本期暂不支持该记忆操作，请查看当前应用的能力说明',
  memoryOperationUnsupported:
    '本期暂不支持该记忆操作，请查看当前应用的能力说明',
  memoryAdminUnauthorized: '需要登录后台后操作记忆',
  memoryInvalidRequest: '记忆请求字段无效，请检查后重试',
  memoryUnavailable: '记忆服务暂不可用',
  memoryAdminApplicationForbidden: '当前后台租户无权管理该记忆应用',
  memoryNotFound: '记忆记录不存在',
  memoryConflict: '记忆版本、任务或幂等键冲突，请刷新后重试',
  memoryScopeInactive: '当前账户生命周期不可用',
  memoryOutcomeUnknown: '检索副本清理或同步尚未确认，请查询操作状态',
  memoryTimeout: '记忆读取已超时，请重试'
}

export const memoryErrorsEn = {
  MEMORY_OPERATION_UNSUPPORTED:
    'This memory operation is unavailable in the current phase.',
  memoryOperationUnsupported:
    'This memory operation is unavailable in the current phase.',
  memoryAdminUnauthorized:
    'Sign in to the management console to manage memories',
  memoryInvalidRequest:
    'Invalid memory request. Check the fields and try again.',
  memoryUnavailable: 'Memory service is temporarily unavailable',
  memoryAdminApplicationForbidden:
    'Your console tenant cannot manage this memory application',
  memoryNotFound: 'Memory record does not exist',
  memoryConflict:
    'The memory version, task or operation ID conflicts. Refresh before retrying.',
  memoryScopeInactive: 'This account lifecycle is unavailable',
  memoryOutcomeUnknown:
    'Retrieval cleanup or synchronization is unconfirmed. Check the operation status.',
  memoryTimeout: 'Memory retrieval timed out. Try again.'
} satisfies typeof memoryErrorsZh
