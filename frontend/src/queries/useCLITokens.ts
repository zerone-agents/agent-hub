import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { message } from 'antd'
import { cliTokensApi, type CLIToken, type IssueTokenResponse } from '@/api/cli-tokens'
import { parseApiError } from '@/api/client'
import { useTranslation } from 'react-i18next'

export function useCLITokens() {
  return useQuery<CLIToken[]>({
    queryKey: ['cli-tokens'],
    queryFn: async () => {
      const res = await cliTokensApi.list()
      if (!res.data.success) throw new Error('Failed to list tokens')
      return res.data.data.items
    }
  })
}

export function useIssueCLIToken() {
  const { t } = useTranslation()
  const qc = useQueryClient()
  return useMutation<IssueTokenResponse, Error, { name: string; ttlDays: number }>({
    mutationFn: ({ name, ttlDays }) =>
      cliTokensApi.issue(name, ttlDays).then((r) => r.data.data),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['cli-tokens'] })
      message.success(t('cliTokens.toast.created'))
    },
    onError: (err) => message.error(parseApiError(err))
  })
}

export function useRevokeCLIToken() {
  const { t } = useTranslation()
  const qc = useQueryClient()
  return useMutation<unknown, Error, number>({
    mutationFn: (id) => cliTokensApi.revoke(id).then(() => undefined),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['cli-tokens'] })
      message.success(t('cliTokens.toast.revoked'))
    },
    onError: (err) => message.error(parseApiError(err))
  })
}
