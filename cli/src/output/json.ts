import { writeStdout } from "./stdout";

export function outputJson(data: unknown, meta: Record<string, unknown> = {}): void {
  writeStdout(JSON.stringify({ data, meta }, null, 2) + "\n");
}

export function outputErrorJson(error: { code: string; message: string; details?: unknown }): void {
  writeStdout(JSON.stringify({ error }, null, 2) + "\n");
}
