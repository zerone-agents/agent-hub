import { useEffect, useState } from 'react'
import { Markdown } from '@lobehub/ui'
import { Segmented, Spin, Tooltip } from 'antd'
import { FileText } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import { tokens as t } from '@/styles/tokens'
import { parseSkillFrontmatter } from './parseSkillFrontmatter'
import type { SkillMdEntry } from './parseSkillMd'
import './SkillMdPreview.css'

const useStyles = createStyles(({ css }) => ({
  wrap: css`
    flex: 1; min-height: 0;
    display: flex; flex-direction: column;
  `,
  idle: css`
    flex: 1; display: flex; flex-direction: column;
    align-items: center; justify-content: center;
    color: ${t.textTertiary}; font-size: ${t.textSm};
    gap: 12px;
  `,
  error: css`
    color: ${t.danger};
  `,
  // Tab bar sits above the markdown body. Only rendered when there are
  // multiple SKILL.md entries (bundle case); the single-skill case keeps
  // its current chrome-less layout for visual continuity.
  tabs: css`
    flex-shrink: 0;
    padding: 4px 0 8px;
    overflow-x: auto;
    & .ant-segmented {
      // ant-segmented's own background sits behind each item; keep it
      // readable on our card surface.
      background: ${t.inkLighter};
    }
  `,
  md: css`
    flex: 1; overflow-y: auto;
    padding: 4px 0;
  `
}))

export interface SkillMdPreviewProps {
  loading: boolean
  /** Entries to display. Empty array = "no content" (placeholder shows).
   * Single entry = current single-skill behaviour. Multiple entries =
   * bundle mode with a tab bar at the top. */
  entries: SkillMdEntry[]
  error: string
  placeholder: string
}

/**
 * Derive a short tab label from a SKILL.md's zip path. Uses the
 * immediate parent directory name (matches the SDK's skill-name
 * semantics: skill name = parent dir of SKILL.md unless overridden by
 * frontmatter). For a root-level "SKILL.md" the label is "SKILL.md"
 * itself so the single-skill case shows something meaningful.
 */
function tabLabel(path: string): string {
  const segments = path.split('/')
  if (segments.length === 1) return segments[0]
  return segments[segments.length - 2]
}

export default function SkillMdPreview({ loading, entries, error, placeholder }: SkillMdPreviewProps) {
  const { styles } = useStyles()

  // Active tab index. Reset to 0 whenever the entry set changes identity
  // (new zip picked, modal re-opened, edit-mode data arrives). Using
  // entries.length as a dep would miss same-length replacements; using
  // the paths string as a dep gives a stable key for the current set.
  const pathsKey = entries.map((e) => e.path).join('|')
  const [activeIdx, setActiveIdx] = useState(0)
  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- reset tab to first entry whenever the entry-set identity changes (new zip picked / modal reopened / remote data arrived)
    setActiveIdx(0)
  }, [pathsKey])

  // Clamp activeIdx in case the entry list shrank (e.g. user picked a
  // new zip with fewer skills). Keeps the rendered entry in bounds.
  const idx = Math.min(activeIdx, Math.max(0, entries.length - 1))
  const active = entries[idx]
  // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- entries[idx] may be undefined at runtime when idx is out of bounds
  const { frontmatter, body } = active?.content
    ? parseSkillFrontmatter(active.content)
    : { frontmatter: null, body: '' }

  // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- same reason as above
  const hasContent = entries.length > 0 && !!active?.content

  return (
    <div className={styles.wrap}>
      {loading && (
        <div className={styles.idle}>
          <Spin size="small" />
        </div>
      )}
      {!loading && error && (
        <div className={styles.idle}>
          <FileText size={32} weight="thin" />
          <span className={styles.error}>{error}</span>
        </div>
      )}
      {!loading && !error && !hasContent && (
        <div className={styles.idle}>
          <FileText size={32} weight="thin" />
          <span>{placeholder}</span>
        </div>
      )}
      {!loading && !error && hasContent && (
        <>
          {entries.length > 1 && (
            <div className={styles.tabs}>
              <Segmented
                size="small"
                value={String(idx)}
                onChange={(v) => { setActiveIdx(Number(v)); }}
                options={entries.map((e, i) => ({
                  label: (
                    <Tooltip title={e.path} placement="bottom">
                      <span>{tabLabel(e.path)}</span>
                    </Tooltip>
                  ),
                  value: String(i),
                }))}
              />
            </div>
          )}
          <div className={`${styles.md} skill-md-preview`}>
            {frontmatter && (
              <table className="skill-md-preview__frontmatter">
                <tbody>
                  {Object.entries(frontmatter).map(([key, value]) => (
                    <tr key={key}>
                      <td>{key}</td>
                      <td className="skill-md-preview__frontmatter-value">
                        <span className="skill-md-preview__frontmatter-text" title={value}>
                          {value}
                        </span>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
            <Markdown>{body}</Markdown>
          </div>
        </>
      )}
    </div>
  )
}
