interface ContentPart {
  type?: string;
  text?: string;
  name?: string;
  [k: string]: unknown;
}

/**
 * 把消息 content（hub 侧为 parts JSON 数组）压成一行预览，只给 table 视图用。
 * json/yaml 输出必须原样透传完整对象（见 commands/tool.ts 的既有分工）。
 * 解析失败按原文返回——存量行与截断数据都不能让 CLI 崩掉。
 */
export function previewText(content: string, max = 80): string {
  const trimmed = (content ?? "").trim();
  if (trimmed === "") return "";
  // null = 不是可解析的 parts 数组（解析失败或缺 "[" 前缀）→ 当纯文本处理
  const parsed = tryParseParts(trimmed);
  const parts = parsed ?? [{ type: "text", text: trimmed }];
  const text = parts
    .filter((p) => p && typeof p === "object" && p.type === "text" && typeof p.text === "string")
    .map((p) => (p.text as string).replace(/\s+/g, " ").trim())
    .filter((s) => s !== "")
    .join(" ");
  const tools = parts
    .filter((p) => p && typeof p === "object" && p.type === "tool_use")
    .map((p) => `[tool:${p.name ?? "?"}]`);
  const joined = [text, ...tools].filter((s) => s !== "").join(" ");
  // 合法但空的 parts 数组 → 空预览（不回退原文）；只有"根本不是 parts"才回退原文行
  const result = parsed ? joined : joined || trimmed.replace(/\s+/g, " ").trim();
  return truncate(result, max);
}

function tryParseParts(raw: string): ContentPart[] | null {
  if (!raw.startsWith("[")) return null;
  try {
    const value: unknown = JSON.parse(raw);
    return Array.isArray(value) ? (value as ContentPart[]) : null;
  } catch {
    return null;
  }
}

// 按码点截断（Array.from），避免中文/emoji 被字节级 slice 切成半个字符。
function truncate(s: string, max: number): string {
  const chars = Array.from(s);
  return chars.length <= max ? s : chars.slice(0, max).join("") + "…";
}
