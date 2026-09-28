import { writeSync } from "node:fs";

/**
 * Write text to stdout, looping until the full payload is flushed.
 *
 * Replacement for `console.log` on the command-output path: with the async
 * console path, a large piped payload was reproducibly truncated at exactly
 * the 65536-byte pipe buffer boundary in one consumer context (Python
 * subprocess capture), while the same payload through a shell pipe was
 * complete. The exact Bun-internal trigger was not isolated; writing
 * synchronously removes the dependency on that path.
 *
 * EAGAIN: stdout is non-blocking when it is a pipe, so a write into a full
 * pipe is retried (short sleep) until the consumer drains it.
 * EPIPE: the consumer closed the pipe (e.g. `| head`) — stop quietly,
 * matching the behaviour before this change.
 */
export function writeStdout(text: string): void {
  const buf = Buffer.from(text, "utf-8");
  let offset = 0;
  while (offset < buf.length) {
    try {
      offset += writeSync(1, buf, offset, buf.length - offset);
    } catch (err) {
      const code = (err as { code?: string } | undefined)?.code;
      const message = String((err as Error | undefined)?.message ?? err);
      if (code === "EAGAIN" || message.includes("EAGAIN")) {
        Bun.sleepSync(5);
        continue;
      }
      if (code === "EPIPE" || message.includes("EPIPE")) {
        return;
      }
      throw err;
    }
  }
}
