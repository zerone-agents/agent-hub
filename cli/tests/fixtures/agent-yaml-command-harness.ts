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
// 模块里除 Command 子类外还有普通函数导出（如 resolveRuntimeUrl），
// 直接 as Record 不满足重叠检查，须经 unknown 双重断言（className 由
// argv 传入，始终是 Command 类名，不会命中函数导出）。
const CommandClass = (commands as unknown as Record<string, new () => any>)[className];
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
