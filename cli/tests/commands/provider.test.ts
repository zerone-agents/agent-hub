import { describe, test, expect, mock, beforeEach } from "bun:test";
import * as realConfig from "../../src/config";

function setupConfigMock() {
  mock.module("../../src/config", () => ({
    ...realConfig,
    getActiveProfile: mock(() =>
      Promise.resolve({ serverUrl: "https://test.local", token: "cli_test" })
    ),
  }));
}

function setupFetchMock(responseData: unknown) {
  const fetchMock = mock(() =>
    Promise.resolve({ success: true, data: responseData })
  );
  mock.module("ofetch", () => ({
    ofetch: fetchMock,
    FetchError: class FetchError extends Error {},
  }));
  return fetchMock;
}

// ── Client-side masking ───────────────────────────────────────

describe("provider list — client-side masking", () => {
  beforeEach(setupConfigMock);

  test("list output never contains plaintext apiKey, even if backend returned it", async () => {
    // Backend bug scenario: plaintext key leaked in response.
    // CLI must defensively mask it.
    const leakyResponse = [
      {
        id: 1,
        name: "Anthropic",
        lockedApiKey: "sk-ant-super-secret-key-12345",
        protocol: "anthropic",
        baseUrl: "https://api.anthropic.com",
      },
    ];
    setupFetchMock(leakyResponse);

    const { listProviders } = await import(
      `../../src/client/provider.ts?t=${Date.now()}`
    );
    const result = await listProviders();

    const serialized = JSON.stringify(result);
    expect(serialized).not.toContain("sk-ant-super-secret-key-12345");
    expect(serialized).toContain("<hidden>");
  });

  test("list with no lockedApiKey field passes through unchanged", async () => {
    const cleanResponse = [
      { id: 2, name: "OpenAI", protocol: "openai", baseUrl: "https://api.openai.com" },
    ];
    setupFetchMock(cleanResponse);

    const { listProviders } = await import(
      `../../src/client/provider.ts?t=${Date.now()}`
    );
    const result = await listProviders();

    expect(result).toHaveLength(1);
    expect(result[0].name).toBe("OpenAI");
    expect(result[0].lockedApiKey).toBeUndefined();
  });
});

// ── List command ──────────────────────────────────────────────

