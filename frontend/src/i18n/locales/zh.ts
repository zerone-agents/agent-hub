const zh = {
  language: {
    zh: '中文',
    en: 'English',
    switch: '切换语言'
  },
  common: {
    edit: '编辑',
    delete: '删除',
    cancel: '取消',
    totalItems: '共 {{total}} 条'
  },
  scenes: {
    pageTitle: '场景管理',
    pageSub: '管理 Agent 场景配置，组合 Agent 与提示词预设',
    create: '新建场景',
    editTitle: '编辑场景',
    update: '更新',
    createSubmit: '创建',
    searchPlaceholder: '搜索场景名称',
    deleteConfirmTitle: '确认删除？',
    deleteConfirm: '删除 "{{name}}"？此操作不可撤销。',
    columns: {
      name: '场景标识',
      title: '场景名称',
      agent: '关联 Agent',
      prompt: '提示词',
      status: '状态',
      createdAt: '创建时间',
      actions: '操作'
    },
    form: {
      basicSection: '基本信息',
      agentRequired: '请选择关联 Agent',
      agentPlaceholder: '选择关联的 Agent',
      titlePlaceholder: '场景展示名称',
      promptSection: '提示词配置',
      promptPlaceholder: '输入该场景的提示词，定义 Agent 的行为和角色',
      enabled: '启用状态'
    }
  }
}

export default zh
