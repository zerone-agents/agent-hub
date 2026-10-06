import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import i18next from '@/i18n'
import { message } from 'antd'
import apiClient, { getAccessToken, parseApiError, unwrapResponse } from '@/api/client'
import { useAuthStore } from '@/stores/auth'
import type { UserInfoResponse } from '@/api/auth'
import type { ApiResponse } from '@/types/api'
import { ownedRequestConfig } from '@/api/requestOwnership'
import {
  knowledgeApi,
  type DatasetFormInput,
  type KnowledgeWriteOwner,
  type DatasetListParams,
  type DatasetListResult,
  type KnowledgeDataset,
  type DocumentListParams,
  type DocumentFilterParams,
  type DocumentIngestInput,
  type ChunkListParams,
  type ChunkFormInput,
  type RetrievalInput
} from '@/api/knowledge'

/** Query key factory covering list / detail / documents / chunks. */
export const knowledgeKeys = {
  all: ['knowledge'] as const,
  datasets: () => [...knowledgeKeys.all, 'datasets'] as const,
  datasetList: (params: DatasetListParams) =>
    [...knowledgeKeys.datasets(), 'list', params] as const,
  datasetListAll: () => [...knowledgeKeys.datasets(), 'list-all'] as const,
  datasetDetail: (id: string) => [...knowledgeKeys.datasets(), 'detail', id] as const,
  documents: (datasetId: string) => [...knowledgeKeys.all, 'documents', datasetId] as const,
  documentList: (datasetId: string, params: DocumentListParams) =>
    [...knowledgeKeys.documents(datasetId), params] as const,
  chunks: (datasetId: string, documentId: string) =>
    [...knowledgeKeys.all, 'chunks', datasetId, documentId] as const,
  chunkList: (datasetId: string, documentId: string, params: ChunkListParams) =>
    [...knowledgeKeys.chunks(datasetId, documentId), params] as const
}

// ---------------------------------------------------------------------------
// Datasets
// ---------------------------------------------------------------------------

export interface KnowledgeListOrigin { id: string; token: string | null; role?: string }
// Private weak metadata binds the displayed object to its verified request.
// Credentials are never placed in JSON, query keys or persisted query data.
const datasetListOrigins = new WeakMap<DatasetListResult, KnowledgeListOrigin>();
const datasetDetailOrigins = new WeakMap<KnowledgeDataset, KnowledgeListOrigin>();
async function verifyKnowledgeIdentity(signal: AbortSignal) {
  const id = useAuthStore.getState().user?.id;
  let token = getAccessToken();
  const current = () => Boolean(id && !signal.aborted && useAuthStore.getState().user?.id === id);
  const owner: KnowledgeWriteOwner = {
    signal,
    isCurrent: () => current() && getAccessToken() === token,
    assertCurrent: (refresh) => {
      if (refresh && current()) token = getAccessToken();
      if (!current() || getAccessToken() !== token) throw new Error(i18next.t('knowledge.list.deleteOwnerChanged'));
    },
  };
  owner.assertCurrent();
  const response = await apiClient.get<ApiResponse<UserInfoResponse>>('/auth/userinfo', ownedRequestConfig(owner));
  owner.assertCurrent();
  const identity = unwrapResponse<UserInfoResponse>(response);
  if ((identity.user_id ?? identity.id) !== id) throw new Error(i18next.t('knowledge.list.deleteOwnerChanged'));
  const roles = identity.roles ?? [];
  return { owner, getOrigin: (): KnowledgeListOrigin => ({ id: id ?? '', token, role: roles.includes('guest') ? 'guest' : roles[0] }) };
}
export async function readOwnedKnowledgeList(params: DatasetListParams, signal: AbortSignal) {
  const { owner, getOrigin } = await verifyKnowledgeIdentity(signal);
  const result = await knowledgeApi.datasets.list(params, owner);
  datasetListOrigins.set(result, getOrigin());
  return result;
}

export async function readOwnedKnowledgeDetail(id: string, signal: AbortSignal) {
  const { owner, getOrigin } = await verifyKnowledgeIdentity(signal);
  const result = await knowledgeApi.datasets.get(id, owner);
  datasetDetailOrigins.set(result, getOrigin());
  return result;
}

