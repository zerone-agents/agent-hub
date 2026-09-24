import { mock } from "bun:test";

// Command output is written synchronously through `writeStdout`
// (src/output/stdout.ts) to avoid Bun truncating large piped output, but
// command tests capture output by temporarily replacing `console.log`.
// Route `writeStdout` back through `console.log` in the test process so the
// existing capture helpers keep working. The real synchronous path is still
// exercised end-to-end by help-version.test.ts, which spawns the CLI as a
// subprocess.
mock.module("../src/output/stdout.ts", () => ({
  writeStdout: (text: string) => {
    console.log(text.endsWith("\n") ? text.slice(0, -1) : text);
  },
}));