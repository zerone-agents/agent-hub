import apiClient, { unwrapResponse } from './client'

export interface MemoryApplication {
  tenantId: string
  applicationId: string
  policyVersion: string
  dataClass: string
  name?: string
  description?: string
  status?: string
  origin?: string
  backend?: 'multirag-memory' | 'multirag-knowledge'
  nativeConfigurationSupported?: boolean
  nativeStatusSupported?: boolean
  nativeForgetSupported?: boolean
  nativeDiagnosticsSupported?: boolean
  nativeMaxContextChars?: number
  nativeDefaultMessages?: number
  nativeMaxMessages?: number
  conversationCorrectionsSupported?: boolean
  automaticExtractionSupported?: boolean
  memoryTypes?: string[]
  rawOnly?: boolean
  correctionSupported?: boolean
  forgetSupported?: boolean
  physicalDeleteSupported?: boolean
  consentRevokeSupported?: boolean
  deactivateSupported?: boolean
}

export interface MemoryScope {
  tenantId: string
  applicationId: string
  userId: string
  accountLifecycleId: string
}

export interface MemorySubject {
  scope: MemoryScope
  state: string
  revision: number
}

export interface MemorySubjectList {
  subjects: MemorySubject[]
  missingScopeCount: number
  limited: boolean
  limit: number
}

export interface MemoryFact {
  factId: string
  version: number
  content: string
  subject: string
  temporal: string
  category: string
  status: string
  syncStatus: string
  consentRef?: string
  updatedAt: string
}

export interface NativeMemoryMessage {
  id: string
  sourceId: string
  sourceSessionId: string
  type: 'raw' | 'semantic' | 'episodic' | 'procedural'
  content: string
  createdAt: string
  validAt?: string
  invalidAt?: string
  status: boolean
  forgottenAt?: string
  parentForgottenAt?: string
  nativeStatus?: boolean
  parentStatus?: boolean
  statusChangeAllowed?: boolean
  extractionStatus?: string
  extractionProgress?: number
  extractionError?: string
}

export interface NativeMemoryContext {
  recentSourceIncluded: boolean
  currentnessNote: string
  budgetLimited: boolean
  messages: NativeMemoryMessage[]
  status: string
}

export interface NativeMemoryConfiguration {
  name: string
  description?: string
  memoryTypes: string[]
  storageType?: string
  chatModel?: string
  embeddingModel?: string
  permissions?: string
  capacityBytes?: number
  forgettingPolicy?: string
  temperature?: number
  createdAt?: string
}

export interface NativeMemorySearchResult {
  status: string
  messages: NativeMemoryMessage[]
  options: {
    limit: number
    similarityThreshold: number
    keywordWeight: number
  }
}

export interface MemoryOperation {
  operationId: string
  status: string
  stoppedSupply: boolean
  physicalCleanup: boolean
  cleanupScope: string
  ledgerRetained: boolean
}

export interface MemoryContext {
  contextVersion: number
  facts: MemoryFact[]
  profile?: string
  status: string
}

export interface MemoryFactMutation {
  scope: MemoryScope
  operationId: string
  expectedVersion: number
}

const base = '/api/v1/admin/memory'

export interface MemoryApplicationModel {
  fullId: string
  name: string
  factory: string
  instanceName?: string
}

export interface CreateMemoryApplication {
  operationId: string
  applicationId: string
  name: string
  description?: string
  chatModel: string
  embeddingModel: string
}

export interface MemoryApplicationReceipt {
  application: MemoryApplication
  applicationToken: string
}