export function useKnowledgeList(params: DatasetListParams = {}, options: { owned?: boolean } = {}) {
  const accountId = useAuthStore((state) => state.user?.id);
  const query = useQuery({
    queryKey: options.owned ? [...knowledgeKeys.datasetList(params), 'owned', accountId] : knowledgeKeys.datasetList(params),
    queryFn: ({ signal }) => options.owned ? readOwnedKnowledgeList(params, signal) : knowledgeApi.datasets.list(params),
    enabled: !options.owned || Boolean(accountId),
    structuralSharing: options.owned ? false : undefined,
  });
  return { ...query, origin: query.data ? datasetListOrigins.get(query.data) : undefined };
}

const KNOWLEDGE_LIST_ALL_PAGE_SIZE = 1000
const KNOWLEDGE_LIST_ALL_MAX_PAGES = 20

// useKnowledgeListAll 分页取全目录（issue #122 review P2）：ghost 判定依赖
// 完整目录，单页缺失不能证明知识库已删除。逐页取满 total；页数上限作
// fail-safe——超限时 datasets.length < total，消费方据此视为「不完整」并
// 禁止注入 ghost。任一页失败即整体失败（liveness 未知，宁缺勿假）。
export function useKnowledgeListAll() {
  return useQuery({
    queryKey: knowledgeKeys.datasetListAll(),
    queryFn: async () => {
      const first = await knowledgeApi.datasets.list({
        page: 1,
        page_size: KNOWLEDGE_LIST_ALL_PAGE_SIZE
      })
      if (first.datasets.length >= first.total || first.datasets.length === 0) {
        return first
      }
      const datasets = [...first.datasets]
      const lastPage = Math.min(
        Math.ceil(first.total / KNOWLEDGE_LIST_ALL_PAGE_SIZE),
        KNOWLEDGE_LIST_ALL_MAX_PAGES
      )
      for (let page = 2; page <= lastPage; page++) {
        const next = await knowledgeApi.datasets.list({
          page,
          page_size: KNOWLEDGE_LIST_ALL_PAGE_SIZE
        })
        if (next.datasets.length === 0) break
        datasets.push(...next.datasets)
        if (datasets.length >= first.total) break
      }
      return { datasets, total: first.total }
    }
  })
}

export function useKnowledgeDetail(id: string, options: { owned?: boolean } = {}) {
  const accountId = useAuthStore((state) => state.user?.id);
  const query = useQuery({
    queryKey: options.owned ? [...knowledgeKeys.datasetDetail(id), 'owned', accountId] : knowledgeKeys.datasetDetail(id),
    queryFn: ({ signal }) => options.owned ? readOwnedKnowledgeDetail(id, signal) : knowledgeApi.datasets.get(id),
    enabled: !!id && (!options.owned || Boolean(accountId)),
    structuralSharing: options.owned ? false : undefined,
  });
  return { ...query, origin: query.data ? datasetDetailOrigins.get(query.data) : undefined };
}

export function useCreateKnowledge(options: { getOwner?: () => KnowledgeWriteOwner | undefined } = {}) {
  const { t } = useTranslation()
  const qc = useQueryClient()
  return useMutation({
    onMutate: () => options.getOwner?.(),
    mutationFn: (input: DatasetFormInput) => knowledgeApi.datasets.create(input, options.getOwner?.()),
    onSuccess: (_saved, _variables, owner) => {
      if (owner && !owner.isCurrent()) return;
      void qc.invalidateQueries({ queryKey: knowledgeKeys.datasets() })
      message.success(t('knowledge.toast.created'))
    },
    onError: (err, _variables, owner) => {
      if (owner && !owner.isCurrent()) return;
      void qc.invalidateQueries({ queryKey: knowledgeKeys.datasets() })
      message.error(parseApiError(err))
    }
  })
}

export function useUpdateKnowledge(options: { getOwner?: () => KnowledgeWriteOwner | undefined } = {}) {
  const { t } = useTranslation()
  const qc = useQueryClient()
  return useMutation({
    onMutate: () => options.getOwner?.(),
    mutationFn: ({ id, data }: { id: string; data: DatasetFormInput }) =>
      knowledgeApi.datasets.update(id, data, options.getOwner?.()),
    onSuccess: (saved, variables, owner) => {
      if (owner && !owner.isCurrent()) return;
      qc.setQueryData(knowledgeKeys.datasetDetail(variables.id), saved)
      void qc.invalidateQueries({ queryKey: knowledgeKeys.datasets() })
      void qc.invalidateQueries({ queryKey: knowledgeKeys.datasetDetail(variables.id) })
      message.success(t('knowledge.toast.updated'))
    },
    onError: (err, _variables, owner) => { if (!owner || owner.isCurrent()) message.error(parseApiError(err)) }
  })
}

