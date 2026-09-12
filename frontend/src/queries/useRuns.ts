import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { message } from 'antd'
import { parseApiError, unwrapResponse } from '@/api/client'
import { runApi, type CapabilityPackage, type CreateRunInput, type PromptSnapshot, type Run, type RunActivity, type RunDetail, type RunEventItem, type RunStateChange, type RunStatus, type ToolResultRecord } from '@/api/runs'

export function useRuns() {
  return useQuery<Run[]>({
    queryKey: ['runs'],
    queryFn: async () => unwrapResponse<Run[]>(await runApi.list()),
  })
}

export function useEnabledCapabilityPackages() {
  return useQuery<CapabilityPackage[]>({
    queryKey: ['capability-packages', 'enabled'],
    queryFn: async () => unwrapResponse<CapabilityPackage[]>(await runApi.listCapabilityPackages()),
  })
}

export function useRun(id?: string) {
  return useQuery<RunDetail>({
    queryKey: ['runs', id],
    queryFn: async () => unwrapResponse<RunDetail>(await runApi.get(id as string)),
    enabled: id !== undefined,
  })
}

export function useRunStateChanges(id?: string) {
  return useQuery<RunStateChange[]>({
    queryKey: ['runs', id, 'state-changes'],
    queryFn: async () => unwrapResponse<RunStateChange[]>(await runApi.listStateChanges(id as string)),
    enabled: id !== undefined,
  })
}

export function useRunActivities(id?: string) {
  return useQuery<RunActivity[]>({
    queryKey: ['runs', id, 'activities'],
    queryFn: async () => unwrapResponse<RunActivity[]>(await runApi.listActivities(id as string)),
    enabled: id !== undefined,
  })
}

export function useRunEvents(id?: string) {
  return useQuery<RunEventItem[]>({ queryKey: ['runs', id, 'events'], queryFn: async () => unwrapResponse<RunEventItem[]>(await runApi.listEvents(id as string)), enabled: id !== undefined })
}

export function useRunToolResults(id?: string) {
  return useQuery<ToolResultRecord[]>({ queryKey: ['runs', id, 'tool-results'], queryFn: async () => unwrapResponse<ToolResultRecord[]>(await runApi.listToolResults(id as string)), enabled: id !== undefined })
}

function useRefreshRuns() {
  const qc = useQueryClient()
  return (id?: string) => {
    void qc.invalidateQueries({ queryKey: ['runs'] })
    if (id) void qc.invalidateQueries({ queryKey: ['runs', id] })
  }
}

export function useCreateRun() {
  const refresh = useRefreshRuns()
  return useMutation<Run, Error, CreateRunInput>({
    mutationFn: async (input) => unwrapResponse<Run>(await runApi.create(input)),
    onSuccess: () => { refresh(); message.success('运行已创建') },
    onError: (error) => message.error(parseApiError(error)),
  })
}

export function useTransitionRun() {
  const refresh = useRefreshRuns()
  return useMutation<Run, Error, { id: string; status: RunStatus }>({
    mutationFn: async ({ id, status }) => unwrapResponse<Run>(await runApi.transition(id, status)),
    onSuccess: (_run, variables) => { refresh(variables.id); message.success('运行状态已更新') },
    onError: (error) => message.error(parseApiError(error)),
  })
}

export function useAddRunAgent() {
  const refresh = useRefreshRuns()
  return useMutation<void, Error, { id: string; agentId: number; role: string }>({
    mutationFn: async ({ id, agentId, role }) => { await runApi.addAgent(id, agentId, role) },
    onSuccess: (_data, variables) => { refresh(variables.id); message.success('Agent 已加入运行') },
    onError: (error) => message.error(parseApiError(error)),
  })
}

export function useComposeRunPrompt() {
  return useMutation<PromptSnapshot, Error, { id: string; agentId: number }>({
    mutationFn: async ({ id, agentId }) => {
	  // Prefer the snapshot actually used by the latest chat execution. A Run
	  // without an execution yet falls back to a clearly labelled preview.
	  try {
	    return unwrapResponse<PromptSnapshot>(await runApi.latestPrompt(id, agentId))
	  } catch {
	    return unwrapResponse<PromptSnapshot>(await runApi.composePrompt(id, agentId))
	  }
	},
    onError: (error) => message.error(parseApiError(error)),
  })
}
