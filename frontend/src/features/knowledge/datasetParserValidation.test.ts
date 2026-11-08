import { describe, expect, it } from "vitest";
import { validateDatasetParserConfig } from "./datasetParserValidation";
import { datasetToFormValues, formValuesToInput } from "./KnowledgeForm";
import { normalizeDataset } from "@/api/knowledge";

describe("dataset parser configuration contract", () => {
  it("accepts NER and graph execution budgets including zero phase timeout", () => {
    expect(() => {
      validateDatasetParserConfig({
        graphrag: {
          method: "ner",
          batch_chunk_token_size: 8196,
          retry_attempts: 10,
          merge_timeout_seconds: 0,
          lock_acquire_timeout_seconds: 0,
          build_subgraph_retry_attempts: 1,
          retry_backoff_seconds: 0.25,
        },
        raptor: {
          scope: "dataset",
          tree_builder: "psi",
          clustering_method: "ahc",
          ext: { psi_bucket_size: 4096 },
        },
        overlapped_percent: 0.5,
      });
    }).not.toThrow();
  });
  it.each([
    { graphrag: { retry_attempts: 0 } },
    { graphrag: { merge_retry_attempts: null } },
    { graphrag: { batch_chunk_token_size: 8197 } },
    { graphrag: { merge_timeout_seconds: 1.1 } },
    { graphrag: { build_subgraph_min_timeout_seconds: 0 } },
    { graphrag: { retry_backoff_seconds: 601 } },
    { graphrag: { method: "unknown" } },
    { graphrag: { use_graphrag: "false" } },
    { raptor: { scope: "all" } },
    { raptor: { tree_builder: "other" } },
    { raptor: { clustering_method: "none" } },
    { raptor: { ext: { psi_bucket_size: true } } },
    { raptor: { max_cluster: 1025 } },
    { overlapped_percent: 91 },
    { chunk_token_num: 2049 },
    { auto_questions: 11 },
    { ext: [] },
  ])("rejects invalid known dataset fields %j", (config) => {
    expect(() => {
      validateDatasetParserConfig(config);
    }).toThrow();
  });
  it("validates effective flattened extensions and keeps sparse GraphRAG/RAPTOR updates", () => {
    expect(() => {
      validateDatasetParserConfig({
        graphrag: { retry_attempts: 2 },
        ext: { graphrag: { retry_attempts: 11 } },
      });
    }).toThrow();
    const original = normalizeDataset({
      id: "kb",
      name: "Graphs",
      parser_config: {
        graphrag: {
          method: "ner",
          merge_timeout_seconds: 0,
          retry_attempts: 3,
          future: true,
        },
        raptor: {
          scope: "dataset",
          tree_builder: "psi",
          ext: { psi_bucket_size: 30, future: true },
        },
      },
    });
    const values = datasetToFormValues(original);
    expect(values.graphrag_compatible).toBe(true);
    expect(values.graphrag_method).toBe("ner");
    values.parser_config_extra = JSON.stringify({
      ...original.parser_config,
      graphrag: {
        ...(original.parser_config.graphrag as object),
        retry_attempts: 4,
      },
      raptor: { ...(original.parser_config.raptor as object), scope: "file" },
    });
    expect(formValuesToInput(values, { original })).toEqual({
      parser_config: {
        graphrag: { retry_attempts: 4 },
        raptor: { scope: "file" },
      },
    });
  });
  it("preserves invalid historical snapshots on unrelated edits but rejects changed invalid data", () => {
    const original = normalizeDataset({
      id: "kb",
      name: "Old",
      parser_config: {
        chunk_token_num: 8192,
        graphrag: { method: "future" },
        table_column_mode: "legacy",
      },
    });
    const values = datasetToFormValues(original);
    expect(
      formValuesToInput({ ...values, name: "Renamed" }, { original }),
    ).toEqual({ name: "Renamed" });
    expect(() =>
      formValuesToInput({ ...values, chunk_token_num: 4096 }, { original }),
    ).toThrow();
  });
});

it("keeps canonical numeric model references and effective graph extensions intact", () => {
  const original = normalizeDataset({ id: "kb", name: "Scoped", embd_id: "9007199254740993", parser_config: { layout_recognize: "9007199254740995", ext: { graphrag: { method: "ner", retry_attempts: 3, merge_timeout_seconds: 0 } } } });
  const values = datasetToFormValues(original);
  expect(values.graphrag_method).toBe("ner");
  expect(formValuesToInput(values)).toMatchObject({ embd_id: "9007199254740993", parser_config: { layout_recognize: "9007199254740995" } });
  expect(formValuesToInput({ ...values, graphrag_execution: { ...values.graphrag_execution, retry_attempts: 4 } }, { original })).toEqual({ parser_config: { ext: { graphrag: { retry_attempts: 4 } } } });
  expect(() => formValuesToInput({ ...values, graphrag_execution: { retry_attempts: 5 }, parser_config_extra: '{"ext":{"graphrag":{"method":"ner","retry_attempts":6}}}' }, { original })).toThrow();
});
