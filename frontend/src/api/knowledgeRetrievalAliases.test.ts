import { expect, it } from 'vitest';
import { normalizeRetrievalChunk, normalizeRetrievalResult } from './knowledge';

it.each([
  { dataset_id: 'selected', kb_id: 'foreign' },
  { document_id: 'selected-doc', doc_id: 'foreign-doc' },
  { id: 'selected-chunk', chunk_id: 'foreign-chunk' },
])('rejects conflicting source aliases before one can hide the other: %j', (aliases) => {
  expect(() => normalizeRetrievalChunk(aliases)).toThrow();
});

it('preserves consistent aliases and dataset attribution on document aggregates', () => {
  const result = normalizeRetrievalResult({
    chunks: [{ id: 'c', chunk_id: 'c', dataset_id: 'k', kb_id: 'k', document_id: 'd', doc_id: 'd' }],
    doc_aggs: [{ kb_id: 'k', doc_id: 'd', doc_name: 'source', count: 1 }],
  });
  expect(result.chunks[0]).toMatchObject({ id: 'c', dataset_id: 'k', document_id: 'd' });
  expect(result.doc_aggs[0]).toEqual({ dataset_id: 'k', doc_id: 'd', doc_name: 'source', count: 1 });
  expect(normalizeRetrievalResult({ doc_aggs: [{ doc_id: 'd' }] }).doc_aggs[0]).not.toHaveProperty('dataset_id');
});

it('rejects conflicting document aggregate attribution', () => {
  expect(() => normalizeRetrievalResult({ doc_aggs: [{ dataset_id: 'selected', kb_id: 'foreign' }] })).toThrow();
});
