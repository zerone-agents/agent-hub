import { writeSync } from "node:fs";

/**
 * Write text to stdout, looping until the full payload is flushed.
 *
 * Why not `console.log`: Bun's async stdout path can drop the tail of large
 * writes when stdout is a pipe (reproduced with `skill list --output json`
 * > 64KB piped from a non-TTY parent: output cut exactly at the 65536-byte
 * pipe buffer boundary).
 *
 * Why the EAGAIN retry: when stdout is a pipe it is non-blocking, so a
 * write into a full pipe fails with EAGAIN until the consumer drains it.
 * Retrying with a short sleep delivers the whole payload.
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
      throw err;
    }
  }
}