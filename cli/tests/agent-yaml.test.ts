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
    ["config is scalar", "id: researcher\nconfig: invalid\n", "config 必须是对象"],
    ["config.config exists", "id: researcher\nconfig:\n  config: {}\n", "不允许 config.config"],
    ["flat and nested title", "id: researcher\ntitle:\n  zh: flat\nconfig:\n  title:\n    zh: nested\n", "title 同时出现在"],
    ["flat model and nested modelId", "id: researcher\nmodel: flat\nconfig:\n  modelId: nested\n", "modelId 同时出现在"],
    ["title is scalar", "id: researcher\ntitle: invalid\n", "title 必须是字符串映射"],
    ["description has non-string value", "id: researcher\ndescription:\n  zh: valid\n  en: 42\n", "description 必须是字符串映射"],
    ["missing id on create", "name: Researcher\n", "必须包含 id"],
    ["non-string id on update", "id: 42\n", "id 必须是字符串", "researcher"],
    ["id differs from expectedName", "id: writer\n", "与命令参数不一致", "researcher"],
    ["desktop is not boolean", "id: researcher\ndesktop: yes\n", "desktop 必须是布尔值"],
    ["isDefault is not boolean", "id: researcher\nisDefault: 1\n", "isDefault 必须是布尔值"],
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
    expect(() => parseAgentYaml(file)).toThrow("providerId 同时出现在");
  });

  test("rejects unknown top-level metadata that conflicts with native config", () => {
    const file = yamlFile(`
id: researcher
config:
  futureField: native
futureField: top-level
`);
    expect(() => parseAgentYaml(file)).toThrow("futureField 同时出现在");
  });

  test("rejects extension metadata that conflicts with unknown top-level metadata", () => {
    const file = yamlFile(`
id: researcher
extensions:
  control-panel:
    futureField: extension
futureField: top-level
`);
    expect(() => parseAgentYaml(file)).toThrow("futureField 同时出现在");
  });
});