describe("provider list command", () => {
  beforeEach(setupConfigMock);

  test("prints table with id, name, protocol, baseUrl, hasKey", async () => {
    const fakeProviders = [
      {
        id: 1,
        name: "Anthropic",
        protocol: "anthropic",
        baseUrl: "https://api.anthropic.com",
        lockedApiKey: "sk-ant-xxx",
        defaultModels: [{ modelId: "claude-sonnet-4", displayName: "Claude Sonnet 4" }],
      },
      {
        id: 2,
        name: "OpenAI",
        protocol: "openai",
        baseUrl: "https://api.openai.com",
        defaultModels: [],
      },
    ];
    const fetchMock = setupFetchMock(fakeProviders);

    const { ProviderListCommand } = await import(
      `../../src/commands/provider.ts?t=${Date.now()}`
    );
    const cmd = new ProviderListCommand();
    (cmd as any).output = "table";

    const logs: string[] = [];
    const origLog = console.log;
    console.log = (s: string) => logs.push(s);
    try {
      const code = await cmd.execute();
      expect(code).toBe(0);
    } finally {
      console.log = origLog;
    }

    const out = logs.join("\n");
    expect(out).toContain("Anthropic");
    expect(out).toContain("OpenAI");
    expect(out).toContain("anthropic");
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  test("--output json masks apiKey", async () => {
    setupFetchMock([
      { id: 1, name: "Test", lockedApiKey: "sk-leaked", protocol: "openai", baseUrl: "x" },
    ]);

    const { ProviderListCommand } = await import(
      `../../src/commands/provider.ts?t=${Date.now()}`
    );
    const cmd = new ProviderListCommand();
    (cmd as any).output = "json";

    const logs: string[] = [];
    const origLog = console.log;
    console.log = (s: string) => logs.push(s);
    try {
      await cmd.execute();
    } finally {
      console.log = origLog;
    }

    const parsed = JSON.parse(logs[0]);
    expect(parsed.data[0].lockedApiKey).toBe("<hidden>");
    expect(logs[0]).not.toContain("sk-leaked");
  });
});

// ── Get command ───────────────────────────────────────────────

describe("provider get command", () => {
  beforeEach(setupConfigMock);

  test("prints provider details in yaml", async () => {
    const fakeProvider = {
      id: 1,
      name: "Kimi",
      protocol: "openai",
      authStyle: "api_key",
      baseUrl: "https://api.moonshot.cn/v1",
      lockedApiKey: "sk-kimi-secret",
      defaultModels: [
        { modelId: "kimi-for-coding", displayName: "Kimi for Coding" },
      ],
    };
    setupFetchMock(fakeProvider);

    const { ProviderGetCommand } = await import(
      `../../src/commands/provider.ts?t=${Date.now()}`
    );
    const cmd = new ProviderGetCommand();
    (cmd as any).id = "1";
    (cmd as any).output = "yaml";

    const logs: string[] = [];
    const origLog = console.log;
    console.log = (s: string) => logs.push(s);
    try {
      const code = await cmd.execute();
      expect(code).toBe(0);
    } finally {
      console.log = origLog;
    }

    const out = logs.join("\n");
    expect(out).toContain("Kimi");
    expect(out).toContain("kimi-for-coding");
    expect(out).toContain("<hidden>");
    expect(out).not.toContain("sk-kimi-secret");
  });
});

// ── Probe command ─────────────────────────────────────────────

describe("provider probe command", () => {
  beforeEach(setupConfigMock);

  test("probe <id> calls POST /admin/providers/:id/probe", async () => {
    const fetchMock = mock(() =>
      Promise.resolve({
        success: true,
        data: { success: true, latencyMs: 234, statusCode: 200 },
      })
    );
    mock.module("ofetch", () => ({
      ofetch: fetchMock,
      FetchError: class FetchError extends Error {},
    }));

    const { ProviderProbeCommand } = await import(
      `../../src/commands/provider.ts?t=${Date.now()}`
    );
    const cmd = new ProviderProbeCommand();
    (cmd as any).id = "1";
    (cmd as any).baseUrl = undefined;
    (cmd as any).output = "text";

    const logs: string[] = [];
    const origLog = console.log;
    console.log = (s: string) => logs.push(s);
    try {
      const code = await cmd.execute();
      expect(code).toBe(0);
    } finally {
      console.log = origLog;
    }

    const calls = fetchMock.mock.calls as any[][];
    expect(calls[0][0]).toContain("/api/v1/admin/providers/1/probe");
    expect(calls[0][1].method).toBe("POST");
    expect(logs.join("\n")).toContain("连接成功");
    expect(logs.join("\n")).toContain("234");
  });

  test("probe --base-url mode sends custom config body", async () => {
    const fetchMock = mock(() =>
      Promise.resolve({
        success: true,
        data: { success: false, latencyMs: 0, error: "timeout" },
      })
    );
    mock.module("ofetch", () => ({
      ofetch: fetchMock,
      FetchError: class FetchError extends Error {},
    }));

    const { ProviderProbeCommand } = await import(
      `../../src/commands/provider.ts?t=${Date.now()}`
    );
    const cmd = new ProviderProbeCommand();
    (cmd as any).id = undefined;
    (cmd as any).baseUrl = "https://api.anthropic.com";
    (cmd as any).apiKey = "sk-test-key";
    (cmd as any).protocol = "anthropic";
    (cmd as any).authStyle = undefined;
    (cmd as any).output = "json";

    const logs: string[] = [];
    const origLog = console.log;
    console.log = (s: string) => logs.push(s);
    try {
      const code = await cmd.execute();
      expect(code).toBe(1); // failure exit code
    } finally {
      console.log = origLog;
    }

    const calls = fetchMock.mock.calls as any[][];
    expect(calls[0][0]).toContain("/api/v1/admin/providers/probe");
    expect(calls[0][1].method).toBe("POST");
    expect(calls[0][1].body.baseUrl).toBe("https://api.anthropic.com");
    expect(calls[0][1].body.apiKey).toBe("sk-test-key");
    expect(calls[0][1].body.authStyle).toBe("api_key");
  });

  test("probe --base-url without --api-key returns error", async () => {
    const { ProviderProbeCommand } = await import(
      `../../src/commands/provider.ts?t=${Date.now()}`
    );
    const cmd = new ProviderProbeCommand();
    (cmd as any).id = undefined;
    (cmd as any).baseUrl = "https://api.anthropic.com";
    (cmd as any).apiKey = undefined;
    (cmd as any).protocol = "anthropic";

    const origErr = process.stderr.write;
    const errs: string[] = [];
    process.stderr.write = ((s: string) => { errs.push(s); return true; }) as any;

    const code = await cmd.execute();
    expect(code).toBe(2);

    process.stderr.write = origErr as any;
    expect(errs.join("")).toContain("--api-key");
  });

  test("probe with neither id nor --base-url returns error", async () => {
    const { ProviderProbeCommand } = await import(
      `../../src/commands/provider.ts?t=${Date.now()}`
    );
    const cmd = new ProviderProbeCommand();
    (cmd as any).id = undefined;
    (cmd as any).baseUrl = undefined;

    const origErr = process.stderr.write;
    const errs: string[] = [];
    process.stderr.write = ((s: string) => { errs.push(s); return true; }) as any;

    const code = await cmd.execute();
    expect(code).toBe(2);

    process.stderr.write = origErr as any;
  });
});

// ── Delete command ────────────────────────────────────────────

describe("provider delete command", () => {
  beforeEach(setupConfigMock);

  test("sends DELETE /admin/providers/:id", async () => {
    const fetchMock = mock(() => Promise.resolve({ success: true }));
    mock.module("ofetch", () => ({
      ofetch: fetchMock,
      FetchError: class FetchError extends Error {},
    }));

    const { ProviderDeleteCommand } = await import(
      `../../src/commands/provider.ts?t=${Date.now()}`
    );
    const cmd = new ProviderDeleteCommand();
    (cmd as any).id = "3";

    const logs: string[] = [];
    const origLog = console.log;
    console.log = (s: string) => logs.push(s);
    try {
      const code = await cmd.execute();
      expect(code).toBe(0);
    } finally {
      console.log = origLog;
    }

    const calls = fetchMock.mock.calls as any[][];
    expect(calls[0][0]).toContain("/api/v1/admin/providers/3");
    expect(calls[0][1].method).toBe("DELETE");
    expect(logs.join("\n")).toContain("已删除");
  });
});
