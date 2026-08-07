import { useQuery } from '@tanstack/react-query'
import { skillApi } from '@/api/skills'
import { parseApiError } from '@/api/client'

export interface SkillMdEntry {
  path: string
  content: string
}

/**
 * Fetch all SKILL.md entries for a skill's stored zip.
 *
 * The backend returns `{entries: [{path, content}, ...]}` — one entry per
 * SKILL.md anywhere in the zip (matches SDK's glob semantics). For a
 * single-skill zip the array has one element; for a bundle zip it has N.
 * The SkillForm preview renders a tab switcher when N > 1.
 *
 * Returns an empty array (not undefined) when the skill exists but has no
 * SKILL.md — that case shouldn't happen because the upload validator
 * rejects such zips, but the type is defensive.
 */
export function useSkillMd(name: string | null) {
  return useQuery<SkillMdEntry[]>({
    queryKey: ['skill-md', name],
    queryFn: async () => {
      if (!name) return []
      try {
        const res = await skillApi.getSkillMd(name)
        if (!res.data.success) throw new Error(res.data.message)
        const entries = (res.data.data as { entries?: SkillMdEntry[] }).entries
        return entries ?? []
      } catch (err) {
        throw new Error(parseApiError(err))
      }
    },
    enabled: !!name,
    staleTime: 0
  })
}
