import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { message } from 'antd'
import { aigcApi, type AigcConfig } from '@/api/aigc'
import { parseApiError } from '@/api/client'
import { useTranslation } from 'react-i18next'

export function useAigcConfig() {
  return useQuery<AigcConfig>({
    queryKey: ['aigc-config'],
    queryFn: async () => {
      const res = await aigcApi.get()
      return res.data.data
    }
  })
}

export function useSaveAigcConfig() {
  const { t } = useTranslation()
  const qc = useQueryClient()
  return useMutation<AigcConfig, Error, { uscc: string; companyName: string }>({
    mutationFn: ({ uscc, companyName }) => aigcApi.save(uscc, companyName).then((r) => r.data.data),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['aigc-config'] })
      message.success(t('aigcConfig.toast.saved'))
    },
    onError: (err) => message.error(parseApiError(err))
  })
}

export function useRotateAigcKey() {
  const { t } = useTranslation()
  const qc = useQueryClient()
  return useMutation<AigcConfig>({
    mutationFn: () => aigcApi.rotateKey().then((r) => r.data.data),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['aigc-config'] })
      message.success(t('aigcConfig.toast.keyRegenerated'))
    },
    onError: (err) => message.error(parseApiError(err))
  })
}

export function useClearAigcConfig() {
  const { t } = useTranslation()
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => aigcApi.clear().then(() => undefined),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['aigc-config'] })
      message.success(t('aigcConfig.toast.cleared'))
    },
    onError: (err) => message.error(parseApiError(err))
  })
}
