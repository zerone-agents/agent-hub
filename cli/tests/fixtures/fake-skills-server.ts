import { join } from "node:path";

/**
 * Fake skills API server for the pipe-truncation test (#191).
 *
 * Runs as a standalone process; killed by the test after the CLI
 * pipeline completes. Serves a >64KB skills payload on a fixed port.
 */

const PORT = 18923;
const SKILL_COUNT = 100;

const skills = Array.from({ length: SKILL_COUNT }, (_, i) => ({
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

const responseBody = JSON.stringify({ data: skills, meta: {} });
if (Buffer.byteLength(responseBody, "utf-8") <= 65536) {
  process.stderr.write(
    `FATAL: payload too small (${Buffer.byteLength(responseBody)} bytes, need > 65536)\n`,
  );
  process.exit(1);
}

Bun.serve({
  port: PORT,
  fetch(req) {
    if (req.url.includes("/api/v1/skills")) {
      return new Response(responseBody, {
        headers: { "Content-Type": "application/json" },
      });
    }
    return new Response("Not found", { status: 404 });
  },
});

// Signal ready + keep running until killed.
process.stderr.write(`READY:${PORT}\n`);
process.on("SIGTERM", () => process.exit(0));
process.on("SIGINT", () => process.exit(0));