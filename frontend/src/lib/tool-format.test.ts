import { describe, it, expect } from 'vitest'
import {
  categorize,
  getToolSummary,
  buildToolInputMarkdown,
  detectResultLang,
  escapeCodeFences
} from './tool-format'

describe('categorize', () => {
  it('classifies known tool names', () => {
    expect(categorize('Read')).toBe('read')
    expect(categorize('Write')).toBe('write')
    expect(categorize('WriteFile')).toBe('write')
    expect(categorize('Create')).toBe('write')
    expect(categorize('Edit')).toBe('edit')
    expect(categorize('MultiEdit')).toBe('edit')
    expect(categorize('Bash')).toBe('bash')
    expect(categorize('Glob')).toBe('glob')
    expect(categorize('Grep')).toBe('grep')
    expect(categorize('Task')).toBe('task')
    expect(categorize('MultiTask')).toBe('task')
  })

  it('classifies mcp tools by prefix', () => {
    expect(categorize('mcp__github__create_issue')).toBe('mcp')
    expect(categorize('mcp_github_create_issue')).toBe('mcp')
  })

  it('falls back to other for unknown names', () => {
    expect(categorize('Unknown')).toBe('other')
    expect(categorize('')).toBe('other')
  })
})

describe('getToolSummary', () => {
  it('uses basename for read', () => {
    expect(getToolSummary('Read', { file_path: '/a/b/c.ts' })).toBe('c.ts')
    expect(getToolSummary('Read', { file_path: 'package.json' })).toBe('package.json')
  })

  it('uses basename for write and edit', () => {
    expect(getToolSummary('Write', { file_path: '/a/b/c.ts' })).toBe('c.ts')
    expect(getToolSummary('Edit', { file_path: '/a/b/c.ts' })).toBe('c.ts')
  })

  it('uses command for bash, truncated to 40 chars', () => {
    expect(getToolSummary('Bash', { command: 'npm test' })).toBe('npm test')
    const longCmd = 'node script.js ' + 'x'.repeat(60)
    expect(getToolSummary('Bash', { command: longCmd })).toHaveLength(40)
  })

  it('uses pattern for glob', () => {
    expect(getToolSummary('Glob', { pattern: '**/*.ts' })).toBe('**/*.ts')
  })

  it('uses quoted pattern for grep', () => {
    expect(getToolSummary('Grep', { pattern: 'foo' })).toBe('"foo"')
  })

  it('returns empty string for mcp / task / other', () => {
    expect(getToolSummary('mcp__x__y', {})).toBe('')
    expect(getToolSummary('Task', {})).toBe('')
    expect(getToolSummary('Unknown', {})).toBe('')
  })

  it('returns empty string when input field is missing', () => {
    expect(getToolSummary('Read', {})).toBe('')
    expect(getToolSummary('Read', undefined)).toBe('')
    expect(getToolSummary('Bash', undefined)).toBe('')
  })
})

