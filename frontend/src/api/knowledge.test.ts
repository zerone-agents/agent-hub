import { describe, it, expect, vi, afterEach } from "vitest";
import apiClient from "./client";
import {
  normalizeDataset,
  normalizeDocument,
  normalizeDocumentRun,
  normalizeChunk,
  normalizeRetrievalResult,
  toDatasetBody,
  toChunkBody,
  buildQuery,
  knowledgeApi,
} from "./knowledge";

describe("knowledge adapter — field anti-corruption layer", () => {
  describe("normalizeDataset", () => {
    it("reads gateway-stable field names", () => {
      const ds = normalizeDataset({
        id: "kb1",
        name: "知识库",
        display_name: "知识库",
        collection_name: "kb_physical_one",
        description: "desc",
        permission: "team",
        doc_num: 3,
        chunk_num: 42,
        parser_id: "qa",
        embd_id: "bge-m3",
        parser_config: { chunk_token_num: 128 },
      });
      expect(ds).toMatchObject({
        id: "kb1",
        name: "知识库",
        display_name: "知识库",
        collection_name: "kb_physical_one",
        doc_num: 3,
        chunk_num: 42,
        parser_id: "qa",
        embd_id: "bge-m3",
      });
      expect(ds.parser_config).toEqual({ chunk_token_num: 128 });
    });

    it("keeps future display_name separate from the physical collection name", () => {
      const ds = normalizeDataset({
        id: "kb1",
        name: "kb_physical_one",
        display_name: "校园网服务指南",
        collection_name: "kb_physical_one",
      });
      expect(ds.name).toBe("校园网服务指南");
      expect(ds.display_name).toBe("校园网服务指南");
      expect(ds.collection_name).toBe("kb_physical_one");
    });

    it("falls back to multirag-native names when stable names are absent", () => {
      const ds = normalizeDataset({
        dataset_id: "kb2",
        name: "raw",
        document_count: 5,
        chunk_count: 100,
        chunk_method: "naive",
        embedding_model: "text-embedding-3",
      });
      expect(ds.id).toBe("kb2");
      expect(ds.doc_num).toBe(5);
      expect(ds.chunk_num).toBe(100);
      expect(ds.parser_id).toBe("naive");
      expect(ds.embd_id).toBe("text-embedding-3");
    });

    it("applies safe defaults for missing fields", () => {
      const ds = normalizeDataset({ id: "kb3", name: "empty" });
      expect(ds.doc_num).toBe(0);
      expect(ds.chunk_num).toBe(0);
      expect(ds.parser_id).toBe("naive");
      expect(ds.embd_id).toBe("");
      expect(ds.permission).toBe("me");
      expect(ds.parser_config).toEqual({});
    });
  });

  describe("normalizeDocument", () => {
    it("maps stable fields and derives enabled from status", () => {
      const doc = normalizeDocument({
        id: "d1",
        name: "a.pdf",
        chunk_num: 12,
        token_num: 900,
        parser_id: "naive",
        run: "1",
        progress: 0.5,
        progress_msg: "parsing",
        status: "1",
        size: 2048,
        meta_fields: [{ key: "author", value: "Ada" }],
        parser_config: { pages: [[1, 2]] },
        nickname: "operator",
        process_begin_at: 123,
        process_duration: 9,
        source_type: "upload",
        thumbnail: "thumb",
      });
      expect(doc).toMatchObject({
        id: "d1",
        chunk_num: 12,
        token_num: 900,
        run: "1",
        progress: 0.5,
        enabled: true,
        meta_fields: [{ key: "author", value: "Ada" }],
        parser_config: { pages: [[1, 2]] },
        nickname: "operator",
        process_begin_at: 123,
        process_duration: 9,
        source_type: "upload",
        thumbnail: "thumb",
      });
    });

    it("falls back to multirag-native names and status 0 → disabled", () => {
      const doc = normalizeDocument({
        doc_id: "d2",
        name: "b.txt",
        chunk_count: 4,
        token_count: 40,
        chunk_method: "qa",
        status: "0",
      });
      expect(doc.id).toBe("d2");
      expect(doc.chunk_num).toBe(4);
      expect(doc.token_num).toBe(40);
      expect(doc.parser_id).toBe("qa");
      expect(doc.enabled).toBe(false);
    });
  });

  describe("normalizeChunk", () => {
    it("maps content / keywords and derives available from available_int", () => {
      const chunk = normalizeChunk({
        id: "c1",
        content: "hello",
        document_id: "d1",
        important_keywords: ["k1"],
        questions: ["q1"],
        available_int: 1,
        img_id: "img-1",
        position_int: [1, 2, 3],
        doc_type_kwd: "image",
        tag_kwd: ["tag1"],
        tag_feas: { source: "manual" },
      });
      expect(chunk).toMatchObject({
        id: "c1",
        content: "hello",
        document_id: "d1",
        important_keywords: ["k1"],
        questions: ["q1"],
        available: true,
        image_id: "img-1",
        positions: [1, 2, 3],
        doc_type: "image",
        tag_kwd: ["tag1"],
        tag_feas: { source: "manual" },
      });
    });

    it("falls back to multirag-native chunk fields", () => {
      const chunk = normalizeChunk({
        chunk_id: "c2",
        content_with_weight: "raw text",
        doc_id: "d9",
        important_kwd: ["a", "b"],
        question_kwd: ["why"],
        available_int: 0,
      });
      expect(chunk.id).toBe("c2");
      expect(chunk.content).toBe("raw text");
      expect(chunk.document_id).toBe("d9");
      expect(chunk.important_keywords).toEqual(["a", "b"]);
      expect(chunk.questions).toEqual(["why"]);
      expect(chunk.available).toBe(false);
    });
  });

  describe("normalizeRetrievalResult", () => {
    it("normalizes chunks, doc name (docnm_kwd) and similarity score", () => {
      const result = normalizeRetrievalResult({
        total: 1,
        chunks: [
          {
            id: "c1",
            content: "matched text",
            document_id: "d1",
            docnm_kwd: "guide.pdf",
            similarity: 0.87,
            vector_similarity: 0.9,
            term_similarity: 0.8,
            highlight: "<em>matched</em> text",
          },
        ],
        doc_aggs: [{ doc_id: "d1", doc_name: "guide.pdf", count: 1 }],
        labels: { topic: "x" },
      });
      expect(result.total).toBe(1);
      expect(result.chunks).toHaveLength(1);
      expect(result.chunks[0]).toMatchObject({
        id: "c1",
        content: "matched text",
        document_name: "guide.pdf",
        similarity: 0.87,
        highlight: "<em>matched</em> text",
      });
      expect(result.doc_aggs[0]).toEqual({
        doc_id: "d1",
        doc_name: "guide.pdf",
        count: 1,
      });
    });

    it("returns empty collections for a blank payload", () => {
      const result = normalizeRetrievalResult({});
      expect(result.total).toBe(0);
      expect(result.chunks).toEqual([]);
      expect(result.doc_aggs).toEqual([]);
    });
  });

  describe("toDatasetBody", () => {
    it("emits only provided fields and keeps stable parser_id / embd_id names", () => {
      const body = toDatasetBody({
        name: "kb",
        description: "d",
        permission: "me",
        parser_id: "naive",
        embd_id: "bge",
        parser_config: { chunk_token_num: 256 },
      });
      expect(body).toEqual({
        name: "kb",
        description: "d",
        permission: "me",
        parser_id: "naive",
        embd_id: "bge",
        parser_config: { chunk_token_num: 256 },
      });
    });

    it("can carry future display_name / collection_name fields without changing page code", () => {
      expect(
        toDatasetBody({
          name: "校园网服务指南",
          display_name: "校园网服务指南",
          collection_name: "kb_physical_one",
        }),
      ).toEqual({
        name: "校园网服务指南",
        display_name: "校园网服务指南",
        collection_name: "kb_physical_one",
      });
    });

    it("omits undefined fields (partial update)", () => {
      const body = toDatasetBody({ name: "only-name" });
      expect(body).toEqual({ name: "only-name" });
      expect("description" in body).toBe(false);
    });
  });

  describe("toChunkBody", () => {
    it("omits fields absent from a partial chunk update", () => {
      expect(toChunkBody({ image_base64: "image", image_update_mode: "replace" })).toEqual({
        image_base64: "image",
        image_update_mode: "replace",
      });
      expect(toChunkBody({ content: "x", tag_kwd: [] })).toEqual({ content: "x", tag_kwd: [] });
    });

    it("passes image and tag fields through", () => {
      expect(
        toChunkBody({
          content: "x",
          important_keywords: ["k"],
          questions: ["q"],
          image_base64: "data:image/png;base64,abc",
          tag_kwd: ["tag"],
          tag_feas: { source: "manual" },
        }),
      ).toEqual({
        content: "x",
        important_keywords: ["k"],
        questions: ["q"],
        image_base64: "abc",
        tag_kwd: ["tag"],
        tag_feas: { source: "manual" },
      });
    });
  });

  describe("buildQuery", () => {
    it("appends repeated array params and skips blank values", () => {
      expect(
        buildQuery({
          page: 2,
          suffix: ["pdf", "docx"],
          run: ["1", "4"],
          keywords: "",
          metadata_condition: "author",
        }),
      ).toBe(
        "?page=2&suffix=pdf&suffix=docx&run=1&run=4&metadata_condition=author",
      );
    });
  });

  describe("resource URLs", () => {
    it("builds admin gateway URLs for downloads and images", () => {
      expect(knowledgeApi.documents.downloadUrl("kb 1", "doc/1")).toBe(
        "/api/v1/admin/knowledge/datasets/kb%201/documents/doc%2F1/download",
      );
      expect(knowledgeApi.images.url("kb 1", "img/1")).toBe(
        "/api/v1/admin/knowledge/datasets/kb%201/images/img%2F1",
      );
    });
  });

  describe("document upload", () => {
    it("uses the large-file upload timeout", async () => {
      const post = vi.spyOn(apiClient, "post").mockResolvedValue({
        data: { success: true, data: [] },
      });
      const file = new File(["hello"], "a.txt", { type: "text/plain" });

      await knowledgeApi.documents.upload("kb 1", [file]);

      expect(post).toHaveBeenCalledWith(
        "/api/v1/admin/knowledge/datasets/kb%201/documents",
        expect.any(FormData),
        expect.objectContaining({ timeout: 60 * 60 * 1000 }),
      );
      post.mockRestore();
    });
  });
});

