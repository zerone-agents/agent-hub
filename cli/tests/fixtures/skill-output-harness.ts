import { mock } from "bun:test";

const className = process.argv[2];
const output = process.argv[3];

const fetchMock = mock(() =>
  Promise.resolve({
    success: true,
    data: className === "SkillDownloadCommand"
      ? { url: "https://example.test/skill.zip", expiresIn: 60 }
      : [],
  })
);

mock.module("ofetch", () => ({
  ofetch: fetchMock,
  FetchError: class FetchError extends Error {},
}));
mock.module("../../src/config", () => ({
  getActiveProfile: mock(() =>
    Promise.resolve({ serverUrl: "https://test.local", token: "cli_test" })
  ),
}));

const commands = await import("../../src/commands/skill");
const CommandClass = (commands as Record<string, new () => any>)[className];
const cmd = new CommandClass();

const fieldsByClass: Record<string, Record<string, unknown>> = {
  SkillListCommand: { type: undefined },
  SkillGetCommand: { name: "test-skill" },
  SkillCreateCommand: { fromDir: "/nonexistent/skill-create", name: undefined },
  SkillUpdateCommand: { name: "test-skill", fromDir: "/nonexistent/skill-update" },
  SkillDownloadCommand: { name: "test-skill" },
};
Object.assign(cmd, fieldsByClass[className], { output });

const errors: string[] = [];
process.stderr.write = ((s: string) => {
  errors.push(s);
  return true;
}) as typeof process.stderr.write;
console.log = () => {};

const code = await cmd.execute();
process.stdout.write(JSON.stringify({ code, errors: errors.join(""), fetchCalls: fetchMock.mock.calls.length }));
