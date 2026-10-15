import { describe, test, expect, mock, beforeEach, beforeAll, afterAll } from "bun:test";
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
// 三形态断言（spec §7.10 / 评审重要 2）：按仓库惯例截获——tests/preload.ts 已把
// writeStdout 路由回 console.log（见 bunfig.toml），所以 stdout 内容截 console.log、
// 分页提示截 process.stderr.write。不要 mock 输出层模块：mock.module 是进程级的，
// 会污染同进程其它测试文件的输出路径（skill.test.ts 曾因此假红）。
const stderrLines: string[] = [];
const origStderrWrite = process.stderr.write.bind(process.stderr);
beforeAll(() => {
  process.stderr.write = ((chunk: unknown) => {
    stderrLines.push(String(chunk));
    return true;
  }) as typeof process.stderr.write;
});
afterAll(() => {
  process.stderr.write = origStderrWrite;
});

/** 与 skill.test.ts 同款：临时替换 console.log 截获 stdout 内容（preload 路由）。 */
async function captureStdout(run: () => Promise<number>): Promise<string> {
  const logs: string[] = [];
  const origLog = console.log;
  console.log = (s: string) => logs.push(s);
  try {
    await run();
  } finally {
    console.log = origLog;
  }
  return logs.join("\n");
}

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

describe("output 三形态（spec §7.10）", () => {
  beforeEach(() => {
    seen.length = 0;
    stderrLines.length = 0;
    // Bun 的 mock.module 是进程级的：table 用例里为「id 不截断」重 stub 过长 id 数据，
    // 每个用例开始前先恢复默认 s1 数据，避免泄漏到后续用例
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
  });

  test("json：meta 携带分页字段（page/pageSize/total/totalPages）", async () => {
    const out = await captureStdout(() => makeCli().run(["chat", "list", "--output", "json"]));
    const body = JSON.parse(out) as { data: unknown[]; meta: Record<string, number> };
    expect(body.meta).toEqual({ page: 1, pageSize: 20, total: 87, totalPages: 5 });
    expect(Array.isArray(body.data)).toBe(true);
  });

  test("table：id 不截断（长 id 全量呈现）、列含 hidden、分页提示走 stderr", async () => {
    const longId = "s-550e8400-e29b-41d4-a716-446655440000";
    mock.module("ofetch", () => ({
      ofetch: (url: string, init?: any) => {
        seen.push({ url, query: (init?.query ?? {}) as Record<string, unknown> });
        return Promise.resolve({
          success: true,
          data: {
            items: [{ id: longId, user_id: "u1", title: "产品讨论", agent_id: "coder", updated_at: "2026-01-01T01:00:00Z" }],
            total: 87, page: 1, page_size: 20, total_pages: 5,
          },
        });
      },
      FetchError: class FetchError extends Error {},
    }));
    const out = await captureStdout(() => makeCli().run(["chat", "list"]));
    // 截断的 id 无法喂 zhub chat get（评审重要 4）：全量 id 必须完整出现在表格里
    expect(out).toContain(longId);
    expect(out).toContain("hidden");
    expect(stderrLines.some((l) => l.includes("第 1/5 页") && l.includes("共 87 条"))).toBe(true);
    expect(out).not.toContain("第 1/5 页"); // 分页提示只走 stderr，stdout 保持可管道
  });

  test("yaml：原样透传 items（不做列级加工）", async () => {
    const out = await captureStdout(() => makeCli().run(["chat", "list", "--output", "yaml"]));
    expect(out).toContain("s1");
    expect(out).toContain("产品讨论");
  });

  test("--text 空串按未传（不报错、不发 message_contains）", async () => {
    const code = await makeCli().run(["chat", "list", "--text", "", "--output", "json"]);
    expect(code).toBe(0);
    expect(seen[0].query).toEqual({});
  });
});
