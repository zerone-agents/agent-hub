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
    totalItems: '{{total}} items in total'
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
