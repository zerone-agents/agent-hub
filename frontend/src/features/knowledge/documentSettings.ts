import type { KnowledgeDocument } from "@/api/knowledge";
import {
  metadataFields,
  validateMetadataValue,
  type MetadataDefinition,
} from "./metadata";
import {
  containsKnowledgeFields,
  parseObjectJSON,
} from "@/api/knowledgeManagement";

type ObjectValue = Record<string, unknown>;
type Rule = (value: unknown) => boolean;
const object = (v: unknown): v is ObjectValue =>
  !!v && typeof v === "object" && !Array.isArray(v);
const bool: Rule = (v) => typeof v === "boolean";
const string: Rule = (v) => typeof v === "string";
const nonempty: Rule = (v) => typeof v === "string" && v.length > 0;
const range =
  (min: number, max = Infinity, integer = false): Rule =>
  (v) =>
    typeof v === "number" &&
    Number.isFinite(v) &&
    v >= min &&
    v <= max &&
    (!integer || Number.isInteger(v));
const strings: Rule = (v) => Array.isArray(v) && v.every(nonempty);
const enumRule =
  (...values: string[]): Rule =>
  (v) =>
    typeof v === "string" && values.includes(v);
const nested: Partial<Record<string, Partial<Record<string, Rule>>>> = {
  raptor: {
    use_raptor: bool,
    prompt: (v) => typeof v === "string" && !!v.trim(),
    max_token: range(1, 2048, true),
    threshold: range(0, 1),
    max_cluster: range(1, 1024, true),
    random_seed: range(0, Infinity, true),
    auto_disable_for_structured_data: bool,
    scope: enumRule("file", "dataset"),
  },
  graphrag: {
    use_graphrag: bool,
    entity_types: strings,
    method: enumRule("light", "general"),
    community: bool,
    resolution: bool,
  },
  parent_child: { use_parent_child: bool, children_delimiter: nonempty },
};
const rules: Partial<Record<string, Rule>> = {
  auto_keywords: range(0, 32, true),
  auto_questions: range(0, 10, true),
  chunk_token_num: range(1, 8192, true),
  delimiter: nonempty,
  html4excel: bool,
  layout_recognize: nonempty,
  enable_children: bool,
  children_delimiter: string,
  tag_kb_ids: strings,
  topn_tags: range(1, 10, true),
  filename_embd_weight: range(0, 1),
  task_page_size: range(1, Infinity, true),
  pages: (v) =>
    Array.isArray(v) &&
    v.every(
      (pair: unknown) =>
        Array.isArray(pair) &&
        pair.length === 2 &&
        pair.every(range(1, Infinity, true)) &&
        Number(pair[0]) < Number(pair[1]),
    ),
  image_context_size: range(0, Infinity, true),
  table_context_size: range(0, Infinity, true),
  toc_extraction: bool,
  overlapped_percent: range(0, 90),
  mineru_parse_method: enumRule("auto", "txt", "ocr"),
  mineru_formula_enable: bool,
  mineru_table_enable: bool,
  mineru_lang: enumRule(
    "English",
    "Chinese",
    "Traditional Chinese",
    "Russian",
    "Ukrainian",
    "Indonesian",
    "Spanish",
    "Vietnamese",
    "Japanese",
    "Korean",
    "Portuguese BR",
    "German",
    "French",
    "Italian",
    "Tamil",
    "Telugu",
    "Kannada",
    "Thai",
    "Greek",
    "Hindi",
    "Bulgarian",
    "Turkish",
  ),
  enable_metadata: bool,
  built_in_metadata: (v) =>
    Array.isArray(v) &&
    v.every((field: unknown) => {
      if (!object(field) || !nonempty(field.key)) return false;
      const allowed = [
        "key",
        "description",
        "descriptions",
        "enum",
        "type",
        "examples",
        "restrict_values",
      ];
      if (Object.keys(field).some((key) => !allowed.includes(key)))
        return false;
      if (
        field.type !== undefined &&
        field.type !== null &&
        (typeof field.type !== "string" ||
          !["string", "number", "list", "time"].includes(field.type))
      )
        return false;
      if (field.description !== undefined && !string(field.description))
        return false;
      if (field.descriptions !== undefined && !string(field.descriptions))
        return false;
      if (field.restrict_values !== undefined && !bool(field.restrict_values))
        return false;
      for (const key of ["enum", "examples"]) {
        const values = field[key];
        if (values === undefined || (key === "examples" && values === null))
          continue;
        if (
          !Array.isArray(values) ||
          values.some(
            (value: unknown) =>
              typeof value !== "string" &&
              !(typeof value === "number" && Number.isFinite(value)),
          )
        )
          return false;
        if (
          field.type === "number" &&
          values.some((value: unknown) => typeof value !== "number")
        )
          return false;
      }
      return true;
    }),
  llm_id: string,
  analyze_hyperlink: bool,
  hyperlink_urls: bool,
  video_prompt: string,
};
export const documentParsers = [
  "naive",
  "manual",
  "qa",
  "table",
  "paper",
  "book",
  "laws",
  "presentation",
  "picture",
  "one",
  "audio",
  "email",
  "tag",
  "resume",
  "knowledge_graph",
];
export function requiredDocumentParser(
  doc: KnowledgeDocument,
): string | undefined {
  const suffix = doc.name.toLowerCase().split(".").pop() ?? "";
  if (
    doc.type === "visual" ||
    [
      "jpg",
      "jpeg",
      "png",
      "gif",
      "bmp",
      "tif",
      "tiff",
      "webp",
      "svg",
      "ico",
      "mp4",
      "mov",
      "avi",
      "flv",
      "mpeg",
      "mpg",
      "webm",
      "wmv",
      "3gp",
      "3gpp",
      "mkv",
    ].includes(suffix)
  )
    return "picture";
  if (
    doc.type === "aural" ||
    [
      "mp3",
      "wav",
      "aac",
      "flac",
      "ogg",
      "aiff",
      "au",
      "midi",
      "wma",
      "da",
      "wave",
      "realaudio",
      "vqf",
      "oggvorbis",
      "ape",
    ].includes(suffix)
  )
    return "audio";
  if (["ppt", "pptx", "pages"].includes(suffix)) return "presentation";
  if (["eml", "msg"].includes(suffix)) return "email";
}
export function sameDocumentValue(a: unknown, b: unknown): boolean {
  if (Array.isArray(a) && Array.isArray(b))
    return (
      a.length === b.length &&
      a.every((v: unknown, i) => sameDocumentValue(v, b[i]))
    );
  if (object(a) && object(b))
    return (
      Object.keys(a).length === Object.keys(b).length &&
      Object.keys(a).every(
        (k) => Object.hasOwn(b, k) && sameDocumentValue(a[k], b[k]),
      )
    );
  return a === b;
}
export function documentMetadataValues(doc: KnowledgeDocument): ObjectValue {
  return Object.fromEntries(
    doc.meta_fields.flatMap((row) =>
      typeof row.key === "string"
        ? [[row.key, row.value]]
        : Object.entries(row),
    ),
  );
}
// Historical snapshots may contain fields rejected by the write DTO. Expose only
// editable fields; the server merges the submitted patch into the stored config.
export function editableDocumentConfig(config: ObjectValue): ObjectValue {
  return Object.fromEntries(
    Object.entries(config).flatMap(([k, v]) => {
      const fields = Object.hasOwn(nested, k) ? nested[k] : undefined;
      if (fields && object(v))
        return [
          [
            k,
            Object.fromEntries(
              Object.entries(v).filter(([field]) =>
                Object.hasOwn(fields, field),
              ),
            ),
          ],
        ];
      return Object.hasOwn(rules, k) ? [[k, v]] : [];
    }),
  );
}
export interface DocumentEditorValues {
  advanced: string;
  parser_id: string;
  leave_pipeline?: boolean;
  enable_metadata?: boolean;
  chunk_token_num?: number | null;
  delimiter?: string;
  metadata: string;
  template: string;
}
export function buildDocumentPatch(
  doc: KnowledgeDocument,
  values: DocumentEditorValues,
): ObjectValue {
  const current = doc.parser_config;
  const advanced = parseObjectJSON(values.advanced);
  if (
    Object.hasOwn(advanced, "enable_metadata") &&
    values.enable_metadata !== undefined &&
    advanced.enable_metadata !== values.enable_metadata
  )
    throw new Error(
      "Conflicting metadata switch values; use the metadata switch control",
    );
  const proposed: ObjectValue = {
    ...advanced,
    ...(values.enable_metadata !== undefined
      ? { enable_metadata: values.enable_metadata }
      : {}),
    ...(values.chunk_token_num != null
      ? { chunk_token_num: values.chunk_token_num }
      : {}),
    ...(values.delimiter !== undefined ? { delimiter: values.delimiter } : {}),
  };
  const patch: ObjectValue = {};
  const configPatch: ObjectValue = {};
  for (const [key, value] of Object.entries(proposed)) {
    const nestedRules = Object.hasOwn(nested, key) ? nested[key] : undefined;
    if (nestedRules) {
      if (!object(value)) throw new Error(`Invalid parser_config.${key}`);
      const fields: ObjectValue = {};
      for (const [field, next] of Object.entries(value)) {
        const rule = Object.hasOwn(nestedRules, field)
          ? nestedRules[field]
          : undefined;
        if (!rule) throw new Error(`Unsupported parser_config.${key}.${field}`);
        if (
          sameDocumentValue(
            next,
            object(current[key]) ? current[key][field] : undefined,
          )
        )
          continue;
        if (!rule(next))
          throw new Error(`Invalid parser_config.${key}.${field}`);
        fields[field] = next;
      }
      if (Object.keys(fields).length) configPatch[key] = fields;
    } else {
      const rule = Object.hasOwn(rules, key) ? rules[key] : undefined;
      if (!rule) throw new Error(`Unsupported parser_config.${key}`);
      if (sameDocumentValue(value, current[key])) continue;
      if (!rule(value)) throw new Error(`Invalid parser_config.${key}`);
      configPatch[key] = value;
    }
  }
  const proposedParent = object(proposed.parent_child)
    ? proposed.parent_child
    : {};
  const storedParent = object(current.parent_child) ? current.parent_child : {};
  if (
    object(configPatch.parent_child) &&
    configPatch.parent_child.use_parent_child === true &&
    storedParent.use_parent_child === false &&
    typeof proposedParent.children_delimiter === "string" &&
    proposedParent.children_delimiter
  )
    configPatch.parent_child.children_delimiter =
      proposedParent.children_delimiter;
  if (
    configPatch.enable_children === true &&
    current.enable_children === false &&
    typeof proposed.children_delimiter === "string" &&
    proposed.children_delimiter
  )
    configPatch.children_delimiter = proposed.children_delimiter;
  const parent = object(configPatch.parent_child)
    ? configPatch.parent_child
    : {};
  if (
    "enable_children" in configPatch &&
    "use_parent_child" in parent &&
    configPatch.enable_children !== parent.use_parent_child
  )
    throw new Error("Conflicting parent-child settings");
  if (
    "children_delimiter" in configPatch &&
    "children_delimiter" in parent &&
    configPatch.children_delimiter !== parent.children_delimiter
  )
    throw new Error("Conflicting child delimiters");
  if (
    configPatch.children_delimiter === "" &&
    configPatch.enable_children !== false &&
    parent.use_parent_child !== false
  )
    throw new Error(
      "An empty child delimiter requires explicitly disabling parent-child parsing",
    );
  if (Object.keys(configPatch).length) patch.parser_config = configPatch;
  if (
    values.parser_id !== doc.parser_id ||
    (values.leave_pipeline && doc.pipeline_id)
  ) {
    if (
      !documentParsers.includes(values.parser_id) ||
      (requiredDocumentParser(doc) &&
        requiredDocumentParser(doc) !== values.parser_id)
    )
      throw new Error("Unsupported document parser");
    patch.parser_id = values.parser_id;
    if (doc.pipeline_id) patch.pipeline_id = "";
  }
  const metadata = parseObjectJSON(values.metadata);
  const previousMetadata = documentMetadataValues(doc);
  if (!sameDocumentValue(metadata, previousMetadata)) {
    const fields = metadataFields(
      doc.parser_config.metadata as MetadataDefinition | undefined,
    );
    for (const [key, value] of Object.entries(metadata)) {
      if (!sameDocumentValue(value, previousMetadata[key]))
        validateMetadataValue(
          value,
          fields.find((field) => (field.key ?? field.name) === key),
        );
    }
    patch.meta_fields = metadata;
  }
  return patch;
}

