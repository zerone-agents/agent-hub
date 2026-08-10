import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { message } from 'antd'
import { parseApiError } from '@/api/client'
import {
  knowledgeApi,
  type DatasetFormInput,
  type DatasetListParams,
  type DocumentListParams,
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

export function useKnowledgeList(params: DatasetListParams = {}) {
  return useQuery({
    queryKey: knowledgeKeys.datasetList(params),
    queryFn: () => knowledgeApi.datasets.list(params)
  })
}

export function useKnowledgeDetail(id: string) {
  return useQuery({
    queryKey: knowledgeKeys.datasetDetail(id),
    queryFn: () => knowledgeApi.datasets.get(id),
    enabled: !!id
  })
}

export function useCreateKnowledge() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: DatasetFormInput) => knowledgeApi.datasets.create(input),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: knowledgeKeys.datasets() })
      message.success('知识库已创建')
    },
    onError: (err) => message.error(parseApiError(err))
  })
}

export function useUpdateKnowledge() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: DatasetFormInput }) =>
      knowledgeApi.datasets.update(id, data),
    onSuccess: (_res, variables) => {
      void qc.invalidateQueries({ queryKey: knowledgeKeys.datasets() })
      void qc.invalidateQueries({ queryKey: knowledgeKeys.datasetDetail(variables.id) })
      message.success('知识库已更新')
    },
    onError: (err) => message.error(parseApiError(err))
  })
}

export function useDeleteKnowledge() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => knowledgeApi.datasets.remove([id]),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: knowledgeKeys.datasets() })
      message.success('知识库已删除')
    },
    onError: (err) => message.error(parseApiError(err))
  })
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

export function useUploadDocuments(datasetId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (files: File[]) => knowledgeApi.documents.upload(datasetId, files),
    onSuccess: (docs) => {
      void qc.invalidateQueries({ queryKey: knowledgeKeys.documents(datasetId) })
      void qc.invalidateQueries({ queryKey: knowledgeKeys.datasets() })
      message.success(`已上传 ${docs.length} 个文档`)
    },
    onError: (err) => message.error(parseApiError(err))
  })
}

export function useUpdateDocument(datasetId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ documentId, patch }: { documentId: string; patch: Record<string, unknown> }) =>
      knowledgeApi.documents.update(datasetId, documentId, patch),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: knowledgeKeys.documents(datasetId) })
      message.success('文档已更新')
    },
    onError: (err) => message.error(parseApiError(err))
  })
}

export function useDeleteDocuments(datasetId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (documentIds: string[]) => knowledgeApi.documents.remove(datasetId, documentIds),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: knowledgeKeys.documents(datasetId) })
      void qc.invalidateQueries({ queryKey: knowledgeKeys.datasets() })
      message.success('文档已删除')
    },
    onError: (err) => message.error(parseApiError(err))
  })
}

export function useParseDocuments(datasetId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (documentIds: string[]) => knowledgeApi.documents.parse(datasetId, documentIds),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: knowledgeKeys.documents(datasetId) })
      message.success('已加入解析队列')
    },
    onError: (err) => message.error(parseApiError(err))
  })
}

export function useStopParsingDocuments(datasetId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (documentIds: string[]) => knowledgeApi.documents.stopParse(datasetId, documentIds),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: knowledgeKeys.documents(datasetId) })
      message.success('已停止解析')
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

export function useCreateChunk(datasetId: string, documentId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: ChunkFormInput) => knowledgeApi.chunks.create(datasetId, documentId, input),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: knowledgeKeys.chunks(datasetId, documentId) })
      void qc.invalidateQueries({ queryKey: knowledgeKeys.documents(datasetId) })
      message.success('分块已新增')
    },
    onError: (err) => message.error(parseApiError(err))
  })
}

export function useUpdateChunk(datasetId: string, documentId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ chunkId, input }: { chunkId: string; input: ChunkFormInput }) =>
      knowledgeApi.chunks.update(datasetId, documentId, chunkId, input),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: knowledgeKeys.chunks(datasetId, documentId) })
      message.success('分块已保存')
    },
    onError: (err) => message.error(parseApiError(err))
  })
}

export function useDeleteChunks(datasetId: string, documentId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (chunkIds: string[]) => knowledgeApi.chunks.remove(datasetId, documentId, chunkIds),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: knowledgeKeys.chunks(datasetId, documentId) })
      void qc.invalidateQueries({ queryKey: knowledgeKeys.documents(datasetId) })
      message.success('分块已删除')
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
