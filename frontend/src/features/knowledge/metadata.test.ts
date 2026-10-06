import { expect, it } from "vitest";
import { metadataReadbackMatches, parseMetadataDefinition } from "./metadata";

it("normalizes finite number values while retaining unknown fields and explicit false", () => {
  expect(
    parseMetadataDefinition(
      JSON.stringify([
        {
          key: "revision",
          type: "number",
          enum: ["1", 2],
          restrict_values: false,
          future: { keep: true },
        },
      ]),
    ),
  ).toEqual([
    {
      key: "revision",
      type: "number",
      enum: [1, 2],
      restrict_values: false,
      future: { keep: true },
    },
  ]);
});
it("rejects ambiguous names and invalid numbers before writing configuration", () => {
  for (const fields of [
    [{ key: "v" }, { key: "v" }],
    [{ key: "v", name: "other" }],
    [{ key: "v", type: "number", enum: ["NaN"] }],
    [{ key: "v", type: "string", enum: [2] }],
  ])
    expect(() => parseMetadataDefinition(JSON.stringify(fields))).toThrow();
});
it("keeps JSON Schema intact and distinguishes omission from explicit clearing", () => {
  const schema = {
    type: "object",
    properties: { version: { type: "string" } },
    custom: { keep: true },
  };
  expect(parseMetadataDefinition(JSON.stringify(schema))).toEqual(schema);
  expect(parseMetadataDefinition("[]")).toEqual([]);
  expect(metadataReadbackMatches(undefined, [])).toBe(false);
  expect(() => parseMetadataDefinition(JSON.stringify(schema), true)).toThrow();
});
it("allows the read API to canonicalize the legacy name alias but detects dropped unknown data", () => {
  expect(
    metadataReadbackMatches(
      [{ key: "author", custom: 1 }],
      [{ name: "author", custom: 1 }],
    ),
  ).toBe(true);
  expect(
    metadataReadbackMatches(
      [{ key: "author" }],
      [{ name: "author", custom: 1 }],
    ),
  ).toBe(false);
});

it("accepts the empty-schema read projection without accepting a stale populated schema", () => {
  expect(metadataReadbackMatches([], {})).toBe(true);
  expect(metadataReadbackMatches({}, {})).toBe(true);
  expect(
    metadataReadbackMatches({ type: "object", properties: { old: {} } }, {}),
  ).toBe(false);
  expect(metadataReadbackMatches(undefined, {})).toBe(false);
});

