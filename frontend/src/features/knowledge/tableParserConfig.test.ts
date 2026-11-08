import { describe, expect, it } from "vitest";
import {
  buildTableUploadConfig,
  datasetTableFields,
  mergeDatasetTableFields,
  validateDatasetTableConfig,
} from "./tableParserConfig";
import { parseTableHeader, extractTableHeaders } from "./tableHeaders";
import { datasetToFormValues, formValuesToInput } from "./KnowledgeForm";
import { normalizeDataset, normalizeDocument } from "@/api/knowledge";
import { buildDocumentPatch, editableDocumentConfig } from "./documentSettings";
const dataset = (parser_config: Record<string, unknown>) =>
  normalizeDataset({
    id: "kb",
    name: "Tables",
    parser_id: "table",
    parser_config,
  });

describe("table dataset and upload contracts", () => {
  it("keeps raw dotted, whitespace and prototype-like names and normalizes vectorize only on writes", () => {
    const config = JSON.parse(
      '{"table_column_mode":"manual","table_column_names":[" a.b ","__proto__"],"table_column_roles":{" a.b ":"vectorize","__proto__":"metadata"}}',
    );
    const original = dataset(config);
    const values = datasetToFormValues(original);
    expect(
      formValuesToInput({ ...values, name: "Renamed" }, { original }),
    ).toEqual({ name: "Renamed" });
    expect(
      formValuesToInput(
        {
          ...values,
          table_config: {
            ...values.table_config,
            table_column_roles: {
              ...values.table_config.table_column_roles,
              "new.col": "vectorize",
            },
          },
        },
        { original },
      ),
    ).toEqual({
      parser_config: {
        table_column_roles: {
          " a.b ": "indexing",
          ["__proto__"]: "metadata",
          "new.col": "indexing",
        },
      },
    });
    expect(config.table_column_roles[" a.b "]).toBe("vectorize");
    expect(validateDatasetTableConfig(config)).toEqual({
      ...config,
      table_column_roles: { " a.b ": "indexing", ["__proto__"]: "metadata" },
    });
  });
  it("keeps dataset defaults separate from per-file upload overrides", () => {
    const defaults = {
      table_column_mode: "manual",
      table_column_roles: { "a.b": "metadata" },
      table_column_names: ["a.b"],
    };
    const values = datasetToFormValues(dataset(defaults));
    expect(formValuesToInput(values).parser_config).toMatchObject(defaults);
    expect(buildTableUploadConfig("inherit", {})).toBeUndefined();
    expect(buildTableUploadConfig("auto", { "a.b": "metadata" })).toEqual({
      table_column_mode: "auto",
      table_column_roles: {},
    });
    expect(buildTableUploadConfig("manual", { "a.b": "both" })).toEqual({
      table_column_mode: "manual",
      table_column_roles: { "a.b": "both" },
    });
    expect(defaults.table_column_roles["a.b"]).toBe("metadata");
  });
  it.each([
    { table_column_mode: "inherit" },
    { table_column_roles: [] },
    { table_column_roles: { x: "drop" } },
    { table_column_names: [2] },
    { table_column_overrides: {} },
    { ext: { table_column_roles: { x: "wrong" } } },
  ])("rejects invalid dataset config %j", (config) => {
    expect(() => validateDatasetTableConfig(config)).toThrow();
  });
  it("validates effective legacy ext fields at the correct level and preserves unrelated entries", () => {
    expect(
      validateDatasetTableConfig({
        ext: {
          table_column_roles: { "a.b": "vectorize" },
          future: { keep: true },
        },
      }),
    ).toEqual({
      ext: {
        table_column_roles: { "a.b": "indexing" },
        future: { keep: true },
      },
    });
    const fields = datasetTableFields({
      table_column_mode: "auto",
      ext: { table_column_mode: "manual" },
    });
    expect(fields.table_config.table_column_mode).toBe("manual");
    expect(
      mergeDatasetTableFields(
        { ext: { table_column_mode: "manual", future: true } },
        { ...fields, table_config: { table_column_mode: "auto" } },
      ),
    ).toEqual({ ext: { table_column_mode: "auto", future: true } });
  });
  it("rejects conflicting advanced and structured edits", () => {
    const values = datasetToFormValues(
      dataset({ table_column_mode: "auto", table_column_roles: { a: "both" } }),
    );
    expect(() =>
      formValuesToInput({
        ...values,
        table_config: {
          ...values.table_config,
          table_column_roles: { a: "metadata" },
        },
        parser_config_extra: '{"table_column_roles":{"a":"indexing"}}',
      }),
    ).toThrow(/Conflicting table/);
  });
  it("does not widen the strict document PATCH or replay table overrides", () => {
    const doc = normalizeDocument({
      id: "d",
      name: "a.csv",
      parser_id: "table",
      parser_config: {
        table_column_mode: "manual",
        table_column_roles: { "a.b": "metadata" },
        table_column_names: ["a.b"],
        table_column_overrides: { table_column_mode: "auto" },
        graphrag: { method: "ner", retry_attempts: 4 },
      },
    });
    expect(editableDocumentConfig(doc.parser_config)).toEqual({
      graphrag: { method: "ner" },
    });
    const base = { parser_id: "table", metadata: "{}", template: "[]" };
    expect(
      buildDocumentPatch(doc, {
        ...base,
        advanced: JSON.stringify(editableDocumentConfig(doc.parser_config)),
      }),
    ).toEqual({});
    for (const advanced of [
      { table_column_mode: "auto" },
      { graphrag: { method: "ner" } },
      { graphrag: { retry_attempts: 3 } },
      { raptor: { tree_builder: "psi" } },
    ]) {
      const fresh = normalizeDocument({
        id: "d",
        name: "a.csv",
        parser_id: "table",
      });
      expect(() =>
        buildDocumentPatch(fresh, {
          ...base,
          advanced: JSON.stringify(advanced),
        }),
      ).toThrow();
    }
  });
  it("rejects upload-only invalid roles without mutating caller state", () => {
    expect(() =>
      buildTableUploadConfig("manual", { x: "vectorize" as "indexing" }),
    ).toThrow();
    expect(() =>
      buildTableUploadConfig("manual", { " ": "metadata" }),
    ).toThrow();
  });
});

