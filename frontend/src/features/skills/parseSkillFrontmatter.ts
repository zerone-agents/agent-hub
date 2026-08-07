export interface ParsedSkillMd {
  frontmatter: Record<string, string> | null
  body: string
}

const BLOCK_SCALAR_INDICATORS = new Set(['>', '|', '>-', '|-', '>+', '|+'])

/**
 * Parse a YAML block scalar (folded `>` or literal `|`) starting at `startIndex`.
 *
 * The first non-empty line determines the base indentation; the scalar continues
 * until a line with less indentation or the end of the frontmatter block.
 */
function parseBlockScalar(
  lines: string[],
  startIndex: number,
  indicator: string
): { value: string; nextIndex: number } {
  let i = startIndex + 1

  // Skip leading blank lines inside the block scalar.
  while (i < lines.length && lines[i].trim() === '') {
    i++
  }

  if (i >= lines.length) {
    return { value: '', nextIndex: i }
  }

  const baseIndent = lines[i].length - lines[i].trimStart().length
  const rawLines: string[] = []

  while (i < lines.length) {
    const line = lines[i]
    if (line.trim() === '') {
      rawLines.push('')
      i++
      continue
    }

    const indent = line.length - line.trimStart().length
    if (indent < baseIndent) {
      break
    }

    rawLines.push(line.slice(baseIndent))
    i++
  }

  if (indicator.startsWith('>')) {
    // Folded scalar: lines within a paragraph are joined with spaces;
    // blank lines separate paragraphs.
    const groups: string[] = []
    let current: string[] = []
    for (const raw of rawLines) {
      if (raw === '') {
        if (current.length > 0) {
          groups.push(current.join(' '))
          current = []
        }
      } else {
        current.push(raw)
      }
    }
    if (current.length > 0) {
      groups.push(current.join(' '))
    }
    return { value: groups.join('\n').trim(), nextIndex: i }
  }

  // Literal scalar: preserve newlines as-is.
  return { value: rawLines.join('\n').trim(), nextIndex: i }
}

/**
 * Parse the YAML frontmatter block at the top of a SKILL.md file.
 *
 * SKILL.md frontmatter is delimited by `---` lines and contains simple
 * key: value pairs. The parser is intentionally lightweight: it handles the
 * subset of YAML used in skill frontmatter (string scalars, optional quotes,
 * colons in values, and multi-line block scalars) without pulling in a full
 * YAML library.
 */
export function parseSkillFrontmatter(content: string): ParsedSkillMd {
  const trimmed = content.trimStart()
  if (!trimmed.startsWith('---')) {
    return { frontmatter: null, body: content }
  }

  const endIndex = trimmed.indexOf('---', 3)
  if (endIndex === -1) {
    return { frontmatter: null, body: content }
  }

  const frontmatterText = trimmed.slice(3, endIndex).trim()
  const body = trimmed.slice(endIndex + 3).trimStart()

  const frontmatter: Record<string, string> = {}
  const lines = frontmatterText.split('\n')
  let i = 0
  while (i < lines.length) {
    const line = lines[i]
    const trimmedLine = line.trim()
    if (!trimmedLine || trimmedLine.startsWith('#')) {
      i++
      continue
    }

    const colonIndex = trimmedLine.indexOf(':')
    if (colonIndex === -1) {
      i++
      continue
    }

    const key = trimmedLine.slice(0, colonIndex).trim()
    const valueStart = trimmedLine.slice(colonIndex + 1).trim()
    if (!key) {
      i++
      continue
    }

    if (BLOCK_SCALAR_INDICATORS.has(valueStart)) {
      const { value, nextIndex } = parseBlockScalar(lines, i, valueStart)
      frontmatter[key] = value
      i = nextIndex
    } else {
      frontmatter[key] = valueStart
      i++
    }
  }

  if (Object.keys(frontmatter).length === 0) {
    return { frontmatter: null, body: content }
  }

  return { frontmatter, body }
}