describe("document status contract", () => {
  it.each([["UNSTART", "0"], ["RUNNING", "1"], ["CANCEL", "2"], ["DONE", "3"], ["FAIL", "4"], ["1", "1"], ["FUTURE", "FUTURE"]])("normalizes %s to %s", (raw, want) => {
    expect(normalizeDocumentRun(raw)).toBe(want);
    expect(normalizeDocument({ id: "doc1", run: raw }).run).toBe(want);
  });
});

describe("metadata map contract", () => {
  it("preserves keys and list values as editable rows", () => {
    const doc = normalizeDocument({ id: "d", meta_fields: { author: "Alice", years: [2025, 2026] } });
    expect(doc.meta_fields).toEqual([{ key: "author", value: "Alice" }, { key: "years", value: [2025, 2026] }]);
  });
});


describe('single document readback', () => {
  it('uses the document ID filter and rejects an unrelated row', async () => {
    const get = vi.spyOn(apiClient, 'get').mockResolvedValueOnce({ data: { success: true, data: { total: 1, documents: [{ id: 'other' }] } } })
    await expect(knowledgeApi.documents.get('kb/1', 'doc/1')).rejects.toThrow('保存后回读未确认')
    expect(get).toHaveBeenCalledWith('/api/v1/admin/knowledge/datasets/kb%2F1/documents?page=1&page_size=1&id=doc%2F1')
    get.mockRestore()
  })
})

