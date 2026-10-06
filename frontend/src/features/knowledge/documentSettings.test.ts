import { expect, it } from "vitest";
import { normalizeDocument } from "@/api/knowledge";
import {
  buildDocumentPatch,
  buildDocumentRetryPatch,
  advanceDocumentDraft,
  documentPatchReadbackMatches,
  documentMetadataValues,
  rebaseDocumentMetadataPatch,
  documentTemplateReadbackMatches,
  documentExactMetadataFilter,
  editableDocumentConfig,
  requiredDocumentParser,
} from "./documentSettings";
const doc = normalizeDocument({
  id: "d",
  name: "notes.txt",
  pipeline_id: "flow",
  parser_config: {
    chunk_token_num: 512,
    delimiter: "\\n",
    control_panel: {},
    raptor: { use_raptor: true, legacy: "x", max_token: 256 },
  },
  meta_fields: { version: "v1" },
});
it.each([
  ['{"__proto__":"x"}', '{"__proto__":["x"]}'],
  ['{"__proto__":"x","revision":0}', '{"__proto__":["x"],"revision":[0]}'],
])("preserves own exact-filter keys in %s", (input, expected) => {
  expect(documentExactMetadataFilter(input)).toBe(expected);
});
it.each([
  '{"revision":1e309}',
  '{"revision":-1e309}',
  '{"revision":[0,1e309]}',
])("rejects non-finite exact-filter numbers in %s", (input) => {
  expect(() => documentExactMetadataFilter(input)).toThrow();
});
it("preserves exponent-looking text filter values", () => {
  expect(documentExactMetadataFilter('{"revision":"1e309"}')).toBe(
    '{"revision":["1e309"]}',
  );
});
const initial = {
  parser_id: doc.parser_id,
  advanced: '{"raptor":{"use_raptor":true,"max_token":256}}',
  chunk_token_num: 512,
  delimiter: "\\n",
  metadata: '{"version":"v1"}',
  template: "[]",
};
it("omits the mode and every unchanged setting", () => {
  expect(buildDocumentPatch(doc, initial)).toEqual({});
});
it("only patches a changed nested key without replaying a snapshot", () => {
  expect(
    buildDocumentPatch(doc, {
      ...initial,
      advanced: '{"raptor":{"use_raptor":false,"max_token":256}}',
    }),
  ).toEqual({ parser_config: { raptor: { use_raptor: false } } });
});
it("leaves omitted settings on the server", () => {
  expect(buildDocumentPatch(doc, { ...initial, advanced: "{}" })).toEqual({});
});
it("omits historical keys from the editable snapshot", () => {
  expect(editableDocumentConfig(doc.parser_config)).toEqual({
    chunk_token_num: 512,
    delimiter: "\\n",
    raptor: { use_raptor: true, max_token: 256 },
  });
});
it("requires an explicit mode change to leave a DataFlow", () => {
  expect(buildDocumentPatch(doc, { ...initial, parser_id: "manual" })).toEqual({
    parser_id: "manual",
    pipeline_id: "",
  });
});
it("clears metadata values with an explicit empty object", () => {
  expect(buildDocumentPatch(doc, { ...initial, metadata: "{}" })).toEqual({
    meta_fields: {},
  });
});
it.each([
  '{"control_panel":{}}',
  '{"raptor":{"legacy":1}}',
  '{"auto_keywords":33}',
  '{"raptor":{"max_token":0}}',
  '{"pages":[[2,1]]}',
  '{"overlapped_percent":91}',
  '{"tag_kb_ids":[""]}',
])("rejects unsupported or invalid changed fields: %s", (advanced) => {
  expect(() => buildDocumentPatch(doc, { ...initial, advanced })).toThrow();
});
it("enforces special-file source restrictions", () => {
  const image = normalizeDocument({ name: "photo.png", parser_id: "picture" });
  expect(requiredDocumentParser(image)).toBe("picture");
  expect(() =>
    buildDocumentPatch(image, { ...initial, parser_id: "naive" }),
  ).toThrow();
});
it("can explicitly leave a flow using its currently retained builtin parser", () => {
  expect(buildDocumentPatch(doc, { ...initial, leave_pipeline: true })).toEqual(
    { parser_id: doc.parser_id, pipeline_id: "" },
  );
});
it.each([
  '{"built_in_metadata":[{"key":"x","future":true}]}',
  '{"parent_child":{"use_parent_child":true},"enable_children":false}',
  '{"children_delimiter":""}',
  '{"constructor":{}}',
])("rejects invalid strict nested configuration: %s", (advanced) => {
  expect(() => buildDocumentPatch(doc, { ...initial, advanced })).toThrow();
});
it("validates only edited metadata values against their template and preserves incompatible history", () => {
  const typed = normalizeDocument({
    id: "d",
    parser_config: { metadata: [{ key: "version", type: "number" }] },
    meta_fields: { version: "historical" },
  });
  expect(
    buildDocumentPatch(typed, {
      ...initial,
      advanced: "{}",
      chunk_token_num: undefined,
      delimiter: undefined,
      metadata: '{"version":"historical"}',
    }),
  ).toEqual({});
  expect(() =>
    buildDocumentPatch(typed, {
      ...initial,
      metadata: '{"version":"invalid changed"}',
    }),
  ).toThrow("Finite number required");
  expect(
    buildDocumentPatch(typed, {
      ...initial,
      advanced: "{}",
      chunk_token_num: undefined,
      delimiter: undefined,
      metadata: '{"version":0}',
    }),
  ).toEqual({ meta_fields: { version: 0 } });
});

