import {
  knowledgeMetadataApi,
  parseObjectJSON,
} from "@/api/knowledgeManagement";
import type { MetadataFilter } from "@/api/knowledge";

export const FILTER_OPERATORS = [
  "=",
  "≠",
  "contains",
  "not contains",
  "start with",
  "end with",
  "empty",
  "not empty",
  ">",
  "<",
  "≥",
  "≤",
  "in",
  "not in",
] as const;
export const isEmptyOperator = (op: string) =>
  op === "empty" || op === "not empty";
export const isUnsupportedExclusionOperator = (op: string) =>
  ["not in", "≠", "not contains"].includes(op);
export const retrievalMetadataKey = (datasetId: string) =>
  ["knowledge", "retrieval-metadata-keys", datasetId] as const;
export async function loadRetrievalMetadataKeys(
  datasetId: string,
): Promise<string[]> {
  const keys = await knowledgeMetadataApi.keys([datasetId]);
  if (
    !Array.isArray(keys) ||
    keys.some((key) => typeof key !== "string" || !key.trim())
  )
    throw new Error("metadataDirectoryRequired");
  return keys;
}
const aliases: Record<string, string> = {
  is: "=",
  "not is": "≠",
  "!=": "≠",
  ">=": "≥",
  "<=": "≤",
};
function operator(value: unknown): string {
  if (typeof value !== "string") throw new Error("Invalid operator");
  const op = aliases[value] ?? value;
  if (!(FILTER_OPERATORS as readonly string[]).includes(op))
    throw new Error("Invalid operator");
  return op;
}
function validateExclusionOperator(op: string) {
  // eb5546c expands each stored list value into a separate document bucket.
  // Negative predicates union those buckets, so [Alice, Bob] incorrectly
  // survives exclusion of Alice via Bob. The directory has no scalar proof.
  if (isUnsupportedExclusionOperator(op))
    throw new Error("unsupportedMetadataExclusion");
}
function record(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value))
    throw new Error("Invalid condition");
  return value as Record<string, unknown>;
}
function field(value: unknown): string {
  if (typeof value !== "string" || !value.trim())
    throw new Error("Invalid field");
  return value.trim();
}
function isListScalar(value: unknown): value is string | boolean | number {
  return (
    typeof value === "string" ||
    typeof value === "boolean" ||
    (typeof value === "number" && Number.isFinite(value))
  );
}

export function metadataFilterFromJSON(
  value: string,
  availableKeys?: readonly string[],
): MetadataFilter | undefined {
  if (!value.trim()) return undefined;
  const raw = parseObjectJSON(value);
  if (!Object.keys(raw).length) return undefined;
  if (raw.logic !== undefined && raw.logic !== "and" && raw.logic !== "or")
    throw new Error("Invalid logic");
  const method = raw.method ?? "manual";
  if (method === "auto") return { method: "auto" };
  if (method === "semi_auto") {
    if (!Array.isArray(raw.semi_auto) || !raw.semi_auto.length)
      throw new Error("Select fields");
    const semiAuto = raw.semi_auto.map((item: unknown) => {
      if (typeof item === "string") return field(item);
      const entry = record(item);
      if (entry.op) validateExclusionOperator(operator(entry.op));
      return {
        key: field(entry.key),
        ...(entry.op ? { op: operator(entry.op) } : {}),
      };
    });
    if (
      availableKeys &&
      semiAuto.some(
        (item) =>
          !availableKeys.includes(typeof item === "string" ? item : item.key),
      )
    )
      throw new Error("unknownMetadataFields");
    return {
      method,
      semi_auto: semiAuto,
    };
  }
  if (method !== "manual") throw new Error("Invalid method");
  const conditions = raw.method ? raw.manual : raw.conditions;
  if (!Array.isArray(conditions)) throw new Error("Invalid conditions");
  if (!conditions.length) return undefined;
  return {
    method,
    logic: raw.logic ?? "and",
    manual: conditions.map((condition: unknown) => {
      const item = record(condition);
      const key = field(raw.method ? item.key : item.name);
      const op = operator(raw.method ? item.op : item.comparison_operator);
      if (
        !isEmptyOperator(op) &&
        (item.value === undefined ||
          item.value === null ||
          (typeof item.value === "number" && !Number.isFinite(item.value)))
      )
        throw new Error("Missing value");
      if (
        !isEmptyOperator(op) &&
        (((op === "in" || op === "not in") && !Array.isArray(item.value)) ||
          (Array.isArray(item.value) &&
            (!item.value.length || !item.value.every(isListScalar))))
      )
        throw new Error("invalidMetadataList");
      // MultiRAG 4ce5dc0 retains scalar types for positive list membership.
      // Other operators cannot use array values; never coerce an advanced JSON
      // draft because that can change the user's requested scope.
      if (
        !isEmptyOperator(op) &&
        Array.isArray(item.value) &&
        !(op === "in" || op === "not in")
      )
        throw new Error("unsupportedMetadataList");
      validateExclusionOperator(op);
      return { key, op, value: isEmptyOperator(op) ? "" : item.value };
    }),
  };
}
