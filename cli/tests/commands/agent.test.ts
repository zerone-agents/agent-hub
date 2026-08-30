import { describe, test, expect, mock, beforeEach } from "bun:test";
import { writeFileSync, mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

// CRITICAL: Bun mock pattern (learned from T2/T3)
// - mock.module replaces ENTIRE namespace, must spread real exports
// - mock().mock.calls is the call log (Jest shape, NOT .mockCalls)
// - For dynamic re-imports after mock setup, use ?t=${Date.now()} cache-busting

import * as realConfig from "../../src/config";
import { resolveRuntimeUrl } from "../../src/commands/agent";

// Real backend response shapes (verified against
// internal/application/services/agent_service.go + internal/handler/agent.go):
//
// GET /api/v1/admin/agents  -> { success: true, data: { agents: AgentDTO[] } }
// GET /api/v1/agents/:name  -> { success: true, data: AgentDTO }
//
// AgentDTO = {
//   id, name,
//   config: { systemPrompt, modelId, providerId, title, description, ... },
//   desktopEnabled, mobileEnabled, isDefault,
//   subagents, tools, skills, mcps,
//   createdAt, updatedAt
// }
//
// Note: platform flags are top-level (NOT config.desktopEnabled), and there is no
// `deploymentStatus` field on the agent DTO (deployment state lives behind
// a separate admin-only endpoint, deferred to T9).

describe("agent list command", () => {
  beforeEach(() => {
    mock.module("../../src/config", () => ({
      ...realConfig,
      getActiveProfile: mock(() =>
        Promise.resolve({ serverUrl: "https://test.local", token: "cli_test" })
      ),
    }));
  });

  test("list calls GET /api/v1/admin/agents and prints JSON with --output json", async () => {
    const fakeAgents = [
      {
        id: 1,
        name: "researcher",
        config: { modelId: "claude-sonnet-4-5", title: { zh: "研究员" } },
        desktopEnabled: true,
      },
      {
        id: 2,
        name: "writer",
        config: { modelId: "glm-4.6", title: { zh: "写手" } },
        desktopEnabled: false,
      },
    ];

    mock.module("ofetch", () => ({
      ofetch: mock(() =>
        Promise.resolve({ success: true, data: { agents: fakeAgents } })
      ),
      FetchError: class FetchError extends Error {},
    }));

    const { AgentListCommand } = await import(
      `../../src/commands/agent.ts?t=${Date.now()}`
    );
    const cmd = new AgentListCommand();
    // clipanion Option.Boolean descriptors don't resolve without runExit,
    // so set every flag explicitly.
    (cmd as any).desktop = false;
    (cmd as any).mobile = false;
    (cmd as any).output = "json";

    const logs: string[] = [];
    const origLog = console.log;
    console.log = (s: string) => logs.push(s);
    try {
      await cmd.execute();
    } finally {
      console.log = origLog;
    }

    expect(logs.length).toBe(1);
    const parsed = JSON.parse(logs[0]);
    expect(parsed.data).toHaveLength(2);
    expect(parsed.data[0].name).toBe("researcher");
  });

  test("list --desktop filters to desktop-enabled agents", async () => {
    const fakeAgents = [
      { name: "active", config: { title: { zh: "活跃" } }, desktopEnabled: true },
      { name: "inactive", config: { title: { zh: "隐藏" } }, desktopEnabled: false },
    ];

    mock.module("ofetch", () => ({
      ofetch: mock(() =>
        Promise.resolve({ success: true, data: { agents: fakeAgents } })
      ),
      FetchError: class FetchError extends Error {},
    }));

    const { AgentListCommand } = await import(
      `../../src/commands/agent.ts?t=${Date.now()}`
    );
    const cmd = new AgentListCommand();
    (cmd as any).desktop = true;
    (cmd as any).mobile = false;
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
    expect(parsed.data).toHaveLength(1);
    expect(parsed.data[0].name).toBe("active");
  });
});

describe("agent get command", () => {
  beforeEach(() => {
    mock.module("../../src/config", () => ({
      ...realConfig,
      getActiveProfile: mock(() =>
        Promise.resolve({ serverUrl: "https://test.local", token: "cli_test" })
      ),
    }));
  });

  test("get calls GET /api/v1/agents/:name and prints yaml by default", async () => {
    const fakeAgent = {
      id: 1,
      name: "researcher",
      config: {
        systemPrompt: "你是研究员",
        modelId: "claude-sonnet-4-5",
        title: { zh: "研究员" },
        maxTurns: 15,
      },
      enabled: true,
    };

    mock.module("ofetch", () => ({
      ofetch: mock(() => Promise.resolve({ success: true, data: fakeAgent })),
      FetchError: class FetchError extends Error {},
    }));

    const { AgentGetCommand } = await import(
      `../../src/commands/agent.ts?t=${Date.now()}`
    );
    const cmd = new AgentGetCommand();
    (cmd as any).name = "researcher";
    (cmd as any).output = "yaml";

    const logs: string[] = [];
    const origLog = console.log;
    console.log = (s: string) => logs.push(s);
    try {
      await cmd.execute();
    } finally {
      console.log = origLog;
    }

    expect(logs.join("\n")).toContain("researcher");
    expect(logs.join("\n")).toContain("claude-sonnet-4-5");
  });
});

// ── Shared mock setup for write commands ──────────────────────

function setupWriteMocks(responseData: unknown) {
  mock.module("../../src/config", () => ({
    ...realConfig,
    getActiveProfile: mock(() =>
      Promise.resolve({ serverUrl: "https://test.local", token: "cli_test" })
    ),
  }));
  const fetchMock = mock(() =>
    Promise.resolve({ success: true, data: responseData })
  );
  mock.module("ofetch", () => ({
    ofetch: fetchMock,
    FetchError: class FetchError extends Error {},
  }));
  return fetchMock;
}

function writeTempYaml(content: string): string {
  const dir = mkdtempSync(join(tmpdir(), "zhub-test-"));
  const filePath = join(dir, "agent.yaml");
  writeFileSync(filePath, content, "utf-8");
  return filePath;
}

// ── Create ────────────────────────────────────────────────────

describe("agent create command", () => {
  beforeEach(() => {
    mock.module("../../src/config", () => ({
      ...realConfig,
      getActiveProfile: mock(() =>
        Promise.resolve({ serverUrl: "https://test.local", token: "cli_test" })
      ),
    }));
  });

  test("create sends POST with YAML-parsed body", async () => {
    const yamlPath = writeTempYaml(`
id: coder
name: 程序员
model: claude-sonnet-4-5
systemPrompt: 你是一个程序员
maxTurns: 20
`);
    const fakeAgent = { id: 10, name: "coder", config: { title: { zh: "程序员" } }, enabled: true };
    const fetchMock = setupWriteMocks(fakeAgent);

    const { AgentCreateCommand } = await import(
      `../../src/commands/agent.ts?t=${Date.now()}`
    );
    const cmd = new AgentCreateCommand();
    (cmd as any).file = yamlPath;
    (cmd as any).output = "yaml";
    (cmd as any).enabled = true;
    (cmd as any).disabled = undefined;
    (cmd as any).default = false;

    const logs: string[] = [];
    const origLog = console.log;
    console.log = (s: string) => logs.push(s);
    try {
      const code = await cmd.execute();
      expect(code).toBe(0);
    } finally {
      console.log = origLog;
    }

    // Verify the API call
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const calls = fetchMock.mock.calls as any[][];
    const callArgs = calls[0][1];
    expect(callArgs.method).toBe("POST");
    expect(callArgs.body.name).toBe("coder");
    expect(callArgs.body.config.title.zh).toBe("程序员");
    expect(callArgs.body.config.modelId).toBe("claude-sonnet-4-5");
    expect(callArgs.body.config.systemPrompt).toBe("你是一个程序员");
    expect(callArgs.body.config.maxTurns).toBe(20);

    rmSync(join(yamlPath, ".."), { recursive: true });
  });

  test("create returns error when --file is missing", async () => {
    const { AgentCreateCommand } = await import(
      `../../src/commands/agent.ts?t=${Date.now()}`
    );
    const cmd = new AgentCreateCommand();
    (cmd as any).file = undefined;

    const origErr = console.error;
    const errs: string[] = [];
    process.stderr.write = ((s: string) => { errs.push(s); }) as any;

    const code = await cmd.execute();
    expect(code).toBe(1);

    process.stderr.write = origErr as any;
  });

});

describe("agent state metadata", () => {
  const stateHarness = join(import.meta.dir, "../fixtures/agent-create-state-harness.ts");

  beforeEach(() => {
    mock.module("../../src/config", () => ({
      ...realConfig,
      getActiveProfile: mock(() =>
        Promise.resolve({ serverUrl: "https://test.local", token: "cli_test" })
      ),
    }));
  });

  test.each([
    ["file values", false, true, undefined, undefined, false, true],
    ["positive CLI overrides", false, false, true, true, true, true],
    ["negative CLI overrides", true, true, false, false, false, false],
  ])(
    "create resolves %s",
    async (_label, fileDesktop, fileDefault, desktop, isDefault, expectedDesktop, expectedDefault) => {
      const yamlPath = writeTempYaml(`id: state-agent\ndesktop: ${fileDesktop}\nisDefault: ${fileDefault}\n`);
      const fetchMock = setupWriteMocks({ id: 1, name: "state-agent" });
      const { AgentCreateCommand } = await import(
        `../../src/commands/agent.ts?state=${Date.now()}-${Math.random()}`
      );
      const cmd = new AgentCreateCommand();
      (cmd as any).file = yamlPath;
      (cmd as any).output = "yaml";
      (cmd as any).desktop = desktop;
      (cmd as any).mobile = undefined;
      (cmd as any).default = isDefault;

      const origLog = console.log;
      console.log = () => {};
      try {
        expect(await cmd.execute()).toBe(0);
      } finally {
        console.log = origLog;
        rmSync(join(yamlPath, ".."), { recursive: true });
      }

      const body = (fetchMock.mock.calls as any[][])[0][1].body;
      expect(body.desktopEnabled).toBe(expectedDesktop);
      expect(body.isDefault).toBe(expectedDefault);
    },
  );

  test("create defaults platform flags off when state is omitted", async () => {
    const yamlPath = writeTempYaml("id: state-agent\n");
    const fetchMock = setupWriteMocks({ id: 1, name: "state-agent" });
    const { AgentCreateCommand } = await import(
      `../../src/commands/agent.ts?state=${Date.now()}-${Math.random()}`
    );
    const cmd = new AgentCreateCommand();
    (cmd as any).file = yamlPath;
    (cmd as any).output = "yaml";
    (cmd as any).desktop = undefined;
    (cmd as any).mobile = undefined;
    (cmd as any).default = undefined;

    const origLog = console.log;
    console.log = () => {};
    try {
      expect(await cmd.execute()).toBe(0);
    } finally {
      console.log = origLog;
      rmSync(join(yamlPath, ".."), { recursive: true });
    }

    expect((fetchMock.mock.calls as any[][])[0][1].body).toMatchObject({
      desktopEnabled: false,
      mobileEnabled: false,
      isDefault: false,
    });
  });

  test.each([
    [["--no-default"], false],
    [["--default", "--no-default"], false],
    [["--no-default", "--default"], true],
  ])("process parsing resolves default flags %j with last argument winning", (args, expected) => {
    const result = Bun.spawnSync([process.execPath, stateHarness, ...args], {
      cwd: join(import.meta.dir, "../.."),
      stdout: "pipe",
      stderr: "pipe",
    });
    expect(result.exitCode).toBe(0);
    expect(result.stderr.toString()).not.toContain("Ambiguous Syntax Error");
    const observed = JSON.parse(result.stdout.toString()) as {
      code: number;
      requestBody?: { isDefault?: boolean };
      fetchCalls: number;
    };
    expect(observed.code).toBe(0);
    expect(observed.fetchCalls).toBe(1);
    expect(observed.requestBody?.isDefault).toBe(expected);
  });

  test.each([
    ["forwards present state", "desktop: false\nmobile: true\nisDefault: true\n", { desktopEnabled: false, mobileEnabled: true, isDefault: true }],
    ["omits absent state", "", {}],
  ])("update %s", async (_label, stateYaml, expectedState) => {
    const yamlPath = writeTempYaml(`id: state-agent\n${stateYaml}`);
    const fetchMock = setupWriteMocks({ id: 1, name: "state-agent" });
    const { AgentUpdateCommand } = await import(
      `../../src/commands/agent.ts?state=${Date.now()}-${Math.random()}`
    );
    const cmd = new AgentUpdateCommand();
    (cmd as any).name = "state-agent";
    (cmd as any).file = yamlPath;
    (cmd as any).output = "yaml";

    const origLog = console.log;
    console.log = () => {};
    try {
      expect(await cmd.execute()).toBe(0);
    } finally {
      console.log = origLog;
      rmSync(join(yamlPath, ".."), { recursive: true });
    }

    const body = (fetchMock.mock.calls as any[][])[0][1].body;
    expect(body).toEqual({ config: {}, ...expectedState });
    if (stateYaml === "") {
      expect(Object.hasOwn(body, "desktopEnabled")).toBe(false);
      expect(Object.hasOwn(body, "mobileEnabled")).toBe(false);
      expect(Object.hasOwn(body, "isDefault")).toBe(false);
    }
  });

  test("update accepts YAML without id and sends one request for the positional agent", async () => {
    const yamlPath = writeTempYaml("config:\n  systemPrompt: updated prompt\n");
    const fetchMock = setupWriteMocks({ id: 1, name: "state-agent" });
    const { AgentUpdateCommand } = await import(
      `../../src/commands/agent.ts?state=${Date.now()}-${Math.random()}`
    );
    const cmd = new AgentUpdateCommand();
    (cmd as any).name = "state-agent";
    (cmd as any).file = yamlPath;
    (cmd as any).output = "yaml";

    const origLog = console.log;
    console.log = () => {};
    try {
      expect(await cmd.execute()).toBe(0);
    } finally {
      console.log = origLog;
      rmSync(join(yamlPath, ".."), { recursive: true });
    }

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect((fetchMock.mock.calls as any[][])[0][0]).toContain(
      "/api/v1/admin/agents/state-agent",
    );
    expect((fetchMock.mock.calls as any[][])[0][1].body).toEqual({
      config: { systemPrompt: "updated prompt" },
    });
  });
});

describe("agent create/update invalid YAML side effects", () => {
  const harness = join(import.meta.dir, "../fixtures/agent-yaml-command-harness.ts");

  test.each([
    ["create", "AgentCreateCommand", "missing-id"],
    ["update", "AgentUpdateCommand", "mismatched-id"],
  ])("%s rejects invalid YAML before API calls", (_name, className, scenario) => {
    const result = Bun.spawnSync([process.execPath, harness, className, scenario], {
      cwd: join(import.meta.dir, "../.."),
      stdout: "pipe",
      stderr: "pipe",
    });
    expect(result.exitCode).toBe(0);
    const observed = JSON.parse(result.stdout.toString()) as {
      code: number;
      errors: string;
      fetchCalls: number;
    };
    expect(observed.code).toBe(2);
    expect(observed.errors).toContain("错误：");
    expect(observed.fetchCalls).toBe(0);
  });
});

// ── Delete ────────────────────────────────────────────────────

describe("agent delete command", () => {
  beforeEach(() => {
    mock.module("../../src/config", () => ({
      ...realConfig,
      getActiveProfile: mock(() =>
        Promise.resolve({ serverUrl: "https://test.local", token: "cli_test" })
      ),
    }));
  });

  test("delete sends DELETE /api/v1/admin/agents/:name", async () => {
    const fetchMock = mock(() => Promise.resolve({ success: true }));
    mock.module("ofetch", () => ({
      ofetch: fetchMock,
      FetchError: class FetchError extends Error {},
    }));

    const { AgentDeleteCommand } = await import(
      `../../src/commands/agent.ts?t=${Date.now()}`
    );
    const cmd = new AgentDeleteCommand();
    (cmd as any).name = "old-agent";

    const logs: string[] = [];
    const origLog = console.log;
    console.log = (s: string) => logs.push(s);
    try {
      const code = await cmd.execute();
      expect(code).toBe(0);
    } finally {
      console.log = origLog;
    }

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const calls = fetchMock.mock.calls as any[][];
    expect(calls[0][0]).toContain("/api/v1/admin/agents/old-agent");
    expect(calls[0][1].method).toBe("DELETE");
    expect(logs.join("\n")).toContain("已删除");
  });
});

// ── Set Tools ─────────────────────────────────────────────────

describe("agent set-tools command", () => {
  beforeEach(() => {
    mock.module("../../src/config", () => ({
      ...realConfig,
      getActiveProfile: mock(() =>
        Promise.resolve({ serverUrl: "https://test.local", token: "cli_test" })
      ),
    }));
  });

  test("set-tools sends PUT with toolNames array", async () => {
    const fetchMock = mock(() => Promise.resolve({ success: true }));
    mock.module("ofetch", () => ({
      ofetch: fetchMock,
      FetchError: class FetchError extends Error {},
    }));

    const { AgentSetToolsCommand } = await import(
      `../../src/commands/agent.ts?t=${Date.now()}`
    );
    const cmd = new AgentSetToolsCommand();
    (cmd as any).name = "researcher";
    (cmd as any).tools = ["bash", "edit", "read"];

    const logs: string[] = [];
    const origLog = console.log;
    console.log = (s: string) => logs.push(s);
    try {
      const code = await cmd.execute();
      expect(code).toBe(0);
    } finally {
      console.log = origLog;
    }

    const calls2 = fetchMock.mock.calls as any[][];
    const callArgs = calls2[0][1];
    expect(callArgs.method).toBe("PUT");
    expect(callArgs.body.toolNames).toEqual(["bash", "edit", "read"]);
    expect(logs.join("")).toContain("3");
  });
});

// ── Set MCPs ──────────────────────────────────────────────────

describe("agent set-mcps command", () => {
  beforeEach(() => {
    mock.module("../../src/config", () => ({
      ...realConfig,
      getActiveProfile: mock(() =>
        Promise.resolve({ serverUrl: "https://test.local", token: "cli_test" })
      ),
    }));
  });

  test("set-mcps sends PUT with mcpNames array", async () => {
    const fetchMock = mock(() => Promise.resolve({ success: true }));
    mock.module("ofetch", () => ({
      ofetch: fetchMock,
      FetchError: class FetchError extends Error {},
    }));

    const { AgentSetMcpsCommand } = await import(
      `../../src/commands/agent.ts?t=${Date.now()}`
    );
    const cmd = new AgentSetMcpsCommand();
    (cmd as any).name = "research-agent";
    (cmd as any).mcpNames = ["knowledge", "web-search"];

    const logs: string[] = [];
    const origLog = console.log;
    console.log = (s: string) => logs.push(s);
    try {
      const code = await cmd.execute();
      expect(code).toBe(0);
    } finally {
      console.log = origLog;
    }

    expect(logs).toContain("已设置 2 个 MCP");
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const calls = fetchMock.mock.calls as any[][];
    expect(calls[0][0]).toContain("/api/v1/admin/agents/research-agent/mcps");
    expect(calls[0][1].method).toBe("PUT");
    expect(calls[0][1].body).toEqual({
      mcpNames: ["knowledge", "web-search"],
    });
  });
});

// ── Deploy ────────────────────────────────────────────────────

describe("agent deploy command", () => {
  beforeEach(() => {
    mock.module("../../src/config", () => ({
      ...realConfig,
      getActiveProfile: mock(() =>
        Promise.resolve({ serverUrl: "https://test.local", token: "cli_test" })
      ),
    }));
  });

  test("deploy sends POST and prints deployment info", async () => {
    const fakeDeploy = {
      status: "running",
      health: "healthy",
      runtimeUrl: "https://agent-1.test.local",
      hostPort: 4001,
    };
    const fetchMock = mock(() =>
      Promise.resolve({ success: true, data: fakeDeploy })
    );
    mock.module("ofetch", () => ({
      ofetch: fetchMock,
      FetchError: class FetchError extends Error {},
    }));

    const { AgentDeployCommand } = await import(
      `../../src/commands/agent.ts?t=${Date.now()}`
    );
    const cmd = new AgentDeployCommand();
    (cmd as any).name = "coder";
    (cmd as any).force = false;
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

    const out = logs.join("\n");
    expect(out).toContain("running");
    expect(out).toContain("healthy");
    expect(out).toContain("https://agent-1.test.local");

    const deployCalls = fetchMock.mock.calls as any[][];
    expect(deployCalls[0][0]).toContain("/api/v1/admin/agents/coder/deploy");
    expect(deployCalls[0][1].method).toBe("POST");
  });

  test("deploy --force adds force=true query", async () => {
    const fetchMock = mock(() =>
      Promise.resolve({ success: true, data: { status: "running" } })
    );
    mock.module("ofetch", () => ({
      ofetch: fetchMock,
      FetchError: class FetchError extends Error {},
    }));

    const { AgentDeployCommand } = await import(
      `../../src/commands/agent.ts?t=${Date.now()}`
    );
    const cmd = new AgentDeployCommand();
    (cmd as any).name = "coder";
    (cmd as any).force = true;
    (cmd as any).output = "text";

    const logs: string[] = [];
    const origLog = console.log;
    console.log = (s: string) => logs.push(s);
    try {
      await cmd.execute();
    } finally {
      console.log = origLog;
    }

    const forceCalls = fetchMock.mock.calls as any[][];
    expect(forceCalls[0][1].query).toEqual({ force: "true" });
  });

  test("deploy resolves relative runtimeUrl (no-Kong) against profile serverUrl", async () => {
    const fakeDeploy = {
      status: "running",
      runtimeUrl: "/runtime/default/coder",
      hostPort: 4001,
    };
    mock.module("ofetch", () => ({
      ofetch: mock(() => Promise.resolve({ success: true, data: fakeDeploy })),
      FetchError: class FetchError extends Error {},
    }));

    const { AgentDeployCommand } = await import(
      `../../src/commands/agent.ts?t=${Date.now()}`
    );
    const cmd = new AgentDeployCommand();
    (cmd as any).name = "coder";
    (cmd as any).force = false;
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

    // 人类可读输出必须打印解析后的绝对 URL，而非原样相对路径
    expect(logs.join("\n")).toContain("https://test.local/runtime/default/coder");
  });
});

// ── Status ────────────────────────────────────────────────────

describe("agent status command", () => {
  beforeEach(() => {
    mock.module("../../src/config", () => ({
      ...realConfig,
      getActiveProfile: mock(() =>
        Promise.resolve({ serverUrl: "https://test.local", token: "cli_test" })
      ),
    }));
  });

  test("status sends GET /deploy and prints info", async () => {
    const fakeStatus = {
      status: "stopped",
      hostPort: 4001,
      deployedAt: "2025-01-01T00:00:00Z",
    };
    const fetchMock = mock(() =>
      Promise.resolve({ success: true, data: fakeStatus })
    );
    mock.module("ofetch", () => ({
      ofetch: fetchMock,
      FetchError: class FetchError extends Error {},
    }));

    const { AgentStatusCommand } = await import(
      `../../src/commands/agent.ts?t=${Date.now()}`
    );
    const cmd = new AgentStatusCommand();
    (cmd as any).name = "coder";
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

    const out = logs.join("\n");
    expect(out).toContain("stopped");
    expect(out).toContain("4001");
    expect((fetchMock.mock.calls as any[][])[0][1].method).toBe("GET");
  });
});

// ── Undeploy ──────────────────────────────────────────────────

describe("agent undeploy command", () => {
  beforeEach(() => {
    mock.module("../../src/config", () => ({
      ...realConfig,
      getActiveProfile: mock(() =>
        Promise.resolve({ serverUrl: "https://test.local", token: "cli_test" })
      ),
    }));
  });

  test("undeploy sends DELETE /deploy", async () => {
    const fetchMock = mock(() => Promise.resolve({ success: true }));
    mock.module("ofetch", () => ({
      ofetch: fetchMock,
      FetchError: class FetchError extends Error {},
    }));

    const { AgentUndeployCommand } = await import(
      `../../src/commands/agent.ts?t=${Date.now()}`
    );
    const cmd = new AgentUndeployCommand();
    (cmd as any).name = "coder";
    (cmd as any).purge = false;

    const logs: string[] = [];
    const origLog = console.log;
    console.log = (s: string) => logs.push(s);
    try {
      const code = await cmd.execute();
      expect(code).toBe(0);
    } finally {
      console.log = origLog;
    }

    expect((fetchMock.mock.calls as any[][])[0][1].method).toBe("DELETE");
    expect(logs.join("\n")).toContain("已归档");
  });

  test("undeploy --purge prints 彻底删除", async () => {
    mock.module("ofetch", () => ({
      ofetch: mock(() => Promise.resolve({ success: true })),
      FetchError: class FetchError extends Error {},
    }));

    const { AgentUndeployCommand } = await import(
      `../../src/commands/agent.ts?t=${Date.now()}`
    );
    const cmd = new AgentUndeployCommand();
    (cmd as any).name = "coder";
    (cmd as any).purge = true;

    const logs: string[] = [];
    const origLog = console.log;
    console.log = (s: string) => logs.push(s);
    try {
      await cmd.execute();
    } finally {
      console.log = origLog;
    }

    expect(logs.join("\n")).toContain("彻底删除");
  });
});

// ── resolveRuntimeUrl helper（issue #77：no-Kong 相对 runtimeUrl 解析）───────
// renderDeployment 的人类可读输出用它把 /runtime/{org}/{agent} 解析成绝对 URL；
// JSON 输出字段原样镜像 API payload，不走此 helper。

describe("resolveRuntimeUrl", () => {
  test("相对路径 + 无尾部斜杠 serverUrl → 绝对 URL", () => {
    expect(resolveRuntimeUrl("/runtime/default/test", "http://localhost:8081")).toBe(
      "http://localhost:8081/runtime/default/test",
    );
  });

  test("相对路径 + 尾部斜杠 serverUrl → 单斜杠拼接", () => {
    expect(resolveRuntimeUrl("/runtime/zerone/agent-1", "http://hub.example.com/")).toBe(
      "http://hub.example.com/runtime/zerone/agent-1",
    );
  });

  // 专家二轮 P1：base-path serverUrl 必须保留 path——API client 是字符串拼接
  // （base.ts `${serverUrl}${path}`，流量实际打到 {serverUrl}/api/...），
  // WHATWG new URL 会把 /hub 整体丢掉，拼接语义必须与 client 一致。
  test("相对路径 + 带 path 的 serverUrl → 保留 base path（与 API client 拼接语义一致）", () => {
    expect(resolveRuntimeUrl("/runtime/default/coder", "https://example.com/hub")).toBe(
      "https://example.com/hub/runtime/default/coder",
    );
  });

  test("相对路径 + 带 path 且尾部多斜杠的 serverUrl → 同样单斜杠拼接", () => {
    expect(resolveRuntimeUrl("/runtime/default/coder", "https://example.com/hub/")).toBe(
      "https://example.com/hub/runtime/default/coder",
    );
    expect(resolveRuntimeUrl("/runtime/default/coder", "https://example.com/hub///")).toBe(
      "https://example.com/hub/runtime/default/coder",
    );
  });

  test("绝对 http/https URL 原样返回（Kong 模式）", () => {
    expect(resolveRuntimeUrl("http://203.0.113.10:32100", "http://localhost:8081")).toBe(
      "http://203.0.113.10:32100",
    );
    expect(
      resolveRuntimeUrl("https://kong.example.com/zerone/agent", "http://localhost:8081"),
    ).toBe("https://kong.example.com/zerone/agent");
  });

  test("serverUrl 缺失 → 相对路径原样返回（不误报）", () => {
    expect(resolveRuntimeUrl("/runtime/default/test", "")).toBe("/runtime/default/test");
  });
});
