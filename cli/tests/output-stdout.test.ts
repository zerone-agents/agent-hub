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

  // Stopped-reader variant: the consumer sleeps before draining, so the pipe
  // stays full and writeStdout's EAGAIN retry loop runs for the whole sleep
  // window (the fast spawnSync test may never trigger EAGAIN because Bun
  // consumers drain eagerly). Not discriminating against the old
  // console.log path — the original truncation only reproduced with the
  // real CLI (see #191 for the discriminating harness) — but it
  // deterministically covers the retry loop that is the heart of the fix.
  test("delivers the full payload while the consumer stops reading (EAGAIN retry loop)", () => {
    const fixture = join(import.meta.dir, "fixtures", "stdout-large-harness.ts");
    const script = `bun ${JSON.stringify(fixture)} | { sleep 2; cat; }`;
    const result = Bun.spawnSync(["bash", "-c", script], {
      stdout: "pipe",
      stderr: "pipe",
    });
    const out = result.stdout.toString();
    expect(result.exitCode).toBe(0);
    expect(out.length).toBe(256 * 1024);
    expect(out.endsWith("END\n")).toBe(true);
  });
});
