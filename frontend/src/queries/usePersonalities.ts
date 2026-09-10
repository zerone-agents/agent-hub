import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { message } from 'antd'
import {
  personalityApi,
  type Personality,
  type PersonalityCreatePayload,
  type PersonalityUpdatePayload,
} from '@/api/personalities'
import { parseApiError, unwrapResponse } from '@/api/client'

export function usePersonalities() {
  return useQuery<Personality[]>({
    queryKey: ['personalities'],
    queryFn: async () =>
      unwrapResponse<Personality[]>(await personalityApi.list()),
  })
}

export function usePersonality(name: string) {
  return useQuery<Personality>({
    queryKey: ['personalities', name],
    queryFn: async () =>
      unwrapResponse<Personality>(await personalityApi.get(name)),
    enabled: Boolean(name),
  })
}

export function useCreatePersonality() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (data: PersonalityCreatePayload) => personalityApi.create(data),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['personalities'] })
      message.success('人格已创建')
    },
    onError: (error) => message.error(parseApiError(error)),
  })
}

export function useUpdatePersonality() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({
      name,
      data,
    }: {
      name: string
      data: PersonalityUpdatePayload
    }) => personalityApi.update(name, data),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['personalities'] })
      message.success('人格已更新')
    },
    onError: (error) => message.error(parseApiError(error)),
  })
}

export function useDeletePersonality() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (name: string) => personalityApi.delete(name),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['personalities'] })
      message.success('人格已删除')
    },
    onError: (error) => message.error(parseApiError(error)),
  })
}
