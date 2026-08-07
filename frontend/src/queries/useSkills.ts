import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { message } from 'antd'
import { skillApi, type Skill, type SkillUpdatePayload } from '@/api/skills'
import { parseApiError, unwrapResponse } from '@/api/client'

export function useSkills() {
  return useQuery<Skill[]>({
    queryKey: ['skills'],
    queryFn: async () =>
      unwrapResponse<Skill[]>(await skillApi.adminList()) ?? []
  })
}

export function useCreateSkill() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (formData: FormData) => skillApi.create(formData),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['skills'] })
      message.success('技能已创建')
    },
    onError: (err) => message.error(parseApiError(err))
  })
}

export function useUpdateSkill() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ name, data }: { name: string; data: SkillUpdatePayload }) =>
      skillApi.update(name, data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['skills'] })
      message.success('技能已更新')
    },
    onError: (err) => message.error(parseApiError(err))
  })
}

export function useDeleteSkill() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (name: string) => skillApi.delete(name),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['skills'] })
      message.success('技能已删除')
    },
    onError: (err) => message.error(parseApiError(err))
  })
}
