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
    throw new AgentYamlError(`${key} must be a string map`);
  }
}

function addConfigValue(
  config: Record<string, unknown>,
  key: string,
  value: unknown,
  source: string,
): void {
  if (Object.hasOwn(config, key)) {
    throw new AgentYamlError(`${key} appears in both config and ${source}`);
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
    throw new AgentYamlError(`Cannot read or parse YAML file ${filePath}: ${message}`);
  }

  if (!isRecord(parsed)) {
    throw new AgentYamlError(`YAML file is empty or malformed: ${filePath}`);
  }

  if (expectedName === undefined) {
    if (typeof parsed.id !== "string" || parsed.id.length === 0) {
      throw new AgentYamlError("agent.yaml must contain an id field (agent identifier)");
    }
  } else if (Object.hasOwn(parsed, "id")) {
    if (typeof parsed.id !== "string") {
      throw new AgentYamlError("agent.yaml id must be a string");
    }
    if (parsed.id !== expectedName) {
      throw new AgentYamlError(`agent.yaml id "${parsed.id}" does not match the command argument (expected "${expectedName}")`);
    }
  }

  for (const key of ["desktop", "mobile", "isDefault"] as const) {
    if (parsed[key] !== undefined && typeof parsed[key] !== "boolean") {
      throw new AgentYamlError(`${key} must be a boolean`);
    }
  }

  if (parsed.config !== undefined && !isRecord(parsed.config)) {
    throw new AgentYamlError("config must be an object");
  }

  const nativeConfig = (parsed.config ?? {}) as Record<string, unknown>;
  if (Object.hasOwn(nativeConfig, "config")) {
    throw new AgentYamlError("config.config is not allowed");
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
    addConfigValue(config, normalizedKey, value, `top-level ${flatKey}`);
  }

  if (!Object.hasOwn(config, "title") && typeof parsed.name === "string") {
    config.title = { zh: parsed.name };
  }

  if (parsed.extensions !== undefined) {
    if (!isRecord(parsed.extensions)) {
      throw new AgentYamlError("extensions must be an object");
    }
    const controlPanel = parsed.extensions["control-panel"];
    if (controlPanel !== undefined) {
      if (!isRecord(controlPanel)) {
        throw new AgentYamlError("extensions.control-panel must be an object");
      }
      for (const [key, value] of Object.entries(controlPanel)) {
        addConfigValue(config, key, value, " extensions.control-panel");
      }
    }
  }

  const flatKeys = new Set(FLAT_ALIASES.keys());
  for (const [key, value] of Object.entries(parsed)) {
    if (RESERVED_KEYS.has(key) || flatKeys.has(key)) continue;
    addConfigValue(config, key, value, `top-level ${key}`);
  }

  const result: ParsedAgentDefinition = { name: expectedName ?? (parsed.id as string), config };
  if (typeof parsed.desktop === "boolean") result.desktopEnabled = parsed.desktop;
  if (typeof parsed.mobile === "boolean") result.mobileEnabled = parsed.mobile;
  if (typeof parsed.isDefault === "boolean") result.isDefault = parsed.isDefault;
  return result;
}
