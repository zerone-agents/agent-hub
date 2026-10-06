import { containsKnowledgeFields } from "@/api/knowledgeManagement";

export type MetadataField = Record<string, unknown> & {
  key?: string;
  name?: string;
  type?: string;
};
export type MetadataDefinition = MetadataField[] | Record<string, unknown>;

export function parseMetadataDefinition(
  text: string,
  arrayOnly = false,
): MetadataDefinition {
  const parsed: unknown = JSON.parse(text);
  if (!Array.isArray(parsed)) {
    if (arrayOnly || !parsed || typeof parsed !== "object")
      throw new Error("Invalid metadata definition");
    const schema = parsed as Record<string, unknown>;
    if (
      Object.keys(schema).length &&
      ((schema.type !== undefined && schema.type !== "object") ||
        !schema.properties ||
        typeof schema.properties !== "object" ||
        Array.isArray(schema.properties))
    )
      throw new Error("Metadata schema requires object properties");
    const normalizeSchemaEnums = (definition: Record<string, unknown>) => {
      if (
        (definition.type === "number" || definition.type === "integer") &&
        Array.isArray(definition.enum)
      )
        definition.enum = metadataNumberOptions(definition.enum as unknown[]);
      if (
        definition.items &&
        typeof definition.items === "object" &&
        !Array.isArray(definition.items)
      )
        normalizeSchemaEnums(definition.items as Record<string, unknown>);
      if (
        definition.properties &&
        typeof definition.properties === "object" &&
        !Array.isArray(definition.properties)
      )
        for (const property of Object.values(definition.properties)) {
          if (
            property &&
            typeof property === "object" &&
            !Array.isArray(property)
          )
            normalizeSchemaEnums(property as Record<string, unknown>);
        }
    };
    normalizeSchemaEnums(schema);
    return schema;
  }
  const keys = new Set<string>();
  return parsed.map((entry: unknown) => {
    if (!entry || typeof entry !== "object" || Array.isArray(entry))
      throw new Error("Invalid metadata field");
    const field = { ...entry } as MetadataField;
    const key = field.key ?? field.name;
    if (
      typeof key !== "string" ||
      !key.trim() ||
      key.trim().length > 255 ||
      keys.has(key.trim())
    )
      throw new Error("Metadata keys must be unique and nonempty");
    if (field.key && field.name && field.key !== field.name)
      throw new Error("Conflicting metadata key and name");
    keys.add(key.trim());
    if (field.key !== undefined) field.key = key.trim();
    if (field.name !== undefined) field.name = key.trim();
    if (
      field.type !== undefined &&
      !["string", "number", "list", "time"].includes(field.type)
    )
      throw new Error("Invalid metadata type");
    for (const property of ["enum", "examples"]) {
      if (field[property] === undefined) continue;
      const values = field[property];
      if (!Array.isArray(values))
        throw new Error("Metadata values require an array");
      if (field.type === "number") {
        field[property] = values.map((value: unknown) => {
          if (
            (typeof value !== "string" && typeof value !== "number") ||
            String(value).trim() === "" ||
            !Number.isFinite(Number(value))
          )
            throw new Error("Number metadata values must be finite numbers");
          return Number(value);
        });
      } else if (values.some((value: unknown) => typeof value !== "string"))
        throw new Error("Metadata values must be strings");
    }
    return field;
  });
}

export function metadataReadbackMatches(
  actual: unknown,
  expected: MetadataDefinition,
): boolean {
  // The read API projects an empty stored JSON Schema to []. Verify genuine
  // emptiness so an unchanged nonempty schema cannot masquerade as a clear.
  if (!Array.isArray(expected) && Object.keys(expected).length === 0) {
    return Array.isArray(actual)
      ? actual.length === 0
      : Boolean(actual) &&
          typeof actual === "object" &&
          Object.keys(actual as Record<string, unknown>).length === 0;
  }
  const canonical = (value: unknown): unknown =>
    Array.isArray(value)
      ? value.map((item: unknown) => {
          if (!item || typeof item !== "object" || Array.isArray(item))
            return item;
          const field = { ...item } as MetadataField;
          if (!field.key && field.name) field.key = field.name;
          if (field.name === field.key) delete field.name;
          if (
            field.restrict_values === true &&
            Array.isArray(field.examples) &&
            field.enum === undefined
          )
            field.enum = field.examples;
          return field;
        })
      : value;
  return metadataValuesMatch(canonical(actual), canonical(expected));
}

