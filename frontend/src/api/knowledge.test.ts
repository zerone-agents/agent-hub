import { describe, it, expect, vi } from "vitest";
import apiClient from "./client";
import {
  normalizeDataset,
  normalizeDocument,
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
    it("defaults keyword arrays to empty", () => {
      expect(toChunkBody({ content: "x" })).toEqual({
        content: "x",
        important_keywords: [],
        questions: [],
      });
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
