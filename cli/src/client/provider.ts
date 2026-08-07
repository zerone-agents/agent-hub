import { apiRequest } from "./base";

export interface CatalogModel {
  modelId: string;
  displayName: string;
  contextWindow?: number;
}

export interface Provider {
  id: number;
  key?: string;
  name: string;
  description?: string;
  descriptionEn?: string;
  protocol: string;
  authStyle: string;
  baseUrl: string;
  defaultModels?: CatalogModel[];
  fields?: Array<{ key: string; label?: string; secret?: boolean }>;
  iconKey?: string;
  builtin?: boolean;
  /** Always masked in CLI output, never exposed as plaintext */
  lockedApiKey?: string;
  createdAt?: string;
  updatedAt?: string;
}

export interface ProbeResult {
  success: boolean;
  latencyMs: number;
  statusCode: number;
  error?: string;
}

// ── Client-side masking (defense-in-depth) ────────────────────
// Even if the backend ever leaks a plaintext key, the CLI strips it
// before any output format (table / json / yaml).
function mask(p: Provider): Provider {
  if (!p.lockedApiKey) return p;
  return { ...p, lockedApiKey: p.lockedApiKey ? "<hidden>" : "" };
}

// ── Read ──────────────────────────────────────────────────────

export async function listProviders(): Promise<Provider[]> {
  const list = await apiRequest<Provider[]>("/api/v1/providers");
  return (list ?? []).map(mask);
}

export async function getProvider(id: number): Promise<Provider> {
  const p = await apiRequest<Provider>(`/api/v1/providers/${id}`);
  return mask(p);
}

// ── Write ─────────────────────────────────────────────────────

export async function createProvider(body: Record<string, unknown>): Promise<Provider> {
  const p = await apiRequest<Provider>("/api/v1/admin/providers", {
    method: "POST",
    body,
  });
  return mask(p);
}

export async function updateProvider(id: number, body: Record<string, unknown>): Promise<Provider> {
  const p = await apiRequest<Provider>(`/api/v1/admin/providers/${id}`, {
    method: "PUT",
    body,
  });
  return mask(p);
}

export async function deleteProvider(id: number): Promise<void> {
  await apiRequest<void>(`/api/v1/admin/providers/${id}`, { method: "DELETE" });
}

// ── Probe ─────────────────────────────────────────────────────

/** Probe a stored provider by ID (backend uses the stored key). */
export async function probeProvider(id: number): Promise<ProbeResult> {
  return apiRequest<ProbeResult>(`/api/v1/admin/providers/${id}/probe`, {
    method: "POST",
  });
}

export interface ProbeConfigInput {
  baseUrl: string;
  apiKey: string;
  protocol: string;
  authStyle: string;
  models?: CatalogModel[];
}

/** Probe a custom config without a stored provider. */
export async function probeProviderConfig(cfg: ProbeConfigInput): Promise<ProbeResult> {
  return apiRequest<ProbeResult>("/api/v1/admin/providers/probe", {
    method: "POST",
    body: cfg,
  });
}
