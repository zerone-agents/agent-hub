import type { RetrievalInput, RetrievalResult } from "@/api/knowledge";

export function retrievalScopeMismatch(
  result: RetrievalResult,
  request: RetrievalInput,
): boolean {
  const documents = request.doc_ids;
  if (documents === null) return true;
  return (
    result.chunks.some(
      (chunk) =>
        !chunk.id ||
        !chunk.document_id ||
        !chunk.dataset_id ||
        !request.dataset_ids.includes(chunk.dataset_id) ||
        (documents !== undefined && !documents.includes(chunk.document_id)),
    ) ||
    result.doc_aggs.some(
      (doc) =>
        !doc.doc_id ||
        (doc.dataset_id !== undefined &&
          !request.dataset_ids.includes(doc.dataset_id)) ||
        (documents !== undefined && !documents.includes(doc.doc_id)),
    )
  );
}

// Retrieval totals describe ranked candidates. A later page must not exceed the
// submitted candidate pool; selecting a page never changes that frozen request.
export function retrievalPageTotal(
  total: number,
  topK: number,
  size: number,
): number {
  return Math.max(
    0,
    Math.min(total, Math.max(size, Math.floor(topK / size) * size)),
  );
}