// Advance the comparison draft with only the user's verified edits. A fresh
// server snapshot may contain concurrent changes that the form never touched.
export function advanceDocumentDraft(
  doc: KnowledgeDocument,
  patch: ObjectValue,
  template?: MetadataDefinition,
): KnowledgeDocument {
  const merge = (stored: ObjectValue, delta: ObjectValue): ObjectValue => {
    const result = { ...stored };
    for (const [key, value] of Object.entries(delta))
      result[key] =
        object(value) && object(stored[key])
          ? merge(stored[key], value)
          : value;
    return result;
  };
  const config = object(patch.parser_config)
    ? merge(doc.parser_config, patch.parser_config)
    : { ...doc.parser_config };
  if (template !== undefined) config.metadata = template;
  return {
    ...doc,
    ...patch,
    parser_config: config,
    meta_fields: object(patch.meta_fields)
      ? Object.entries(patch.meta_fields).map(([key, value]) => ({
          key,
          value,
        }))
      : doc.meta_fields,
  };
}

// meta_fields is a replacement contract. Merge only edited/deleted keys into a
// fresh read so edits to one value do not replay untouched stale values.
export function rebaseDocumentMetadataPatch(
  baseline: KnowledgeDocument,
  patch: ObjectValue,
  latest: KnowledgeDocument,
  pending?: ObjectValue,
): ObjectValue {
  if (!object(patch.meta_fields)) return patch;
  const before = documentMetadataValues(baseline);
  const desired = patch.meta_fields;
  const attempted = object(pending?.meta_fields) ? pending.meta_fields : before;
  let values = documentMetadataValues(latest);
  for (const key of new Set([
    ...Object.keys(before),
    ...Object.keys(attempted),
    ...Object.keys(desired),
  ])) {
    if (
      Object.hasOwn(before, key) === Object.hasOwn(desired, key) &&
      sameDocumentValue(before[key], desired[key]) &&
      Object.hasOwn(attempted, key) === Object.hasOwn(desired, key) &&
      sameDocumentValue(attempted[key], desired[key])
    )
      continue;
    if (!Object.hasOwn(desired, key))
      values = Object.fromEntries(
        Object.entries(values).filter(([name]) => name !== key),
      );
    else values = { ...values, [key]: desired[key] };
  }
  return { ...patch, meta_fields: values };
}

