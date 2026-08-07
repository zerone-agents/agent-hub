import { mock } from "bun:test";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { Cli } from "clipanion";

let requestBody: Record<string, unknown> | undefined;
const fetchMock = mock((_url: string, options: { body?: Record<string, unknown> }) => {
  requestBody = options.body;
  return Promise.resolve({ success: true, data: { id: 1, name: "state-agent" } });
});

mock.module("ofetch", () => ({
  ofetch: fetchMock,
  FetchError: class FetchError extends Error {},
}));
mock.module("../../src/config", () => ({
  getActiveProfile: mock(() =>
    Promise.resolve({ serverUrl: "https://test.local", token: "cli_test" })
  ),
}));

const dir = mkdtempSync(join(tmpdir(), "agent-create-state-"));
const file = join(dir, "agent.yaml");
writeFileSync(file, "id: state-agent\nisDefault: true\n", "utf8");

const { AgentCreateCommand } = await import("../../src/commands/agent");
const cli = new Cli({ binaryName: "zhub" });
cli.register(AgentCreateCommand);
console.log = () => {};

try {
  const code = await cli.run(["agent", "create", "--file", file, ...process.argv.slice(2)]);
  process.stdout.write(JSON.stringify({ code, requestBody, fetchCalls: fetchMock.mock.calls.length }));
} finally {
  rmSync(dir, { recursive: true, force: true });
}
