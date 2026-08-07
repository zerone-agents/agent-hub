import { apiRequest } from "./base";

export interface Tool {
  id: number;
  name: string;
  title: string;
  description: string;
  isDefault: boolean;
  createdAt?: string;
  updatedAt?: string;
}

export type CreateToolInput = Omit<Tool, "id" | "createdAt" | "updatedAt">;
export type UpdateToolInput = Partial<CreateToolInput>;

export async function listTools(): Promise<Tool[]> {
  return apiRequest<Tool[]>("/api/v1/admin/tools");
}

export async function getTool(name: string): Promise<Tool> {
  return apiRequest<Tool>(`/api/v1/admin/tools/${encodeURIComponent(name)}`);
}

export async function createTool(body: CreateToolInput): Promise<Tool> {
  return apiRequest<Tool>("/api/v1/admin/tools", { method: "POST", body });
}

export async function updateTool(
  name: string,
  body: UpdateToolInput,
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