it("advances only the confirmed edit and never imports concurrent readback fields into the draft", () => {
  const confirmed = advanceDocumentDraft(doc, {
    parser_config: { chunk_token_num: 256 },
  });
  expect(
    buildDocumentPatch(confirmed, { ...initial, chunk_token_num: 256 }),
  ).toEqual({});
  expect(
    buildDocumentPatch(confirmed, { ...initial, chunk_token_num: 128 }),
  ).toEqual({ parser_config: { chunk_token_num: 128 } });
});
it("merges edited/deleted metadata keys into the latest replacement without touching other keys", () => {
  const baseline = normalizeDocument({
    meta_fields: { edited: "old", other: "before", remove: "before" },
  });
  const latest = normalizeDocument({
    meta_fields: {
      edited: "old",
      other: "concurrent",
      remove: "before",
      added: 0,
    },
  });
  expect(
    rebaseDocumentMetadataPatch(
      baseline,
      { meta_fields: { edited: "new", other: "before" } },
      latest,
    ),
  ).toEqual({ meta_fields: { edited: "new", other: "concurrent", added: 0 } });
});
it("requires all canonical parent-child disable aliases and the rest of the requested patch", () => {
  const actual = normalizeDocument({
    parser_config: {
      parent_child: {},
      enable_children: false,
      children_delimiter: "",
      auto_questions: 2,
    },
  });
  const patch = {
    parser_config: {
      parent_child: { use_parent_child: false, children_delimiter: "x" },
      auto_questions: 2,
    },
  };
  expect(documentPatchReadbackMatches(actual, patch)).toBe(true);
  expect(
    documentPatchReadbackMatches(
      {
        ...actual,
        parser_config: { ...actual.parser_config, enable_children: true },
      },
      patch,
    ),
  ).toBe(false);
  expect(
    documentPatchReadbackMatches(actual, {
      parser_config: { ...patch.parser_config, auto_questions: 3 },
    }),
  ).toBe(false);
});
it("matches the full real legacy metadata projection including constraints and examples", () => {
  const legacy = [
    {
      name: "tags",
      type: "list",
      descriptions: "Tags",
      examples: ["a", "b"],
      restrict_values: true,
    },
    { key: "created", type: "time" },
    { key: "price", type: "number", enum: ["0", "2"], examples: ["0"] },
  ];
  const projected = {
    type: "object",
    properties: {
      tags: {
        description: "Tags",
        type: "array",
        items: { type: "string", enum: ["a", "b"] },
      },
      created: { description: "", type: "string" },
      price: { description: "", type: "number", enum: [0, 2], examples: [0] },
    },
    additionalProperties: false,
  };
  expect(documentTemplateReadbackMatches(projected, legacy)).toBe(true);
  expect(
    documentTemplateReadbackMatches(
      { ...projected, additionalProperties: true },
      legacy,
    ),
  ).toBe(false);
  expect(
    documentTemplateReadbackMatches(
      {
        ...projected,
        properties: { ...projected.properties, undeleted: { type: "string" } },
      },
      legacy,
    ),
  ).toBe(false);
  expect(
    documentTemplateReadbackMatches(projected, {
      ...projected,
      required: ["price"],
      custom: "retained",
    }),
  ).toBe(false);
  expect(
    documentTemplateReadbackMatches(
      { ...projected, required: ["price"], custom: "retained" },
      { ...projected, required: ["price"], custom: "retained" },
    ),
  ).toBe(true);
  expect(documentTemplateReadbackMatches([], [])).toBe(true);
  expect(documentTemplateReadbackMatches(projected, [])).toBe(false);
});
it("wraps exact-filter zero and false in arrays while preserving their types", () => {
  expect(
    documentExactMetadataFilter(
      '{"revision":0,"active":false,"version":"v2","tags":["a","b"]}',
    ),
  ).toBe('{"revision":[0],"active":[false],"version":["v2"],"tags":["a","b"]}');
  expect(documentExactMetadataFilter("{}")).toBe("");
});
it.each([
  '{"revision":[]}',
  '{"revision":null}',
  '{"revision":""}',
  '{"revision":[null]}',
  '{"revision":[""]}',
])(
  "rejects exact predicates that MultiRAG would silently omit: %s",
  (input) => {
    expect(() => documentExactMetadataFilter(input)).toThrow();
  },
);

