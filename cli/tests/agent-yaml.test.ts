import { afterEach, describe, expect, test } from "bun:test";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { AgentYamlError, parseAgentYaml } from "../src/agent-yaml";

const tempDirs: string[] = [];

function yamlFile(content: string): string {
  const dir = mkdtempSync(join(tmpdir(), "agent-yaml-"));
  tempDirs.push(dir);
  const file = join(dir, "agent.yaml");
  writeFileSync(file, content, "utf8");
  return file;
}

afterEach(() => {
  for (const dir of tempDirs.splice(0)) {
    rmSync(dir, { recursive: true, force: true });
  }
});

describe("parseAgentYaml normalization", () => {
  test("normalizes flat agent metadata", () => {
    const flatFile = yamlFile(`
id: researcher
desktop: true
isDefault: false
title:
  zh: 研究助手
  en: Research Assistant
description:
  zh: 研究分析
  en: Research and analysis
systemPrompt: |
  你是一名研究助手。
model: claude-sonnet-4-5
`);

    expect(parseAgentYaml(flatFile)).toEqual({
      name: "researcher",
      desktopEnabled: true,
      isDefault: false,
      config: {
        title: { zh: "研究助手", en: "Research Assistant" },
        description: { zh: "研究分析", en: "Research and analysis" },
        systemPrompt: "你是一名研究助手。\n",
        modelId: "claude-sonnet-4-5",
      },
    });
  });

  test("preserves native config metadata and future fields", () => {
    const nestedFile = yamlFile(`
id: researcher
config:
  title:
    zh: 研究助手
    en: Research Assistant
  description:
    zh: 研究分析
    en: Research and analysis
  systemPrompt: nested prompt
  modelId: claude-sonnet-4-5
  futureField: preserved
`);

    expect(parseAgentYaml(nestedFile)).toEqual({
      name: "researcher",
      config: {
        title: { zh: "研究助手", en: "Research Assistant" },
        description: { zh: "研究分析", en: "Research and analysis" },
        systemPrompt: "nested prompt",
        modelId: "claude-sonnet-4-5",
        futureField: "preserved",
      },
    });
  });

  test("supports legacy display name and model aliases", () => {
    const legacyFile = yamlFile(`
id: coder
name: 程序员
model: claude-sonnet-4-5
`);

    expect(parseAgentYaml(legacyFile)).toEqual({
      name: "coder",
      config: {
        title: { zh: "程序员" },
        modelId: "claude-sonnet-4-5",
      },
    });
  });

  test("forwards extension and unknown top-level metadata", () => {
    const file = yamlFile(`
id: researcher
extensions:
  control-panel:
    providerId: anthropic
futureField: preserved
`);

    expect(parseAgentYaml(file).config).toEqual({
      providerId: "anthropic",
      futureField: "preserved",
    });
  });

  test("allows update YAML to omit id and uses the positional name", () => {
    const file = yamlFile(`
config:
  systemPrompt: updated prompt
`);

    expect(parseAgentYaml(file, "researcher")).toEqual({
      name: "researcher",
      config: { systemPrompt: "updated prompt" },
    });
  });
});

describe("parseAgentYaml validation", () => {
  const cases: Array<[string, string, string, string?]> = [
    ["config is scalar", "id: researcher\nconfig: invalid\n", "config must be an object"],
    ["config.config exists", "id: researcher\nconfig:\n  config: {}\n", "config.config is not allowed"],
    ["flat and nested title", "id: researcher\ntitle:\n  zh: flat\nconfig:\n  title:\n    zh: nested\n", "title appears in both"],
    ["flat model and nested modelId", "id: researcher\nmodel: flat\nconfig:\n  modelId: nested\n", "modelId appears in both"],
    ["title is scalar", "id: researcher\ntitle: invalid\n", "title must be a string map"],
    ["description has non-string value", "id: researcher\ndescription:\n  zh: valid\n  en: 42\n", "description must be a string map"],
    ["missing id on create", "name: Researcher\n", "must contain an id"],
    ["non-string id on update", "id: 42\n", "id must be a string", "researcher"],
    ["id differs from expectedName", "id: writer\n", "does not match the command argument", "researcher"],
    ["desktop is not boolean", "id: researcher\ndesktop: yes\n", "desktop must be a boolean"],
    ["isDefault is not boolean", "id: researcher\nisDefault: 1\n", "isDefault must be a boolean"],
  ];

  for (const [name, yaml, message, expectedName] of cases) {
    test(name, () => {
      const file = yamlFile(yaml);
      expect(() => parseAgentYaml(file, expectedName)).toThrow(AgentYamlError);
      expect(() => parseAgentYaml(file, expectedName)).toThrow(message);
    });
  }

  test("rejects conflicts from forwarded metadata", () => {
    const file = yamlFile(`
id: researcher
config:
  providerId: native
extensions:
  control-panel:
    providerId: extension
`);
    expect(() => parseAgentYaml(file)).toThrow("providerId appears in both");
  });

  test("rejects unknown top-level metadata that conflicts with native config", () => {
    const file = yamlFile(`
id: researcher
config:
  futureField: native
futureField: top-level
`);
    expect(() => parseAgentYaml(file)).toThrow("futureField appears in both");
  });

  test("rejects extension metadata that conflicts with unknown top-level metadata", () => {
    const file = yamlFile(`
id: researcher
extensions:
  control-panel:
    futureField: extension
futureField: top-level
`);
    expect(() => parseAgentYaml(file)).toThrow("futureField appears in both");
  });
});

describe("parseAgentYaml guest flag", () => {
  test("top-level guest maps to guestEnabled state", () => {
    const file = yamlFile(`
id: guest-agent
guest: true
`);
    expect(parseAgentYaml(file)).toMatchObject({
      name: "guest-agent",
      guestEnabled: true,
      config: {},
    });
  });

  test("guest is not folded into config", () => {
    const file = yamlFile(`
id: guest-agent
guest: false
`);
    const parsed = parseAgentYaml(file);
    expect(parsed.config).not.toHaveProperty("guest");
    expect(parsed.guestEnabled).toBe(false);
  });

  test("guest must be a boolean", () => {
    const file = yamlFile(`
id: guest-agent
guest: yes
`);
    expect(() => parseAgentYaml(file)).toThrow("guest must be a boolean");
  });
});