export function documentPatchReadbackMatches(
  actual: KnowledgeDocument,
  patch: ObjectValue,
): boolean {
  const expected = { ...patch };
  delete expected.meta_fields;
  if (patch.pipeline_id === "") {
    if (typeof actual.pipeline_id === "string" && actual.pipeline_id)
      return false;
    delete expected.pipeline_id;
  }
  if (object(patch.parser_config)) {
    const config = { ...patch.parser_config };
    const parent = object(config.parent_child) ? config.parent_child : {};
    if (parent.use_parent_child === false || config.enable_children === false) {
      // MultiRAG flattens an explicit disable to {}, false and an empty delimiter.
      if (
        !sameDocumentValue(actual.parser_config.parent_child, {}) ||
        actual.parser_config.enable_children !== false ||
        actual.parser_config.children_delimiter !== ""
      )
        return false;
      delete config.parent_child;
      delete config.enable_children;
      delete config.children_delimiter;
    }
    expected.parser_config = config;
  }
  return (
    containsKnowledgeFields(actual, expected) &&
    (!Object.hasOwn(patch, "meta_fields") ||
      sameDocumentValue(documentMetadataValues(actual), patch.meta_fields))
  );
}

// Mirror MultiRAG eb5546c common.metadata_config.field_schema / turn2jsonschema for the
// document GET projection. Full schemas remain untouched so required, deletion
// and custom constraints are verified, not reduced to a list of field names.
export function documentTemplateReadbackMatches(
  actual: unknown,
  expected: MetadataDefinition,
): boolean {
  const project = (definition: unknown): unknown => {
    if (!Array.isArray(definition)) return definition;
    if (!definition.length) return {};
    const properties: [string, ObjectValue][] = [];
    for (const entry of definition as unknown[]) {
      if (!object(entry)) return undefined;
      // eslint-disable-next-line @typescript-eslint/prefer-nullish-coalescing -- mirror Python key or name, including empty-string fallback
      const key = entry.key || entry.name;
      if (typeof key !== "string" || !key) return undefined;
      const controls = [
        "key",
        "name",
        "type",
        "description",
        "enum",
        "examples",
        "restrict_values",
        "descriptions",
      ];
      const property: ObjectValue = Object.fromEntries(
        Object.entries(entry).filter(([name]) => !controls.includes(name)),
      );
      // eslint-disable-next-line @typescript-eslint/prefer-nullish-coalescing -- match committed field_schema description fallback
      property.description = entry.description || entry.descriptions || "";
      const kind = entry.type;
      const examples = entry.examples;
      const values = Object.hasOwn(entry, "enum")
        ? entry.enum
        : entry.restrict_values === true
          ? examples
          : undefined;
      const numeric = (items: unknown[]) =>
        items.map((value) => {
          if (
            (typeof value !== "number" && typeof value !== "string") ||
            (typeof value === "string" && !value.trim()) ||
            !Number.isFinite(Number(value))
          )
            throw new Error("Invalid metadata numeric projection");
          return Number(value);
        });
      if (kind === "list") {
        let items = Object.hasOwn(property, "items") ? property.items : {};
        if (!object(items) && typeof items !== "boolean") return undefined;
        if (object(items))
          items = {
            ...items,
            type: Object.hasOwn(items, "type") ? items.type : "string",
          };
        else if (items && Array.isArray(values) && values.length)
          items = { type: "string" };
        if (object(items) && Array.isArray(values) && values.length)
          items = { ...items, enum: values };
        property.type = "array";
        property.items = items;
      } else {
        if (kind) property.type = kind === "time" ? "string" : kind;
        if (Array.isArray(values) && values.length) {
          property.enum =
            kind === "number" ? numeric(values as unknown[]) : values;
          property.type ??= "string";
        }
      }
      if (Array.isArray(examples) && entry.restrict_values !== true)
        property.examples =
          kind === "number"
            ? numeric(examples as unknown[])
            : kind === "list"
              ? (examples as unknown[]).map((value) => [value])
              : examples;
      properties.push([key, property]);
    }
    return {
      type: "object",
      properties: Object.fromEntries(properties),
      additionalProperties: false,
    };
  };
  try {
    const projectedActual = project(actual);
    const projectedExpected = project(expected);
    return (
      projectedActual !== undefined &&
      projectedExpected !== undefined &&
      sameDocumentValue(projectedActual, projectedExpected)
    );
  } catch {
    return false;
  }
}

