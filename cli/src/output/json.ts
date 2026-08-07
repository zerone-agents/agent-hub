export function outputJson(data: unknown, meta: Record<string, unknown> = {}): void {
  console.log(JSON.stringify({ data, meta }, null, 2));
}

export function outputErrorJson(error: { code: string; message: string; details?: unknown }): void {
  console.log(JSON.stringify({ error }, null, 2));
}
