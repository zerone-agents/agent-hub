import i18next from "@/i18n";

export interface DatasetGraphFields {
  graphrag_compatible: boolean;
  graphrag_initial?: Record<string, unknown>;
  graphrag_enabled: boolean;
  graphrag_entity_types: string[];
  graphrag_method: string;
  graphrag_community: boolean;
  graphrag_resolution: boolean;
}
const mapping = {
  graphrag_enabled: "use_graphrag",
  graphrag_entity_types: "entity_types",
  graphrag_method: "method",
  graphrag_community: "community",
  graphrag_resolution: "resolution",
} as const;
const defaults = {
  use_graphrag: false,
  entity_types: ["organization", "person", "geo", "event", "category"],
  method: "light",
  community: false,
  resolution: false,
};
function record(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value);
}
function supported(value: unknown): boolean {
  if (value === undefined) return true;
  if (!record(value)) return false;
  return Object.entries(value).every(([key, entry]) => {
    if (["use_graphrag", "enabled", "community", "resolution"].includes(key))
      return typeof entry === "boolean";
    if (key === "entity_types")
      return (
        Array.isArray(entry) &&
        (entry as unknown[]).every((item) => typeof item === "string")
      );
    if (key === "method") return entry === "light" || entry === "general";
    return true;
  });
}
function initialConfig(value?: Record<string, unknown>) {
  return {
    ...defaults,
    ...value,
    use_graphrag: value?.use_graphrag ?? value?.enabled ?? false,
  };
}
export function datasetGraphFields(value: unknown): DatasetGraphFields {
  const original = record(value) ? value : undefined;
  const config = initialConfig(original);
  return {
    graphrag_compatible: supported(value),
    graphrag_initial: original,
    graphrag_enabled: config.use_graphrag as boolean,
    graphrag_entity_types: config.entity_types,
    graphrag_method: config.method,
    graphrag_community: config.community,
    graphrag_resolution: config.resolution,
  };
}
// Keep advanced JSON authoritative for untouched controls. Only a user-changed
// structured field overrides it; conflicting edits require an explicit choice.
export function mergeDatasetGraphFields(
  extra: Record<string, unknown>,
  values: DatasetGraphFields,
): Record<string, unknown> {
  if (!values.graphrag_compatible) return extra;
  const initial = initialConfig(values.graphrag_initial);
  const graph = record(extra.graphrag) ? { ...extra.graphrag } : {};
  let changed = false;
  for (const [field, key] of Object.entries(mapping)) {
    const value = values[field as keyof typeof mapping];
    if (JSON.stringify(value) === JSON.stringify(initial[key])) continue;
    if (
      Object.hasOwn(graph, key) &&
      JSON.stringify(graph[key]) !==
        JSON.stringify(values.graphrag_initial?.[key]) &&
      JSON.stringify(graph[key]) !== JSON.stringify(value)
    ) {
      throw new Error(i18next.t("knowledge.form.graphConflict"));
    }
    graph[key] = value;
    changed = true;
  }
  if (
    !changed &&
    (extra.graphrag !== undefined || values.graphrag_initial !== undefined)
  )
    return extra;
  if (changed && extra.graphrag !== undefined && !record(extra.graphrag))
    throw new Error(i18next.t("knowledge.form.graphConflict"));
  return { ...extra, graphrag: changed ? graph : { ...defaults } };
}

// REST update deeply merges parser_config. Omitting untouched nested keys also
// retains unknown fields written by another operation after this form loaded.
export function datasetGraphDelta(
  next: Record<string, unknown>,
  before: Record<string, unknown>,
): Record<string, unknown> {
  return Object.fromEntries(
    Object.entries(next).flatMap(([key, value]) => {
      if (JSON.stringify(value) === JSON.stringify(before[key])) return [];
      if (record(value) && record(before[key])) {
        const delta = datasetGraphDelta(value, before[key]);
        return Object.keys(delta).length ? [[key, delta]] : [];
      }
      return [[key, value]];
    }),
  );
}