export class EmptyDocumentFilterValue extends Error {}
export class ReservedDocumentFilterKey extends Error {}
export class NonFiniteDocumentFilterValue extends Error {}
export function documentExactMetadataFilter(text: string): string {
  const fields = parseObjectJSON(text.trim() || "{}");
  const entries: [string, unknown[]][] = [];
  for (const [key, value] of Object.entries(fields)) {
    if (key === "empty_metadata") throw new ReservedDocumentFilterKey();
    const values = Array.isArray(value) ? value : [value];
    if (
      !values.length ||
      values.some(
        (item: unknown) =>
          item === null || (typeof item === "string" && !item.trim()),
      )
    )
      throw new EmptyDocumentFilterValue();
    if (
      values.some(
        (item: unknown) =>
          !["string", "number", "boolean"].includes(typeof item),
      )
    )
      throw new Error(
        "Exact metadata filters require scalar values or scalar lists",
      );
    if (
      values.some(
        (item: unknown) => typeof item === "number" && !Number.isFinite(item),
      )
    )
      throw new NonFiniteDocumentFilterValue();
    entries.push([key, values]);
  }
  return entries.length ? JSON.stringify(Object.fromEntries(entries)) : "";
}

export class UnconfirmedDocumentModeChange extends Error {}
export function sameDocumentMode(
  a: KnowledgeDocument,
  b: KnowledgeDocument,
): boolean {
  const pipeline = (doc: KnowledgeDocument) =>
    typeof doc.pipeline_id === "string" ? doc.pipeline_id : "";
  return a.parser_id === b.parser_id && pipeline(a) === pipeline(b);
}

