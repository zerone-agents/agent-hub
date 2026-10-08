import { describe, expect, it } from "vitest";
import {
  datasetToFormValues,
  formValuesToInput,
  type DatasetFormValues,
} from "./KnowledgeForm";

describe("KnowledgeForm parser_config mapping", () => {
  it("hydrates structured parser fields and preserves unknown config", () => {
    const values = datasetToFormValues({
      id: "kb1",
      name: "Docs",
      display_name: "Docs",
      collection_name: "kb_docs",
      description: "desc",
      permission: "team",
      parser_id: "naive",
      embd_id: "bge@test",
      doc_num: 0,
      chunk_num: 0,
      parser_config: {
        layout_recognize: "MinerU",
        chunk_token_num: 900,
        image_context_size: 4,
        auto_keywords: 3,
        mineru_parse_method: "ocr",
        parent_child: {
          use_parent_child: true,
          children_delimiter: "\n##",
        },
        raptor: { use_raptor: true },
        metadata: { source: "manual" },
      },
    });

    expect(values).toMatchObject({
      layout_recognize: "MinerU",
      chunk_token_num: 900,
      image_table_context_window: 4,
      auto_keywords: 3,
      mineru_parse_method: "ocr",
      enable_children: true,
      children_delimiter: "\n##",
    });
    expect(JSON.parse(values.parser_config_extra)).toEqual({
      raptor: { use_raptor: true },
      metadata: { source: "manual" },
    });
  });

  it("merges structured fields back into parser_config without losing unknown fields", () => {
    const values: DatasetFormValues = {
      ...datasetToFormValues(null),
      name: " Docs ",
      description: "desc",
      permission: "team",
      parser_id: "naive",
      embd_id: " bge@test ",
      layout_recognize: "DeepDOC",
      chunk_token_num: 256,
      delimiter: "\n",
      enable_children: true,
      children_delimiter: "\n###",
      image_table_context_window: 5,
      auto_keywords: 2,
      auto_questions: 4,
      toc_extraction: true,
      html4excel: true,
      mineru_parse_method: "auto",
      mineru_lang: "Chinese",
      mineru_formula_enable: false,
      mineru_table_enable: true,
      parser_config_extra: JSON.stringify({
        raptor: { use_raptor: true },
        graphrag: { enabled: false },
      }),
    };

    const input = formValuesToInput(values);

    expect(input).toMatchObject({
      name: "Docs",
      embd_id: "bge@test",
      permission: "team",
    });
    expect(input.parser_config).toMatchObject({
      raptor: { use_raptor: true },
      graphrag: { enabled: false },
      layout_recognize: "DeepDOC",
      chunk_token_num: 256,
      children_delimiter: "\n###",
      image_table_context_window: 5,
      image_context_size: 5,
      table_context_size: 5,
      auto_keywords: 2,
      auto_questions: 4,
      toc_extraction: true,
      html4excel: true,
      mineru_lang: "Chinese",
      mineru_formula_enable: false,
    });
  });

  it("rejects non-object advanced JSON", () => {
    const values: DatasetFormValues = {
      ...datasetToFormValues(null),
      name: "bad",
      parser_config_extra: "[]",
    };

    expect(() => formValuesToInput(values)).toThrow(
      "高级 JSON 必须是 JSON 对象",
    );
  });
});

describe("formValuesToInput candidate-value decoding", () => {
  const baseValues = (): DatasetFormValues => ({
    ...datasetToFormValues(null),
    name: "KB",
    permission: "me",
    parser_id: "naive",
  });

  it("decodes multirag: embd_id to its raw fullId", () => {
    const input = formValuesToInput({
      ...baseValues(),
      embd_id: "multirag:bge-m3@ZHIPU-AI",
    });
    expect(input.embd_id).toBe("bge-m3@ZHIPU-AI");
  });

  it("decodes local: embd_id to the raw modelId", () => {
    const input = formValuesToInput({
      ...baseValues(),
      embd_id: "local:42:bge-large-zh",
    });
    expect(input.embd_id).toBe("bge-large-zh");
  });

  it("decodes builtin: / multirag: / local: layout_recognize to raw", () => {
    expect(
      formValuesToInput({
        ...baseValues(),
        layout_recognize: "builtin:DeepDOC",
      }).parser_config?.layout_recognize,
    ).toBe("DeepDOC");
    expect(
      formValuesToInput({
        ...baseValues(),
        layout_recognize: "multirag:MinerU",
      }).parser_config?.layout_recognize,
    ).toBe("MinerU");
    expect(
      formValuesToInput({
        ...baseValues(),
        layout_recognize: "local:7:paddleocr",
      }).parser_config?.layout_recognize,
    ).toBe("paddleocr");
  });

  it("passes unprefixed (legacy) values through unchanged", () => {
    const input = formValuesToInput({
      ...baseValues(),
      embd_id: " bge@test ",
      layout_recognize: "MinerU",
    });
    expect(input.embd_id).toBe("bge@test");
    expect(input.parser_config?.layout_recognize).toBe("MinerU");
  });

  it("omits embd_id when the model is locked by existing chunks", () => {
    const input = formValuesToInput(
      {
        ...baseValues(),
        embd_id: "bge@test",
      },
      { includeEmbeddingModel: false },
    );

    expect(input).not.toHaveProperty("embd_id");
  });
});


