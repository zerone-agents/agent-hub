import { describe, test, expect, mock } from "bun:test";
import * as realConfig from "../../src/config";

mock.module("../../src/config", () => ({
  ...realConfig,
  getActiveProfile: () => Promise.resolve({ serverUrl: "https://test.local", token: "cli_test" }),
}));

const PAGE = { success: true, data: { items: [], total: 0, page: 1, page_size: 20, total_pages: 0 } };

/**
 * 关键：apiRequest 把筛选条件交给 ofetch 的 `query` 选项（ofetch 内部才拼 URL），
 * 所以断言必须读 init.query —— 读 url 字符串永远拿不到参数。
 * 沿用仓库既有做法（tests/client.test.ts）：先 mock.module("ofetch")，再动态 import 被测模块。
 */
function captureRequests() {
  const seen: { url: string; query: Record<string, unknown> }[] = [];
  mock.module("ofetch", () => ({
    ofetch: (url: string, init?: any) => {
      seen.push({ url, query: (init?.query ?? {}) as Record<string, unknown> });
      return Promise.resolve(PAGE);
    },
    FetchError: class FetchError extends Error {},
  }));
  return seen;
}

describe("chat client query", () => {
  test("只带显式给出的参数；空值与未指定分页一律不发送", async () => {
    const seen = captureRequests();
    const { listChatSessions } = await import("../../src/client/chat");
    await listChatSessions({ agentId: "coder", keyword: "" });
    expect(seen[0].url).toBe("https://test.local/api/v1/admin/chat/sessions");
    expect(seen[0].query).toEqual({ agent_id: "coder" });
  });

  test("camelCase 入参映射为后端 snake_case", async () => {
    const seen = captureRequests();
    const { listChatSessions } = await import("../../src/client/chat");
    await listChatSessions({ messageContains: "body", from: "2026-01-01T00:00:00Z", page: 2, pageSize: 50 });
    expect(seen[0].query).toEqual({
      message_contains: "body", from: "2026-01-01T00:00:00Z", page: 2, page_size: 50,
    });
  });

  test("messages 走各自端点，id 做 URL 编码，未指定分页不发送", async () => {
    const seen = captureRequests();
    const { listChatMessages } = await import("../../src/client/chat");
    await listChatMessages("s 1", {});
    expect(seen[0].url).toBe("https://test.local/api/v1/admin/chat/sessions/s%201/messages");
    expect(seen[0].query).toEqual({});
  });
});