export function useDeleteKnowledge() {
  const { t } = useTranslation()
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => knowledgeApi.datasets.remove([id]),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: knowledgeKeys.datasets() })
      message.success(t('knowledge.toast.deleted'))
    },
    onError: (err) => message.error(parseApiError(err))
  })
}

// Deleted IDs produce a business error in MR's filtered GET. Only a complete,
// unfiltered, consistent directory can independently prove absence.
export async function readKnowledgeDirectory(owner?: KnowledgeWriteOwner) {
  const pageSize = 100;
  const maxPages = 200;
  const ids = new Set<string>();
  let total: number | undefined;
  for (let page = 1; page <= maxPages; page++) {
    const result = await knowledgeApi.datasets.list({ page, page_size: pageSize }, owner);
    total ??= result.total;
    if (result.total !== total || total > maxPages * pageSize || result.datasets.length !== Math.min(pageSize, total - (page - 1) * pageSize)) throw new Error(i18next.t('knowledge.list.deleteReadback'));
    for (const dataset of result.datasets) {
      if (!dataset.id || ids.has(dataset.id)) throw new Error(i18next.t('knowledge.list.deleteReadback'));
      ids.add(dataset.id);
    }
    if (ids.size === total) return ids;
  }
  throw new Error(i18next.t('knowledge.list.deleteReadback'));
}

// Delete only explicitly selected IDs. Business errors stay failures even when
// another selected deletion succeeds; unreadable directories confirm nothing.
export async function deleteKnowledgeSelection(ids: string[], owner?: KnowledgeWriteOwner) {
  if (!ids.length || ids.some((id) => !id.trim())) throw new Error('Empty dataset selection');
  const candidates: string[] = [];
  const deletedIds: string[] = [];
  const failed: { id: string; error: unknown }[] = [];
  for (const id of new Set(ids)) {
    owner?.assertCurrent();
    try {
      await knowledgeApi.datasets.remove([id], owner);
      candidates.push(id);
    } catch (error) {
      owner?.assertCurrent();
      failed.push({ id, error });
    }
  }
  if (candidates.length) {
    try {
      const directory = await readKnowledgeDirectory(owner);
      for (const id of candidates) {
        if (directory.has(id)) failed.push({ id, error: new Error(i18next.t('knowledge.list.deleteReadback')) });
        else deletedIds.push(id);
      }
    } catch (error) {
      owner?.assertCurrent();
      failed.push(...candidates.map((id) => ({ id, error })));
    }
  }
  return { deletedIds, failed };
}

export function useDeleteKnowledgeSelection() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ ids, owner }: { ids: string[]; owner: KnowledgeWriteOwner }) => deleteKnowledgeSelection(ids, owner),
    onSuccess: async (result, { owner }) => {
      if (!owner.isCurrent()) return;
      await qc.invalidateQueries({ queryKey: knowledgeKeys.datasets() });
      if (owner.isCurrent() && !result.failed.length) message.success(t('knowledge.toast.deleted'));
    },
  });
}

// ---------------------------------------------------------------------------
// Documents
// ---------------------------------------------------------------------------

export function useDocuments(datasetId: string, params: DocumentListParams = {}) {
  return useQuery({
    queryKey: knowledgeKeys.documentList(datasetId, params),
    queryFn: () => knowledgeApi.documents.list(datasetId, params),
    enabled: !!datasetId
  })
}

export function useDocumentFilters(datasetId: string, params: DocumentFilterParams = {}) {
  return useQuery({
    queryKey: [...knowledgeKeys.documents(datasetId), 'filters', params],
    queryFn: () => knowledgeApi.documents.filters(datasetId, params),
    enabled: !!datasetId,
  })
}

export function useUploadDocuments(datasetId: string) {
  const { t } = useTranslation()
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (files: File[]) => knowledgeApi.documents.upload(datasetId, files),
    onSuccess: (docs) => {
      void qc.invalidateQueries({ queryKey: knowledgeKeys.documents(datasetId) })
      void qc.invalidateQueries({ queryKey: knowledgeKeys.datasets() })
      message.success(t('knowledge.toast.docsUploaded', { n: docs.length }))
    },
    onError: (err) => message.error(parseApiError(err))
  })
}

export function useUpdateDocument(datasetId: string) {
  const { t } = useTranslation()
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ documentId, patch }: { documentId: string; patch: Record<string, unknown> }) =>
      knowledgeApi.documents.update(datasetId, documentId, patch),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: knowledgeKeys.documents(datasetId) })
      message.success(t('knowledge.toast.docUpdated'))
    },
    onError: (err) => message.error(parseApiError(err))
  })
}

