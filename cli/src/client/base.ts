import { ofetch, FetchError } from "ofetch";
import { getActiveProfile } from "../config";
import { exitFromHttpStatus, EXIT } from "../output/error";

export interface RequestOptions {
  method?: "GET" | "POST" | "PUT" | "DELETE";
  body?: unknown;
  query?: Record<string, string | number | boolean | undefined>;
  // 用于绕过 profile（如 login 命令本身）
  serverUrl?: string;
  token?: string;
}

export async function apiRequest<T = unknown>(path: string, opts: RequestOptions = {}): Promise<T> {
  const profile = opts.serverUrl && opts.token
    ? { serverUrl: opts.serverUrl, token: opts.token }
    : await getActiveProfile();
  const url = `${profile.serverUrl}${path}`;
  try {
    const response = await ofetch<unknown>(url, {
      method: opts.method ?? "GET",
      body: opts.body as BodyInit | Record<string, any> | null | undefined,
      query: opts.query,
      headers: { Authorization: `Bearer ${profile.token}` },
      retry: 1,
      timeout: 60000,
    });
    // Auto-unwrap the standard backend envelope `{ success: true, data: X }`.
    // Responses without this shape (e.g. raw payloads) are returned as-is.
    if (
      response &&
      typeof response === "object" &&
      "success" in response &&
      "data" in (response as Record<string, unknown>)
    ) {
      return (response as unknown as { data: T }).data;
    }
    return response as T;
  } catch (e) {
    if (e instanceof FetchError) {
      const status = e.response?.status ?? 0;
      const msg = (e.data as { error?: string })?.error ?? e.message;
      process.stderr.write(`错误：${msg}\n`);
      process.exit(exitFromHttpStatus(status));
    }
    process.stderr.write(`无法连接到 ${profile.serverUrl}，检查网络或 server_url 配置\n`);
    process.exit(EXIT.NETWORK_ERROR);
  }
}
