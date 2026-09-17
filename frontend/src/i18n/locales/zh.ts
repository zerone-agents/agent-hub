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
    totalItems: '共 {{total}} 条',
    brandSubtitle: 'AI Agent 管理平台',
    logout: '退出登录',
    loading: '加载中...'
  },
  components: {
    confirmDelete: {
      title: '确认删除？',
      description: '删除后不可恢复'
    },
    statusBadge: {
      active: '启用',
      inactive: '停用'
    },
    notFound: {
      subTitle: '抱歉，您访问的页面不存在。',
      back: '回到首页'
    },
    appHeader: {
      users: '用户管理',
      auditLogs: '审计日志',
      aigcConfig: 'AIGC 标识配置',
      toggleSidebar: '切换侧边栏',
      menu: '菜单'
    },
    userDropdown: {
      changePassword: '修改密码'
    },
    errorBoundary: {
      title: '页面出错了',
      refresh: '刷新页面'
    },
    themeControls: {
      light: '浅色',
      dark: '深色',
      system: '跟随系统',
      settings: '主题设置',
      switchColor: '切换配色',
      switchMode: '切换明暗模式'
    },
    headerLinks: {
      github: 'GitHub 仓库',
      site: '官方网站',
      ariaLabel: '相关链接'
    },
    nameSearch: {
      placeholder: '搜索名称'
    },
    passwordInput: {
      show: '显示密码',
      hide: '隐藏密码'
    },
    manualCopy: {
      title: '手动复制',
      close: '关闭',
      hint: '浏览器限制非 HTTPS 页面自动复制。请选中下方内容后按 ⌘C / Ctrl+C 复制，完成后关闭。'
    }
  },
  auth: {
    guestTitle: '体验模式',
    guestHint: '账号尚未开通管理权限（待审核）。可先前往 Agent 聊天页继续体验，或联系管理员分配角色。',
    goChat: '前往 Agent 聊天'
  },
  setup: {
    passwordRule: '密码至少 8 位，且需包含字母和数字',
    passwordMismatch: '两次输入的密码不一致',
    initTitle: '初始化系统',
    initSub: '创建管理员账号（用户名固定为 admin）',
    passwordPlaceholder: '设置管理员密码',
    confirmPlaceholder: '确认密码',
    submit: '创建并登录'
  },
  register: {
    title: '加入 Agent Hub',
    sub: '填写信息完成注册',
    inviteNote: '邀请备注：{{note}}',
    usernamePlaceholder: '用户名（3-32 位字母数字下划线连字符）',
    nicknamePlaceholder: '昵称（可选）',
    passwordPlaceholder: '密码（至少 8 位，含字母和数字）',
    submit: '注册并登录',
    invalidTitle: '邀请链接无效',
    invalidDesc: '邀请链接无效或已失效，请联系管理员重新获取。',
    backToLogin: '返回登录'
  },
  audit: {
    pageTitle: '审计日志',
    pageSub: '管理操作审计记录（只读）',
    searchPlaceholder: '搜索用户',
    refresh: '刷新',
    noDetail: '无详情',
    status: {
      success: '成功',
      failure: '失败',
      partial: '部分生效'
    },
    columns: {
      time: '时间',
      user: '用户',
      action: '动作',
      target: '对象',
      result: '结果'
    }
  },
  cliTokens: {
    pageSub: '管理 CLI 认证 Token，用于 zhub CLI 工具的长期身份验证',
    create: '创建 CLI Token',
    empty: '暂无 Token，点击上方按钮创建',
    neverUsed: '从未使用',
    revokeTitle: '确认撤销？',
    revokeConfirm: '撤销 "{{name}}" 后，使用该 Token 的 CLI 将无法继续认证。',
    revoke: '撤销',
    ttlDays: '{{days}} 天',
    nameRuleLabel: 'Token 名称',
    nameTaken: '该名称已存在，请使用其他名称',
    createdTitle: 'Token 已创建',
    saved: '我已保存',
    createSubmit: '创建',
    saveNowTitle: '请立即保存此 Token',
    saveNowDesc: '关闭此窗口后将无法再次查看。请将 Token 安全存储。',
    copied: '已复制',
    copyFailed: '复制失败，请手动选择复制',
    copy: '复制',
    nameLabel: '名称',
    namePlaceholder: '例如：my-macbook',
    ttlLabel: '有效期',
    columns: {
      name: '名称',
      createdAt: '创建时间',
      lastUsed: '最后使用',
      expiresAt: '过期时间',
      actions: '操作'
    }
  },
  dashboard: {
    pageTitle: '仪表盘',
    pageSub: '查看智能体资源、配置健康度与最近变化。',
    loadError: '仪表盘数据加载失败，请稍后刷新重试',
    synced: '数据已同步',
    overviewLabel: '系统总览',
    heroLabel: '已接入资源',
    heroUnit: '项配置',
    healthTitle: '配置健康度',
    healthNote: '基于桌面端代理占比、模型提供方接入和 MCP 探测结果综合计算。',
    statsLabel: '资源统计',
    growthTitle: '资源增长',
    growthSub: '按七天聚合的新增配置',
    trendLabel: '最近八周资源新增趋势',
    compositionTitle: '资源构成',
    compositionSub: '当前工作空间的能力分布',
    activityTitle: '最近活动',
    activitySub: '跨资源的最新配置记录',
    noActivity: '暂无最近活动',
    stats: {
      tools: '工具',
      mcps: 'MCP 配置',
      skills: '技能',
      providers: '提供方',
      models: '模型',
      knowledge: '知识库',
      scenes: '场景'
    },
    metric: {
      desktopAgents: '桌面端代理',
      modelsProviders: '模型 / 提供方',
      knowledgeChunks: '知识文档 / 切块',
      chatSessions: '聊天会话'
    },
    readiness: {
      desktopAgents: '桌面端代理',
      providers: 'Provider 接入',
      mcps: 'MCP 正常'
    },
    activityType: {
      knowledge: '知识库'
    }
  },
  aigcConfig: {
    pageTitle: 'AIGC 标识配置',
    pageSub: '依据 GB 45438-2025，配置后部署 Agent 将自动携带 AI 生成内容标识；签名密钥由后端自动生成并保管。',
    usccLabel: '统一社会信用代码',
    usccRequired: '请输入 18 位统一社会信用代码',
    usccPattern: '须为 18 位数字与大写字母（不含 I/O/S/V/Z）',
    usccPlaceholder: '18 位统一社会信用代码',
    companyLabel: '公司完整名称',
    companyRequired: '请输入公司完整名称',
    companyPlaceholder: '与营业执照一致的公司全称',
    save: '保存配置',
    producerCode: '服务提供者编码',
    signingKey: '签名密钥',
    keyConfigured: '已配置（由后端保管）',
    keyMissing: '未配置',
    modelCodes: '模型 AIGC 码',
    modelCodesHint1: '模型 AIGC 码在',
    providersLink: '模型管理',
    modelCodesHint2: '中按模型自动分配',
    regenerateTitle: '确认重新生成签名密钥？',
    regenerateDesc: '重新生成后，历史内容的签名将无法用新密钥验签。',
    regenerate: '重新生成',
    regenerateKey: '重新生成密钥',
    clearTitle: '确认清除配置？',
    clearDesc: '清除后部署 Agent 将不再携带 AIGC 标识。',
    clear: '清除',
    clearConfig: '清除配置'
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
