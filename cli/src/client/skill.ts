import { ofetch } from "ofetch";
import { getActiveProfile } from "../config";
import { exitFromHttpStatus, EXIT } from "../output/error";

export interface Skill {
  id: number;
  name: string;
  type?: string;
  title?: string;
  titleEn?: string;
  description?: string;
  descriptionEn?: string;
  url?: string;
  fileHash?: string;
  fileSize?: number;
  createdAt?: string;
  updatedAt?: string;
}

export interface DownloadResult {
  url: string;
  expiresIn: number;
}

// ── Read ──────────────────────────────────────────────────────

export async function listSkills(type?: string): Promise<Skill[]> {
  const query = type ? { type } : undefined;
  const data = await skillRequest<Skill[]>("/api/v1/skills", { query });
  return data ?? [];
}

export async function getSkill(name: string): Promise<Skill> {
  return skillRequest<Skill>(`/api/v1/skills/${encodeURIComponent(name)}`);
}

export async function downloadSkill(name: string): Promise<DownloadResult> {
  return skillRequest<DownloadResult>(`/api/v1/skills/${encodeURIComponent(name)}/download`);
}

// ── Delete ────────────────────────────────────────────────────

export async function deleteSkill(name: string): Promise<void> {
  await skillRequest<void>(`/api/v1/admin/skills/${encodeURIComponent(name)}`, {
    method: "DELETE",
  });
}

// ── Create (multipart upload) ─────────────────────────────────

export interface CreateSkillInput {
  name: string;
  title?: string;
  titleEn?: string;
  description?: string;
  descriptionEn?: string;
  type?: string;
  zipBuffer: Buffer;
  fileName?: string;
}

export async function createSkill(input: CreateSkillInput): Promise<Skill> {
  return uploadPackage("/api/v1/admin/skills", input);
}

export async function updateSkill(name: string, input: Omit<CreateSkillInput, "name">): Promise<Skill> {
  return uploadPackage(`/api/v1/admin/skills/${encodeURIComponent(name)}`, {
    ...input,
    name,
  }, true);
}

/**
 * Upload a zip package via multipart form data.
 * Uses ofetch directly (not the JSON-based skillRequest) because the
 * standard apiRequest in base.ts assumes JSON bodies.
 */
async function uploadPackage(
  url: string,
  input: CreateSkillInput,
  isUpdate = false,
): Promise<Skill> {
  const profile = await getActiveProfile();
  const fullUrl = `${profile.serverUrl}${url}`;

  const formData = new FormData();
  if (!isUpdate) {
    formData.append("name", input.name);
  }
  if (input.title) formData.append("title", input.title);
  if (input.titleEn) formData.append("titleEn", input.titleEn);
  if (input.description) formData.append("description", input.description);
  if (input.descriptionEn) formData.append("descriptionEn", input.descriptionEn);
  if (input.type) formData.append("type", input.type);

  const blob = new Blob([new Uint8Array(input.zipBuffer)], { type: "application/zip" });
  formData.append("file", blob, input.fileName ?? `${input.name}.zip`);

  try {
    const response = await ofetch<unknown>(fullUrl, {
      method: isUpdate ? "PUT" : "POST",
      body: formData,
      headers: { Authorization: `Bearer ${profile.token}` },
      retry: 1,
      timeout: 120000,
    });

    if (
      response &&
      typeof response === "object" &&
      "success" in response &&
      "data" in (response as Record<string, unknown>)
    ) {
      return (response as unknown as { data: Skill }).data;
    }
    return response as Skill;
  } catch (e: any) {
    const status = e?.response?.status ?? 0;
    const msg = e?.data?.error ?? e?.message ?? "上传失败";
    process.stderr.write(`错误：${msg}\n`);
    process.exit(exitFromHttpStatus(status) || EXIT.SERVER_ERROR);
  }
}

// ── Base request helper (JSON) ────────────────────────────────

interface RequestOptions {
  method?: "GET" | "POST" | "PUT" | "DELETE";
  query?: Record<string, string | undefined>;
}

async function skillRequest<T>(path: string, opts: RequestOptions = {}): Promise<T> {
  const profile = await getActiveProfile();
  const url = `${profile.serverUrl}${path}`;

  try {
    const response = await ofetch<unknown>(url, {
      method: opts.method ?? "GET",
      query: opts.query,
      headers: { Authorization: `Bearer ${profile.token}` },
      retry: 1,
      timeout: 60000,
    });

    if (
      response &&
      typeof response === "object" &&
      "success" in response &&
      "data" in (response as Record<string, unknown>)
    ) {
      return (response as unknown as { data: T }).data;
    }
    return response as T;
  } catch (e: any) {
    if (e?.response) {
      const status = e.response.status ?? 0;
      const msg = e.data?.error ?? e.message;
      process.stderr.write(`错误：${msg}\n`);
      process.exit(exitFromHttpStatus(status));
    }
    process.stderr.write(`无法连接到 ${profile.serverUrl}\n`);
    process.exit(EXIT.NETWORK_ERROR);
  }
}
