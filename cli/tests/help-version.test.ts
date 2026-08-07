import { describe, test, expect } from "bun:test";
import { resolve, dirname } from "node:path";

const CLI_DIR = dirname(import.meta.dir);

async function runZhub(args: string[]): Promise<{ exitCode: number; stdout: string; stderr: string }> {
  const proc = Bun.spawn(["bun", "run", resolve(CLI_DIR, "src/index.ts"), ...args], {
    cwd: CLI_DIR,
    stdout: "pipe",
    stderr: "pipe",
  });
  const [stdoutBuf, stderrBuf] = await Promise.all([new Response(proc.stdout).arrayBuffer(), new Response(proc.stderr).arrayBuffer()]);
  const exitCode = await proc.exited;
  return {
    exitCode,
    stdout: new TextDecoder().decode(stdoutBuf),
    stderr: new TextDecoder().decode(stderrBuf),
  };
}

describe("zhub global flags", () => {
  test("--help prints usage and exits 0", async () => {
    const { exitCode, stdout } = await runZhub(["--help"]);
    expect(exitCode).toBe(0);
    expect(stdout).toContain("zhub agent list");
    expect(stdout).toContain("zhub tool list");
    expect(stdout).toContain("zhub mcp list");
    expect(stdout).toContain("General commands");
  });

  test("-h prints usage and exits 0", async () => {
    const { exitCode, stdout } = await runZhub(["-h"]);
    expect(exitCode).toBe(0);
    expect(stdout).toContain("zhub <command>");
  });

  test("--version prints version and exits 0", async () => {
    const { exitCode, stdout } = await runZhub(["--version"]);
    expect(exitCode).toBe(0);
    expect(stdout.trim()).toMatch(/^\d+\.\d+\.\d+$/);
  });

  test("-v prints version and exits 0", async () => {
    const { exitCode, stdout } = await runZhub(["-v"]);
    expect(exitCode).toBe(0);
    expect(stdout.trim()).toMatch(/^\d+\.\d+\.\d+$/);
  });
});

describe("zhub skill commands", () => {
  test("skill update is registered", async () => {
    const result = await runZhub(["skill", "update", "demo"]);
    expect(result.exitCode).toBe(1);
    expect(result.stderr).toContain("必须提供 --from-dir");
    expect(result.stderr).not.toContain("Command not found");
  });

  test.each(["create", "update"])("skill %s help documents metadata flags", async (command) => {
    const result = await runZhub(["skill", command, "--help"]);
    expect(result.exitCode).toBe(0);
    expect(result.stdout).toContain("--title-en");
    expect(result.stdout).toMatch(/--description(?:\s|$)/);
    expect(result.stdout).toContain("--description-en");
  });
});

describe("zhub agent command help", () => {
  test("agent create documents platform flags", async () => {
    const result = await runZhub(["agent", "create", "--help"]);
    expect(result.exitCode).toBe(0);
    expect(result.stdout).toContain("--desktop");
    expect(result.stdout).toContain("--mobile");
    expect(result.stdout).toContain("--default");
    expect(result.stdout).toContain("--no-default");
  });
});
