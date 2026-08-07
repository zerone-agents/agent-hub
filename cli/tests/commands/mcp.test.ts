import { writeFileSync, mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { describe, test, expect, mock, beforeEach, afterEach } from "bun:test";
import { Cli } from "clipanion";
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
  const fetchMock = mock((url: string, _opts?: any) => {
    if (typeof url === "string" && url.includes("/probe")) {
      return Promise.resolve({
        success: true,
        data: {
          status: "success",
          tools: [{ name: "test_tool", description: "Test tool" }],
        },
      });
    }
    return Promise.resolve({ success: true, data: responseData });
  });
  mock.module("ofetch", () => ({
    ofetch: fetchMock,
    FetchError: class FetchError extends Error {},
  }));
  return fetchMock;
}

const tempDirs: string[] = [];

function writeTempYaml(content: string): string {
  const dir = mkdtempSync(join(tmpdir(), "zhub-test-"));
  tempDirs.push(dir);
  const filePath = join(dir, "input.yaml");
  writeFileSync(filePath, content, "utf-8");
  return filePath;
}

afterEach(() => {
  for (const dir of tempDirs) {
    try {
      rmSync(dir, { recursive: true });
    } catch {}
  }
  tempDirs.length = 0;
  mock.restore();
});

// ── List command ──────────────────────────────────────────────

