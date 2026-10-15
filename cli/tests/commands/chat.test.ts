import { describe, test, expect, mock, beforeEach } from "bun:test";
import { Cli } from "clipanion";
import * as realConfig from "../../src/config";
import { ChatListCommand, ChatGetCommand, ChatMessagesCommand } from "../../src/commands/chat";

mock.module("../../src/config", () => ({
  ...realConfig,
  getActiveProfile: () => Promise.resolve({ serverUrl: "https://test.local", token: "cli_test" }),
}));

// 断言读 init.query（ofetch 的查询选项）而非 url —— 与 Task 4 同一原理。
// 另外：url 断言必须带 test profile 的 serverUrl 前缀（apiRequest 拼的是
// `${profile.serverUrl}${path}`，见 client/base.ts:18），写裸路径会假红。
const seen: { url: string; query: Record<string, unknown> }[] = [];
mock.module("ofetch", () => ({
  ofetch: (url: string, init?: any) => {
    seen.push({ url, query: (init?.query ?? {}) as Record<string, unknown> });
    return Promise.resolve({
      success: true,
      data: {
        items: [{ id: "s1", user_id: "u1", title: "产品讨论", agent_id: "coder", updated_at: "2026-01-01T01:00:00Z" }],
        total: 87, page: 1, page_size: 20, total_pages: 5,
      },
    });
  },
  FetchError: class FetchError extends Error {},
}));

// 惯例对齐 tool.test.ts：Cli.from 工厂注册（clipanion 的 register 返回 void，
// 不能链式调用；run() 直接返回退出码数字，不是对象）。
function makeCli() {
  return Cli.from([ChatListCommand, ChatGetCommand, ChatMessagesCommand]);
}

describe("chat commands", () => {
  beforeEach(() => { seen.length = 0; });

  test("--text 单独使用仍发请求（是否需搭配收窄条件只有服务端知道）", async () => {
    const code = await makeCli().run(["chat", "list", "--text", "body", "--output", "json"]);
    expect(code).toBe(0);
    expect(seen.length).toBe(1);
    expect(seen[0].query).toEqual({ message_contains: "body" });
  });

  test("--from > --to 本地拒绝且不发请求", async () => {
    const code = await makeCli().run([
      "chat", "list", "--from", "2026-05-01T00:00:00Z", "--to", "2026-04-01T00:00:00Z",
    ]);
    expect(code).toBe(2);
    expect(seen.length).toBe(0);
  });

  test("--text 长度 <2 本地拒绝且不发请求", async () => {
    const code = await makeCli().run(["chat", "list", "--text", "a"]);
    expect(code).toBe(2);
    expect(seen.length).toBe(0);
  });

  test("未传分页参数时不发送 page/page_size（默认口径只在服务端）", async () => {
    await makeCli().run(["chat", "list", "--agent", "coder", "--output", "json"]);
    expect(seen[0].query).toEqual({ agent_id: "coder" });
  });

  test("chat messages / chat get 打各自端点", async () => {
    await makeCli().run(["chat", "messages", "s1", "--output", "json"]);
    expect(seen[0].url).toBe("https://test.local/api/v1/admin/chat/sessions/s1/messages");
    await makeCli().run(["chat", "get", "s1", "--output", "json"]);
    expect(seen[1].url).toBe("https://test.local/api/v1/admin/chat/sessions/s1");
  });
});