import { useQuery } from '@tanstack/react-query'
import { multiragApi, type MultiRAGModel } from '@/api/multirag'
import { unwrapResponse } from '@/api/client'

export const multiragKeys = {
  all: ['multirag'] as const,
  models: (type: string) => [...multiragKeys.all, 'models', type] as const,
}

export function useMultiragModels(type: 'embedding' | 'ocr') {
  return useQuery<MultiRAGModel[]>({
    queryKey: multiragKeys.models(type),
    queryFn: async () => {
      const res = await multiragApi.getModels(type)
      return unwrapResponse<MultiRAGModel[] | null>(res) ?? []
    },
  })
}
