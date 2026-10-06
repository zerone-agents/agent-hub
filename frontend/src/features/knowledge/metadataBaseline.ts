import type { MetadataField } from "./metadata";
import type { KnowledgeObject } from "@/api/knowledgeManagement";

// Committed eb5546c flattens stored values to strings; comparisons can equate 0 and
// False, and membership/empty operators do not retain the stored value type.
// Keep batch filtering to unambiguous text values. Typed writes remain usable.
export const BASELINE_METADATA_OPERATORS = [
  "=",
  ">",
  "<",
  "≥",
  "≤",
  "contains",
  "start with",
  "end with",
  "in",
] as const;
const SCALAR_COMPARISONS = new Set(["=", "≠", ">", "<", "≥", "≤"]);
export const BASELINE_LIST_EXCLUSIONS = new Set([
  "≠",
  "not in",
  "not contains",
  "!=",
  "not is",
]);
// Over-approximate Python literals, including alternate bases, underscores,
// imaginary/complex numbers and comments. Never reinterpret a user's text.
const PYTHON_REAL =
  "(?:0[xob][0-9a-f_]+|(?:[0-9][0-9_]*(?:\\.[0-9_]*)?|\\.[0-9_]+)(?:e[+-]?[0-9_]+)?)";
const PYTHON_NUMBER = new RegExp(
  `^[+-]?\\s*(?:${PYTHON_REAL}j?|${PYTHON_REAL}\\s*[+-]\\s*${PYTHON_REAL}j)$`,
  "i",
);
function safeText(value: unknown, operator: string): value is string {
  if (typeof value !== "string") return false;
  if (!SCALAR_COMPARISONS.has(operator)) return true;
  const start = value.trimStart().replace(/^(?:#[^\n]*(?:\n|$)\s*)+/, "");
  if (/^(?:[[{('"]|(?:br|rb|[bru])(?:'[^']*'|"[^"]*"))/i.test(start))
    return false;
  const literal = start.replace(/#[^\n]*/g, "").trim();
  return (
    !PYTHON_NUMBER.test(literal) &&
    !/^(?:true|false|none|null|\.\.\.|set\s*\(\s*\))$/i.test(literal) &&
    !/^(?:[[{('"]|(?:br|rb|[bru])(?:'[^']*'|"[^"]*"))/i.test(literal)
  );
}
export function baselineMetadataFieldAllowed(
  field: MetadataField | undefined,
  flattened: unknown,
  operator = "=",
): boolean {
  if (field?.type === "list" && BASELINE_LIST_EXCLUSIONS.has(operator))
    return false;
  if (field?.type && !["string", "time", "list"].includes(field.type))
    return false;
  if (
    field?.type === "list" &&
    field.items &&
    typeof field.items === "object" &&
    !Array.isArray(field.items) &&
    (field.items as KnowledgeObject).type !== "string"
  )
    return false;
  if (!flattened || typeof flattened !== "object" || Array.isArray(flattened))
    return false;
  const values = Object.keys(flattened);
  if (
    field?.type === "time" &&
    values.some((value) => !/^\d{4}-\d{2}-\d{2}$/.test(value))
  )
    return false;
  return (
    values.length > 0 && values.every((value) => safeText(value, operator))
  );
}
export function baselineMetadataConditionAllowed(
  condition: { name: string; comparison_operator: string; value: unknown },
  fields: MetadataField[],
  flattened?: KnowledgeObject,
): boolean {
  if (BASELINE_LIST_EXCLUSIONS.has(condition.comparison_operator)) return false;
  const operator =
    condition.comparison_operator === "is"
      ? "="
      : condition.comparison_operator;
  return (
    (BASELINE_METADATA_OPERATORS as readonly string[]).includes(operator) &&
    (["in", "not in"].includes(operator)
      ? Array.isArray(condition.value) &&
        condition.value.length > 0 &&
        condition.value.every((value) => safeText(value, operator))
      : safeText(condition.value, operator)) &&
    baselineMetadataFieldAllowed(
      fields.find((field) => (field.key ?? field.name) === condition.name),
      flattened?.[condition.name],
      operator,
    )
  );
}

export function baselineMetadataDocumentAllowed(
  metadata: KnowledgeObject,
  conditions: { name: string; comparison_operator?: string; value?: unknown }[],
  logic = "and",
): boolean {
  let invalid =
    (logic !== "and" && logic !== "or") ||
    conditions.some((condition) =>
      BASELINE_LIST_EXCLUSIONS.has(condition.comparison_operator ?? ""),
    );
  const matches = conditions.map((condition) => {
    const value = metadata[condition.name];
    if (value === undefined || value === null) return false;
    const op =
      condition.comparison_operator === "is"
        ? "="
        : (condition.comparison_operator ?? "");
    const values: unknown[] = Array.isArray(value)
      ? (value as unknown[])
      : [value];
    if (
      values.some((item) => !safeText(item, op)) ||
      (Array.isArray(value) && BASELINE_LIST_EXCLUSIONS.has(op))
    ) {
      invalid = true;
      return false;
    }
    if (!op) return true;
    const text = (values as string[]).map((item) => item.toLowerCase());
    if (op === "in" || op === "not in") {
      if (
        !Array.isArray(condition.value) ||
        condition.value.some((item: unknown) => typeof item !== "string")
      ) {
        invalid = true;
        return false;
      }
      const candidates = (condition.value as string[]).map((item) =>
        item.toLowerCase(),
      );
      const matched = text.some((item) => candidates.includes(item));
      return op === "in" ? matched : !matched;
    }
    if (typeof condition.value !== "string") {
      invalid = true;
      return false;
    }
    const target = condition.value.toLowerCase();
    switch (op) {
      case "=":
        return text.some((item) => item === target);
      case "≠":
        return text.every((item) => item !== target);
      case ">":
        return text.some((item) => item > target);
      case "<":
        return text.some((item) => item < target);
      case "≥":
        return text.some((item) => item >= target);
      case "≤":
        return text.some((item) => item <= target);
      case "contains":
        return text.some((item) => item.includes(target));
      case "not contains":
        return text.every((item) => !item.includes(target));
      case "start with":
        return text.some((item) => item.startsWith(target));
      case "end with":
        return text.some((item) => item.endsWith(target));
      default:
        invalid = true;
        return false;
    }
  });
  return (
    !invalid &&
    (logic === "or" ? matches.some(Boolean) : matches.every(Boolean))
  );
}

// The committed batch writer skips a record when Python dict equality sees no
// change. Boolean/numeric aliases can therefore hide a type-only replacement.
export function baselineMetadataEquivalent(a: unknown, b: unknown): boolean {
  if (a === b) return true;
  if (
    (typeof a === "boolean" && typeof b === "number") ||
    (typeof a === "number" && typeof b === "boolean")
  )
    return Number(a) === Number(b);
  if (Array.isArray(a) && Array.isArray(b))
    return (
      a.length === b.length &&
      (a as unknown[]).every((value, index) =>
        baselineMetadataEquivalent(value, (b as unknown[])[index]),
      )
    );
  if (
    a &&
    b &&
    typeof a === "object" &&
    typeof b === "object" &&
    !Array.isArray(a) &&
    !Array.isArray(b)
  ) {
    const left = a as KnowledgeObject;
    const right = b as KnowledgeObject;
    return (
      Object.keys(left).length === Object.keys(right).length &&
      Object.entries(left).every(
        ([key, value]) =>
          Object.hasOwn(right, key) &&
          baselineMetadataEquivalent(value, right[key]),
      )
    );
  }
  return false;
}
