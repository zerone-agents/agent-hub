import { describe, test, expect } from "bun:test";
import { join } from "node:path";

describe("writeStdout", () => {
  // The test process mocks writeStdout via the preload (see bunfig.toml);
  // spawn a subprocess to exercise the real implementation. 256KB far
  // exceeds the 64KB pipe buffer, guarding against truncation regressions.
  test("delivers >64KB payloads through a non-TTY pipe without truncation", () => {
    const fixture = join(
      import.meta.dir,
      "fixtures",
      "stdout-large-harness.ts",
    );
    const result = Bun.spawnSync([process.execPath, fixture], {
      stdout: "pipe",
      stderr: "pipe",
    });
    const out = result.stdout.toString();
    expect(result.exitCode).toBe(0);
    expect(out.length).toBe(256 * 1024);
    expect(out.endsWith("END\n")).toBe(true);
  });
});
