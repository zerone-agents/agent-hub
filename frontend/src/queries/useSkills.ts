import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { message } from 'antd'
import { skillApi, type Skill, type SkillUpdatePayload } from '@/api/skills'
import { parseApiError, unwrapResponse } from '@/api/client'
import { useTranslation } from 'react-i18next'

export function useSkills() {
  return useQuery<Skill[]>({
    queryKey: ['skills'],
    queryFn: async () =>
      unwrapResponse<Skill[]>(await skillApi.adminList())
  })
}

export function useCreateSkill() {
  const { t } = useTranslation()
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (formData: FormData) => skillApi.create(formData),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['skills'] })
      message.success(t('skills.toast.created'))
    },
    onError: (err) => message.error(parseApiError(err))
  })
}

export function useUpdateSkill() {
  const { t } = useTranslation()
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ name, data }: { name: string; data: SkillUpdatePayload }) =>
      skillApi.update(name, data),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['skills'] })
      message.success(t('skills.toast.updated'))
    },
    onError: (err) => message.error(parseApiError(err))
  })
}

export function useDeleteSkill() {
  const { t } = useTranslation()
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (name: string) => skillApi.delete(name),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['skills'] })
      message.success(t('skills.toast.deleted'))
    },
    onError: (err) => message.error(parseApiError(err))
  })
}
