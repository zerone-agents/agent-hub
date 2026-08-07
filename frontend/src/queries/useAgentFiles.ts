import { useQuery } from '@tanstack/react-query'
import { agentFilesApi } from '@/api/agent-files'

/**
 * useDirEntries — lazy directory listing with per-path caching.
 *
 * - queryKey includes agentName + dirPath so different agents and different
 *   subdirectories don't share cache. Switching agents automatically drops
 *   stale entries via gcTime.
 * - staleTime is 5 minutes: directory contents rarely change during a chat
 *   session, and frequent refetches would just load the runtime. The user
 *   can still force-refresh by remounting the panel.
 * - retry: 1 absorbs transient runtime hiccups; on permanent failure the
 *   consuming CwdFilePanel silently hides (decorator pattern, see
 *   AgentDetailBar.tsx for the precedent).
 */
export function useDirEntries(agentName: string, dirPath: string, enabled: boolean) {
  return useQuery({
    queryKey: ['agent-files', agentName, 'list', dirPath],
    queryFn: () => agentFilesApi.list(agentName, { path: dirPath }).then((r) => r.data),
    enabled,
    staleTime: 5 * 60 * 1000,
    retry: 1,
  })
}
