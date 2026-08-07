import { describe, test, expect, mock, beforeEach, afterEach } from "bun:test";
import { mkdtempSync, rmSync, writeFileSync, mkdirSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import * as realConfig from "../../src/config";

function setupConfigMock() {
  mock.module("../../src/config", () => ({
    ...realConfig,
    getActiveProfile: mock(() =>
      Promise.resolve({ serverUrl: "https://test.local", token: "cli_test" })
    ),
  }));
}

function makeTmpDir(): string {
  return mkdtempSync(join(tmpdir(), "zhub-skill-"));
}

function writeValidSkill(dir: string) {
  writeFileSync(
    join(dir, "SKILL.md"),
    `---
name: test-skill
description: A test skill for validation
---
# Test Skill
This is a test.`,
  );
}

// ── zip.ts: validateSkillDir ──────────────────────────────────

describe("validateSkillDir", () => {
  let tmpDir: string;

  beforeEach(() => {
    tmpDir = makeTmpDir();
  });

  afterEach(() => {
    rmSync(tmpDir, { recursive: true, force: true });
  });

  test("valid skill: SKILL.md with frontmatter name+description", async () => {
    writeValidSkill(tmpDir);
    const { validateSkillDir } = await import(`../../src/zip.ts?t=${Date.now()}`);
    const result = await validateSkillDir(tmpDir);
    expect(result.valid).toBe(true);
    expect(result.errors).toEqual([]);
  });

  test("missing SKILL.md fails", async () => {
    const { validateSkillDir } = await import(`../../src/zip.ts?t=${Date.now()}`);
    const result = await validateSkillDir(tmpDir);
    expect(result.valid).toBe(false);
    expect(result.errors.join("\n")).toMatch(/SKILL\.md/);
  });

  test("SKILL.md missing frontmatter description fails", async () => {
    writeFileSync(join(tmpDir, "SKILL.md"), `---\nname: test\n---\n# Test`);
    const { validateSkillDir } = await import(`../../src/zip.ts?t=${Date.now()}`);
    const result = await validateSkillDir(tmpDir);
    expect(result.valid).toBe(false);
    expect(result.errors.join("\n")).toMatch(/description/);
  });

  test("error is prefixed with the SKILL.md relative path", async () => {
    writeFileSync(join(tmpDir, "SKILL.md"), `---\nname: test\n---\n# Test`);
    const { validateSkillDir } = await import(`../../src/zip.ts?t=${Date.now()}`);
    const result = await validateSkillDir(tmpDir);
    expect(result.errors.join("\n")).toMatch(/^SKILL\.md:.*description/);
  });

  test("single nested SKILL.md is found and validated", async () => {
    mkdirSync(join(tmpDir, "commit"), { recursive: true });
    writeFileSync(
      join(tmpDir, "commit", "SKILL.md"),
      `---\nname: commit\ndescription: Commit changes\n---\n# Commit`,
    );
    const { validateSkillDir } = await import(`../../src/zip.ts?t=${Date.now()}`);
    const result = await validateSkillDir(tmpDir);
    expect(result.valid).toBe(true);
    expect(result.skills).toHaveLength(1);
    expect(result.skills[0].path).toBe("commit/SKILL.md");
    expect(result.skills[0].name).toBe("commit");
    expect(result.skills[0].description).toBe("Commit changes");
  });

  test("bundle of multiple nested SKILL.md at various depths all valid", async () => {
    mkdirSync(join(tmpDir, "commit"), { recursive: true });
    mkdirSync(join(tmpDir, "team", "review"), { recursive: true });
    mkdirSync(join(tmpDir, "team", "sub", "deploy"), { recursive: true });
    writeFileSync(
      join(tmpDir, "commit", "SKILL.md"),
      `---\nname: commit\ndescription: Commit changes\n---\n# Commit`,
    );
    writeFileSync(
      join(tmpDir, "team", "review", "SKILL.md"),
      `---\nname: review\ndescription: Review code\n---\n# Review`,
    );
    writeFileSync(
      join(tmpDir, "team", "sub", "deploy", "SKILL.md"),
      `---\nname: deploy\ndescription: Deploy stuff\n---\n# Deploy`,
    );

    const { validateSkillDir } = await import(`../../src/zip.ts?t=${Date.now()}`);
    const result = await validateSkillDir(tmpDir);
    expect(result.valid).toBe(true);
    expect(result.skills).toHaveLength(3);
    // Sorted by path
    expect(result.skills.map((s: { path: string }) => s.path)).toEqual([
      "commit/SKILL.md",
      "team/review/SKILL.md",
      "team/sub/deploy/SKILL.md",
    ]);
    expect(result.skills.map((s: { name?: string }) => s.name)).toEqual(["commit", "review", "deploy"]);
  });

  test("bundle with one invalid SKILL.md fails and lists that file's path", async () => {
    mkdirSync(join(tmpDir, "commit"), { recursive: true });
    mkdirSync(join(tmpDir, "review"), { recursive: true });
    writeFileSync(
      join(tmpDir, "commit", "SKILL.md"),
      `---\nname: commit\ndescription: Commit changes\n---\n# Commit`,
    );
    // review missing description
    writeFileSync(
      join(tmpDir, "review", "SKILL.md"),
      `---\nname: review\n---\n# Review`,
    );

    const { validateSkillDir } = await import(`../../src/zip.ts?t=${Date.now()}`);
    const result = await validateSkillDir(tmpDir);
    expect(result.valid).toBe(false);
    // Error specifically names the broken file
    expect(result.errors.join("\n")).toMatch(/^review\/SKILL\.md:.*description/);
    // commit/SKILL.md is NOT in errors
    expect(result.errors.join("\n")).not.toMatch(/commit\/SKILL\.md/);
    // Both entries still surfaced in skills[] (the invalid one has no description)
    expect(result.skills).toHaveLength(2);
    const review = result.skills.find((s: { path: string }) => s.path === "review/SKILL.md");
    expect(review?.name).toBe("review");
    expect(review?.description).toBeUndefined();
  });

  test("bundle with multiple broken SKILL.md lists all errors with paths", async () => {
    mkdirSync(join(tmpDir, "a"), { recursive: true });
    mkdirSync(join(tmpDir, "b"), { recursive: true });
    // a: missing name
    writeFileSync(join(tmpDir, "a", "SKILL.md"), `---\ndescription: A skill\n---\n# A`);
    // b: missing description
    writeFileSync(join(tmpDir, "b", "SKILL.md"), `---\nname: b\n---\n# B`);

    const { validateSkillDir } = await import(`../../src/zip.ts?t=${Date.now()}`);
    const result = await validateSkillDir(tmpDir);
    expect(result.valid).toBe(false);
    expect(result.errors).toEqual([
      "a/SKILL.md: frontmatter 缺少 name 字段",
      "b/SKILL.md: frontmatter 缺少 description 字段",
    ]);
  });

  test("SKILL.md inside .git / node_modules / dist is excluded", async () => {
    mkdirSync(join(tmpDir, ".git"), { recursive: true });
    mkdirSync(join(tmpDir, "node_modules"), { recursive: true });
    mkdirSync(join(tmpDir, "real"), { recursive: true });
    writeFileSync(
      join(tmpDir, ".git", "SKILL.md"),
      `---\nname: ghost\ndescription: should be excluded\n---\n# Ghost`,
    );
    writeFileSync(
      join(tmpDir, "node_modules", "SKILL.md"),
      `---\nname: dep\ndescription: should be excluded\n---\n# Dep`,
    );
    writeFileSync(
      join(tmpDir, "real", "SKILL.md"),
      `---\nname: real\ndescription: real skill\n---\n# Real`,
    );

    const { validateSkillDir } = await import(`../../src/zip.ts?t=${Date.now()}`);
    const result = await validateSkillDir(tmpDir);
    expect(result.valid).toBe(true);
    expect(result.skills.map((s: { path: string }) => s.path)).toEqual(["real/SKILL.md"]);
  });

  test("missing frontmatter block (no ---) fails with path prefix", async () => {
    writeFileSync(join(tmpDir, "SKILL.md"), `# No frontmatter\nJust body`);
    const { validateSkillDir } = await import(`../../src/zip.ts?t=${Date.now()}`);
    const result = await validateSkillDir(tmpDir);
    expect(result.valid).toBe(false);
    expect(result.errors.join("\n")).toMatch(/^SKILL\.md:.*frontmatter/);
  });
});

// ── zip.ts: packDir ───────────────────────────────────────────

describe("packDir", () => {
  let tmpDir: string;

  beforeEach(() => {
    tmpDir = makeTmpDir();
  });

  afterEach(() => {
    rmSync(tmpDir, { recursive: true, force: true });
  });

  test("excludes hidden directories and packs files at archive root (no wrapper)", async () => {
    writeValidSkill(tmpDir);
    mkdirSync(join(tmpDir, ".git"), { recursive: true });
    mkdirSync(join(tmpDir, "node_modules"), { recursive: true });
    writeFileSync(join(tmpDir, ".git", "config"), "hidden");
    writeFileSync(join(tmpDir, "node_modules", "pkg.json"), "{}");

    const { packDir } = await import(`../../src/zip.ts?t=${Date.now()}`);
    const buf = await packDir(tmpDir, "test-skill");

    expect(buf.length).toBeGreaterThan(0);

    // Verify zip contents
    const JSZip = (await import("jszip")).default;
    const zip = await JSZip.loadAsync(buf);
    const entries: string[] = [];
    zip.forEach((path) => entries.push(path));

    // Files should be at archive root, NOT wrapped under "test-skill/".
    // The deployer extracts under skillsDir/<hub-name>/, so a wrapper here
    // would produce a redundant doubled path.
    expect(entries.some((e) => e === "SKILL.md")).toBe(true);
    expect(entries.some((e) => e.startsWith("test-skill/"))).toBe(false);
    expect(entries.some((e) => e.includes(".git"))).toBe(false);
    expect(entries.some((e) => e.includes("node_modules"))).toBe(false);
  });
});

// ── skill list command ────────────────────────────────────────

describe("skill list command", () => {
  beforeEach(setupConfigMock);

  test("prints table with skill names", async () => {
    const fakeSkills = [
      { id: 1, name: "code-review", title: "代码审查", type: "expert", fileSize: 51200, fileHash: "abc123def456" },
      { id: 2, name: "doc-writer", title: "文档撰写", type: "community", fileSize: 0 },
    ];
    mock.module("ofetch", () => ({
      ofetch: mock(() => Promise.resolve({ success: true, data: fakeSkills })),
      FetchError: class FetchError extends Error {},
    }));

    const { SkillListCommand } = await import(`../../src/commands/skill.ts?t=${Date.now()}`);
    const cmd = new SkillListCommand();
    (cmd as any).output = "table";
    (cmd as any).type = undefined;

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
    expect(out).toContain("code-review");
    expect(out).toContain("doc-writer");
  });

  test("--output json returns skill array", async () => {
    mock.module("ofetch", () => ({
      ofetch: mock(() =>
        Promise.resolve({ success: true, data: [{ id: 1, name: "test-skill" }] })
      ),
      FetchError: class FetchError extends Error {},
    }));

    const { SkillListCommand } = await import(`../../src/commands/skill.ts?t=${Date.now()}`);
    const cmd = new SkillListCommand();
    (cmd as any).output = "json";
    (cmd as any).type = undefined;

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
    expect(parsed.data[0].name).toBe("test-skill");
  });
});

// ── output validation ───────────────────────────────────────────────

describe("skill commands reject invalid output", () => {
  const harness = join(import.meta.dir, "../fixtures/skill-output-harness.ts");

  function runHarness(className: string, output: string) {
    const result = Bun.spawnSync([process.execPath, harness, className, output], {
      cwd: join(import.meta.dir, "../.."),
      stdout: "pipe",
      stderr: "pipe",
    });
    expect(result.exitCode).toBe(0);
    return JSON.parse(result.stdout.toString()) as {
      code: number;
      errors: string;
      fetchCalls: number;
    };
  }

  test("isolated harness observes the API call on a valid list command", () => {
    const result = runHarness("SkillListCommand", "table");
    expect(result.code).toBe(0);
    expect(result.fetchCalls).toBe(1);
  });

  test.each([
    ["list", "SkillListCommand"],
    ["get", "SkillGetCommand"],
    ["create", "SkillCreateCommand"],
    ["update", "SkillUpdateCommand"],
    ["download", "SkillDownloadCommand"],
  ])("%s rejects invalid output before side effects", (_name, className) => {
    const result = runHarness(className, "xml");
    expect(result.code).toBe(2);
    expect(result.errors).toContain("--output");
    expect(result.fetchCalls).toBe(0);
  });
});

// ── skill delete command ──────────────────────────────────────

describe("skill update command", () => {
  beforeEach(setupConfigMock);

  test("valid output with an invalid directory fails validation without an API call", () => {
    const harness = join(import.meta.dir, "../fixtures/skill-output-harness.ts");
    const result = Bun.spawnSync(
      [process.execPath, harness, "SkillUpdateCommand", "json"],
      {
        cwd: join(import.meta.dir, "../.."),
        stdout: "pipe",
        stderr: "pipe",
      },
    );

    expect(result.exitCode).toBe(0);
    const observed = JSON.parse(result.stdout.toString()) as {
      code: number;
      errors: string;
      fetchCalls: number;
    };
    expect(observed.code).toBe(2);
    expect(observed.errors).toContain("SKILL.md");
    expect(observed.fetchCalls).toBe(0);
  });

  test("updates a skill using the positional name for the archive layout", async () => {
    const tmpDir = makeTmpDir();
    writeValidSkill(tmpDir);
    const fetchMock = mock(() =>
      Promise.resolve({ success: true, data: { id: 1, name: "published/skill" } })
    );
    mock.module("ofetch", () => ({
      ofetch: fetchMock,
      FetchError: class FetchError extends Error {},
    }));

    const { SkillUpdateCommand } = await import(`../../src/commands/skill.ts?t=${Date.now()}`);
    const cmd = new SkillUpdateCommand();
    (cmd as any).name = "published/skill";
    (cmd as any).fromDir = tmpDir;
    (cmd as any).title = undefined;
    (cmd as any).titleEn = undefined;
    (cmd as any).description = undefined;
    (cmd as any).descriptionEn = undefined;
    (cmd as any).type = undefined;
    (cmd as any).output = "json";
    const logs: string[] = [];
    const errors: string[] = [];
    const origLog = console.log;
    const origWrite = process.stderr.write;
    console.log = (s: string) => logs.push(s);
    process.stderr.write = ((s: string) => {
      errors.push(s);
      return true;
    }) as typeof process.stderr.write;
    try {
      const code = await cmd.execute();
      expect(code).toBe(0);
      const calls = fetchMock.mock.calls as any[][];
      expect(calls[0][0]).toContain("/api/v1/admin/skills/published%2Fskill");
      expect(calls[0][1].method).toBe("PUT");
      expect(calls[0][1].body).toBeInstanceOf(FormData);
      const body = calls[0][1].body as FormData;
      expect(body.get("name")).toBeNull();
      expect(body.get("title")).toBeNull();
      expect(body.get("titleEn")).toBeNull();
      // --description not passed and frontmatter fallback removed, so the
      // field is omitted entirely (client skips falsy values).
      expect(body.get("description")).toBeNull();
      expect(body.get("descriptionEn")).toBeNull();
      expect(body.get("type")).toBeNull();
      const file = body.get("file") as File;
      const JSZip = (await import("jszip")).default;
      const zip = await JSZip.loadAsync(await file.arrayBuffer());
      // No wrapper: SKILL.md is at archive root, not under "published/skill/".
      expect(zip.file("SKILL.md")).not.toBeNull();
      expect(JSON.parse(logs[0]).data.name).toBe("published/skill");
      expect(errors.join("")).toContain("已更新 skill：published/skill");
    } finally {
      console.log = origLog;
      process.stderr.write = origWrite;
      rmSync(tmpDir, { recursive: true, force: true });
    }
  });

  test("skill metadata sends explicit multilingual fields on update", async () => {
    const tmpDir = makeTmpDir();
    writeValidSkill(tmpDir);
    const fetchMock = mock(() =>
      Promise.resolve({ success: true, data: { id: 1, name: "test-skill" } })
    );
    mock.module("ofetch", () => ({
      ofetch: fetchMock,
      FetchError: class FetchError extends Error {},
    }));

    const { SkillUpdateCommand } = await import(`../../src/commands/skill.ts?t=${Date.now()}`);
    const cmd = new SkillUpdateCommand();
    (cmd as any).name = "test-skill";
    (cmd as any).fromDir = tmpDir;
    (cmd as any).title = "中文标题";
    (cmd as any).titleEn = "English title";
    (cmd as any).description = "显式中文描述";
    (cmd as any).descriptionEn = "English description";
    (cmd as any).type = "expert";
    (cmd as any).output = "json";

    try {
      expect(await cmd.execute()).toBe(0);
      const body = (fetchMock.mock.calls as any[][])[0][1].body as FormData;
      expect(body.get("title")).toBe("中文标题");
      expect(body.get("titleEn")).toBe("English title");
      expect(body.get("description")).toBe("显式中文描述");
      expect(body.get("descriptionEn")).toBe("English description");
    } finally {
      rmSync(tmpDir, { recursive: true, force: true });
    }
  });
});

describe("skill create command", () => {
  beforeEach(setupConfigMock);

  test("skill metadata sends explicit multilingual fields on create", async () => {
    const tmpDir = makeTmpDir();
    writeValidSkill(tmpDir);
    const fetchMock = mock(() =>
      Promise.resolve({ success: true, data: { id: 1, name: "test-skill" } })
    );
    mock.module("ofetch", () => ({
      ofetch: fetchMock,
      FetchError: class FetchError extends Error {},
    }));

    const { SkillCreateCommand } = await import(`../../src/commands/skill.ts?t=${Date.now()}`);
    const cmd = new SkillCreateCommand();
    (cmd as any).fromDir = tmpDir;
    (cmd as any).name = "test-skill";
    (cmd as any).title = "中文标题";
    (cmd as any).titleEn = "English title";
    (cmd as any).description = "显式中文描述";
    (cmd as any).descriptionEn = "English description";
    (cmd as any).type = undefined;
    (cmd as any).output = "json";

    try {
      expect(await cmd.execute()).toBe(0);
      const body = (fetchMock.mock.calls as any[][])[0][1].body as FormData;
      expect(body.get("title")).toBe("中文标题");
      expect(body.get("titleEn")).toBe("English title");
      expect(body.get("description")).toBe("显式中文描述");
      expect(body.get("descriptionEn")).toBe("English description");
    } finally {
      rmSync(tmpDir, { recursive: true, force: true });
    }
  });

  test("requires --name even when the directory is valid", async () => {
    const tmpDir = makeTmpDir();
    writeValidSkill(tmpDir);
    const errors: string[] = [];
    const origWrite = process.stderr.write;
    process.stderr.write = ((s: string) => {
      errors.push(s);
      return true;
    }) as typeof process.stderr.write;
    try {
      const { SkillCreateCommand } = await import(`../../src/commands/skill.ts?t=${Date.now()}`);
      const cmd = new SkillCreateCommand();
      (cmd as any).fromDir = tmpDir;
      (cmd as any).name = undefined;
      (cmd as any).output = "yaml";

      expect(await cmd.execute()).toBe(2);
      // Name check fires before directory validation now — directory is valid
      // but the command still fails because --name is required.
      expect(errors.join("")).toContain("必须用 --name");
      // Validation did NOT run (no "SKILL.md" error, since dir is valid).
      expect(errors.join("")).not.toContain("SKILL.md");
    } finally {
      process.stderr.write = origWrite;
      rmSync(tmpDir, { recursive: true, force: true });
    }
  });
});

// ── skill delete command ──

describe("skill delete command", () => {
  beforeEach(setupConfigMock);

  test("sends DELETE /admin/skills/:name", async () => {
    const fetchMock = mock(() => Promise.resolve({ success: true }));
    mock.module("ofetch", () => ({
      ofetch: fetchMock,
      FetchError: class FetchError extends Error {},
    }));

    const { SkillDeleteCommand } = await import(`../../src/commands/skill.ts?t=${Date.now()}`);
    const cmd = new SkillDeleteCommand();
    (cmd as any).name = "old-skill";

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
    expect(calls[0][0]).toContain("/api/v1/admin/skills/old-skill");
    expect(calls[0][1].method).toBe("DELETE");
    expect(logs.join("\n")).toContain("已删除");
  });
});

// ── skill download command ────────────────────────────────────

describe("skill download command", () => {
  beforeEach(setupConfigMock);

  test("prints presigned URL", async () => {
    mock.module("ofetch", () => ({
      ofetch: mock(() =>
        Promise.resolve({
          success: true,
          data: { url: "https://oss.example.com/skills/test.zip?token=xxx", expiresIn: 3600 },
        })
      ),
      FetchError: class FetchError extends Error {},
    }));

    const { SkillDownloadCommand } = await import(`../../src/commands/skill.ts?t=${Date.now()}`);
    const cmd = new SkillDownloadCommand();
    (cmd as any).name = "test-skill";
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
    expect(out).toContain("https://oss.example.com/skills/test.zip");
    expect(out).toContain("3600");
  });
});