import {
  documentMetadata,
  metadataBatchBody,
  metadataFields,
  metadataValuesMatch,
  previewMetadataOperation,
  validateMetadataValue,
} from "./metadata";
it("never omits the batch ID boundary and distinguishes explicit null, empty list and deletion", () => {
  expect(() =>
    metadataBatchBody([], { key: "v", mode: "set", value: 0 }),
  ).toThrow();
  expect(() => metadataBatchBody(["d"], { key: "v", mode: "set" })).toThrow();
  expect(
    metadataBatchBody(["d", "d"], { key: "v", mode: "set", value: null }),
  ).toEqual({
    selector: { document_ids: ["d"] },
    updates: [{ key: "v", value: null }],
    deletes: [],
  });
  expect(
    metadataBatchBody(["d"], { key: "v", mode: "set", value: [] }).updates[0]
      .value,
  ).toEqual([]);
  expect(
    metadataBatchBody(["d"], { key: "v", mode: "deleteField" }).deletes,
  ).toEqual([{ key: "v" }]);
});
it("previews the actual append, match and delimiter semantics while preserving other fields", () => {
  const original = { list: ["A", "B"], count: 0, flag: false };
  expect(
    previewMetadataOperation(original, { key: "list", mode: "set", value: [] }),
  ).toEqual(original);
  expect(
    previewMetadataOperation(original, {
      key: "list",
      mode: "set",
      value: ["B", "C;D"],
    }),
  ).toEqual({ ...original, list: ["A", "B", "C", "D"] });
  expect(
    previewMetadataOperation(original, {
      key: "list",
      mode: "replace",
      match: "B",
      value: "C",
    }),
  ).toEqual({ ...original, list: ["A", "C"] });
  expect(
    previewMetadataOperation(original, {
      key: "list",
      mode: "deleteValue",
      match: "A",
    }),
  ).toEqual({ ...original, list: ["B"] });
  expect(
    previewMetadataOperation(
      { list: ["A"] },
      { key: "list", mode: "deleteValue", match: "A" },
    ),
  ).toEqual({});
  expect(original).toEqual({ list: ["A", "B"], count: 0, flag: false });
});
it("extracts candidates from schemas and verifies full values including removals", () => {
  expect(
    metadataFields({
      properties: {
        count: { type: "integer" },
        tags: { type: "array", custom: true },
      },
    }),
  ).toEqual([
    { key: "count", type: "number" },
    { key: "tags", type: "list", custom: true },
  ]);
  expect(
    documentMetadata([
      { key: "count", value: 0 },
      { flag: false },
      { key: "list", value: [] },
    ]),
  ).toEqual({ count: 0, flag: false, list: [] });
  expect(metadataValuesMatch({ old: 1 }, {})).toBe(false);
  expect(metadataValuesMatch({ count: 0 }, { count: "0" })).toBe(false);
});
it("validates typed values and restricted enum choices without confusing zero or null", () => {
  expect(() => {
    validateMetadataValue(0, { type: "number" });
  }).not.toThrow();
  expect(() => {
    validateMetadataValue(null, { type: "number" });
  }).not.toThrow();
  expect(() => {
    validateMetadataValue("0", { type: "number" });
  }).toThrow();
  expect(() => {
    validateMetadataValue(["x"], {
      type: "list",
      restrict_values: true,
      enum: ["y"],
    });
  }).toThrow();
  expect(() => {
    validateMetadataValue([], { type: "list" });
  }).not.toThrow();
});
it("rejects a stale schema after a property or unknown field is removed", () => {
  expect(
    metadataReadbackMatches(
      { type: "object", properties: { old: { type: "string" } } },
      { type: "object", properties: {} },
    ),
  ).toBe(false);
  expect(
    metadataReadbackMatches([{ key: "v", custom: "old" }], [{ key: "v" }]),
  ).toBe(false);
  expect(
    metadataReadbackMatches(
      [{ key: "v", restrict_values: true, examples: ["x"], enum: ["x"] }],
      [{ key: "v", restrict_values: true, examples: ["x"] }],
    ),
  ).toBe(true);
});
it("supports an explicit empty string deletion without making blank replacement unconditional", () => {
  expect(
    metadataBatchBody(["d"], { key: "v", mode: "deleteValue", match: "" })
      .deletes,
  ).toEqual([{ key: "v", value: "" }]);
  expect(() =>
    metadataBatchBody(["d"], {
      key: "v",
      mode: "replace",
      match: "",
      value: "x",
    }),
  ).toThrow();
  expect(
    previewMetadataOperation(
      { list: [1, "1"] },
      { key: "list", mode: "set", value: [] },
    ),
  ).toEqual({ list: [1] });
});