describe("mcp list command", () => {
  beforeEach(setupConfigMock);

  test("prints table with name, title, description", async () => {
    const fakeMcps = [
      {
        id: 1,
        name: "filesystem",
        title: "Filesystem",
        description: "File access MCP",
        transportType: "sse",
        hasHeaders: false,
        isBuiltin: false,
      },
      {
        id: 2,
        name: "weather",
        title: "Weather",
        description: "Weather MCP",
        transportType: "http",
        hasHeaders: true,
        isBuiltin: false,
      },
    ];
    const fetchMock = setupFetchMock(fakeMcps);

    const { McpListCommand } = await import(
      `../../src/commands/mcp.ts?t=${Date.now()}`
    );
    const cmd = new McpListCommand();
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
    expect(out).toContain("filesystem");
    expect(out).toContain("Filesystem");
    expect(out).toContain("File access MCP");
    expect(out).toContain("weather");
    expect(out).toContain("Weather");
    expect(out).toContain("Weather MCP");
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  test("defaults to table output", async () => {
    const fakeMcps = [
      {
        id: 1,
        name: "filesystem",
        title: "Filesystem",
        description: "File access MCP",
        transportType: "sse",
        hasHeaders: false,
        isBuiltin: false,
      },
    ];
    setupFetchMock(fakeMcps);

    const { McpListCommand } = await import(
      `../../src/commands/mcp.ts?t=${Date.now()}`
    );
    const cli = Cli.from([McpListCommand]);

    const logs: string[] = [];
    const origLog = console.log;
    console.log = (s: string) => logs.push(s);
    try {
      const code = await cli.run(["mcp", "list"]);
      expect(code).toBe(0);
    } finally {
      console.log = origLog;
    }

    const out = logs.join("\n");
    expect(out).toContain("filesystem");
    expect(out).toContain("Filesystem");
    expect(out).toContain("File access MCP");
  });

  test("returns error 2 with invalid --output", async () => {
    const { McpListCommand } = await import(
      `../../src/commands/mcp.ts?t=${Date.now()}`
    );
    const cmd = new McpListCommand();
    (cmd as any).output = "xml";

    const origErr = process.stderr.write;
    const errs: string[] = [];
    process.stderr.write = ((s: string) => {
      errs.push(s);
      return true;
    }) as any;

    try {
      const code = await cmd.execute();
      expect(code).toBe(2);
      expect(errs.join("")).toContain("--output 必须是 table / json / yaml");
    } finally {
      process.stderr.write = origErr as any;
    }
  });
});

// ── Get command ─────────────────────────────────────────────────

describe("mcp get command", () => {
  beforeEach(setupConfigMock);

  test("prints mcp details in yaml with headers masked", async () => {
    const fakeMcp = {
      id: 1,
      name: "filesystem",
      title: "Filesystem",
      description: "File access MCP",
      transportType: "sse",
      url: "http://localhost:3001/sse",
      hasHeaders: true,
      isBuiltin: false,
      headers: { Authorization: "Bearer super-secret" },
      createdAt: "2026-01-01T00:00:00Z",
      updatedAt: "2026-01-01T00:00:00Z",
    };
    setupFetchMock(fakeMcp);

    const { McpGetCommand } = await import(
      `../../src/commands/mcp.ts?t=${Date.now()}`
    );
    const cmd = new McpGetCommand();
    (cmd as any).name = "filesystem";
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
    expect(out).toContain("filesystem");
    expect(out).toContain("Filesystem");
    expect(out).toContain("File access MCP");
    expect(out).toContain("<hidden>");
    expect(out).not.toContain("super-secret");
  });

  test("defaults to yaml output", async () => {
    const fakeMcp = {
      id: 1,
      name: "filesystem",
      title: "Filesystem",
      description: "File access MCP",
      transportType: "sse",
      url: "http://localhost:3001/sse",
      hasHeaders: false,
      isBuiltin: false,
      createdAt: "2026-01-01T00:00:00Z",
      updatedAt: "2026-01-01T00:00:00Z",
    };
    setupFetchMock(fakeMcp);

    const { McpGetCommand } = await import(
      `../../src/commands/mcp.ts?t=${Date.now()}`
    );
    const cli = Cli.from([McpGetCommand]);

    const logs: string[] = [];
    const origLog = console.log;
    console.log = (s: string) => logs.push(s);
    try {
      const code = await cli.run(["mcp", "get", "filesystem"]);
      expect(code).toBe(0);
    } finally {
      console.log = origLog;
    }

    const out = logs.join("\n");
    expect(out).toContain("name: filesystem");
    expect(out).toContain("title: Filesystem");
    expect(out).toContain("description: File access MCP");
    expect(out).toContain("transportType: sse");
  });
});

// ── Create command ──────────────────────────────────────────────

describe("mcp create command", () => {
  beforeEach(setupConfigMock);

  test("sends POST /api/v1/admin/mcps with body from --file", async () => {
    const fakeMcp = {
      id: 1,
      name: "filesystem",
      title: "Filesystem",
      description: "File access MCP",
      transportType: "sse",
      hasHeaders: false,
      isBuiltin: false,
    };
    const fetchMock = setupFetchMock(fakeMcp);

    const filePath = writeTempYaml(
      "name: filesystem\ntitle: Filesystem\ndescription: File access MCP\ntransportType: sse"
    );

    const { McpCreateCommand } = await import(
      `../../src/commands/mcp.ts?t=${Date.now()}`
    );
    const cmd = new McpCreateCommand();
    (cmd as any).file = filePath;
    (cmd as any).json = undefined;
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

    const calls = fetchMock.mock.calls as any[][];
    // calls[0] is the probe call to /api/v1/admin/mcps/probe
    expect(calls[0][0]).toContain("/api/v1/admin/mcps/probe");
    expect(calls[0][1].method).toBe("POST");
    // calls[1] is the create call
    expect(calls[1][0]).toContain("/api/v1/admin/mcps");
    expect(calls[1][1].method).toBe("POST");
    expect(calls[1][1].body.name).toBe("filesystem");
    expect(calls[1][1].body.title).toBe("Filesystem");
    expect(calls[1][1].body.description).toBe("File access MCP");
    expect(calls[1][1].body.transportType).toBe("sse");
    expect(calls[1][1].body.tools).toEqual([{ name: "test_tool", description: "Test tool" }]);
    expect(logs.join("\n")).toContain("filesystem");
  });

  test("sends POST with body from --json", async () => {
    const fakeMcp = {
      id: 2,
      name: "weather",
      title: "Weather",
      description: "Weather MCP",
      transportType: "http",
      hasHeaders: false,
      isBuiltin: false,
    };
    const fetchMock = setupFetchMock(fakeMcp);

    const { McpCreateCommand } = await import(
      `../../src/commands/mcp.ts?t=${Date.now()}`
    );
    const cmd = new McpCreateCommand();
    (cmd as any).file = undefined;
    (cmd as any).json = JSON.stringify({
      name: "weather",
      title: "Weather",
      description: "Weather MCP",
      transportType: "http",
    });
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

    const calls = fetchMock.mock.calls as any[][];
    // calls[0] is the probe call
    expect(calls[0][0]).toContain("/api/v1/admin/mcps/probe");
    // calls[1] is the create call
    expect(calls[1][0]).toContain("/api/v1/admin/mcps");
    expect(calls[1][1].method).toBe("POST");
    expect(calls[1][1].body.name).toBe("weather");
    expect(calls[1][1].body.transportType).toBe("http");
  });

  test("returns error 2 without --file or --json", async () => {
    const { McpCreateCommand } = await import(
      `../../src/commands/mcp.ts?t=${Date.now()}`
    );
    const cmd = new McpCreateCommand();
    (cmd as any).file = undefined;
    (cmd as any).json = undefined;
    (cmd as any).output = "yaml";

    const origErr = process.stderr.write;
    const errs: string[] = [];
    process.stderr.write = ((s: string) => {
      errs.push(s);
      return true;
    }) as any;

    try {
      const code = await cmd.execute();
      expect(code).toBe(2);
      expect(errs.join("")).toContain("必须提供 --file 或 --json");
    } finally {
      process.stderr.write = origErr as any;
    }
  });

  test("returns error 2 with invalid --output", async () => {
    const { McpCreateCommand } = await import(
      `../../src/commands/mcp.ts?t=${Date.now()}`
    );
    const cmd = new McpCreateCommand();
    (cmd as any).file = undefined;
    (cmd as any).json = JSON.stringify({ name: "x", transportType: "sse" });
    (cmd as any).output = "xml";

    const origErr = process.stderr.write;
    const errs: string[] = [];
    process.stderr.write = ((s: string) => {
      errs.push(s);
      return true;
    }) as any;

    try {
      const code = await cmd.execute();
      expect(code).toBe(2);
      expect(errs.join("")).toContain("--output 必须是 table / json / yaml");
    } finally {
      process.stderr.write = origErr as any;
    }
  });

  test("returns error 2 when --file read fails", async () => {
    const { McpCreateCommand } = await import(
      `../../src/commands/mcp.ts?t=${Date.now()}`
    );
    const cmd = new McpCreateCommand();
    (cmd as any).file = "/tmp/nonexistent-zhub-missing.yaml";
    (cmd as any).json = undefined;
    (cmd as any).output = "yaml";

    const origErr = process.stderr.write;
    const errs: string[] = [];
    process.stderr.write = ((s: string) => {
      errs.push(s);
      return true;
    }) as any;

    try {
      const code = await cmd.execute();
      expect(code).toBe(2);
      expect(errs.join("")).toContain("无法读取或解析输入文件/JSON");
    } finally {
      process.stderr.write = origErr as any;
    }
  });

  test("returns error 2 when --json is invalid JSON", async () => {
    const { McpCreateCommand } = await import(
      `../../src/commands/mcp.ts?t=${Date.now()}`
    );
    const cmd = new McpCreateCommand();
    (cmd as any).file = undefined;
    (cmd as any).json = "not valid json";
    (cmd as any).output = "yaml";

    const origErr = process.stderr.write;
    const errs: string[] = [];
    process.stderr.write = ((s: string) => {
      errs.push(s);
      return true;
    }) as any;

    try {
      const code = await cmd.execute();
      expect(code).toBe(2);
      expect(errs.join("")).toContain("无法读取或解析输入文件/JSON");
    } finally {
      process.stderr.write = origErr as any;
    }
  });
});

// ── Update command ──────────────────────────────────────────────

describe("mcp update command", () => {
  beforeEach(setupConfigMock);

  test("sends PUT /api/v1/admin/mcps/:name with body", async () => {
    const fakeMcp = {
      id: 1,
      name: "filesystem",
      title: "Filesystem Pro",
      description: "Advanced file access MCP",
      transportType: "sse",
      url: "http://localhost:3001/sse",
      hasHeaders: false,
      isBuiltin: false,
    };
    const fetchMock = setupFetchMock(fakeMcp);

    const filePath = writeTempYaml(
      "title: Filesystem Pro\ndescription: Advanced file access MCP"
    );

    const { McpUpdateCommand } = await import(
      `../../src/commands/mcp.ts?t=${Date.now()}`
    );
    const cmd = new McpUpdateCommand();
    (cmd as any).name = "filesystem";
    (cmd as any).file = filePath;
    (cmd as any).json = undefined;
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

    const calls = fetchMock.mock.calls as any[][];
    // calls[0] = GET /api/v1/admin/mcps/filesystem (existing)
    // calls[1] = POST /api/v1/admin/mcps/filesystem/probe (configChanged=true since body.url is undefined)
    // calls[2] = PUT /api/v1/admin/mcps/filesystem
    expect(calls[2][0]).toContain("/api/v1/admin/mcps/filesystem");
    expect(calls[2][1].method).toBe("PUT");
    expect(calls[2][1].body.title).toBe("Filesystem Pro");
    expect(calls[2][1].body.description).toBe("Advanced file access MCP");
  });

  test("returns error 2 without --file or --json", async () => {
    const { McpUpdateCommand } = await import(
      `../../src/commands/mcp.ts?t=${Date.now()}`
    );
    const cmd = new McpUpdateCommand();
    (cmd as any).name = "filesystem";
    (cmd as any).file = undefined;
    (cmd as any).json = undefined;
    (cmd as any).output = "yaml";

    const origErr = process.stderr.write;
    const errs: string[] = [];
    process.stderr.write = ((s: string) => {
      errs.push(s);
      return true;
    }) as any;

    try {
      const code = await cmd.execute();
      expect(code).toBe(2);
      expect(errs.join("")).toContain("必须提供 --file 或 --json");
    } finally {
      process.stderr.write = origErr as any;
    }
  });
});

// ── Delete command ──────────────────────────────────────────────

describe("mcp delete command", () => {
  beforeEach(setupConfigMock);

  test("sends DELETE /api/v1/admin/mcps/:name", async () => {
    const fetchMock = setupFetchMock({});

    const { McpDeleteCommand } = await import(
      `../../src/commands/mcp.ts?t=${Date.now()}`
    );
    const cmd = new McpDeleteCommand();
    (cmd as any).name = "filesystem";

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
    expect(calls[0][0]).toContain("/api/v1/admin/mcps/filesystem");
    expect(calls[0][1].method).toBe("DELETE");
    expect(logs.join("\n")).toContain("已删除 MCP：filesystem");
  });
});

// ── Probe command ───────────────────────────────────────────────

describe("mcp probe command", () => {
  beforeEach(setupConfigMock);

  test("returns error 2 without name or --file/--json", async () => {
    const { McpProbeCommand } = await import(
      `../../src/commands/mcp.ts?t=${Date.now()}`
    );
    const cmd = new McpProbeCommand();
    (cmd as any).name = undefined;
    (cmd as any).file = undefined;
    (cmd as any).json = undefined;
    (cmd as any).output = "table";

    const origErr = process.stderr.write;
    const errs: string[] = [];
    process.stderr.write = ((s: string) => {
      errs.push(s);
      return true;
    }) as any;

    try {
      const code = await cmd.execute();
      expect(code).toBe(2);
      expect(errs.join("")).toContain("必须提供 MCP name 或 --file 或 --json");
    } finally {
      process.stderr.write = origErr as any;
    }
  });

  test("probes by name and renders table", async () => {
    const fetchMock = setupFetchMock({});

    const { McpProbeCommand } = await import(
      `../../src/commands/mcp.ts?t=${Date.now()}`
    );
    const cmd = new McpProbeCommand();
    (cmd as any).name = "filesystem";
    (cmd as any).file = undefined;
    (cmd as any).json = undefined;
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

    const calls = fetchMock.mock.calls as any[][];
    expect(calls[0][0]).toContain("/api/v1/admin/mcps/filesystem/probe");
    expect(calls[0][1].method).toBe("POST");
    const out = logs.join("\n");
    expect(out).toContain("探测成功");
    expect(out).toContain("test_tool");
  });

  test("probes by --file config", async () => {
    const fetchMock = setupFetchMock({});

    const filePath = writeTempYaml(
      "transportType: sse\nurl: http://localhost:3001/sse"
    );

    const { McpProbeCommand } = await import(
      `../../src/commands/mcp.ts?t=${Date.now()}`
    );
    const cmd = new McpProbeCommand();
    (cmd as any).name = undefined;
    (cmd as any).file = filePath;
    (cmd as any).json = undefined;
    (cmd as any).output = "json";

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
    expect(calls[0][0]).toContain("/api/v1/admin/mcps/probe");
    expect(calls[0][1].method).toBe("POST");
  });

  test("returns error 1 when probe fails", async () => {
    // Override the fetch mock to return a failed probe result
    const fetchMock = mock(() =>
      Promise.resolve({
        success: true,
        data: { status: "failed", error: "Connection refused" },
      })
    );
    mock.module("ofetch", () => ({
      ofetch: fetchMock,
      FetchError: class FetchError extends Error {},
    }));

    const { McpProbeCommand } = await import(
      `../../src/commands/mcp.ts?t=${Date.now()}`
    );
    const cmd = new McpProbeCommand();
    (cmd as any).name = "bad-mcp";
    (cmd as any).file = undefined;
    (cmd as any).json = undefined;
    (cmd as any).output = "table";

    const logs: string[] = [];
    const origLog = console.log;
    console.log = (s: string) => logs.push(s);
    try {
      const code = await cmd.execute();
      expect(code).toBe(1);
    } finally {
      console.log = origLog;
    }

    const out = logs.join("\n");
    expect(out).toContain("探测失败");
    expect(out).toContain("Connection refused");
  });
});

