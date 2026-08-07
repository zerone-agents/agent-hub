import JSZip from 'jszip'

/**
 * One SKILL.md discovered inside a zip.
 *
 * `path` is the zip-internal relative path (e.g. "commit/SKILL.md" or
 * "SKILL.md" at archive root). `content` is the raw markdown text.
 */
export interface SkillMdEntry {
  path: string
  content: string
}

/**
 * Dirs whose contents are never real skills. Mirrors the CLI's EXCLUDED
 * set and the backend's skillExcludedDirs so all three layers agree on
 * what counts as a publishable SKILL.md.
 */
const EXCLUDED_DIRS = new Set(['.git', 'node_modules', 'dist', 'build', '.zerone-uploads'])

/**
 * Extract every SKILL.md from a zip File in the browser.
 *
 * Matches the backend's FindAllSkillMd glob semantics: any depth, sorted
 * by path, excluding entries under .git / node_modules / dist / build /
 * .zerone-uploads. The first entry corresponds to what the backend's
 * FindSkillMd (preview endpoint) would return, so the single-skill case
 * is unchanged from the user's perspective.
 *
 * @throws if no SKILL.md is found anywhere in the zip.
 */
export async function parseSkillMd(file: File): Promise<SkillMdEntry[]> {
  const zip = await JSZip.loadAsync(file)

  const candidates = Object.values(zip.files).filter((f) => {
    if (f.dir) return false
    const segments = f.name.split('/')
    if (segments[segments.length - 1] !== 'SKILL.md') return false
    // Skip paths passing through excluded parent dirs.
    return !segments.slice(0, -1).some((s) => EXCLUDED_DIRS.has(s))
  })

  if (candidates.length === 0) {
    throw new Error('该 zip 包中未找到 SKILL.md')
  }

  // Sort by path for stable order — matches backend FindAllSkillMd.
  candidates.sort((a, b) => a.name.localeCompare(b.name))

  const contents = await Promise.all(candidates.map((c) => c.async('string')))
  return candidates.map((c, i) => ({ path: c.name, content: contents[i] }))
}