// An acknowledged write can have an unavailable readback. Keep the submitted
// paths pending even if the user changes the draft back to its original value.
export function buildDocumentRetryPatch(
  baseline: KnowledgeDocument,
  values: DocumentEditorValues,
  latest: KnowledgeDocument,
  pending?: ObjectValue,
): ObjectValue {
  const normal = buildDocumentPatch(baseline, values);
  if (!pending) return normal;
  const attempted = advanceDocumentDraft(baseline, pending);
  const retry = buildDocumentPatch(attempted, values);
  const patch = { ...normal, ...retry };
  const config = object(normal.parser_config)
    ? { ...normal.parser_config }
    : {};
  if (object(retry.parser_config))
    for (const [key, value] of Object.entries(retry.parser_config))
      config[key] =
        object(value) && object(config[key])
          ? { ...config[key], ...value }
          : value;
  if (Object.keys(config).length) patch.parser_config = config;
  if (
    Object.hasOwn(pending, "parser_id") ||
    Object.hasOwn(pending, "pipeline_id")
  ) {
    // Reverting an unconfirmed flow exit cannot pretend to roll back its reset.
    if (
      baseline.pipeline_id &&
      !values.leave_pipeline &&
      values.parser_id === baseline.parser_id
    ) {
      if (!sameDocumentMode(latest, baseline))
        throw new UnconfirmedDocumentModeChange();
      delete patch.parser_id;
      delete patch.pipeline_id;
    } else {
      if (
        !sameDocumentMode(latest, baseline) &&
        !sameDocumentMode(latest, attempted)
      )
        throw new UnconfirmedDocumentModeChange();
      patch.parser_id = values.parser_id;
      patch.pipeline_id = "";
    }
  }
  const pendingConfig = object(pending.parser_config)
    ? pending.parser_config
    : {};
  const pendingParent = object(pendingConfig.parent_child)
    ? pendingConfig.parent_child
    : {};
  const proposed = parseObjectJSON(values.advanced);
  const proposedParent = object(proposed.parent_child)
    ? proposed.parent_child
    : {};
  if (
    (pendingParent.use_parent_child === false ||
      pendingConfig.enable_children === false) &&
    latest.parser_config.enable_children === false &&
    (proposedParent.use_parent_child === true ||
      proposed.enable_children === true)
  ) {
    const delimiter =
      proposedParent.children_delimiter ?? proposed.children_delimiter;
    if (typeof delimiter === "string" && delimiter) {
      if (object(config.parent_child))
        config.parent_child = {
          ...config.parent_child,
          children_delimiter: delimiter,
        };
      else config.children_delimiter = delimiter;
      patch.parser_config = config;
    }
  }
  return patch;
}