describe('buildToolInputMarkdown', () => {
  it('returns empty string for empty input', () => {
    expect(buildToolInputMarkdown('Read', {})).toBe('')
    expect(buildToolInputMarkdown('Read', undefined)).toBe('')
  })

  it('returns empty string after filtering out description', () => {
    const input = { description: 'some description' }
    expect(buildToolInputMarkdown('Read', input)).toBe('')
  })

  it('renders edit category as table only', () => {
    const md = buildToolInputMarkdown('Edit', {
      file_path: 'src/foo.ts',
      old_string: 'a',
      new_string: 'b',
      description: 'ignored'
    })
    expect(md).toContain('| file_path | src/foo.ts |')
    expect(md).toContain('| old_string | a |')
    expect(md).toContain('| new_string | b |')
    expect(md).not.toContain('description')
    expect(md).not.toContain('```')  // no code block
  })

  it('renders bash category as table with command', () => {
    const md = buildToolInputMarkdown('Bash', {
      command: 'npm test',
      workdir: '/tmp',
      timeout: 30000
    })
    expect(md).toContain('| command | npm test |')
    expect(md).toContain('| workdir | /tmp |')
    expect(md).toContain('| timeout | 30000 |')
  })

  it('renders write category as table + code block with inferred language', () => {
    const md = buildToolInputMarkdown('Write', {
      file_path: 'hello.py',
      content: "print('hi')\n"
    })
    expect(md).toContain('| file_path | hello.py |')
    expect(md).not.toContain('| content |')  // content goes to code block, not table
    expect(md).toContain('```py')
    expect(md).toContain("print('hi')")
    expect(md).toContain('```')
  })

  it('infers language from extension', () => {
    const cases = [
      ['file.ts', 'ts'],
      ['file.js', 'js'],
      ['file.tsx', 'tsx'],
      ['file.py', 'py'],
      ['file.go', 'go'],
      ['file.md', 'markdown'],
      ['file.json', 'json'],
      ['Makefile', 'text'],
      ['noext', 'text']
    ] as const
    for (const [name, lang] of cases) {
      const md = buildToolInputMarkdown('Write', { file_path: name, content: 'x' })
      expect(md).toContain('```' + lang)
    }
  })

  it('renders read/glob/grep as plain table', () => {
    const md = buildToolInputMarkdown('Grep', { pattern: 'foo', path: '/a' })
    expect(md).toContain('| pattern | foo |')
    expect(md).toContain('| path | /a |')
    expect(md).not.toContain('```')
  })

  it('switches to paragraph layout when any cell value is multi-line', () => {
    const md = buildToolInputMarkdown('Edit', {
      file_path: 'a.ts',
      old_string: 'line1\nline2'
    })
    // Multi-line value must trigger paragraph layout, not a table.
    expect(md).not.toContain('| Key | Value |')
    // Single-line field rendered as a bolded key:value paragraph.
    expect(md).toContain('**file_path:** a.ts')
    // Multi-line field has a bolded label followed by a fenced text block.
    expect(md).toContain('**old_string:**')
    expect(md).toContain('```text')
    // The fenced block preserves the original multi-line text verbatim.
    expect(md).toContain('line1\nline2')
  })

  it('paragraph layout: pipe characters in multi-line values are NOT escaped', () => {
    // Multi-line value forces paragraph layout; the embedded pipe must be
    // left untouched because pipes are only meaningful inside tables.
    const md = buildToolInputMarkdown('Edit', {
      file_path: 'a.ts',
      old_string: 'x | y\nz'
    })
    expect(md).toContain('x | y')
    expect(md).not.toContain('x \\| y')
    expect(md).not.toContain('| Key | Value |')
  })

  it('table layout: pipe characters in cell values are escaped', () => {
    const md = buildToolInputMarkdown('Bash', {
      command: 'grep foo | wc -l'
    })
    expect(md).toContain('| command | grep foo \\| wc -l |')
  })


  it('JSON-stringifies object values in cells', () => {
    const md = buildToolInputMarkdown('Custom', { nested: { a: 1 } })
    expect(md).toContain('| nested | {"a":1} |')
  })
})

describe('detectResultLang', () => {
  it('returns json for parseable JSON', () => {
    expect(detectResultLang('{"a":1}')).toBe('json')
    expect(detectResultLang('[1,2,3]')).toBe('json')
    expect(detectResultLang('  {"a":1}  ')).toBe('json')
  })

  it('returns text for non-JSON', () => {
    expect(detectResultLang('hello world')).toBe('text')
    expect(detectResultLang('Error: ENOENT')).toBe('text')
    expect(detectResultLang('{invalid json}')).toBe('text')
  })

  it('returns text for empty string', () => {
    expect(detectResultLang('')).toBe('text')
    expect(detectResultLang('   ')).toBe('text')
  })
})

describe('escapeCodeFences', () => {
  it('returns input unchanged when no fences', () => {
    expect(escapeCodeFences('hello world')).toBe('hello world')
  })

  it('escapes triple backticks', () => {
    expect(escapeCodeFences('before ``` inside ``` after')).toBe(
      'before \\`\\`\\` inside \\`\\`\\` after'
    )
  })

  it('handles multiple occurrences', () => {
    expect(escapeCodeFences('```a```b```')).toBe('\\`\\`\\`a\\`\\`\\`b\\`\\`\\`')
  })

  it('does not touch single backticks', () => {
    expect(escapeCodeFences('`code`')).toBe('`code`')
  })
})
