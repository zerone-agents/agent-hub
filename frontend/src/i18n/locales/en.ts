import type zh from './zh'

const en = {
  language: {
    zh: '中文',
    en: 'English',
    switch: 'Switch language'
  },
  common: {
    edit: 'Edit',
    delete: 'Delete',
    cancel: 'Cancel',
    totalItems: '{{total}} items in total',
    brandSubtitle: 'AI Agent Management Platform',
    logout: 'Log Out',
    loading: 'Loading...',
    form: {
      required: 'Please enter {{label}}',
      maxLength: '{{label}} must be at most {{max}} characters',
      identifierCharset: '{{label}} can only contain letters, digits, dots, underscores and hyphens',
      agentCharset: '{{label}} can only contain lowercase letters, digits and hyphens, must start with a letter, and hyphens must not be consecutive or at either end'
    }
  },
  apiErrors: {
    requestFailed: 'Request failed',
    unauthorized: 'Session expired, please sign in again',
    forbidden: 'You do not have permission to perform this action',
    notFound: 'Resource does not exist or has been deleted',
    serverBusy: 'Server is busy, please try again later',
    timeout: 'Request timed out, please check your network',
    networkError: 'Network connection failed',
    operationFailed: 'Operation failed, please retry',
    uploadFailed: 'Upload failed'
  },
  time: {
    justNow: 'just now',
    minutesAgo: '{{n}} minutes ago',
    hoursAgo: '{{n}} hours ago',
    daysAgo: '{{n}} days ago'
  },
  // antd Form validateMessages: values keep antd's ${label}/${min} syntax —
  // i18next interpolation is {{}} by default; ${} passes through to antd.
  validate: {
    default: 'Validation failed',
    required: 'Please enter ${label}',
    enum: '${label} must be one of [${enum}]',
    whitespace: '${label} cannot be whitespace only',
    types: {
      email: '${label} is not a valid email',
      url: '${label} is not a valid url'
    },
    string: {
      len: '${label} must be exactly ${len} characters',
      min: '${label} must be at least ${min} characters',
      max: '${label} must be at most ${max} characters'
    },
    number: {
      min: '${label} cannot be less than ${min}',
      max: '${label} cannot be greater than ${max}'
    }
  },
  nav: {
    home: 'Home',
    settings: 'Settings',
    detail: 'Details',
    settingsLabels: {
      aigc: 'AIGC Labeling',
      users: 'Users',
      auditLogs: 'Audit Logs'
    },
    dashboard: 'Dashboard',
    agents: 'Agents',
    tools: 'Tools',
    mcps: 'MCP Configs',
    skills: 'Skills',
    providers: 'Models',
    knowledge: 'Knowledge Bases',
    scenes: 'Scenes',
    chat: 'Chat History'
  },
  agentIcons: {
    chat: 'General Chat',
    chart: 'Data Analysis',
    shield: 'Security & Compliance',
    crosshair: 'Precision Targeting',
    userCircle: 'User Management',
    terminal: 'Terminal Ops',
    robot: 'Smart Assistant',
    lightbulb: 'Creative Ideas',
    cpu: 'Computing',
    detective: 'Investigation',
    compass: 'Navigation',
    rocket: 'Fast Execution',
    gear: 'Engineering Config',
    code: 'Development',
    education: 'Education',
    globe: 'Globalization',
    puzzle: 'Integration',
    eye: 'Monitoring',
    megaphone: 'Marketing',
    notebook: 'Knowledge Management',
    firstAid: 'Healthcare',
    scales: 'Legal',
    presentation: 'Business Presentation',
    clipboard: 'Task Management',
    headset: 'Customer Service',
    wrench: 'Repair Tools',
    lightning: 'Rapid Response',
    finance: 'Finance'
  },
  components: {
    confirmDelete: {
      title: 'Confirm deletion?',
      description: 'This cannot be undone'
    },
    statusBadge: {
      active: 'Enabled',
      inactive: 'Disabled'
    },
    notFound: {
      subtitle: 'Sorry, the page you visited does not exist.',
      back: 'Back to Home'
    },
    appHeader: {
      users: 'Users',
      auditLogs: 'Audit Logs',
      aigcConfig: 'AIGC Labeling',
      toggleSidebar: 'Toggle sidebar',
      menu: 'Menu'
    },
    userDropdown: {
      changePassword: 'Change Password'
    },
    errorBoundary: {
      title: 'Something went wrong',
      refresh: 'Refresh Page'
    },
    themeControls: {
      light: 'Light',
      dark: 'Dark',
      system: 'System',
      settings: 'Theme settings',
      switchColor: 'Switch color scheme',
      switchMode: 'Switch light/dark mode'
    },
    headerLinks: {
      github: 'GitHub Repository',
      site: 'Official Website',
      ariaLabel: 'Related links'
    },
    nameSearch: {
      placeholder: 'Search name'
    },
    passwordInput: {
      show: 'Show password',
      hide: 'Hide password'
    },
    manualCopy: {
      title: 'Manual Copy',
      close: 'Close',
      hint: 'Browsers restrict automatic copying on non-HTTPS pages. Select the content below and press ⌘C / Ctrl+C to copy, then close.'
    }
  },
  auth: {
    guestTitle: 'Experience Mode',
    guestHint: 'This account has no management permissions yet (pending review). You can continue exploring the Agent chat page, or contact an administrator to assign a role.',
    goChat: 'Go to Agent Chat'
  },
  setup: {
    passwordRule: 'Password must be at least 8 characters and contain letters and numbers',
    passwordMismatch: 'The two passwords do not match',
    initTitle: 'Initialize System',
    initSub: 'Create the admin account (username is fixed to admin)',
    passwordPlaceholder: 'Set admin password',
    confirmPlaceholder: 'Confirm password',
    submit: 'Create & Sign In'
  },
  register: {
    title: 'Join Agent Hub',
    sub: 'Fill in the form to complete registration',
    inviteNote: 'Invitation note: {{note}}',
    usernamePlaceholder: 'Username (3-32 letters, digits, underscore or hyphen)',
    nicknamePlaceholder: 'Nickname (optional)',
    passwordPlaceholder: 'Password (at least 8 characters, letters and numbers)',
    submit: 'Sign Up & Sign In',
    invalidTitle: 'Invalid Invitation Link',
    invalidDesc: 'This invitation link is invalid or expired. Please contact an administrator for a new one.',
    backToLogin: 'Back to Sign In'
  },
  audit: {
    pageTitle: 'Audit Logs',
    pageSub: 'Read-only audit trail of management operations',
    searchPlaceholder: 'Search user',
    refresh: 'Refresh',
    noDetail: 'No details',
    status: {
      success: 'Success',
      failure: 'Failure',
      partial: 'Partial'
    },
    columns: {
      time: 'Time',
      user: 'User',
      action: 'Action',
      target: 'Target',
      result: 'Result'
    }
  },
  cliTokens: {
    pageSub: 'Manage CLI auth tokens for long-term authentication of the zhub CLI tool',
    create: 'Create CLI Token',
    empty: 'No tokens yet. Click the button above to create one',
    neverUsed: 'Never used',
    revokeTitle: 'Confirm revocation?',
    revokeConfirm: 'After revoking "{{name}}", CLIs using this token will no longer be able to authenticate.',
    revoke: 'Revoke',
    ttlDays: '{{days}} days',
    nameRuleLabel: 'Token name',
    nameTaken: 'This name already exists. Please use another one',
    createdTitle: 'Token Created',
    saved: 'I have saved it',
    createSubmit: 'Create',
    saveNowTitle: 'Save this token now',
    saveNowDesc: 'It cannot be viewed again after closing this window. Store the token securely.',
    copied: 'Copied',
    copyFailed: 'Copy failed. Please select and copy manually',
    copy: 'Copy',
    nameLabel: 'Name',
    namePlaceholder: 'e.g. my-macbook',
    ttlLabel: 'Validity',
    columns: {
      name: 'Name',
      createdAt: 'Created At',
      lastUsed: 'Last Used',
      expiresAt: 'Expires At',
      actions: 'Actions'
    },
    toast: {
      created: 'Token created',
      revoked: 'Token revoked'
    }
  },
  dashboard: {
    pageTitle: 'Dashboard',
    pageSub: 'Overview of agent resources, configuration health and recent changes.',
    loadError: 'Failed to load dashboard data. Please refresh and try again later',
    synced: 'Data synced',
    overviewLabel: 'System overview',
    heroLabel: 'Connected Resources',
    heroUnit: 'configurations',
    healthTitle: 'Configuration Health',
    healthNote: 'Computed from desktop agent coverage, model provider integration and MCP probe results.',
    statsLabel: 'Resource statistics',
    growthTitle: 'Resource Growth',
    growthSub: 'New configurations aggregated over seven days',
    trendLabel: 'Resource growth trend of the last eight weeks',
    compositionTitle: 'Resource Composition',
    compositionSub: 'Capability distribution of the current workspace',
    activityTitle: 'Recent Activity',
    activitySub: 'Latest configuration records across resources',
    noActivity: 'No recent activity',
    stats: {
      tools: 'Tools',
      mcps: 'MCP Configs',
      skills: 'Skills',
      providers: 'Providers',
      models: 'Models',
      knowledge: 'Knowledge Bases',
      scenes: 'Scenes'
    },
    metric: {
      desktopAgents: 'Desktop Agents',
      modelsProviders: 'Models / Providers',
      knowledgeChunks: 'Knowledge Docs / Chunks',
      chatSessions: 'Chat Sessions'
    },
    readiness: {
      desktopAgents: 'Desktop Agents',
      providers: 'Provider Integration',
      mcps: 'MCP Healthy'
    },
    activityType: {
      knowledge: 'Knowledge Base'
    }
  },
  aigcConfig: {
    pageTitle: 'AIGC Labeling Configuration',
    pageSub: 'Per GB 45438-2025, deployed agents will automatically carry AI-generated content labels once configured; the signing key is generated and kept by the backend.',
    usccLabel: 'Unified Social Credit Code',
    usccRequired: 'Please enter the 18-character unified social credit code',
    usccPattern: 'Must be 18 characters of digits and uppercase letters (excluding I/O/S/V/Z)',
    usccPlaceholder: '18-character unified social credit code',
    companyLabel: 'Full Company Name',
    companyRequired: 'Please enter the full company name',
    companyPlaceholder: 'Full company name matching the business license',
    save: 'Save Configuration',
    producerCode: 'Content Producer Code',
    signingKey: 'Signing Key',
    keyConfigured: 'Configured (kept by backend)',
    keyMissing: 'Not configured',
    modelCodes: 'Model AIGC Codes',
    modelCodesHint1: 'Model AIGC codes are',
    providersLink: 'Model Management',
    modelCodesHint2: 'assigned automatically per model in',
    regenerateTitle: 'Regenerate the signing key?',
    regenerateDesc: 'After regeneration, signatures of historical content can no longer be verified with the new key.',
    regenerate: 'Regenerate',
    regenerateKey: 'Regenerate Key',
    clearTitle: 'Clear the configuration?',
    clearDesc: 'After clearing, deployed agents will no longer carry AIGC labels.',
    clear: 'Clear',
    clearConfig: 'Clear Configuration',
    toast: {
      saved: 'AIGC labeling configuration saved',
      keyRegenerated: 'Signing key regenerated',
      cleared: 'AIGC labeling configuration cleared'
    }
  },
  tools: {
    toast: {
      uploaded: 'Custom tool uploaded',
      fileUpdated: 'Tool file updated',
      updated: 'Tool updated',
      deleted: 'Tool deleted'
    }
  },
  agents: {
    toast: {
      knowledgeUpdated: 'Knowledge bases updated',
      agentCreated: 'Agent created',
      agentUpdated: 'Agent updated',
      agentDeleted: 'Agent deleted',
      subagentUpdated: 'Subagent updated',
      toolsUpdated: 'Tools updated',
      skillsUpdated: 'Skills updated'
    }
  },
  knowledge: {
    toast: {
      created: 'Knowledge base created',
      updated: 'Knowledge base updated',
      deleted: 'Knowledge base deleted',
      docUpdated: 'Document updated',
      docDeleted: 'Document deleted',
      docsUploaded: '{{n}} documents uploaded',
      parseQueued: 'Queued for parsing',
      parseStopped: 'Parsing stopped',
      chunkAdded: 'Chunk added',
      chunkSaved: 'Chunk saved',
      chunkDeleted: 'Chunk deleted'
    }
  },
  mcps: {
    toast: {
      created: 'MCP created',
      updated: 'MCP updated',
      deleted: 'MCP deleted',
      agentsUpdated: 'Agent MCP relations updated',
      probeDone: 'Probe finished',
      probeFailed: 'Probe failed'
    }
  },
  skills: {
    toast: {
      created: 'Skill created',
      updated: 'Skill updated',
      deleted: 'Skill deleted'
    }
  },
  providers: {
    toast: {
      created: 'Provider created',
      updated: 'Provider updated',
      deleted: 'Provider deleted'
    }
  },
  chat: {
    toast: {
      sessionDeleted: 'Session deleted'
    }
  },
  scenes: {
    pageTitle: 'Scenes',
    pageSub: 'Manage agent scene presets combining agents and prompts',
    create: 'New Scene',
    editTitle: 'Edit Scene',
    update: 'Update',
    createSubmit: 'Create',
    searchPlaceholder: 'Search scene name',
    deleteConfirmTitle: 'Confirm deletion?',
    deleteConfirm: 'Delete "{{name}}"? This cannot be undone.',
    columns: {
      name: 'ID',
      title: 'Name',
      agent: 'Linked Agent',
      prompt: 'Prompt',
      status: 'Status',
      createdAt: 'Created At',
      actions: 'Actions'
    },
    form: {
      basicSection: 'Basics',
      agentRequired: 'Please select a linked agent',
      agentPlaceholder: 'Select a linked agent',
      titlePlaceholder: 'Scene display name',
      promptSection: 'Prompt Configuration',
      promptPlaceholder: "Define the agent's behavior and role for this scene",
      enabled: 'Enabled'
    },
    toast: {
      created: 'Scene created',
      updated: 'Scene updated',
      deleted: 'Scene deleted'
    }
  }
} satisfies typeof zh

export default en
