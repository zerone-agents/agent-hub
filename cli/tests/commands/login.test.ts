import { describe, test, expect, beforeEach, afterEach } from "bun:test";
import { mkdtemp, rm } from "node:fs/promises";
import { join } from "node:path";
import { tmpdir } from "node:os";

describe("login command", () => {
  let tmpHome: string;
  let origHome: string;

  beforeEach(async () => {
    tmpHome = await mkdtemp(join(tmpdir(), "zhub-login-"));
    origHome = process.env.HOME!;
    process.env.HOME = tmpHome;
  });

  afterEach(async () => {
    process.env.HOME = origHome;
    await rm(tmpHome, { recursive: true, force: true });
  });

  test("rejects token without cli_ prefix", async () => {
    const { LoginCommand } = await import(`../../src/commands/login.ts?t=${Date.now()}`);
    const cmd = new LoginCommand();
    (cmd as any).url = "https://x";
    (cmd as any).token = "fake_no_prefix";
    (cmd as any).profile = "default";

    const result = await cmd.execute();
    expect(result).toBe(2); // exit code 2 = BAD_REQUEST
  });

  test("writes config on valid token", async () => {
    const { LoginCommand } = await import(`../../src/commands/login.ts?t=${Date.now()}`);
    const cmd = new LoginCommand();
    (cmd as any).url = "https://hub.example.com";
    (cmd as any).token = "cli_abc123";
    (cmd as any).profile = "default";

    const result = await cmd.execute();
    expect(result).toBe(0);

    const { loadConfig } = await import(`../../src/config.ts?t=${Date.now()}`);
    const cfg = await loadConfig();
    expect(cfg.currentProfile).toBe("default");
    expect(cfg.profiles.default.serverUrl).toBe("https://hub.example.com");
    expect(cfg.profiles.default.token).toBe("cli_abc123");
  });
});