it('only confirms ingest acceptance for the strict true response', async () => {
  const input = { doc_ids: ['d1'], run: 1 as const, delete: false, apply_kb: false }
  const post = vi.spyOn(apiClient, 'post').mockResolvedValueOnce({ data: { success: true, data: {} } }).mockResolvedValueOnce({ data: { success: true, data: true } })
  await expect(knowledgeApi.documents.ingest(input)).rejects.toThrow('解析请求未被确认')
  await expect(knowledgeApi.documents.ingest(input)).resolves.toBe(true)
  expect(post).toHaveBeenCalledWith('/api/v1/admin/knowledge/documents/ingest', input)
  post.mockRestore()
})

it('preserves search text and document and reference metadata from the new contract', () => {
  const result = normalizeRetrievalResult({ total: 1, reference_metadata: { include: true }, chunks: [{ id: 'c1', text: 'native search text', document_metadata: { version: 2 }, reference_metadata: { fields: ['version'] } }] })
  expect(result.chunks[0]).toMatchObject({ content: 'native search text', document_metadata: { version: 2 }, reference_metadata: { fields: ['version'] } })
  expect(result.reference_metadata).toEqual({ include: true })
})

it('sends precise document filters and keeps an explicit false flag', async () => {
  const get = vi.spyOn(apiClient, 'get').mockResolvedValueOnce({ data: { success: true, data: { total: 0, documents: [] } } });
  await knowledgeApi.documents.list('kb1', { ids: ['d1', 'd2'], types: ['doc', 'visual'], metadata: '{"version":["v2"]}', return_empty_metadata: false });
  const url = new URL(get.mock.calls[0][0], 'http://fixture');
  expect(url.searchParams.getAll('ids')).toEqual(['d1', 'd2']);
  expect(url.searchParams.getAll('types')).toEqual(['doc', 'visual']);
  expect(url.searchParams.get('metadata')).toBe('{"version":["v2"]}');
  expect(url.searchParams.get('return_empty_metadata')).toBe('false');
  get.mockRestore();
});


