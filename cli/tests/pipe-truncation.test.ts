import { describe, test, expect, beforeAll, afterAll } from "bun:test";
import { mkdtempSync, rmSync, mkdirSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { stringify } from "yaml";

/**
 * Discriminating regression test for piped-output truncation (#191).
 *
 * Architecture: the fake API server runs in a separate subprocess
 * (fake-skills-server.ts) so Bun.spawnSync in this test cannot block
 * its event loop. The CLI runs through a stopped-reader bash pipeline
 * — the only consumer shape that reproduces the original truncation
 * with the real CLI (bare fixtures pass because Bun consumers drain
 * eagerly; see #190 review for the discriminating analysis).
 */
describe("pipe truncation (discriminating)", () => {
  let serverProc: ReturnType<typeof Bun.spawn> | undefined;
  let tmpHome: string | undefined;
  let expectedByteLength: number;

  beforeAll(async () => {
    const serverScript = join(
      import.meta.dir,
      "fixtures",
      "fake-skills-server.ts",
    );
    const PORT = 18923;

    // Start the fake server as a background process.
    serverProc = Bun.spawn([process.execPath, serverScript], {
      stdout: "pipe",
      stderr: "pipe",
    });

    // Wait for the server to accept connections (poll health check).
    let serverReady = false;
    for (let i = 0; i < 20; i++) {
      await new Promise((r) => setTimeout(r, 100));
      try {
        const res = await fetch(`http://127.0.0.1:${PORT}/api/v1/skills`);
        if (res.ok) {
          serverReady = true;
          break;
        }
      } catch {
        // Server not ready yet, keep polling.
      }
    }
    if (!serverReady) {
      throw new Error("Fake server failed to start within 2s");
    }

    // Compute expected payload size (100 skills, same as server fixture).
    const probeSkills = Array.from({ length: 100 }, (_, i) => ({
      id: i + 1,
      name: `discriminating-skill-${i}`,
      type: "community",
      title: `Discriminating Skill ${i}`,
      titleEn: "",
      description:
        `A skill description long enough to push the total JSON payload ` +
        `well past the 65536-byte pipe buffer boundary. Entry #${i}. ` +
        `This payload exercises the real CLI stdout write path under ` +
        `pipe backpressure, which is the only consumer shape that ` +
        `reproduces the original truncation (bare fixtures pass because ` +
        `Bun consumers drain eagerly — see #190 review). `.repeat(2),
      descriptionEn: "",
      url: `https://example.test/skills/discriminating-skill-${i}.zip`,
      fileHash: "a".repeat(64),
      fileSize: 12345 + i,
      createdAt: "2026-01-01T00:00:00Z",
      updatedAt: "2026-01-01T00:00:00Z",
    }));
    expectedByteLength = Buffer.byteLength(
      JSON.stringify({ data: probeSkills, meta: {} }),
      "utf-8",
    );

    // Set up temp HOME with CLI config.
    tmpHome = mkdtempSync(join(tmpdir(), "zhub-e2e-"));
    mkdirSync(join(tmpHome, ".zhub"), { recursive: true });
    writeFileSync(
      join(tmpHome, ".zhub", "config.yaml"),
      stringify({
        currentProfile: "default",
        profiles: {
          default: {
            serverUrl: `http://127.0.0.1:${PORT}`,
            token: "cli_test_token",
          },
        },
      }),
      "utf-8",
    );
  });

  afterAll(() => {
    serverProc?.kill();
    if (tmpHome) rmSync(tmpHome, { recursive: true, force: true });
  });

  test(
    "real CLI delivers >64KB through a stopped-reader pipe without truncation",
    () => {
      const cliPath = join(import.meta.dir, "..", "src", "index.ts");

      // Stopped-reader pipeline: consumer sleeps before draining, so the
      // pipe stays full for the sleep window (the discriminating shape).
      // The fake server runs in a separate process, so Bun.spawnSync here
      // does not block it.
      const script =
        `HOME=${tmpHome} bun ${JSON.stringify(cliPath)} ` +
        `skill list --output json 2>/dev/null | { sleep 1; cat; }`;

      const result = Bun.spawnSync(["bash", "-c", script], {
        stdout: "pipe",
        stderr: "pipe",
      });

      const out = result.stdout.toString();
      const outBytes = Buffer.byteLength(out, "utf-8");

      // Full payload arrived — comfortably above the 64KB pipe buffer
      // (the exact byte count depends on CLI pretty-printing, so assert
      // the discriminating properties rather than an exact match).
      expect(outBytes).toBeGreaterThan(65536);
      expect(result.exitCode).toBe(0);

      // JSON is valid (truncation at the pipe buffer breaks parsing).
      // CLI wraps the API response: {"data": {data: [...], meta: {}}, "meta": {}}
      const parsed = JSON.parse(out);
      expect(parsed.data.data).toHaveLength(100);
      expect(parsed.data.data[0].name).toBe("discriminating-skill-0");
      expect(parsed.data.data[99].name).toBe("discriminating-skill-99");
    },
    30_000,
  );
});