describe("bounded table header preview", () => {
  it("preserves original headers, commas, quote escapes, tabs and multiline names", () => {
    expect(
      parseTableHeader('\uFEFF" a.b ","x,y","a""b",__proto__\r\n1,2,3,4'),
    ).toEqual([" a.b ", "x,y", 'a"b', "__proto__"]);
    expect(parseTableHeader('a.b\t"multi\nline"\r\n1\t2', "\t")).toEqual([
      "a.b",
      "multi\nline",
    ]);
  });
  it.each(["", "a,a", "a, ", '"unclosed,a', '"x"y,a', 'x"y,a'])(
    "refuses ambiguous headers %j",
    (text) => {
      expect(() => parseTableHeader(text)).toThrow();
    },
  );
  it("reads only supported file headers and leaves unknown formats inheritable", async () => {
    expect(
      await extractTableHeaders(new File(["a.b,b\n1,2"], "a.csv")),
    ).toEqual(["a.b", "b"]);
    await expect(
      extractTableHeaders(new File(["binary"], "a.xlsx")),
    ).rejects.toThrow();
  });
});

it("replaces the complete role map, retaining untouched columns and honoring explicit removals", () => {
  const original = dataset({
    table_column_mode: "manual",
    table_column_roles: { A: "metadata", B: "indexing" },
  });
  const values = datasetToFormValues(original);
  expect(
    formValuesToInput(
      {
        ...values,
        table_config: {
          ...values.table_config,
          table_column_roles: { A: "both", B: "indexing" },
        },
      },
      { original },
    ),
  ).toEqual({
    parser_config: { table_column_roles: { A: "both", B: "indexing" } },
  });
  expect(
    formValuesToInput(
      {
        ...values,
        parser_config_extra: '{"table_column_roles":{"A":"metadata"}}',
      },
      { original },
    ),
  ).toEqual({ parser_config: { table_column_roles: { A: "metadata" } } });
  expect(
    formValuesToInput(
      { ...values, parser_config_extra: '{"table_column_roles":{}}' },
      { original },
    ),
  ).toEqual({ parser_config: { table_column_roles: {} } });
  const legacy = dataset({
    ext: { table_column_roles: { A: "metadata", B: "indexing" } },
  });
  const hydrated = datasetToFormValues(legacy);
  expect(
    formValuesToInput(
      {
        ...hydrated,
        parser_config_extra:
          '{"ext":{"table_column_roles":{"A":"both","B":"indexing"}}}',
      },
      { original: legacy },
    ),
  ).toEqual({
    parser_config: {
      ext: { table_column_roles: { A: "both", B: "indexing" } },
    },
  });
  expect(
    formValuesToInput(
      { ...hydrated, parser_config_extra: '{"ext":{"table_column_roles":{}}}' },
      { original: legacy },
    ),
  ).toEqual({ parser_config: { ext: { table_column_roles: {} } } });
});