export function documentMetadata(
  rows: Record<string, unknown>[],
): Record<string, unknown> {
  return Object.fromEntries(
    rows.flatMap((row) =>
      typeof row.key === "string"
        ? [[row.key, row.value]]
        : Object.entries(row),
    ),
  );
}

export function metadataFields(
  definition?: MetadataDefinition,
): MetadataField[] {
  if (Array.isArray(definition)) return definition;
  if (
    !definition?.properties ||
    typeof definition.properties !== "object" ||
    Array.isArray(definition.properties)
  )
    return [];
  return Object.entries(definition.properties).map(([key, value]) => {
    const schema =
      value && typeof value === "object" && !Array.isArray(value)
        ? (value as Record<string, unknown>)
        : {};
    return {
      ...schema,
      ...(schema.type !== "array" && Array.isArray(schema.enum)
        ? { restrict_values: true }
        : {}),
      ...(schema.type === "array" &&
      schema.items &&
      typeof schema.items === "object" &&
      !Array.isArray(schema.items) &&
      Array.isArray((schema.items as Record<string, unknown>).enum)
        ? {
            enum: (schema.items as Record<string, unknown>).enum,
            restrict_values: true,
          }
        : {}),
      key,
      type:
        schema.type === "array"
          ? "list"
          : schema.type === "integer"
            ? "number"
            : typeof schema.type === "string"
              ? schema.type
              : undefined,
    };
  });
}

export interface MetadataOperation {
  key: string;
  mode: "set" | "replace" | "deleteField" | "deleteValue";
  value?: unknown;
  match?: string;
}
export function metadataBatchBody(
  ids: string[],
  operation: MetadataOperation,
  condition?: Record<string, unknown>,
) {
  if (!ids.length || ids.some((id) => !id.trim()) || !operation.key.trim())
    throw new Error("Explicit documents and field required");
  if (
    (operation.mode === "replace" && !operation.match?.trim()) ||
    (operation.mode === "deleteValue" && operation.match === undefined)
  )
    throw new Error("Match value required");
  if (
    (operation.mode === "set" || operation.mode === "replace") &&
    operation.value === undefined
  )
    throw new Error("Explicit value required");
  return {
    selector: {
      document_ids: [...new Set(ids)],
      ...(condition ? { metadata_condition: condition } : {}),
    },
    updates:
      operation.mode === "set" || operation.mode === "replace"
        ? [
            {
              key: operation.key,
              value: operation.value,
              ...(operation.mode === "replace"
                ? { match: operation.match }
                : {}),
            },
          ]
        : [],
    deletes:
      operation.mode === "deleteField" || operation.mode === "deleteValue"
        ? [
            {
              key: operation.key,
              ...(operation.mode === "deleteValue"
                ? { value: operation.match }
                : {}),
            },
          ]
        : [],
  };
}
const equalValue = (a: unknown, b: unknown): boolean =>
  containsKnowledgeFields(a, b) && containsKnowledgeFields(b, a);
export function metadataValuesMatch(a: unknown, b: unknown): boolean {
  return equalValue(a, b);
}
const pythonString = (value: unknown): string =>
  value === null
    ? "None"
    : value === true
      ? "True"
      : value === false
        ? "False"
        : typeof value === "string"
          ? value
          : JSON.stringify(value);
const dedupe = (values: unknown[]): unknown[] =>
  values.filter(
    (value, index) =>
      values.findIndex((item) => pythonString(item) === pythonString(value)) ===
      index,
  );
export function previewMetadataOperation(
  original: Record<string, unknown>,
  operation: MetadataOperation,
  normalize = true,
): Record<string, unknown> {
  const next = { ...original };
  const { key, mode, match, value } = operation;
  const has = Object.hasOwn(next, key);
  const current = next[key];
  const currentList = Array.isArray(current)
    ? (current as unknown[])
    : undefined;
  if (mode === "deleteField") Reflect.deleteProperty(next, key);
  else if (mode === "deleteValue") {
    if (currentList) {
      const remaining = currentList.filter(
        (item) => pythonString(item) !== match,
      );
      if (remaining.length !== currentList.length) {
        if (remaining.length) next[key] = remaining;
        else Reflect.deleteProperty(next, key);
      }
    } else if (has && pythonString(current) === match)
      Reflect.deleteProperty(next, key);
  } else if (mode === "replace") {
    if (currentList)
      next[key] = dedupe(
        currentList.map((item) =>
          pythonString(item) === match ? value : item,
        ),
      );
    else if (has && pythonString(current) === match) next[key] = value;
  } else
    next[key] = currentList
      ? dedupe([
          ...currentList,
          ...(Array.isArray(value) ? (value as unknown[]) : [value]),
        ])
      : Array.isArray(value)
        ? dedupe(value as unknown[])
        : value;
  // MultiRAG normalizes list strings before persisting the complete batch.
  if (normalize && !equalValue(next, original))
    return normalizeMetadataLists(next);
  return next;
}

