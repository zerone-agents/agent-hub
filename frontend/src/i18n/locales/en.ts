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
    logout: 'Log Out'
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
    clearConfig: 'Clear Configuration'
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
    }
  }
} satisfies typeof zh

export default en
