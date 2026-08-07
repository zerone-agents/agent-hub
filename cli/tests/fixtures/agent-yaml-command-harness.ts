import { mock } from "bun:test";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

const className = process.argv[2];
const scenario = process.argv[3];
const fetchMock = mock(() => Promise.resolve({ success: true, data: {} }));

mock.module("ofetch", () => ({
  ofetch: fetchMock,
  FetchError: class FetchError extends Error {},
}));
mock.module("../../src/config", () => ({
  getActiveProfile: mock(() =>
    Promise.resolve({ serverUrl: "https://test.local", token: "cli_test" })
  ),
}));

const dir = mkdtempSync(join(tmpdir(), "agent-yaml-command-"));
const file = join(dir, "agent.yaml");
writeFileSync(
  file,
  scenario === "missing-id" ? "name: Missing id\n" : "id: writer\nconfig: {}\n",
  "utf8",
);

const commands = await import("../../src/commands/agent");
const CommandClass = (commands as Record<string, new () => any>)[className];
const cmd = new CommandClass();
Object.assign(cmd, {
  file,
  name: className === "AgentUpdateCommand" ? "researcher" : undefined,
  enabled: true,
  disabled: undefined,
  default: false,
  output: "yaml",
});

const errors: string[] = [];
process.stderr.write = ((chunk: string) => {
  errors.push(chunk);
  return true;
}) as typeof process.stderr.write;
console.log = () => {};

try {
  const code = await cmd.execute();
  process.stdout.write(JSON.stringify({
    code,
    errors: errors.join(""),
    fetchCalls: fetchMock.mock.calls.length,
  }));
} finally {
  rmSync(dir, { recursive: true, force: true });
}