describe("dataset write readback and scoped candidates", () => {
  afterEach(() => { vi.restoreAllMocks(); });
  it("confirms only requested dataset fields with an independent GET", async () => {
    vi.spyOn(apiClient, "put").mockResolvedValue({ data: { success: true, data: null } });
    const get = vi.spyOn(apiClient, "get").mockResolvedValue({ data: { success: true, data: { id: "kb/1", name: "Updated", parser_config: { custom: true, auto_keywords: 2 } } } });
    await expect(knowledgeApi.datasets.update("kb/1", { name: "Updated", parser_config: { auto_keywords: 2 } })).resolves.toMatchObject({ name: "Updated" });
    expect(get).toHaveBeenCalledWith("/api/v1/admin/knowledge/datasets/kb%2F1");
  });
  it.each([{ id: "other", name: "Updated" }, { id: "kb1", name: "Old" }])("rejects a stale or unrelated dataset readback", async (data) => {
    vi.spyOn(apiClient, "put").mockResolvedValue({ data: { success: true, data: null } });
    vi.spyOn(apiClient, "get").mockResolvedValue({ data: { success: true, data } });
    await expect(knowledgeApi.datasets.update("kb1", { name: "Updated" })).rejects.toThrow("配置读回未确认");
  });
  it("verifies canonical parent-child flags and resolved model references", async () => {
    const data = { id: "kb1", embd_id: "local-embed@Anthropic", parser_config: { layout_recognize: "MinerU", parent_child: { use_parent_child: false, children_delimiter: "|" }, raptor: { use_raptor: false } } };
    vi.spyOn(apiClient, "put").mockResolvedValue({ data: { success: true, data } });
    vi.spyOn(apiClient, "get").mockResolvedValue({ data: { success: true, data } });
    await expect(knowledgeApi.datasets.update("kb1", { embd_id: "local-embed", parser_config: { layout_recognize: "local-ocr", enable_children: false, children_delimiter: "|", raptor: { enabled: false } } })).resolves.toMatchObject({ id: "kb1" });
  });
  it("accepts MultiRAG's empty parent-child object after disabling the mode", async () => {
    const saved = { id: "kb1", parser_config: { parent_child: {}, chunk_token_num: 512 } };
    vi.spyOn(apiClient, "put").mockResolvedValue({ data: { success: true, data: saved } });
    vi.spyOn(apiClient, "get").mockResolvedValue({ data: { success: true, data: saved } });
    await expect(knowledgeApi.datasets.update("kb1", { parser_config: { enable_children: false, chunk_token_num: 512 } })).resolves.toMatchObject({ id: "kb1" });
    saved.parser_config.chunk_token_num = 256;
    await expect(knowledgeApi.datasets.update("kb1", { parser_config: { enable_children: false, chunk_token_num: 512 } })).rejects.toThrow("配置读回未确认");
  });
  it("only reads the current configuration when the form has no changes", async () => {
    const put = vi.spyOn(apiClient, "put");
    vi.spyOn(apiClient, "get").mockResolvedValue({ data: { success: true, data: { id: "kb1" } } });
    await expect(knowledgeApi.datasets.update("kb1", {})).resolves.toMatchObject({ id: "kb1" });
    expect(put).not.toHaveBeenCalled();
  });
  it("does not read back or claim success for a business rejection", async () => {
    vi.spyOn(apiClient, "put").mockResolvedValue({ data: { success: false, error: "Rejected" } });
    const get = vi.spyOn(apiClient, "get");
    await expect(knowledgeApi.datasets.update("kb1", { name: "Updated" })).rejects.toThrow("Rejected");
    expect(get).not.toHaveBeenCalled();
  });
  it("fetches dataset-wide filter counts with repeated query values", async () => {
    const result = { total: 2, filter: { suffix: { pdf: 2 }, run_status: { "3": 2 }, metadata: { year: { "2026": 2 } } } };
    const get = vi.spyOn(apiClient, "get").mockResolvedValue({ data: { success: true, data: result } });
    await expect(knowledgeApi.documents.filters("kb/1", { types: ["doc", "visual"], run: ["3", "4"] })).resolves.toEqual(result);
    const url = new URL(get.mock.calls[0][0], "https://fixture.invalid");
    expect(url.pathname).toBe("/api/v1/admin/knowledge/datasets/kb%2F1/documents/filters");
    expect(url.searchParams.getAll("types")).toEqual(["doc", "visual"]);
    expect(url.searchParams.getAll("run")).toEqual(["3", "4"]);
  });
  it("preserves explicit image update mode and omits it for older consumers", () => {
    expect(toChunkBody({ content: "text", image_base64: "image", image_update_mode: "replace" })).toMatchObject({ image_update_mode: "replace", image_base64: "image" });
    expect(toChunkBody({ content: "text" })).not.toHaveProperty("image_update_mode");
  });
  it("rejects a dataset deletion business error despite HTTP success", async () => {
    vi.spyOn(apiClient, "delete").mockResolvedValue({ data: { success: false, error: "Bound dataset" } });
    await expect(knowledgeApi.datasets.remove(["kb1"])).rejects.toThrow("Bound dataset");
  });
});
