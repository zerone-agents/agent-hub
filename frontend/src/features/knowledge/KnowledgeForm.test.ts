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
