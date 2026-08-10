import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { message } from 'antd'
import { providerApi, type Provider, type ProbeConfig, type AttrRules, type CatalogModel } from '@/api/providers'
import { parseApiError } from '@/api/client'

export function useProviders(type?: 'llm' | 'ocr' | 'embedding' | 'vlm') {
  return useQuery<Provider[]>({
    queryKey: ['providers', type ?? 'all'],
    queryFn: async () => {
      const res = await providerApi.list(type)
      if (!res.data.success) throw new Error(res.data.message)
      return res.data.data ?? []
    },
  })
}

// useProviderAttrRules loads the attribute rules used to dynamically
// render the provider form. Fetched once (full map) and cached;
// the form filters by the selected protocol locally.
export function useProviderAttrRules() {
  return useQuery<AttrRules>({
    queryKey: ['provider-attr-rules'],
    queryFn: async () => {
      const res = await providerApi.attrRules()
      if (!res.data.success) throw new Error(res.data.message)
      return res.data.data ?? {}
    },
  })
}

export function useCreateProvider() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: Partial<Provider>) => providerApi.create(data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['providers'] })
      message.success('Provider 已创建')
    },
    onError: (err) => message.error(parseApiError(err)),
  })
}

export function useUpdateProvider() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: number; data: Partial<Provider> }) =>
      providerApi.update(id, data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['providers'] })
      message.success('Provider 已更新')
    },
    onError: (err) => message.error(parseApiError(err)),
  })
}

export function useDeleteProvider() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => providerApi.delete(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['providers'] })
      message.success('Provider 已删除')
    },
    onError: (err) => message.error(parseApiError(err)),
  })
}

export function useProbeProvider() {
  return useMutation({
    mutationFn: ({ id, ...payload }: { id: number; apiKey?: string; baseUrl?: string; models?: CatalogModel[] }) =>
      providerApi.probe(id, payload),
    onError: (err) => message.error(parseApiError(err)),
  })
}

export function useProbeConfig() {
  return useMutation({
    mutationFn: (config: ProbeConfig) => providerApi.probeConfig(config),
    onError: (err) => message.error(parseApiError(err)),
  })
}

export function useSyncProviderMultiRAG() {
  return useMutation({
    mutationFn: ({ id, verifyOnly = false, modelIds }: { id: number; verifyOnly?: boolean; modelIds?: string[] }) =>
      providerApi.syncMultiRAG(id, { verifyOnly, modelIds }).then((res) => res.data),
    onError: (err) => message.error(parseApiError(err)),
  })
}
