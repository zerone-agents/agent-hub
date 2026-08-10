import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { ConfigProvider } from 'antd'
import { antdTheme } from '@/lib/antd-theme'
import SkillMdPreview from './SkillMdPreview'
import type { SkillMdEntry } from './parseSkillMd'

// Mock @lobehub/ui Markdown to avoid loading 3.6MB of dependencies in tests.
// SkillMdPreview tests verify frontmatter parsing and tab logic, not markdown rendering.
// Strip simple markdown syntax (headings, bold) so text assertions still work.
vi.mock('@lobehub/ui', () => ({
  Markdown: ({ children }: { children: React.ReactNode }) => {
    const text = String(children)
      .replace(/^#{1,6}\s+/gm, '')  // strip heading markers
      .replace(/\*\*(.+?)\*\*/g, '$1')  // strip bold
    return <div data-testid="mock-markdown">{text}</div>
  },
}))

const SINGLE_ENTRY: SkillMdEntry = {
  path: 'SKILL.md',
  content: `---
name: my-skill
description: A test skill
---
# Hello World`,
}

describe('SkillMdPreview', () => {
  it('renders placeholder when idle', () => {
    render(
      <ConfigProvider theme={antdTheme}>
        <SkillMdPreview loading={false} entries={[]} error="" placeholder="选择 zip 文件后预览" />
      </ConfigProvider>
    )
    expect(screen.getByText('选择 zip 文件后预览')).toBeInTheDocument()
  })

  it('renders error message', () => {
    render(
      <ConfigProvider theme={antdTheme}>
        <SkillMdPreview loading={false} entries={[]} error="无法读取 SKILL.md" placeholder="" />
      </ConfigProvider>
    )
    expect(screen.getByText('无法读取 SKILL.md')).toBeInTheDocument()
  })

  it('renders frontmatter as a table above markdown body for single entry', async () => {
    render(
      <ConfigProvider theme={antdTheme}>
        <SkillMdPreview loading={false} entries={[SINGLE_ENTRY]} error="" placeholder="" />
      </ConfigProvider>
    )

    // Frontmatter keys/values are rendered as table cells
    expect(screen.getByText('name')).toBeInTheDocument()
    expect(screen.getByText('my-skill')).toBeInTheDocument()
    expect(screen.getByText('description')).toBeInTheDocument()
    expect(screen.getByText('A test skill')).toBeInTheDocument()

    // Markdown body is rendered without the raw frontmatter delimiters
    // Use findByText (async) because Markdown is lazy-loaded via Suspense
    expect(await screen.findByText('Hello World')).toBeInTheDocument()
  })

  it('does not render tab bar when there is only one entry', () => {
    render(
      <ConfigProvider theme={antdTheme}>
        <SkillMdPreview loading={false} entries={[SINGLE_ENTRY]} error="" placeholder="" />
      </ConfigProvider>
    )
    // No Segmented control in the DOM
    expect(document.querySelector('.ant-segmented')).toBeNull()
  })

  it('renders tab bar with parent-dir labels when entries.length > 1', () => {
    const entries: SkillMdEntry[] = [
      { path: 'commit/SKILL.md', content: '# Commit' },
      { path: 'team/review/SKILL.md', content: '# Review' },
      { path: 'team/sub/deploy/SKILL.md', content: '# Deploy' },
    ]
    render(
      <ConfigProvider theme={antdTheme}>
        <SkillMdPreview loading={false} entries={entries} error="" placeholder="" />
      </ConfigProvider>
    )
    // Tab labels use immediate parent dir name (matches SDK skill-name semantics)
    expect(screen.getByText('commit')).toBeInTheDocument()
    expect(screen.getByText('review')).toBeInTheDocument()
    expect(screen.getByText('deploy')).toBeInTheDocument()

    // First entry is shown by default
    expect(screen.getByText('Commit')).toBeInTheDocument()
    expect(screen.queryByText('Review')).not.toBeInTheDocument()
    expect(screen.queryByText('Deploy')).not.toBeInTheDocument()
  })

  it('switches active entry when tab is clicked', () => {
    const entries: SkillMdEntry[] = [
      { path: 'commit/SKILL.md', content: '# Commit Body' },
      { path: 'review/SKILL.md', content: '# Review Body' },
    ]
    render(
      <ConfigProvider theme={antdTheme}>
        <SkillMdPreview loading={false} entries={entries} error="" placeholder="" />
      </ConfigProvider>
    )
    expect(screen.getByText('Commit Body')).toBeInTheDocument()

    // Click the "review" tab
    fireEvent.click(screen.getByText('review'))
    expect(screen.getByText('Review Body')).toBeInTheDocument()
    expect(screen.queryByText('Commit Body')).not.toBeInTheDocument()
  })

  it('shows SKILL.md as its own label for root-level entry in bundle', () => {
    const entries: SkillMdEntry[] = [
      { path: 'SKILL.md', content: '# Root' },
      { path: 'commit/SKILL.md', content: '# Commit' },
    ]
    render(
      <ConfigProvider theme={antdTheme}>
        <SkillMdPreview loading={false} entries={entries} error="" placeholder="" />
      </ConfigProvider>
    )
    // Root-level SKILL.md uses its own filename as label (no parent dir)
    expect(screen.getByText('SKILL.md')).toBeInTheDocument()
    expect(screen.getByText('commit')).toBeInTheDocument()
  })

  it('resets to first tab when entry set changes', () => {
    const { rerender } = render(
      <ConfigProvider theme={antdTheme}>
        <SkillMdPreview
          loading={false}
          entries={[
            { path: 'a/SKILL.md', content: '# A' },
            { path: 'b/SKILL.md', content: '# B' },
          ]}
          error=""
          placeholder=""
        />
      </ConfigProvider>
    )
    // Switch to second tab
    fireEvent.click(screen.getByText('b'))
    expect(screen.getByText('B')).toBeInTheDocument()

    // Replace with a different set of entries
    rerender(
      <ConfigProvider theme={antdTheme}>
        <SkillMdPreview
          loading={false}
          entries={[
            { path: 'x/SKILL.md', content: '# X' },
            { path: 'y/SKILL.md', content: '# Y' },
          ]}
          error=""
          placeholder=""
        />
      </ConfigProvider>
    )
    // Should reset to first tab of new set
    expect(screen.getByText('X')).toBeInTheDocument()
    expect(screen.queryByText('Y')).not.toBeInTheDocument()
  })
})
