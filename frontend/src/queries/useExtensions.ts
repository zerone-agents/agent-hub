import { useMutation, useQuery } from '@tanstack/react-query'
import { extensionApi, type ExtensionValidationResult, type H0AcceptanceInfo } from '@/api/extensions'
import { unwrapResponse } from '@/api/client'

export function useH0AcceptanceInfo() {
  return useQuery<H0AcceptanceInfo>({
    queryKey: ['extensions', 'h0'],
    queryFn: async () =>
      unwrapResponse<H0AcceptanceInfo>(await extensionApi.getH0AcceptanceInfo()),
  })
}

export function useValidateExtension() {
  return useMutation<ExtensionValidationResult, Error, string>({
    mutationFn: async (manifest) =>
      unwrapResponse<ExtensionValidationResult>(await extensionApi.validate(manifest)),
  })
}
