import { apiRequest } from "./base";

export type McpTransportType = "sse" | "http";

export interface McpTool {
  name: string;
  description: string;
}

export interface Mcp {
  id: number;
  name: string;
  title: string;
  description: string;
  transportType: McpTransportType;
  url?: string;
  hasHeaders: boolean;
  isBuiltin: boolean;
  retryMaxRetries?: number | null;
  retryTimeoutMs?: number | null;
  tools?: McpTool[];
  probeStatus?: string;
  lastProbedAt?: string | null;
  createdAt?: string;
  updatedAt?: string;
}

export interface McpDetail extends Mcp {
  headers: Record<string, string>;
}

export type CreateMcpInput = Omit<
  Mcp,
  "id" | "hasHeaders" | "isBuiltin" | "createdAt" | "updatedAt" | "probeStatus" | "lastProbedAt"
>;
export type UpdateMcpInput = Partial<CreateMcpInput>;

export interface McpProbeResult {
  status: "success" | "failed" | "unsupported";
  tools?: McpTool[];
  error?: string;
}

export interface McpProbeInput {
  transportType: McpTransportType;
  url?: string;
  headers?: Record<string, string>;
}

function maskHeaders(mcp: McpDetail): McpDetail {
  const masked: Record<string, string> = {};
  for (const [k, v] of Object.entries(mcp.headers ?? {})) {
    masked[k] = v ? "<hidden>" : "";
  }
  return { ...mcp, headers: masked };
}

export async function listMcps(): Promise<Mcp[]> {
  return apiRequest<Mcp[]>("/api/v1/admin/mcps");
}

export async function getMcp(name: string): Promise<McpDetail> {
  const mcp = await apiRequest<McpDetail>(
    `/api/v1/admin/mcps/${encodeURIComponent(name)}`,
  );
  return maskHeaders(mcp);
}

export async function createMcp(body: CreateMcpInput): Promise<Mcp> {
  return apiRequest<Mcp>("/api/v1/admin/mcps", { method: "POST", body });
}

export async function updateMcp(
  name: string,
  body: UpdateMcpInput,
): Promise<Mcp> {
  return apiRequest<Mcp>(`/api/v1/admin/mcps/${encodeURIComponent(name)}`, {
    method: "PUT",
    body,
  });
}

export async function deleteMcp(name: string): Promise<void> {
  await apiRequest<void>(`/api/v1/admin/mcps/${encodeURIComponent(name)}`, {
    method: "DELETE",
  });
}

export async function probeMcp(name: string): Promise<McpProbeResult> {
  return apiRequest<McpProbeResult>(
    `/api/v1/admin/mcps/${encodeURIComponent(name)}/probe`,
    { method: "POST" },
  );
}

export async function probeMcpByConfig(input: McpProbeInput): Promise<McpProbeResult> {
  return apiRequest<McpProbeResult>("/api/v1/admin/mcps/probe", {
    method: "POST",
    body: input,
  });
}
