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
  const fetchMock = mock(() =>
    Promise.resolve({ success: true, data: responseData })
  );
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

function writeTempSource(content: string): string {
  const dir = mkdtempSync(join(tmpdir(), "zhub-test-"));
  tempDirs.push(dir);
  const filePath = join(dir, "tool.ts");
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

describe("tool list command", () => {
  beforeEach(setupConfigMock);

  test("prints table with name, title, description, isDefault", async () => {
    const fakeTools = [
      {
        id: 1,
        name: "calculator",
        title: "Calculator",
        description: "Math tool",
        isDefault: true,
      },
      {
        id: 2,
        name: "weather",
        title: "Weather",
        description: "Weather tool",
        isDefault: false,
      },
    ];
    const fetchMock = setupFetchMock(fakeTools);

    const { ToolListCommand } = await import(
      `../../src/commands/tool.ts?t=${Date.now()}`
    );
    const cmd = new ToolListCommand();
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
    expect(out).toContain("calculator");
    expect(out).toContain("Weather");
    expect(out).toContain("Math tool");
    expect(out).toContain("✓");
    expect(out).toContain("✗");
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  test("defaults to table output", async () => {
    const fakeTools = [
      {
        id: 1,
        name: "calculator",
        title: "Calculator",
        description: "Math tool",
        isDefault: true,
      },
    ];
    setupFetchMock(fakeTools);

    const { ToolListCommand } = await import(
      `../../src/commands/tool.ts?t=${Date.now()}`
    );
    // Run through clipanion so the declared --output default is applied.
    const cli = Cli.from([ToolListCommand]);

    const logs: string[] = [];
    const origLog = console.log;
    console.log = (s: string) => logs.push(s);
    try {
      const code = await cli.run(["tool", "list"]);
      expect(code).toBe(0);
    } finally {
      console.log = origLog;
    }

    const out = logs.join("\n");
    expect(out).toContain("calculator");
    expect(out).toContain("Calculator");
    expect(out).toContain("Math tool");
    expect(out).toContain("✓");
  });

  test("returns error 2 with invalid --output", async () => {
    const { ToolListCommand } = await import(
      `../../src/commands/tool.ts?t=${Date.now()}`
    );
    const cmd = new ToolListCommand();
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

describe("tool get command", () => {
  beforeEach(setupConfigMock);

  test("prints tool details in yaml", async () => {
    const fakeTool = {
      id: 1,
      name: "calculator",
      title: "Calculator",
      description: "Math tool",
      isDefault: true,
      createdAt: "2026-01-01T00:00:00Z",
      updatedAt: "2026-01-01T00:00:00Z",
    };
    setupFetchMock(fakeTool);

    const { ToolGetCommand } = await import(
      `../../src/commands/tool.ts?t=${Date.now()}`
    );
    const cmd = new ToolGetCommand();
    (cmd as any).name = "calculator";
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
    expect(out).toContain("calculator");
    expect(out).toContain("Calculator");
    expect(out).toContain("Math tool");
    expect(out).toContain("isDefault: true");
    expect(out).toContain("createdAt");
    expect(out).toContain("updatedAt");
  });

  test("defaults to yaml output", async () => {
    const fakeTool = {
      id: 1,
      name: "calculator",
      title: "Calculator",
      description: "Math tool",
      isDefault: true,
      createdAt: "2026-01-01T00:00:00Z",
      updatedAt: "2026-01-01T00:00:00Z",
    };
    setupFetchMock(fakeTool);

    const { ToolGetCommand } = await import(
      `../../src/commands/tool.ts?t=${Date.now()}`
    );
    // Run through clipanion so the declared --output default is applied.
    const cli = Cli.from([ToolGetCommand]);

    const logs: string[] = [];
    const origLog = console.log;
    console.log = (s: string) => logs.push(s);
    try {
      const code = await cli.run(["tool", "get", "calculator"]);
      expect(code).toBe(0);
    } finally {
      console.log = origLog;
    }

    const out = logs.join("\n");
    expect(out).toContain("name: calculator");
    expect(out).toContain("title: Calculator");
    expect(out).toContain("description: Math tool");
    expect(out).toContain("isDefault: true");
  });

  test("returns error 2 with invalid --output", async () => {
    const { ToolGetCommand } = await import(
      `../../src/commands/tool.ts?t=${Date.now()}`
    );
    const cmd = new ToolGetCommand();
    (cmd as any).name = "calculator";
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

// ── Create command ──────────────────────────────────────────────

describe("tool create command", () => {
  beforeEach(setupConfigMock);

  test("sends POST /api/v1/admin/tools with multipart from --file", async () => {
    const fakeTool = {
      id: 1,
      name: "calculator",
      title: "Calculator",
      description: "Math tool",
      isDefault: true,
    };
    const fetchMock = setupFetchMock(fakeTool);

    const filePath = writeTempYaml(
      "name: calculator\ntitle: Calculator\ndescription: Math tool\nisDefault: true"
    );
    const sourcePath = writeTempSource("export default {}\n");

    const { ToolCreateCommand } = await import(
      `../../src/commands/tool.ts?t=${Date.now()}`
    );
    const cmd = new ToolCreateCommand();
    (cmd as any).file = filePath;
    (cmd as any).json = undefined;
    (cmd as any).source = sourcePath;
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
    expect(calls[0][0]).toContain("/api/v1/admin/tools");
    expect(calls[0][1].method).toBe("POST");
    expect(calls[0][1].body).toBeInstanceOf(FormData);
    expect(calls[0][1].body.get("name")).toBe("calculator");
    expect(calls[0][1].body.get("title")).toBe("Calculator");
    expect(calls[0][1].body.get("description")).toBe("Math tool");
    expect(calls[0][1].body.get("isDefault")).toBe(null);
    const file = calls[0][1].body.get("file");
    expect(file).toBeInstanceOf(Blob);
    expect((file as File).name).toBe("tool.ts");
    expect(logs.join("\n")).toContain("calculator");
  });

  test("sends POST with multipart from --json", async () => {
    const fakeTool = {
      id: 2,
      name: "weather",
      title: "Weather",
      description: "Weather tool",
      isDefault: false,
    };
    const fetchMock = setupFetchMock(fakeTool);
    const sourcePath = writeTempSource("export const x = 1;\n");

    const { ToolCreateCommand } = await import(
      `../../src/commands/tool.ts?t=${Date.now()}`
    );
    const cmd = new ToolCreateCommand();
    (cmd as any).file = undefined;
    (cmd as any).json = JSON.stringify({
      name: "weather",
      title: "Weather",
      description: "Weather tool",
    });
    (cmd as any).source = sourcePath;
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
    expect(calls[0][0]).toContain("/api/v1/admin/tools");
    expect(calls[0][1].method).toBe("POST");
    expect(calls[0][1].body).toBeInstanceOf(FormData);
    expect(calls[0][1].body.get("name")).toBe("weather");
    const file = calls[0][1].body.get("file");
    expect((file as File).name).toBe("tool.ts");
  });

  test("returns error 2 without --file or --json", async () => {
    const { ToolCreateCommand } = await import(
      `../../src/commands/tool.ts?t=${Date.now()}`
    );
    const cmd = new ToolCreateCommand();
    (cmd as any).file = undefined;
    (cmd as any).json = undefined;
    (cmd as any).output = "yaml";

    const origErr = process.stderr.write;
    const errs: string[] = [];
    process.stderr.write = ((s: string) => {
      errs.push(s);
      return true;
    }) as any;

    const code = await cmd.execute();
    expect(code).toBe(2);

    process.stderr.write = origErr as any;
    expect(errs.join("")).toContain("必须提供 --file 或 --json");
  });

  test("returns error 2 with invalid --output", async () => {
    const { ToolCreateCommand } = await import(
      `../../src/commands/tool.ts?t=${Date.now()}`
    );
    const cmd = new ToolCreateCommand();
    (cmd as any).file = undefined;
    (cmd as any).json = JSON.stringify({ name: "x", isDefault: true });
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
    const { ToolCreateCommand } = await import(
      `../../src/commands/tool.ts?t=${Date.now()}`
    );
    const cmd = new ToolCreateCommand();
    (cmd as any).file = "/tmp/nonexistent-zhub-tool-missing.yaml";
    (cmd as any).json = undefined;
    (cmd as any).source = writeTempSource("export default {}\n");
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
    const { ToolCreateCommand } = await import(
      `../../src/commands/tool.ts?t=${Date.now()}`
    );
    const cmd = new ToolCreateCommand();
    (cmd as any).file = undefined;
    (cmd as any).json = "not valid json";
    (cmd as any).source = writeTempSource("export default {}\n");
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

  test("returns error 2 when --json is non-object JSON (e.g., array)", async () => {
    const { ToolCreateCommand } = await import(
      `../../src/commands/tool.ts?t=${Date.now()}`
    );
    const cmd = new ToolCreateCommand();
    (cmd as any).file = undefined;
    (cmd as any).json = JSON.stringify([{ name: "x" }]);
    (cmd as any).source = writeTempSource("export default {}\n");
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

describe("tool update command", () => {
  beforeEach(setupConfigMock);

  test("sends PUT /api/v1/admin/tools/:name with body", async () => {
    const fakeTool = {
      id: 1,
      name: "calculator",
      title: "Calculator Pro",
      description: "Advanced math tool",
      isDefault: true,
    };
    const fetchMock = setupFetchMock(fakeTool);

    const filePath = writeTempYaml(
      "title: Calculator Pro\ndescription: Advanced math tool"
    );

    const { ToolUpdateCommand } = await import(
      `../../src/commands/tool.ts?t=${Date.now()}`
    );
    const cmd = new ToolUpdateCommand();
    (cmd as any).name = "calculator";
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
    expect(calls[0][0]).toContain("/api/v1/admin/tools/calculator");
    expect(calls[0][1].method).toBe("PUT");
    expect(calls[0][1].body.title).toBe("Calculator Pro");
    expect(calls[0][1].body.description).toBe("Advanced math tool");
  });

  test("returns error 2 without --file or --json", async () => {
    const { ToolUpdateCommand } = await import(
      `../../src/commands/tool.ts?t=${Date.now()}`
    );
    const cmd = new ToolUpdateCommand();
    (cmd as any).name = "calculator";
    (cmd as any).file = undefined;
    (cmd as any).json = undefined;
    (cmd as any).output = "yaml";

    const origErr = process.stderr.write;
    const errs: string[] = [];
    process.stderr.write = ((s: string) => {
      errs.push(s);
      return true;
    }) as any;

    const code = await cmd.execute();
    expect(code).toBe(2);

    process.stderr.write = origErr as any;
    expect(errs.join("")).toContain("必须提供 --file 或 --json");
  });

  test("returns error 2 with invalid --output", async () => {
    const { ToolUpdateCommand } = await import(
      `../../src/commands/tool.ts?t=${Date.now()}`
    );
    const cmd = new ToolUpdateCommand();
    (cmd as any).name = "calculator";
    (cmd as any).file = undefined;
    (cmd as any).json = JSON.stringify({ title: "x" });
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

// ── Delete command ──────────────────────────────────────────────

describe("tool delete command", () => {
  beforeEach(setupConfigMock);

  test("sends DELETE /api/v1/admin/tools/:name", async () => {
    const fetchMock = setupFetchMock({});

    const { ToolDeleteCommand } = await import(
      `../../src/commands/tool.ts?t=${Date.now()}`
    );
    const cmd = new ToolDeleteCommand();
    (cmd as any).name = "calculator";

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
    expect(calls[0][0]).toContain("/api/v1/admin/tools/calculator");
    expect(calls[0][1].method).toBe("DELETE");
    expect(logs.join("\n")).toContain("已删除");
  });
});

// ── Create command (custom tools) ──────────────────────────────

describe("tool create command (custom tools)", () => {
  beforeEach(setupConfigMock);

  test("fails without --source", async () => {
    // Mock ofetch too so the pre-fix code path can't hit the network
    // (the command should exit 2 before any request once implemented).
    setupFetchMock({});
    const { ToolCreateCommand } = await import(
      `../../src/commands/tool.ts?t=${Date.now()}`
    );
    const cli = new Cli({ binaryName: "zhub" });
    cli.register(ToolCreateCommand);
    const rc = await cli.run(["tool", "create", "--json", '{"name":"SayHello"}']);
    expect(rc).toBe(2);
  });

  test("returns error 2 when metadata lacks name", async () => {
    setupFetchMock({});
    const { ToolCreateCommand } = await import(
      `../../src/commands/tool.ts?t=${Date.now()}`
    );
    const cmd = new ToolCreateCommand();
    (cmd as any).file = undefined;
    (cmd as any).json = JSON.stringify({ title: "no name" });
    (cmd as any).source = writeTempSource("export default {}\n");
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
      expect(errs.join("")).toContain("name");
    } finally {
      process.stderr.write = origErr as any;
    }
  });
});

// ── Upload command ─────────────────────────────────────────────

describe("tool upload command", () => {
  beforeEach(setupConfigMock);

  test("PUTs multipart to /:name/file with --source", async () => {
    const fakeTool = {
      id: 1,
      name: "calculator",
      title: "Calculator",
      description: "Math tool",
      isDefault: false,
    };
    const fetchMock = setupFetchMock(fakeTool);
    const sourcePath = writeTempSource("export default { hi: 1 }\n");

    const { ToolUploadCommand } = await import(
      `../../src/commands/tool.ts?t=${Date.now()}`
    );
    const cmd = new ToolUploadCommand();
    (cmd as any).name = "calculator";
    (cmd as any).source = sourcePath;
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
    expect(calls[0][0]).toContain("/api/v1/admin/tools/calculator/file");
    expect(calls[0][1].method).toBe("PUT");
    expect(calls[0][1].body).toBeInstanceOf(FormData);
    expect(calls[0][1].body.get("name")).toBe(null);
    const file = calls[0][1].body.get("file");
    expect(file).toBeInstanceOf(Blob);
    expect((file as File).name).toBe("tool.ts");
    expect(logs.join("\n")).toContain("calculator");
  });

  test("returns error 2 without --source", async () => {
    const { ToolUploadCommand } = await import(
      `../../src/commands/tool.ts?t=${Date.now()}`
    );
    const cmd = new ToolUploadCommand();
    (cmd as any).name = "calculator";
    (cmd as any).source = undefined;
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
      expect(errs.join("")).toContain("--source");
    } finally {
      process.stderr.write = origErr as any;
    }
  });
});

// ── Download command ───────────────────────────────────────────

describe("tool download command", () => {
  beforeEach(setupConfigMock);

  test("GETs /:name/download and prints the url", async () => {
    const fetchMock = setupFetchMock({
      url: "https://cdn.local/tools/say.ts",
      expiresIn: 600,
    });

    const { ToolDownloadCommand } = await import(
      `../../src/commands/tool.ts?t=${Date.now()}`
    );
    const cmd = new ToolDownloadCommand();
    (cmd as any).name = "calculator";

    const logs: string[] = [];
    const origLog = console.log;
    const origErr = process.stderr.write;
    const errs: string[] = [];
    console.log = (s: string) => logs.push(s);
    process.stderr.write = ((s: string) => {
      errs.push(s);
      return true;
    }) as any;
    try {
      const code = await cmd.execute();
      expect(code).toBe(0);
    } finally {
      console.log = origLog;
      process.stderr.write = origErr as any;
    }

    expect(logs.join("\n")).toContain("https://cdn.local/tools/say.ts");
    expect(errs.join("")).toContain("600");
    const calls = fetchMock.mock.calls as any[][];
    expect(calls[0][0]).toContain("/api/v1/admin/tools/calculator/download");
    expect(calls[0][1].method).toBe("GET");
  });
});
