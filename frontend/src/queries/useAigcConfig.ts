import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { message } from 'antd'
import { aigcApi, type AigcConfig } from '@/api/aigc'
import { parseApiError } from '@/api/client'

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
  const qc = useQueryClient()
  return useMutation<AigcConfig, Error, { uscc: string; companyName: string }>({
    mutationFn: ({ uscc, companyName }) => aigcApi.save(uscc, companyName).then((r) => r.data.data),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['aigc-config'] })
      message.success('AIGC 标识配置已保存')
    },
    onError: (err) => message.error(parseApiError(err))
  })
}

export function useRotateAigcKey() {
  const qc = useQueryClient()
  return useMutation<AigcConfig>({
    mutationFn: () => aigcApi.rotateKey().then((r) => r.data.data),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['aigc-config'] })
      message.success('签名密钥已重新生成')
    },
    onError: (err) => message.error(parseApiError(err))
  })
}

export function useClearAigcConfig() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => aigcApi.clear().then(() => undefined),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['aigc-config'] })
      message.success('AIGC 标识配置已清除')
    },
    onError: (err) => message.error(parseApiError(err))
  })
}
