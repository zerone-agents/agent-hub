/** Dataset ParserConfig differs from the strict document PATCH schema. */
const record = (value: unknown): value is Record<string, unknown> =>
  value !== null && typeof value === "object" && !Array.isArray(value);
const numeric = (value: unknown, min: number, max: number, integer = true) =>
  typeof value === "number" &&
  Number.isFinite(value) &&
  value >= min &&
  value <= max &&
  (!integer || Number.isInteger(value));
export const graphBudgetRules: Record<string, [number, number, boolean?]> = {
  batch_chunk_token_size: [512, 8196],
  retry_attempts: [1, 10],
  retry_backoff_seconds: [0, 600, false],
  retry_backoff_max_seconds: [0, 3600, false],
  build_subgraph_timeout_per_chunk_seconds: [1, 86400],
  build_subgraph_min_timeout_seconds: [1, 86400],
  merge_timeout_seconds: [0, 86400],
  resolution_timeout_seconds: [0, 86400],
  community_timeout_seconds: [0, 86400],
  lock_acquire_timeout_seconds: [0, 86400],
  build_subgraph_retry_attempts: [1, 10],
  merge_retry_attempts: [1, 10],
  resolution_retry_attempts: [1, 10],
  community_retry_attempts: [1, 10],
};
export function validateDatasetParserConfig(
  config: Record<string, unknown>,
): void {
  const invalid = (field: string): never => {
    throw new Error(`Invalid parser_config.${field}`);
  };
  // Dataset ext is flattened before merging/validation.
  if (config.ext !== undefined && !record(config.ext)) invalid("ext");
  const effective = { ...config, ...(record(config.ext) ? config.ext : {}) };
  for (const [key, min, max] of [
    ["chunk_token_num", 1, 2048],
    ["auto_keywords", 0, 32],
    ["auto_questions", 0, 10],
  ] as const)
    if (key in effective && !numeric(effective[key], min, max)) invalid(key);
  if (
    "overlapped_percent" in effective &&
    !numeric(effective.overlapped_percent, 0, 90, false)
  )
    invalid("overlapped_percent");
  if ("graphrag" in effective) {
    if (!record(effective.graphrag)) invalid("graphrag");
    for (const [key, value] of Object.entries(
      effective.graphrag as Record<string, unknown>,
    )) {
      const bounds = Object.hasOwn(graphBudgetRules, key)
        ? graphBudgetRules[key]
        : undefined;
      if (bounds && !numeric(value, ...bounds)) invalid(`graphrag.${key}`);
      if (
        ["use_graphrag", "community", "resolution"].includes(key) &&
        typeof value !== "boolean"
      )
        invalid(`graphrag.${key}`);
      if (
        key === "method" &&
        !["light", "general", "ner"].includes(value as string)
      )
        invalid("graphrag.method");
      if (
        key === "entity_types" &&
        (!Array.isArray(value) ||
          value.some((item) => typeof item !== "string"))
      )
        invalid("graphrag.entity_types");
    }
  }
  if ("raptor" in effective) {
    if (!record(effective.raptor)) invalid("raptor");
    const raptor = effective.raptor as Record<string, unknown>;
    for (const [key, value] of Object.entries(raptor)) {
      if (
        ["use_raptor", "auto_disable_for_structured_data"].includes(key) &&
        typeof value !== "boolean"
      )
        invalid(`raptor.${key}`);
      if (key === "prompt" && (typeof value !== "string" || !value.trim()))
        invalid("raptor.prompt");
      if (key === "scope" && !["file", "dataset"].includes(value as string))
        invalid("raptor.scope");
      if (key === "ext" && !record(value)) invalid("raptor.ext");
      const ranges: Record<string, [number, number, boolean?]> = {
        max_token: [1, 2048],
        threshold: [0, 1, false],
        max_cluster: [1, 1024],
        random_seed: [0, Infinity],
      };
      if (Object.hasOwn(ranges, key) && !numeric(value, ...ranges[key]))
        invalid(`raptor.${key}`);
    }
    for (const source of [
      raptor,
      ...(record(raptor.ext) ? [raptor.ext] : []),
    ]) {
      for (const [key, choices] of [
        ["tree_builder", ["raptor", "psi"]],
        ["clustering_method", ["gmm", "ahc"]],
      ] as const)
        if (key in source && !choices.includes(source[key] as never))
          invalid(`raptor.${key}`);
      if (source !== raptor)
        for (const key of ["psi_exact_max_leaves", "psi_bucket_size"])
          if (key in source && !numeric(source[key], 2, 4096))
            invalid(`raptor.ext.${key}`);
    }
  }
}
