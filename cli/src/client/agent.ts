import { apiRequest } from "./base";

// Shape mirrors AgentDTO from internal/application/services/agent_service.go.
export interface Agent {
  id: number;
  name: string;
  config?: {
    systemPrompt?: string;
    model?: string;
    modelId?: string;
    providerId?: number;
    title?: { zh?: string; en?: string };
    description?: { zh?: string; en?: string };
    permissionMode?: string;
    maxTurns?: number;
    maxSessionTurns?: number;
    iconName?: string;
    iconColor?: string;
    iconBgColor?: string;
  };
  desktopEnabled?: boolean;
  mobileEnabled?: boolean;
  isDefault?: boolean;
  subagents?: string[];
  tools?: string[];
  skills?: string[];
  mcps?: string[];
  contentHash?: string;
  createdAt?: string;
  updatedAt?: string;
}

interface AgentsResponse {
  agents: Agent[];
}

// ── CRUD ──────────────────────────────────────────────────────

// Management command: uses the admin endpoint so agents hidden from all
// client platforms still show up.
export async function listAgents(): Promise<Agent[]> {
  const r = await apiRequest<AgentsResponse>("/api/v1/admin/agents");
  return r?.agents ?? [];
}

export async function getAgent(name: string): Promise<Agent> {
  return apiRequest<Agent>(`/api/v1/agents/${encodeURIComponent(name)}`);
}

export async function createAgent(body: {
  name: string;
  config: Record<string, unknown>;
  desktopEnabled?: boolean;
  mobileEnabled?: boolean;
  isDefault?: boolean;
}): Promise<Agent> {
  return apiRequest<Agent>("/api/v1/admin/agents", { method: "POST", body });
}

export async function updateAgent(
  name: string,
  body: {
    config?: Record<string, unknown>;
    desktopEnabled?: boolean;
    mobileEnabled?: boolean;
    isDefault?: boolean;
  },
): Promise<Agent> {
  return apiRequest<Agent>(`/api/v1/admin/agents/${encodeURIComponent(name)}`, {
    method: "PUT",
    body,
  });
}

export async function deleteAgent(name: string): Promise<void> {
  await apiRequest<void>(`/api/v1/admin/agents/${encodeURIComponent(name)}`, {
    method: "DELETE",
  });
}

// ── Relations ─────────────────────────────────────────────────

export async function setAgentSubagents(name: string, subagents: string[]): Promise<void> {
  await apiRequest<void>(`/api/v1/admin/agents/${encodeURIComponent(name)}/subagents`, {
    method: "PUT",
    body: { subagents },
  });
}

export async function setAgentTools(name: string, toolNames: string[]): Promise<void> {
  await apiRequest<void>(`/api/v1/admin/agents/${encodeURIComponent(name)}/tools`, {
    method: "PUT",
    body: { toolNames },
  });
}

export async function setAgentSkills(name: string, skillNames: string[]): Promise<void> {
  await apiRequest<void>(`/api/v1/admin/agents/${encodeURIComponent(name)}/skills`, {
    method: "PUT",
    body: { skillNames },
  });
}

export async function setAgentMcps(name: string, mcpNames: string[]): Promise<void> {
  await apiRequest<void>(`/api/v1/admin/agents/${encodeURIComponent(name)}/mcps`, {
    method: "PUT",
    body: { mcpNames },
  });
}

// ── Deploy lifecycle ──────────────────────────────────────────

export interface DeploymentInfo {
  status: string;
  health?: string;
  runtimeUrl?: string;
  apiKey?: string;
  containerName?: string;
  deployedAt?: string;
  hostPort?: number;
  message?: string;
}

export async function deployAgent(name: string, force = false): Promise<DeploymentInfo> {
  return apiRequest<DeploymentInfo>(
    `/api/v1/admin/agents/${encodeURIComponent(name)}/deploy`,
    { method: "POST", query: force ? { force: "true" } : undefined },
  );
}

export async function undeployAgent(name: string, purge = false): Promise<void> {
  await apiRequest<void>(
    `/api/v1/admin/agents/${encodeURIComponent(name)}/deploy`,
    { method: "DELETE", query: purge ? { purge: "true" } : undefined },
  );
}

export async function startAgent(name: string): Promise<DeploymentInfo> {
  return apiRequest<DeploymentInfo>(
    `/api/v1/admin/agents/${encodeURIComponent(name)}/deploy/start`,
    { method: "POST" },
  );
}

export async function stopAgent(name: string): Promise<void> {
  await apiRequest<void>(
    `/api/v1/admin/agents/${encodeURIComponent(name)}/deploy/stop`,
    { method: "POST" },
  );
}

export async function getDeploymentStatus(name: string): Promise<DeploymentInfo> {
  return apiRequest<DeploymentInfo>(
    `/api/v1/admin/agents/${encodeURIComponent(name)}/deploy`,
  );
}
