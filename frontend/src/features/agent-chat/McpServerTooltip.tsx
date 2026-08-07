import { Tag, Tooltip, theme } from 'antd'
import { createStyles } from 'antd-style'
import type { McpServerSummary } from '@/api/agents'
import { tokens as t } from '@/styles/tokens'

const useStyles = createStyles(({ css }) => ({
  overlay: css`
    width: 320px;
    max-width: 320px;
    font-size: 12px;
    background: var(--mcp-tooltip-bg);
    border-radius: 8px;
    padding: 10px 12px;
  `,
  header: css`
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding-bottom: 6px;
    margin-bottom: 6px;
    border-bottom: 1px solid var(--mcp-tooltip-border);
    color: var(--mcp-tooltip-text);
    font-weight: 500;
  `,
  transport: css`
    background: var(--mcp-tooltip-pill-bg);
    color: var(--mcp-tooltip-text);
    padding: 1px 6px;
    border-radius: 3px;
    font-size: 11px;
    font-family: ${t.fontMono};
  `,
  row: css`
    display: flex;
    gap: 8px;
    padding: 2px 0;
    align-items: flex-start;
  `,
  rowKey: css`
    flex: 0 0 70px;
    color: var(--mcp-tooltip-text-muted);
    font-family: ${t.fontMono};
  `,
  rowValue: css`
    flex: 1;
    color: var(--mcp-tooltip-text);
    font-family: ${t.fontMono};
    word-break: break-all;
  `,
  redacted: css`
    color: var(--mcp-tooltip-text-subtle);
  `,
  // Tag styling for the trigger Tag (overrides antd default)
  triggerTag: css`
    cursor: help;
    &:hover {
      background: ${t.inkSubtle};
    }
  `,
}))

interface Props {
  name: string
  server: McpServerSummary
}

/**
 * Renders the Tooltip overlay content (used in tests as a standalone export
 * to bypass antd Tooltip's hover-trigger complexity).
 */
export function McpServerTooltipOverlay({ name, server }: Props) {
  const { styles } = useStyles()
  const { useToken } = theme
  const { token } = useToken()

  const cssVars = {
    '--mcp-tooltip-text': token.colorText,
    '--mcp-tooltip-bg': token.colorBgElevated,
    '--mcp-tooltip-border': `color-mix(in srgb, ${token.colorText}, transparent 85%)`,
    '--mcp-tooltip-pill-bg': `color-mix(in srgb, ${token.colorText}, transparent 92%)`,
    '--mcp-tooltip-text-muted': `color-mix(in srgb, ${token.colorText}, transparent 55%)`,
    '--mcp-tooltip-text-subtle': `color-mix(in srgb, ${token.colorText}, transparent 70%)`,
  } as React.CSSProperties

  const rows: { k: string; v: string; redacted?: boolean }[] = []

  if (server.transport === 'stdio') {
    if (server.command) rows.push({ k: 'command', v: server.command })
    if (server.args && server.args.length > 0) {
      rows.push({ k: 'args', v: server.args.join(' ') })
    }
    if (server.env) {
      for (const [k, v] of Object.entries(server.env)) {
        rows.push({ k, v, redacted: v === '***' })
      }
    }
  } else {
    // sse / http
    if (server.url) rows.push({ k: 'url', v: server.url })
    if (server.headers) {
      for (const [k, v] of Object.entries(server.headers)) {
        rows.push({ k, v, redacted: v === '***' })
      }
    }
  }

  return (
    <div className={styles.overlay} style={cssVars}>
      <div className={styles.header}>
        <span>{name}</span>
        <span className={styles.transport}>{server.transport}</span>
      </div>
      {rows.map((r) => (
        <div key={r.k} className={styles.row}>
          <div className={styles.rowKey}>{r.k}</div>
          <div className={r.redacted ? `${styles.rowValue} ${styles.redacted}` : styles.rowValue}>
            {r.v}
          </div>
        </div>
      ))}
    </div>
  )
}

/**
 * Default export: a Tag that shows the MCP server name + transport, with
 * a Tooltip on hover revealing the (already-redacted) full configuration.
 */
export default function McpServerTooltip({ name, server }: Props) {
  const { styles } = useStyles()
  return (
    <Tooltip title={<McpServerTooltipOverlay name={name} server={server} />} placement="bottom">
      <Tag className={styles.triggerTag} tabIndex={0}>
        {name} · {server.transport}
      </Tag>
    </Tooltip>
  )
}
