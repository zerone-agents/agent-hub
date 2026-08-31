import { ofetch } from "ofetch";
import { getActiveProfile } from "../config";
import { exitFromHttpStatus, EXIT } from "../output/error";
import { apiRequest } from "./base";

export interface Tool {
  id: number;
  name: string;
  title: string;
  description: string;
  isDefault: boolean;
  source?: string;
  artifactStatus?: string;
  fileName?: string;
  fileHash?: string;
  fileSize?: number;
  createdAt?: string;
  updatedAt?: string;
}

export interface CustomToolInput {
  name: string;
  title?: string;
  description?: string;
  fileBuffer: Buffer;
  fileName: string;
}

export interface DownloadResult {
  url: string;
  expiresIn: number;
}

export async function listTools(): Promise<Tool[]> {
  return apiRequest<Tool[]>("/api/v1/admin/tools");
}

export async function getTool(name: string): Promise<Tool> {
  return apiRequest<Tool>(`/api/v1/admin/tools/${encodeURIComponent(name)}`);
}

// updateTool 仅更新展示元数据（title/description）；isDefault 已随 issue #88 移除。
export async function updateTool(
  name: string,
  body: { title?: string; description?: string },
): Promise<Tool> {
  return apiRequest<Tool>(`/api/v1/admin/tools/${encodeURIComponent(name)}`, {
    method: "PUT",
    body,
  });
}

export async function deleteTool(name: string): Promise<void> {
  await apiRequest<void>(`/api/v1/admin/tools/${encodeURIComponent(name)}`, {
    method: "DELETE",
  });
}

export async function createTool(input: CustomToolInput): Promise<Tool> {
  return uploadToolPackage("/api/v1/admin/tools", input, false);
}

export async function uploadToolFile(
  name: string,
  input: Omit<CustomToolInput, "name">,
): Promise<Tool> {
  return uploadToolPackage(
    `/api/v1/admin/tools/${encodeURIComponent(name)}/file`,
    { ...input, name },
    true,
  );
}

export async function downloadTool(name: string): Promise<DownloadResult> {
  return apiRequest<DownloadResult>(
    `/api/v1/admin/tools/${encodeURIComponent(name)}/download`,
  );
}

// multipart 上传（模式同 client/skill.ts 的 uploadPackage：ofetch 直传 FormData，
// base.ts 的 apiRequest 假设 JSON body 不适用）。
async function uploadToolPackage(
  url: string,
  input: CustomToolInput,
  isUpdate: boolean,
): Promise<Tool> {
  const profile = await getActiveProfile();
  const fullUrl = `${profile.serverUrl}${url}`;

  const formData = new FormData();
  if (!isUpdate) formData.append("name", input.name);
  if (input.title) formData.append("title", input.title);
  if (input.description) formData.append("description", input.description);

  const blob = new Blob([new Uint8Array(input.fileBuffer)]);
  formData.append("file", blob, input.fileName);

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
      return (response as unknown as { data: Tool }).data;
    }
    return response as Tool;
  } catch (e: any) {
    const status = e?.response?.status ?? 0;
    const msg = e?.data?.error ?? e?.message ?? "上传失败";
    process.stderr.write(`错误：${msg}\n`);
    process.exit(exitFromHttpStatus(status) || EXIT.SERVER_ERROR);
  }
}
