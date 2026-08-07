import { createStyles } from 'antd-style'
import { tokens as t } from '@/styles/tokens'

const useStyles = createStyles(({ css }) => ({
  card: css`
    border: 1px solid color-mix(in srgb, var(--foreground) 10%, transparent);
    border-radius: 6px;
    background: ${t.surface};
    overflow: hidden;
  `,
  header: css`
    display: flex; align-items: center; gap: 6px;
    padding: 6px 10px;
    background: color-mix(in srgb, var(--foreground) 3%, transparent);
    font-size: 12px; color: ${t.textSecondary};
  `,
  tag: css`
    display: inline-block; padding: 0 5px; border-radius: 2px;
    font-size: 10px; font-weight: 600;
    background: rgba(5, 150, 105, 0.12); color: #059669;
  `,
  body: css`
    padding: 8px 10px; font-family: ${t.fontMono}; font-size: 11px;
    white-space: pre-wrap; word-break: break-word;
    max-height: 400px; overflow-y: auto; color: ${t.text};
  `
}))

function safeStringify(v: unknown): string {
  if (typeof v === 'string') return v
  try {
    return JSON.stringify(v, null, 2)
  } catch {
    return String(v)
  }
}

export interface LegacyToolResultProps {
  content: unknown
}

/**
 * DEPRECATED: Fallback renderer for persisted tool_result segments that pre-date
 * the `toolUseId` field (cannot be paired with their tool_use). Renders the same
 * flat `<pre>` view as the old PartTool(isResult) path. Safe to remove once old
 * messages age out of the database.
 */
export default function LegacyToolResult({ content }: LegacyToolResultProps) {
  const { styles } = useStyles()
  const body = safeStringify(content ?? '')
  return (
    <div className={styles.card}>
      <div className={styles.header}>
        <span className={styles.tag}>结果</span>
        <span>tool_result (legacy)</span>
      </div>
      {body && <pre className={styles.body}>{body}</pre>}
    </div>
  )
}
