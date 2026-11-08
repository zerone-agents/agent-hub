/** Public dataset/upload fields. Worker-owned table_column_overrides is never a write DTO. */
export type TableColumnRole = "indexing" | "metadata" | "both";
export interface TableUploadConfig {
  table_column_mode: "auto" | "manual";
  table_column_roles: Record<string, TableColumnRole>;
}
export interface TableDatasetConfig {
  table_column_mode?: "auto" | "manual" | null;
  table_column_roles?: Record<string, TableColumnRole | "vectorize"> | null;
  table_column_names?: string[] | null;
}
export interface DatasetTableFields {
  table_config: TableDatasetConfig;
  table_initial: TableDatasetConfig;
}
export const tableKeys = [
  "table_column_mode",
  "table_column_roles",
  "table_column_names",
] as const;
const record = (v: unknown): v is Record<string, unknown> =>
  v !== null && typeof v === "object" && !Array.isArray(v);
const same = (a: unknown, b: unknown) =>
  JSON.stringify(a) === JSON.stringify(b);

export function datasetTableFields(
  config: Record<string, unknown>,
): DatasetTableFields {
  // ext is flattened by the dataset service and wins over outer keys.
  const effective = { ...config, ...(record(config.ext) ? config.ext : {}) };
  const selected = Object.fromEntries(
    tableKeys
      .filter((key) => Object.hasOwn(effective, key))
      .map((key) => [key, effective[key]]),
  );
  return { table_config: selected, table_initial: selected };
}
export function mergeDatasetTableFields(
  extra: Record<string, unknown>,
  values: DatasetTableFields,
): Record<string, unknown> {
  let result = { ...extra };
  for (const key of tableKeys) {
    const next = values.table_config[key];
    if (same(next, values.table_initial[key])) continue;
    const extension = record(result.ext) ? result.ext : {};
    const current = Object.hasOwn(extension, key)
      ? extension[key]
      : result[key];
    if (
      current !== undefined &&
      !same(current, values.table_initial[key]) &&
      !same(current, next)
    )
      throw new Error(`Conflicting table form and advanced JSON: ${key}`);
    result = Object.hasOwn(extension, key)
      ? { ...result, ext: { ...extension, [key]: next } }
      : { ...result, [key]: next };
  }
  return result;
}

/** Validate only changed known dataset fields; historical snapshots are not write DTOs. */
export function validateDatasetTableConfig(
  config: Record<string, unknown>,
  previous: Record<string, unknown> = {},
): Record<string, unknown> {
  const result = { ...config };
  for (const [key, value] of Object.entries(config)) {
    if (same(value, previous[key])) continue;
    if (key === "table_column_overrides")
      throw new Error("table_column_overrides is worker-owned");
    if (key === "ext" && record(value)) {
      result.ext = validateDatasetTableConfig(
        value,
        record(previous.ext) ? previous.ext : {},
      );
      continue;
    }
    if (value === null && (tableKeys as readonly string[]).includes(key))
      continue;
    if (key === "table_column_mode" && value !== "auto" && value !== "manual")
      throw new Error("Invalid table_column_mode");
    if (
      key === "table_column_names" &&
      (!Array.isArray(value) || value.some((v) => typeof v !== "string"))
    )
      throw new Error("Invalid table_column_names");
    if (key === "table_column_roles") {
      if (
        !record(value) ||
        Object.values(value).some(
          (v) =>
            typeof v !== "string" ||
            !["indexing", "metadata", "both", "vectorize"].includes(v),
        )
      )
        throw new Error("Invalid table_column_roles");
      result[key] = Object.fromEntries(
        Object.entries(value).map(([name, role]) => [
          name,
          role === "vectorize" ? "indexing" : role,
        ]),
      );
    }
  }
  return result;
}
export function buildTableUploadConfig(
  mode: string,
  roles: Record<string, TableColumnRole>,
): TableUploadConfig | undefined {
  if (mode === "inherit") return undefined;
  if (
    (mode !== "auto" && mode !== "manual") ||
    Object.entries(roles).some(
      ([key, role]) =>
        !key.trim() || !["indexing", "metadata", "both"].includes(role),
    )
  )
    throw new Error("Invalid upload table roles");
  return {
    table_column_mode: mode,
    table_column_roles: mode === "manual" ? { ...roles } : {},
  };
}