export const memoryApi = {
  applicationModels: (signal?: AbortSignal) =>
    apiClient.get(base + '/application-models', { signal }).then((res) =>
      unwrapResponse<{
        chat: MemoryApplicationModel[]
        embedding: MemoryApplicationModel[]
      }>(res)
    ),
  createApplication: (request: CreateMemoryApplication) =>
    apiClient
      .post(base + '/applications', request)
      .then((res) => unwrapResponse<MemoryApplicationReceipt>(res)),
  applicationProvisioning: (applicationId: string) =>
    apiClient
      .get(
        base +
          '/applications/' +
          encodeURIComponent(applicationId) +
          '/provisioning'
      )
      .then((res) => unwrapResponse<MemoryApplication>(res)),
  applications: (signal?: AbortSignal) =>
    apiClient
      .get(base + '/applications', { signal })
      .then((res) => unwrapResponse<MemoryApplication[]>(res)),
  subjects: (
    application: Pick<MemoryApplication, 'tenantId' | 'applicationId'>,
    signal?: AbortSignal
  ) =>
    apiClient
      .get(base + '/subjects', {
        params: {
          tenantId: application.tenantId,
          applicationId: application.applicationId
        },
        signal
      })
      .then((res) => unwrapResponse<MemorySubjectList>(res)),
  nativeMessages: (scope: MemoryScope, signal?: AbortSignal) =>
    apiClient
      .get(base + '/native-messages', { params: scope, signal })
      .then((res) => unwrapResponse<NativeMemoryMessage[]>(res)),
  nativeConfig: (
    application: Pick<MemoryApplication, 'tenantId' | 'applicationId'>,
    signal?: AbortSignal
  ) =>
    apiClient
      .get(base + '/native-config', {
        params: {
          tenantId: application.tenantId,
          applicationId: application.applicationId
        },
        signal
      })
      .then((res) => unwrapResponse<NativeMemoryConfiguration>(res)),
  setNativeStatus: (
    id: string,
    request: { scope: MemoryScope; enabled: boolean; expectedEnabled: boolean }
  ) =>
    apiClient
      .patch(base + '/native-messages/' + encodeURIComponent(id), request)
      .then((res) => unwrapResponse<NativeMemoryMessage>(res)),
  forgetNativeMessage: (id: string, scope: MemoryScope) =>
    apiClient
      .post(base + '/native-messages/' + encodeURIComponent(id) + '/forget', {
        scope
      })
      .then((res) => unwrapResponse<NativeMemoryMessage>(res)),
  nativeSearch: (request: {
    scope: MemoryScope
    query: string
    limit?: number
    similarityThreshold?: number
    keywordWeight?: number
  }) =>
    apiClient
      .post(base + '/native-search', request)
      .then((res) => unwrapResponse<NativeMemorySearchResult>(res)),
  nativeContext: (request: {
    scope: MemoryScope
    query: string
    limit?: number
  }) =>
    apiClient
      .post(base + '/native-context', request)
      .then((res) => unwrapResponse<NativeMemoryContext>(res)),
  facts: (scope: MemoryScope, signal?: AbortSignal) =>
    apiClient
      .get(base + '/facts', { params: scope, signal })
      .then((res) => unwrapResponse<MemoryFact[]>(res)),
  update: (factId: string, request: MemoryFactMutation & { content: string }) =>
    apiClient
      .patch(base + '/facts/' + encodeURIComponent(factId), request)
      .then((res) => unwrapResponse<MemoryFact>(res)),
  delete: (factId: string, request: MemoryFactMutation) =>
    apiClient
      .delete(base + '/facts/' + encodeURIComponent(factId), { data: request })
      .then((res) => unwrapResponse<MemoryOperation>(res)),
  revoke: (request: {
    scope: MemoryScope
    consentRef: string
    operationId: string
  }) =>
    apiClient
      .post(base + '/consents/revoke', request)
      .then((res) => unwrapResponse<MemoryOperation>(res)),
  context: (request: { scope: MemoryScope; query: string }) =>
    apiClient
      .post(base + '/context', request)
      .then((res) => unwrapResponse<MemoryContext>(res)),
  operation: (scope: MemoryScope, operationId: string, signal?: AbortSignal) =>
    apiClient
      .get(base + '/operations/' + encodeURIComponent(operationId), {
        params: scope,
        signal
      })
      .then((res) => unwrapResponse<MemoryOperation>(res))
}

export function memoryScopeKey(scope: MemoryScope): string {
  return JSON.stringify([
    scope.tenantId,
    scope.applicationId,
    scope.userId,
    scope.accountLifecycleId
  ])
}

export function memoryApplicationKey(
  application: Pick<MemoryApplication, 'tenantId' | 'applicationId'>
): string {
  return JSON.stringify([application.tenantId, application.applicationId])
}
