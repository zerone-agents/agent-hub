import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { message } from 'antd'
import { sceneApi, type Scene, type SceneCreatePayload, type SceneUpdatePayload } from '@/api/scenes'
import { parseApiError, unwrapResponse } from '@/api/client'

export function useScenes() {
  return useQuery<Scene[]>({
    queryKey: ['scenes'],
    queryFn: async () =>
      unwrapResponse<Scene[]>(await sceneApi.adminList()) ?? []
  })
}

export function useCreateScene() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: SceneCreatePayload) => sceneApi.create(data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['scenes'] })
      message.success('场景已创建')
    },
    onError: (err) => message.error(parseApiError(err))
  })
}

export function useUpdateScene() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ name, data }: { name: string; data: SceneUpdatePayload }) =>
      sceneApi.update(name, data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['scenes'] })
      message.success('场景已更新')
    },
    onError: (err) => message.error(parseApiError(err))
  })
}

export function useDeleteScene() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (name: string) => sceneApi.delete(name),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['scenes'] })
      message.success('场景已删除')
    },
    onError: (err) => message.error(parseApiError(err))
  })
}

/**
 * Returns enabled scenes belonging to the given agent, sorted alphabetically by `name`.
 * Uses the public `GET /api/v1/scenes` endpoint and filters client-side because the
 * agent identifier in the URL is a `name` (string) but the API filter param is numeric.
 */
export function useAgentScenes(agentName: string) {
  return useQuery<Scene[]>({
    queryKey: ['agent-scenes', agentName],
    queryFn: async () => {
      const all = unwrapResponse<Scene[]>(await sceneApi.list()) ?? []
      return all
        .filter((s) => s.enabled && s.agent === agentName)
        .sort((a, b) => a.name.localeCompare(b.name))
    },
    enabled: !!agentName,
    staleTime: 60_000
  })
}
