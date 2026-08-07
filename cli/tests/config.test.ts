import { describe, test, expect, beforeEach, afterEach } from "bun:test";
import { mkdtemp, rm, readFile, stat } from "node:fs/promises";
import { join } from "node:path";
import { tmpdir } from "node:os";

describe("config", () => {
  let tmpHome: string;
  let origHome: string;

  beforeEach(async () => {
    tmpHome = await mkdtemp(join(tmpdir(), "zhub-test-"));
    origHome = process.env.HOME!;
    process.env.HOME = tmpHome;
  });

  afterEach(async () => {
    process.env.HOME = origHome;
    await rm(tmpHome, { recursive: true, force: true });
  });

  test("loadConfig returns default when file missing", async () => {
    const { loadConfig } = await import(`../src/config.ts?t=${Date.now()}`);
    const cfg = await loadConfig();
    expect(cfg).toEqual({ currentProfile: "default", profiles: {} });
  });

  test("saveConfig + loadConfig roundtrip", async () => {
    const { saveConfig, loadConfig } = await import(`../src/config.ts?t=${Date.now()}`);
    await saveConfig({
      currentProfile: "prod",
      profiles: { prod: { serverUrl: "https://x", token: "cli_abc" } },
    });
    const cfg = await loadConfig();
    expect(cfg.currentProfile).toBe("prod");
    expect(cfg.profiles.prod.token).toBe("cli_abc");
  });

  test("saveConfig sets file permission 0o600", async () => {
    const { saveConfig } = await import(`../src/config.ts?t=${Date.now()}`);
    await saveConfig({ currentProfile: "default", profiles: {} });
    const st = await stat(join(tmpHome, ".zhub", "config.yaml"));
    expect(st.mode & 0o777).toBe(0o600);
  });
});
