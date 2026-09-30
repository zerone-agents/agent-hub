import { parse } from "yaml";

// ── agent update --set 增量补丁（issue #202 方案 A）────────────
//
// 纯函数层，不触网。命令层负责 read-modify-write：
//   get → buildSetUpdate(current.config, sets) → put
//
// key 分类规则：
//   - 平台标志别名（desktop/desktopEnabled、mobile/mobileEnabled、
//     isDefault/default、guest/guestEnabled）→ 顶层布尔标志，值必须是 boolean
//   - 其余一律视为 config 路径（点号分段，支持 title.zh 这类嵌套）；
//     可选的 config. 前缀会被剥掉；config 顶层的 model 别名归一为 modelId
//     （与 agent-yaml.ts 的 FLAT_ALIASES 约定一致）
//
// 值按 YAML 语法解析：100 → 数字、true → 布尔、桌面端 → 字符串、
// [a, b] / {zh: x} → 数组/对象。

export class SetPatchError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "SetPatchError";
  }
}

export interface SetUpdate {
  config?: Record<string, unknown>;
  desktopEnabled?: boolean;
  mobileEnabled?: boolean;
  isDefault?: boolean;
  guestEnabled?: boolean;
}

const FLAG_ALIASES = new Map<string, "desktopEnabled" | "mobileEnabled" | "isDefault" | "guestEnabled">([
  ["desktop", "desktopEnabled"],
  ["desktopEnabled", "desktopEnabled"],
  ["mobile", "mobileEnabled"],
  ["mobileEnabled", "mobileEnabled"],
  ["isDefault", "isDefault"],
  ["default", "isDefault"],
  ["guest", "guestEnabled"],
  ["guestEnabled", "guestEnabled"],
]);

// config 顶层字段别名，与 agent-yaml.ts FLAT_ALIASES 的 flat 写法对齐。
const CONFIG_ALIASES = new Map([["model", "modelId"]]);

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

export function parseSetArg(arg: string): { key: string; value: unknown } {
  const eq = arg.indexOf("=");
  if (eq < 0) {
    throw new SetPatchError(`invalid --set "${arg}": expected key=value`);
  }
  const key = arg.slice(0, eq).trim();
  if (key.length === 0) {
    throw new SetPatchError(`invalid --set "${arg}": key must not be empty`);
  }
  const rawValue = arg.slice(eq + 1);
  let value: unknown;
  try {
    value = parse(rawValue);
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    throw new SetPatchError(`invalid --set "${arg}": cannot parse value as YAML: ${message}`);
  }
  const flag = FLAG_ALIASES.get(key);
  if (flag !== undefined && typeof value !== "boolean") {
    throw new SetPatchError(`invalid --set "${arg}": ${key} must be a boolean`);
  }
  return { key, value };
}

// 命令层在任何网络调用前先跑这一步：格式错误（缺 =、空 key、YAML
// 解析失败、标志非布尔）应在零请求下拒绝。
export function parseSetArgs(args: string[]): { key: string; value: unknown }[] {
  return args.map(parseSetArg);
}

function setConfigPath(
  config: Record<string, unknown>,
  rawPath: string,
  value: unknown,
): void {
  const path = rawPath.startsWith("config.") ? rawPath.slice("config.".length) : rawPath;
  const segments = path.split(".");
  if (segments.some((s) => s.length === 0)) {
    throw new SetPatchError(`invalid --set key "${rawPath}": empty path segment`);
  }
  const first = segments[0];
  if (segments.length === 1 && CONFIG_ALIASES.has(first)) {
    segments[0] = CONFIG_ALIASES.get(first)!;
  }
  let node = config;
  for (const segment of segments.slice(0, -1)) {
    const next = node[segment];
    if (next === undefined) {
      const created: Record<string, unknown> = {};
      node[segment] = created;
      node = created;
    } else if (isRecord(next)) {
      node = next;
    } else {
      throw new SetPatchError(
        `invalid --set key "${rawPath}": "${segment}" is not an object in current config`,
      );
    }
  }
  node[segments[segments.length - 1]] = value;
}

// 把一组 --set key=value 依序合并到 currentConfig 的深拷贝上。
// 只有 config 路径被设置时才返回 config 键（全量合并后的 config 供 PUT
// 全量替换语义使用）；纯标志更新不携带 config——后端 req.Config == nil
// 即不变更，避免空对象误触发全量清空。
export function buildSetUpdate(
  currentConfig: Record<string, unknown> | undefined,
  sets: string[],
): SetUpdate {
  const update: SetUpdate = {};
  let config: Record<string, unknown> | undefined;
  for (const { key, value } of parseSetArgs(sets)) {
    const flag = FLAG_ALIASES.get(key);
    if (flag !== undefined) {
      update[flag] = value as boolean;
      continue;
    }
    config ??= structuredClone(currentConfig ?? {});
    setConfigPath(config, key, value);
  }
  if (config !== undefined) {
    update.config = config;
  }
  return update;
}
