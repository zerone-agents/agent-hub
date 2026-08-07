export type ToolCategory =
  | 'read' | 'write' | 'edit' | 'bash'
  | 'glob' | 'grep' | 'mcp' | 'task' | 'other'

const WRITE_NAMES = new Set(['Write', 'WriteFile', 'Create'])
const EDIT_NAMES = new Set(['Edit', 'MultiEdit'])
const TASK_NAMES = new Set(['Task', 'MultiTask'])

const EXT_LANG: Record<string, string> = {
  ts: 'ts',
  tsx: 'tsx',
  js: 'js',
  jsx: 'jsx',
  py: 'py',
  go: 'go',
  rs: 'rust',
  java: 'java',
  rb: 'ruby',
  php: 'php',
  sh: 'bash',
  bash: 'bash',
  zsh: 'bash',
  json: 'json',
  yml: 'yaml',
  yaml: 'yaml',
  toml: 'toml',
  xml: 'xml',
  html: 'html',
  css: 'css',
  scss: 'scss',
  sql: 'sql',
  md: 'markdown',
  markdown: 'markdown'
}

export function categorize(toolName: string, _input?: Record<string, unknown>): ToolCategory {
  if (!toolName) return 'other'
  if (toolName === 'Read') return 'read'
  if (WRITE_NAMES.has(toolName)) return 'write'
  if (EDIT_NAMES.has(toolName)) return 'edit'
  if (toolName === 'Bash') return 'bash'
  if (toolName === 'Glob') return 'glob'
  if (toolName === 'Grep') return 'grep'
  if (TASK_NAMES.has(toolName)) return 'task'
  if (toolName.startsWith('mcp__') || toolName.startsWith('mcp_')) return 'mcp'
  return 'other'
}

function basename(p: string): string {
  if (!p) return ''
  const norm = p.replace(/\\/g, '/')
  const idx = norm.lastIndexOf('/')
  return idx >= 0 ? norm.slice(idx + 1) : norm
}

function truncate(s: string, max: number): string {
  return s.length > max ? s.slice(0, max) : s
}

export function getToolSummary(toolName: string, input?: Record<string, unknown>): string {
  if (!input || typeof input !== 'object') return ''
  const cat = categorize(toolName, input)
  switch (cat) {
    case 'read':
    case 'write':
    case 'edit': {
      const fp = input.file_path
      return typeof fp === 'string' ? basename(fp) : ''
    }
    case 'bash': {
      const cmd = input.command
      return typeof cmd === 'string' ? truncate(cmd, 40) : ''
    }
    case 'glob': {
      const p = input.pattern
      return typeof p === 'string' ? p : ''
    }
    case 'grep': {
      const p = input.pattern
      return typeof p === 'string' ? `"${p}"` : ''
    }
    default:
      return ''
  }
}

function inferLang(filePath: string): string {
  if (!filePath) return 'text'
  const base = basename(filePath)
  if (!base.includes('.')) return 'text'
  const ext = base.split('.').pop()!.toLowerCase()
  return EXT_LANG[ext] ?? 'text'
}

function isMultiLineValue(v: unknown): boolean {
  return typeof v === 'string' && v.includes('\n')
}

function formatCellValue(value: unknown): string {
  if (value === null || value === undefined) return ''
  if (typeof value === 'string') {
    // Multi-line values cannot live inside a GFM table cell (newlines end the
    // row). Callers must route multi-line inputs through renderParagraphLayout.
    // Single-line string: escape pipe so it stays within the cell per GFM.
    return value.replace(/\|/g, '\\|')
  }
  if (typeof value === 'number' || typeof value === 'boolean') return String(value)
  try {
    return JSON.stringify(value)
  } catch {
    return String(value)
  }
}

function renderTable(entries: [string, unknown][]): string {
  if (entries.length === 0) return ''
  const lines = ['| Key | Value |', '| --- | ----- |']
  for (const [k, v] of entries) {
    lines.push(`| ${k} | ${formatCellValue(v)} |`)
  }
  return lines.join('\n')
}

function renderParagraphLayout(entries: [string, unknown][]): string {
  if (entries.length === 0) return ''
  const blocks: string[] = []
  for (const [k, v] of entries) {
    let body: string
    if (v === null || v === undefined) {
      body = ''
    } else if (typeof v === 'string') {
      body = v
    } else if (typeof v === 'number' || typeof v === 'boolean') {
      body = String(v)
    } else {
      try {
        body = JSON.stringify(v)
      } catch {
        body = String(v)
      }
    }
    if (typeof v === 'string' && v.includes('\n')) {
      blocks.push(`**${k}:**\n\`\`\`text\n${body}\n\`\`\``)
    } else {
      blocks.push(`**${k}:** ${body}`)
    }
  }
  return blocks.join('\n\n')
}

export function buildToolInputMarkdown(
  toolName: string,
  input?: Record<string, unknown>
): string {
  if (!input || typeof input !== 'object') return ''

  const fields: Record<string, unknown> = { ...input }
  delete fields.description
  if (Object.keys(fields).length === 0) return ''

  const cat = categorize(toolName, input)

  if (cat === 'write' && typeof fields.content === 'string') {
    const filePath = typeof fields.file_path === 'string' ? fields.file_path : ''
    const lang = inferLang(filePath)
    const metaEntries = Object.entries(fields).filter(([k]) => k !== 'content')
    // If any metadata field is multi-line, the whole tool input goes paragraph.
    const meta = renderLayout(metaEntries)
    const codeBlock = `\`\`\`${lang}\n${fields.content}\n\`\`\``
    return meta ? `${meta}\n\n${codeBlock}` : codeBlock
  }

  return renderLayout(Object.entries(fields))
}

function renderLayout(entries: [string, unknown][]): string {
  if (entries.length === 0) return ''
  if (entries.some(([, v]) => isMultiLineValue(v))) {
    return renderParagraphLayout(entries)
  }
  return renderTable(entries)
}

export function detectResultLang(s: string): 'json' | 'text' {
  if (!s.trim()) return 'text'
  try {
    JSON.parse(s)
    return 'json'
  } catch {
    return 'text'
  }
}

/**
 * Neutralize triple backticks in arbitrary text so they cannot open/close a
 * fenced code block at the paragraph or inline level.
 *
 * Limitation: this only works at the paragraph/inline level. CommonMark does
 * NOT process backslash escapes inside fenced code blocks, so applying this to
 * content that will itself be placed inside a ``` fence will NOT prevent the
 * inner backticks from terminating the outer fence. Callers must ensure the
 * escaped text is not nested inside another fenced code block (for multi-line
 * cell values, see renderParagraphLayout, which emits its own fence around the
 * raw value rather than relying on this function).
 */
export function escapeCodeFences(s: string): string {
  return s.replace(/```/g, '\\`\\`\\`')
}