export function useDeleteDocuments(datasetId: string) {
  const { t } = useTranslation()
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (documentIds: string[]) => knowledgeApi.documents.remove(datasetId, documentIds),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: knowledgeKeys.documents(datasetId) })
      void qc.invalidateQueries({ queryKey: knowledgeKeys.datasets() })
      message.success(t('knowledge.toast.docDeleted'))
    },
    onError: (err) => message.error(parseApiError(err))
  })
}

export function useParseDocuments(datasetId: string) {
  const { t } = useTranslation()
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (documentIds: string[]) => knowledgeApi.documents.parse(datasetId, documentIds),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: knowledgeKeys.documents(datasetId) })
      message.success(t('knowledge.toast.parseQueued'))
    },
    onError: (err) => message.error(parseApiError(err))
  })
}

export function useIngestDocuments(datasetId: string) {
  const { t } = useTranslation()
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: DocumentIngestInput) => knowledgeApi.documents.ingest(input),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: knowledgeKeys.documents(datasetId) })
      void qc.invalidateQueries({ queryKey: knowledgeKeys.datasets() })
      void qc.invalidateQueries({ queryKey: [...knowledgeKeys.all, 'chunks', datasetId] })
      message.success(t('knowledge.manage.ingestAccepted'))
    },
  })
}

export function useStopParsingDocuments(datasetId: string) {
  const { t } = useTranslation()
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (documentIds: string[]) => knowledgeApi.documents.stopParse(datasetId, documentIds),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: knowledgeKeys.documents(datasetId) })
      message.success(t('knowledge.toast.parseStopped'))
    },
    onError: (err) => message.error(parseApiError(err))
  })
}

// ---------------------------------------------------------------------------
// Chunks
// ---------------------------------------------------------------------------

export function useChunks(datasetId: string, documentId: string, params: ChunkListParams = {}) {
  return useQuery({
    queryKey: knowledgeKeys.chunkList(datasetId, documentId, params),
    queryFn: () => knowledgeApi.chunks.list(datasetId, documentId, params),
    enabled: !!datasetId && !!documentId
  })
}

export function useCreateChunk(datasetId: string, documentId: string, opts: { notify?: boolean } = {}) {
  const { t } = useTranslation()
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: ChunkFormInput) => knowledgeApi.chunks.create(datasetId, documentId, input),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: knowledgeKeys.chunks(datasetId, documentId) })
      void qc.invalidateQueries({ queryKey: knowledgeKeys.documents(datasetId) })
      if (opts.notify !== false) message.success(t('knowledge.toast.chunkAdded'))
    },
    onError: (err) => { if (opts.notify !== false) message.error(parseApiError(err)) }
  })
}

export function useUpdateChunk(datasetId: string, documentId: string, opts: { notify?: boolean } = {}) {
  const { t } = useTranslation()
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ chunkId, input }: { chunkId: string; input: ChunkFormInput }) =>
      knowledgeApi.chunks.update(datasetId, documentId, chunkId, input),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: knowledgeKeys.chunks(datasetId, documentId) })
      if (opts.notify !== false) message.success(t('knowledge.toast.chunkSaved'))
    },
    onError: (err) => { if (opts.notify !== false) message.error(parseApiError(err)) }
  })
}

export function useDeleteChunks(datasetId: string, documentId: string) {
  const { t } = useTranslation()
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (chunkIds: string[]) => knowledgeApi.chunks.remove(datasetId, documentId, chunkIds),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: knowledgeKeys.chunks(datasetId, documentId) })
      void qc.invalidateQueries({ queryKey: knowledgeKeys.documents(datasetId) })
      message.success(t('knowledge.toast.chunkDeleted'))
    },
    onError: (err) => message.error(parseApiError(err))
  })
}

export function useSwitchChunks(datasetId: string, documentId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ chunkIds, available }: { chunkIds: string[]; available: boolean }) =>
      knowledgeApi.chunks.switch(datasetId, documentId, chunkIds, available),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: knowledgeKeys.chunks(datasetId, documentId) })
    },
    onError: (err) => message.error(parseApiError(err))
  })
}

// ---------------------------------------------------------------------------
// Retrieval test
// ---------------------------------------------------------------------------

export function useRetrievalTest() {
  return useMutation({
    mutationFn: (input: RetrievalInput) => knowledgeApi.retrieval.test(input),
    onError: (err) => message.error(parseApiError(err))
  })
}
