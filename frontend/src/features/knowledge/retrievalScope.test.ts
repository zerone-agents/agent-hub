import { describe, expect, it } from "vitest";
import type { RetrievalResult } from "@/api/knowledge";
import { retrievalPageTotal, retrievalScopeMismatch } from "./retrievalScope";

const chunk = {
  id: "c",
  document_id: "d",
  dataset_id: "a",
  content: "evidence",
  document_name: "doc",
  similarity: 1,
  vector_similarity: 1,
  term_similarity: 0,
};
const result: RetrievalResult = {
  total: 1,
  chunks: [chunk],
  doc_aggs: [],
  labels: {},
};
const request = { question: "q", dataset_ids: ["a", "b"], doc_ids: ["d"] };
describe("retrieval source scope", () => {
  it("accepts both selected libraries and their exact source identities", () => {
    expect(retrievalScopeMismatch(result, request)).toBe(false);
    expect(
      retrievalScopeMismatch(
        { ...result, chunks: [{ ...chunk, dataset_id: "b" }] },
        request,
      ),
    ).toBe(false);
  });
  it.each([
    { dataset_id: "foreign" },
    { dataset_id: undefined },
    { document_id: "foreign" },
    { document_id: "" },
    { id: "" },
  ])("hides invalid source %j", (patch) => {
    expect(
      retrievalScopeMismatch(
        { ...result, chunks: [{ ...chunk, ...patch }] },
        request,
      ),
    ).toBe(true);
  });
  it("treats an explicitly empty document selection as empty", () => {
    expect(retrievalScopeMismatch(result, { ...request, doc_ids: [] })).toBe(
      true,
    );
  });
  it("rejects a null document selection instead of broadening it", () => {
    expect(retrievalScopeMismatch(result, { ...request, doc_ids: null })).toBe(
      true,
    );
  });
  it("rejects aggregates outside selected documents", () => {
    expect(
      retrievalScopeMismatch(
        {
          ...result,
          doc_aggs: [{ doc_id: "other", doc_name: "other", count: 1 }],
        },
        request,
      ),
    ).toBe(true);
  });
  it("caps pagination at whole pages within the submitted candidate pool", () => {
    expect(retrievalPageTotal(5000, 1024, 30)).toBe(1020);
    expect(retrievalPageTotal(5000, 31, 30)).toBe(30);
    expect(retrievalPageTotal(7, 30, 30)).toBe(7);
  });
});