it("restores only explicitly reverted pending fields, without importing a concurrent snapshot", () => {
  const pending = { parser_config: { chunk_token_num: 256 } };
  const latest = normalizeDocument({
    ...doc,
    parser_config: {
      ...doc.parser_config,
      chunk_token_num: 256,
      auto_questions: 9,
    },
  });
  expect(buildDocumentRetryPatch(doc, initial, latest, pending)).toEqual({
    parser_config: { chunk_token_num: 512 },
  });
});
it("removes a reverted pending metadata insertion and preserves concurrent keys", () => {
  const baseline = normalizeDocument({ meta_fields: {} });
  const latest = normalizeDocument({
    meta_fields: { inserted: 0, other: "concurrent" },
  });
  expect(
    rebaseDocumentMetadataPatch(baseline, { meta_fields: {} }, latest, {
      meta_fields: { inserted: 0 },
    }),
  ).toEqual({ meta_fields: { other: "concurrent" } });
});
it("fails closed when reverting an unconfirmed flow exit would conceal a reset", () => {
  const pending = { parser_id: "manual", pipeline_id: "" };
  const latest = normalizeDocument({
    ...doc,
    parser_id: "manual",
    pipeline_id: "",
  });
  expect(() =>
    buildDocumentRetryPatch(doc, initial, latest, pending),
  ).toThrow();
  expect(buildDocumentRetryPatch(doc, initial, doc, pending)).toEqual({});
});
it("restores the visible preferred delimiter when re-enabling a previously disabled parent-child parser", () => {
  const baseline = normalizeDocument({
    parser_config: {
      parent_child: { use_parent_child: false, children_delimiter: "x" },
    },
  });
  const patch = buildDocumentPatch(baseline, {
    ...initial,
    parser_id: baseline.parser_id,
    chunk_token_num: undefined,
    delimiter: undefined,
    metadata: "{}",
    advanced:
      '{"parent_child":{"use_parent_child":true,"children_delimiter":"x"}}',
  });
  expect(patch).toEqual({
    parser_config: {
      parent_child: { use_parent_child: true, children_delimiter: "x" },
    },
  });
});
it("matches eb5546c legacy projection without dropping extension or list item constraints", () => {
  const legacy = [
    {
      key: "tags",
      type: "list",
      items: { type: "integer", minimum: 0, enum: [1, 2] },
      minItems: 1,
      maxItems: 2,
      custom: { keep: true },
    },
    {
      key: "code",
      type: "string",
      pattern: "^[A-Z]+$",
      format: "custom",
      maxLength: 8,
    },
  ];
  const projected = {
    type: "object",
    properties: {
      tags: {
        description: "",
        type: "array",
        items: { type: "integer", minimum: 0, enum: [1, 2] },
        minItems: 1,
        maxItems: 2,
        custom: { keep: true },
      },
      code: {
        description: "",
        type: "string",
        pattern: "^[A-Z]+$",
        format: "custom",
        maxLength: 8,
      },
    },
    additionalProperties: false,
  };
  expect(documentTemplateReadbackMatches(projected, legacy)).toBe(true);
  expect(
    documentTemplateReadbackMatches(
      {
        ...projected,
        properties: {
          ...projected.properties,
          tags: {
            ...projected.properties.tags,
            items: { type: "integer", enum: [1, 2] },
          },
        },
      },
      legacy,
    ),
  ).toBe(false);
});
it("matches boolean item schemas and the committed enum overlay semantics", () => {
  const legacy = [
    { key: "closed", type: "list", items: false, enum: ["a"] },
    { key: "open", type: "list", items: true },
    { key: "restricted", type: "list", items: true, enum: ["a"] },
  ];
  const projected = {
    type: "object",
    properties: {
      closed: { description: "", type: "array", items: false },
      open: { description: "", type: "array", items: true },
      restricted: {
        description: "",
        type: "array",
        items: { type: "string", enum: ["a"] },
      },
    },
    additionalProperties: false,
  };
  expect(documentTemplateReadbackMatches(projected, legacy)).toBe(true);
});
it("rejects the reserved exact-filter marker instead of silently changing scope", () => {
  expect(() =>
    documentExactMetadataFilter('{"empty_metadata":false,"revision":0}'),
  ).toThrow();
});
it("preserves metadata keys that are legal on the backend but special to JavaScript objects", () => {
  const row = normalizeDocument({
    meta_fields: JSON.parse('{"__proto__":{"keep":true},"other":"before"}'),
  });
  expect(documentMetadataValues(row)).toEqual(
    JSON.parse('{"__proto__":{"keep":true},"other":"before"}'),
  );
  const values = buildDocumentPatch(row, {
    ...initial,
    parser_id: row.parser_id,
    advanced: "{}",
    chunk_token_num: undefined,
    delimiter: undefined,
    metadata: JSON.stringify({
      ...documentMetadataValues(row),
      other: "after",
    }),
  });
  expect(rebaseDocumentMetadataPatch(row, values, row)).toEqual({
    meta_fields: JSON.parse('{"__proto__":{"keep":true},"other":"after"}'),
  });
});
it("does not treat an invalid explicit item schema as an omitted default", () => {
  const schema = {
    type: "object",
    properties: {
      tags: { description: "", type: "array", items: { type: "string" } },
    },
    additionalProperties: false,
  };
  expect(
    documentTemplateReadbackMatches(schema, [
      { key: "tags", type: "list", items: null },
    ]),
  ).toBe(false);
});
it("writes only an explicitly edited extraction switch through parser configuration", () => {
  const row = normalizeDocument({
    parser_config: { enable_metadata: true, metadata: [] },
  });
  const values = {
    ...initial,
    parser_id: row.parser_id,
    advanced: "{}",
    chunk_token_num: undefined,
    delimiter: undefined,
    metadata: "{}",
    enable_metadata: false,
  };
  expect(buildDocumentPatch(row, values)).toEqual({
    parser_config: { enable_metadata: false },
  });
  expect(() =>
    buildDocumentPatch(row, {
      ...values,
      advanced: '{"enable_metadata":true}',
    }),
  ).toThrow();
});