import {
  metadataBatchOperationsBody,
  previewMetadataOperations,
} from "./metadata";
it("combines multiple field changes while keeping every deletion and the same ID boundary", () => {
  const operations = [
    { key: "count", mode: "set" as const, value: 0 },
    { key: "old", mode: "deleteField" as const },
    { key: "tags", mode: "set" as const, value: ["x"] },
  ];
  expect(metadataBatchOperationsBody(["d1"], operations)).toEqual({
    selector: { document_ids: ["d1"] },
    updates: [
      { key: "count", value: 0 },
      { key: "tags", value: ["x"] },
    ],
    deletes: [{ key: "old" }],
  });
  expect(
    previewMetadataOperations({ old: false, tags: ["y"] }, operations),
  ).toEqual({ count: 0, tags: ["y", "x"] });
  expect(() =>
    metadataBatchOperationsBody(
      ["d1"],
      [...operations, { key: "count", mode: "deleteField" }],
    ),
  ).toThrow();
});
it("normalizes schema number choices and validates schema scalar list types", () => {
  expect(
    parseMetadataDefinition(
      JSON.stringify({
        properties: {
          n: { type: "number", enum: ["0", "2.5"], custom: false },
        },
      }),
    ),
  ).toEqual({
    properties: { n: { type: "number", enum: [0, 2.5], custom: false } },
  });
  expect(() =>
    parseMetadataDefinition(
      JSON.stringify({ properties: { n: { type: "number", enum: ["NaN"] } } }),
    ),
  ).toThrow();
  expect(() => {
    validateMetadataValue([0, 2], { type: "list", items: { type: "number" } });
  }).not.toThrow();
  expect(() => {
    validateMetadataValue(["2"], { type: "list", items: { type: "number" } });
  }).toThrow();
});
it("keeps an empty list when deleting a value that does not exist", () => {
  expect(
    previewMetadataOperations({ v: [] }, [
      { key: "v", mode: "deleteValue", match: "x" },
    ]),
  ).toEqual({ v: [] });
  expect(
    previewMetadataOperations({ v: ["y"] }, [
      { key: "v", mode: "deleteValue", match: "x" },
    ]),
  ).toEqual({ v: ["y"] });
});
it("normalizes and validates number choices in nested list schemas without losing item constraints", () => {
  const schema = {
    type: "object",
    custom: false,
    properties: {
      n: {
        type: "array",
        items: { type: "number", enum: [0, "2", "3"], minimum: 0 },
      },
    },
  };
  expect(parseMetadataDefinition(JSON.stringify(schema))).toEqual({
    ...schema,
    properties: {
      n: {
        type: "array",
        items: { type: "number", enum: [0, 2, 3], minimum: 0 },
      },
    },
  });
  expect(() =>
    parseMetadataDefinition(
      JSON.stringify({
        properties: {
          n: { type: "array", items: { type: "number", enum: ["bad"] } },
        },
      }),
    ),
  ).toThrow();
});

it("validates boolean values without treating false as missing", () => {
  expect(() => {
    validateMetadataValue(false, { type: "boolean" });
  }).not.toThrow();
  expect(() => {
    validateMetadataValue("", { type: "boolean" });
  }).toThrow();
});

it("enforces scalar schema enum choices while preserving traditional unrestricted field semantics", () => {
  const field = metadataFields({
    properties: {
      category: { type: "string", enum: ["allowed"], custom: false },
    },
  })[0];
  expect(field).toEqual({
    key: "category",
    type: "string",
    enum: ["allowed"],
    custom: false,
    restrict_values: true,
  });
  expect(() => {
    validateMetadataValue("allowed", field);
  }).not.toThrow();
  expect(() => {
    validateMetadataValue("outside", field);
  }).toThrow();
  expect(() => {
    validateMetadataValue(null, field);
  }).toThrow();
  const legacy = [
    {
      key: "category",
      type: "string",
      enum: ["allowed"],
      restrict_values: false,
    },
  ];
  expect(metadataFields(legacy)).toEqual(legacy);
  expect(() => {
    validateMetadataValue("outside", metadataFields(legacy)[0]);
  }).not.toThrow();
});
it("honors committed boolean list item schemas while retaining explicit empty arrays", () => {
  expect(() =>
    { validateMetadataValue([], { type: "list", items: false }); },
  ).not.toThrow();
  expect(() =>
    { validateMetadataValue(["x"], { type: "list", items: false }); },
  ).toThrow();
  expect(() =>
    { validateMetadataValue([0, false, { retain: true }], {
      type: "list",
      items: true,
    }); },
  ).not.toThrow();
});