export function validateMetadataValue(
  value: unknown,
  field?: MetadataField,
): void {
  if (!field) return;
  if (field.restrict_values === true && Array.isArray(field.enum)) {
    const allowed = field.enum as unknown[];
    const values = Array.isArray(value) ? (value as unknown[]) : [value];
    if (
      values.some(
        (item) => !allowed.some((option) => metadataValuesMatch(item, option)),
      )
    )
      throw new Error("Value outside field options");
  }
  if (value === null) return;
  if (
    field.type === "number" &&
    (typeof value !== "number" || !Number.isFinite(value))
  )
    throw new Error("Finite number required");
  if (field.type === "boolean" && typeof value !== "boolean")
    throw new Error("Boolean value required");
  if (
    (field.type === "string" || field.type === "time") &&
    typeof value !== "string"
  )
    throw new Error("Text value required");
  if (field.type === "list") {
    if (!Array.isArray(value)) throw new Error("List value required");
    if (field.items === false && value.length > 0)
      throw new Error("This list schema disallows every item");
    const itemType =
      field.items === true
        ? undefined
        : field.items &&
            typeof field.items === "object" &&
            !Array.isArray(field.items)
          ? (field.items as Record<string, unknown>).type
          : "string";
    for (const item of value as unknown[]) {
      if (itemType === "string" && typeof item !== "string")
        throw new Error("Text list required");
      if (
        (itemType === "number" || itemType === "integer") &&
        (typeof item !== "number" || !Number.isFinite(item))
      )
        throw new Error("Number list required");
      if (itemType === "boolean" && typeof item !== "boolean")
        throw new Error("Boolean list required");
    }
  }
}

export function metadataBatchOperationsBody(
  ids: string[],
  operations: MetadataOperation[],
  condition?: Record<string, unknown>,
) {
  if (
    !operations.length ||
    new Set(operations.map((operation) => operation.key)).size !==
      operations.length
  )
    throw new Error("Each field needs one explicit operation");
  const bodies = operations.map((operation) =>
    metadataBatchBody(ids, operation, condition),
  );
  return {
    selector: bodies[0].selector,
    updates: bodies.flatMap((body) => body.updates),
    deletes: bodies.flatMap((body) => body.deletes),
  };
}
export function previewMetadataOperations(
  original: Record<string, unknown>,
  operations: MetadataOperation[],
): Record<string, unknown> {
  // One operation per field avoids the upstream insert path skipping a delete
  // of a newly inserted field. Lists normalize only after the complete batch.
  const sorted = [
    ...operations.filter(
      (operation) => operation.mode === "set" || operation.mode === "replace",
    ),
    ...operations.filter(
      (operation) =>
        operation.mode === "deleteField" || operation.mode === "deleteValue",
    ),
  ];
  let next = original;
  for (const operation of sorted)
    next = previewMetadataOperation(next, operation, false);
  if (!metadataValuesMatch(original, next)) next = normalizeMetadataLists(next);
  return next;
}
function normalizeMetadataLists(
  values: Record<string, unknown>,
): Record<string, unknown> {
  return Object.fromEntries(
    Object.entries(values).map(([key, value]) => [
      key,
      Array.isArray(value)
        ? dedupe(
            (value as unknown[]).flatMap((entry) => {
              if (typeof entry !== "string") return [entry];
              const parts = entry
                .trim()
                .split(/[\u3001,\uff0c;\uff1b|]+/)
                .map((part) => part.trim())
                .filter(Boolean);
              return parts.length ? parts : [entry];
            }),
          )
        : value,
    ]),
  );
}

export function metadataNumberOptions(values: unknown[]): number[] {
  return values.map((value) => {
    if (
      (typeof value !== "string" && typeof value !== "number") ||
      String(value).trim() === "" ||
      !Number.isFinite(Number(value))
    )
      throw new Error("Finite number options required");
    return Number(value);
  });
}

export function metadataDefaultValue(type: string): unknown {
  return type === "list"
    ? []
    : type === "number"
      ? 0
      : type === "boolean"
        ? false
        : type === "null"
          ? null
          : "";
}
