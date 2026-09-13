import apiClient from "./client";

export type WorkflowStepType = "task" | "handoff" | "approval" | "decision";
export interface WorkflowStepDefinition {
  key: string;
  name: string;
  type: WorkflowStepType;
  actorType: string;
  actorRef: string;
  dependsOn?: string[];
  config?: Record<string, unknown>;
  timeoutSeconds?: number;
  maxRetries?: number;
  escalationStepKey?: string;
  compensationStepKey?: string;
}
export interface WorkflowTransition {
  fromStepKey: string;
  toStepKey: string;
  condition?: Record<string, unknown>;
  priority?: number;
}
export interface WorkflowVersion {
  id: string;
  workflowId: string;
  version: number;
  status: "draft" | "published";
  inputSchema?: Record<string, unknown>;
  outputSchema?: Record<string, unknown>;
  steps: WorkflowStepDefinition[];
  transitions?: WorkflowTransition[];
  createdAt?: string;
}
export interface Workflow {
  id: string;
  name: string;
  description?: string;
  versions?: WorkflowVersion[];
  createdAt?: string;
  updatedAt?: string;
}
export interface ApprovalDecision {
  id: string;
  actorId: string;
  decision: "approve" | "reject" | "conditional_approve";
  reason?: string;
  conditions?: Record<string, unknown>;
  createdAt: string;
}
export interface WorkflowApproval {
  id: string;
  stepRunId: string;
  status: string;
  policy?: string;
  quorum?: number;
  decisions?: ApprovalDecision[];
}
export interface WorkflowStepRun {
  id: string;
  stepKey: string;
  name?: string;
  type?: WorkflowStepType;
  status: string;
  actorRef?: string;
  attempt?: number;
  error?: string;
  startedAt?: string;
  completedAt?: string;
  approval?: WorkflowApproval;
}
export interface WorkflowExecution {
  id: string;
  versionId: string;
  workflowName?: string;
  runId?: string;
  status: string;
  input?: Record<string, unknown>;
  output?: Record<string, unknown>;
  currentStepKey?: string;
  stepRuns?: WorkflowStepRun[];
  approvals?: WorkflowApproval[];
  startedAt?: string;
  createdAt?: string;
  completedAt?: string;
}
export interface WorkflowAudit {
  id: string;
  action: string;
  stepKey?: string;
  actorId?: string;
  description?: string;
  before?: Record<string, unknown>;
  after?: Record<string, unknown>;
  createdAt: string;
}

export const workflowApi = {
  list: () => apiClient.get("/api/v1/admin/workflows"),
  create: (data: { name: string; description?: string }) =>
    apiClient.post("/api/v1/admin/workflows", data),
  get: (id: string) => apiClient.get(`/api/v1/admin/workflows/${id}`),
  createVersion: (
    id: string,
    data: {
      inputSchema?: Record<string, unknown>;
      outputSchema?: Record<string, unknown>;
      steps: WorkflowStepDefinition[];
      transitions?: WorkflowTransition[];
    },
  ) => apiClient.post(`/api/v1/admin/workflows/${id}/versions`, data),
  publishVersion: (id: string) =>
    apiClient.post(`/api/v1/admin/workflow-versions/${id}/publish`),
  start: (
    id: string,
    data: {
      runId?: string;
      input?: Record<string, unknown>;
      idempotencyKey: string;
    },
  ) => apiClient.post(`/api/v1/admin/workflow-versions/${id}/executions`, data),
  execution: (id: string) =>
    apiClient.get(`/api/v1/admin/workflow-executions/${id}`),
  executions: (workflowId?: string) =>
    apiClient.get("/api/v1/admin/workflow-executions", {
      params: workflowId ? { workflowId } : {},
    }),
  executionAudit: (id: string) =>
    apiClient.get(`/api/v1/admin/workflow-executions/${id}/audit`),
  decideApproval: (
    id: string,
    data: {
      decision: "approve" | "reject" | "conditional_approve";
      reason?: string;
      conditions?: Record<string, unknown>;
      idempotencyKey: string;
    },
  ) => apiClient.post(`/api/v1/admin/workflow-approvals/${id}/decisions`, data),
  processTimeouts: (id: string) =>
    apiClient.post(`/api/v1/admin/workflow-executions/${id}/process-timeouts`),
};
