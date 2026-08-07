import { readFileSync } from "node:fs";
import { parse } from "yaml";

export interface ParsedAgentDefinition {
  name: string;
  config: Record<string, unknown>;
  desktopEnabled?: boolean;
  mobileEnabled?: boolean;
  isDefault?: boolean;
}

export class AgentYamlError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "AgentYamlError";
  }
}

const FLAT_ALIASES = new Map([
  ["model", "modelId"],
  ["systemPrompt", "systemPrompt"],
  ["maxTurns", "maxTurns"],
  ["permissionMode", "permissionMode"],
  ["iconName", "iconName"],
  ["iconColor", "iconColor"],
  ["iconBgColor", "iconBgColor"],
  ["title", "title"],
  ["description", "description"],
]);

const RESERVED_KEYS = new Set([
  "id",
  "name",
  "config",
  "desktop",
  "mobile",
  "isDefault",
  "extensions",
  "allowedTools",
  "subagents",
  "skills",
  "tools",
]);

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function validateStringMap(key: "title" | "description", value: unknown): void {
  if (!isRecord(value) || Object.values(value).some((item) => typeof item !== "string")) {
    throw new AgentYamlError(`${key} 必须是字符串映射`);
  }
}

function addConfigValue(
  config: Record<string, unknown>,
  key: string,
  value: unknown,
  source: string,
): void {
  if (Object.hasOwn(config, key)) {
    throw new AgentYamlError(`${key} 同时出现在 config 和${source}`);
  }
  if (key === "title" || key === "description") {
    validateStringMap(key, value);
  }
  config[key] = value;
}

export function parseAgentYaml(
  filePath: string,
  expectedName?: string,
): ParsedAgentDefinition {
  let parsed: unknown;
  try {
    parsed = parse(readFileSync(filePath, "utf8"));
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    throw new AgentYamlError(`无法读取或解析 YAML 文件 ${filePath}: ${message}`);
  }

  if (!isRecord(parsed)) {
    throw new AgentYamlError(`YAML 文件为空或格式错误: ${filePath}`);
  }

  if (expectedName === undefined) {
    if (typeof parsed.id !== "string" || parsed.id.length === 0) {
      throw new AgentYamlError("agent.yaml 必须包含 id 字段（agent 标识名）");
    }
  } else if (Object.hasOwn(parsed, "id")) {
    if (typeof parsed.id !== "string") {
      throw new AgentYamlError("agent.yaml id 必须是字符串");
    }
    if (parsed.id !== expectedName) {
      throw new AgentYamlError(`agent.yaml id “${parsed.id}” 与命令参数不一致（应为 “${expectedName}”）`);
    }
  }

  for (const key of ["desktop", "mobile", "isDefault"] as const) {
    if (parsed[key] !== undefined && typeof parsed[key] !== "boolean") {
      throw new AgentYamlError(`${key} 必须是布尔值`);
    }
  }

  if (parsed.config !== undefined && !isRecord(parsed.config)) {
    throw new AgentYamlError("config 必须是对象");
  }

  const nativeConfig = (parsed.config ?? {}) as Record<string, unknown>;
  if (Object.hasOwn(nativeConfig, "config")) {
    throw new AgentYamlError("不允许 config.config");
  }

  const config = { ...nativeConfig };
  for (const key of ["title", "description"] as const) {
    if (Object.hasOwn(config, key)) validateStringMap(key, config[key]);
  }

  for (const [flatKey, normalizedKey] of FLAT_ALIASES) {
    if (!Object.hasOwn(parsed, flatKey)) continue;
    const value = parsed[flatKey];
    if (flatKey === "title" || flatKey === "description") {
      validateStringMap(flatKey, value);
    }
    addConfigValue(config, normalizedKey, value, `顶层 ${flatKey}`);
  }

  if (!Object.hasOwn(config, "title") && typeof parsed.name === "string") {
    config.title = { zh: parsed.name };
  }

  if (parsed.extensions !== undefined) {
    if (!isRecord(parsed.extensions)) {
      throw new AgentYamlError("extensions 必须是对象");
    }
    const controlPanel = parsed.extensions["control-panel"];
    if (controlPanel !== undefined) {
      if (!isRecord(controlPanel)) {
        throw new AgentYamlError("extensions.control-panel 必须是对象");
      }
      for (const [key, value] of Object.entries(controlPanel)) {
        addConfigValue(config, key, value, " extensions.control-panel");
      }
    }
  }

  const flatKeys = new Set(FLAT_ALIASES.keys());
  for (const [key, value] of Object.entries(parsed)) {
    if (RESERVED_KEYS.has(key) || flatKeys.has(key)) continue;
    addConfigValue(config, key, value, `顶层 ${key}`);
  }

  const result: ParsedAgentDefinition = { name: expectedName ?? (parsed.id as string), config };
  if (typeof parsed.desktop === "boolean") result.desktopEnabled = parsed.desktop;
  if (typeof parsed.mobile === "boolean") result.mobileEnabled = parsed.mobile;
  if (typeof parsed.isDefault === "boolean") result.isDefault = parsed.isDefault;
  return result;
}