describe("dataset update preservation", () => {
  const original = {
    id: "kb1", name: "Docs", display_name: "Docs", collection_name: "kb_docs", description: "", permission: "me", parser_id: "future_parser", embd_id: "embed@test", doc_num: 1, chunk_num: 2,
    parser_config: { pipeline_id: "existing-flow", custom_extension: { nested: true }, parent_child: { use_parent_child: true, children_delimiter: "|", legacy: true }, image_context_size: 3, table_context_size: 7 },
  };
  it("omits unchanged modes, models and synthetic defaults for unrelated updates", () => {
    expect(formValuesToInput({ ...datasetToFormValues(original), name: "Renamed" }, { original })).toEqual({ name: "Renamed" });
  });
  it("preserves independent context windows and unknown config on a parser option change", () => {
    const input = formValuesToInput({ ...datasetToFormValues(original), auto_keywords: 3 }, { original });
    expect(input).not.toHaveProperty("parser_id");
    expect(input.parser_config).toEqual({ auto_keywords: 3 });
  });
  it("sends only explicit mode changes", () => {
    expect(formValuesToInput({ ...datasetToFormValues(original), parser_id: "resume" }, { original })).toEqual({ parser_id: "resume" });
  });
});

describe('structured GraphRAG dataset settings', () => {
  const original = {
    id: 'graph-kb', name: 'Graph KB', display_name: 'Graph KB', collection_name: 'graph_kb', description: '', permission: 'me', parser_id: 'pipeline', embd_id: 'embed@test', doc_num: 1, chunk_num: 1,
    pipeline_id: 'existing-flow',
    parser_config: { future: { keep: true }, graphrag: { use_graphrag: true, entity_types: ['person', 'custom'], method: 'general', community: true, resolution: true, future_graph: { nested: 'keep' } } },
  };
  it('patches explicit false and empty lists without resending unknown fields or parser mode', () => {
    const values = datasetToFormValues(original);
    expect(values).toMatchObject({ graphrag_enabled: true, graphrag_entity_types: ['person', 'custom'], graphrag_method: 'general', graphrag_community: true, graphrag_resolution: true });
    expect(formValuesToInput({ ...values, graphrag_enabled: false, graphrag_entity_types: [], graphrag_community: false, graphrag_resolution: false }, { original })).toEqual({ parser_config: { graphrag: { use_graphrag: false, entity_types: [], community: false, resolution: false } } });
    expect(formValuesToInput({ ...values, name: 'Renamed' }, { original })).toEqual({ name: 'Renamed' });
    expect(formValuesToInput(values, { original })).toEqual({});
  });
  it('retains all extensions when building a complete input and supports strict schema defaults', () => {
    const values = datasetToFormValues(original);
    expect(formValuesToInput({ ...values, graphrag_method: 'light' }).parser_config).toMatchObject({ future: { keep: true }, graphrag: { ...original.parser_config.graphrag, method: 'light' } });
    expect(formValuesToInput({ ...datasetToFormValues(null), name: 'New', graphrag_enabled: true }).parser_config?.graphrag).toEqual({ use_graphrag: true });
    expect(formValuesToInput(datasetToFormValues(null)).parser_config?.graphrag).toEqual({ use_graphrag: false, entity_types: ['organization', 'person', 'geo', 'event', 'category'], method: 'light', community: false, resolution: false });
  });
  it.each([{ method: 'future', use_graphrag: true }, { use_graphrag: 'false' }, { entity_types: [2] }, { resolution: 0.5 }, 'legacy', null])('keeps incompatible configuration %j in advanced JSON', (graphrag) => {
    const legacy = { ...original, parser_config: { ...original.parser_config, graphrag } };
    const values = datasetToFormValues(legacy);
    expect(values.graphrag_compatible).toBe(false);
    expect(formValuesToInput({ ...values, description: 'Edited' }, { original: legacy })).toEqual({ description: 'Edited' });
    expect(formValuesToInput(values).parser_config?.graphrag).toEqual(graphrag);
  });
  it('preserves advanced edits to untouched fields and rejects conflicting edits', () => {
    const values = datasetToFormValues(original);
    values.parser_config_extra = JSON.stringify({ ...original.parser_config, graphrag: { ...original.parser_config.graphrag, entity_types: ['advanced-type'] } });
    expect(formValuesToInput(values, { original })).toEqual({ parser_config: { graphrag: { entity_types: ['advanced-type'] } } });
    expect(() => formValuesToInput({ ...values, graphrag_entity_types: ['structured-type'] }, { original })).toThrow('GraphRAG 表单与高级 JSON');
  });
  it('omitting advanced properties does not clear stored GraphRAG or extension fields', () => {
    const values = { ...datasetToFormValues(original), parser_config_extra: '', description: 'Updated' };
    expect(formValuesToInput(values, { original })).toEqual({ description: 'Updated' });
  });
});
