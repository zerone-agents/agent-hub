import apiClient from './client'

export interface ExtensionExample {
  id: string
  name: string
  kind: string
  description: string
  manifest: string
}

export interface ContributionCategory {
  key: string
  label: string
  description?: string
}

export interface H0AcceptanceInfo {
  platformVersion: string
  protocolVersion: string
  status: string
  examples: ExtensionExample[]
  contributionCategories: ContributionCategory[]
}

export interface ExtensionValidationError {
  path?: string
  message: string
}

export interface ExtensionValidationResult {
  valid: boolean
  package?: {
    name?: string
    namespace?: string
    version?: string
  }
  contributions?: { key: string; count: number }[]
  errors?: ExtensionValidationError[]
}

export const extensionApi = {
  getH0AcceptanceInfo: () =>
    apiClient.get('/api/v1/admin/extensions/h0'),
  validate: (manifest: string) =>
    apiClient.post('/api/v1/admin/extensions/validate', { manifest }),
}
