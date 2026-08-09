import { useState, useMemo } from 'react'
import { CaretRightIcon, CaretDownIcon, CheckCircleIcon, XCircleIcon, SpinnerIcon } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import { tokens as t } from '@/styles/tokens'
import {
  getToolSummary,
  buildToolInputMarkdown,
  detectResultLang,
  escapeCodeFences
} from '@/lib/tool-format'
import ChatMarkdown from './ChatMarkdown'

const useStyles = createStyles(({ css }) => ({
  card: css`
    border: 1px solid color-mix(in srgb, var(--foreground) 10%, transparent);
    border-radius: 6px;
    background: ${t.surface};
    overflow: hidden;
    max-width: 100%;
    box-sizing: border-box;
  `,
  cardError: css`
    border-color: rgba(220, 38, 38, 0.3);
  `,
  title: css`
    display: flex; align-items: center; gap: 8px;
    padding: 6px 10px; cursor: pointer; user-select: none;
    background: color-mix(in srgb, var(--foreground) 3%, transparent); font-size: 12px;
    &:hover { background: color-mix(in srgb, var(--foreground) 6%, transparent); }
  `,
  titleError: css`
    background: rgba(220, 38, 38, 0.05);
    &:hover { background: rgba(220, 38, 38, 0.08); }
  `,
  toolName: css`
    font-family: ${t.fontMono}; font-weight: 600; color: ${t.text};
  `,
  summary: css`
    color: ${t.textSecondary}; font-size: 12px;
    overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    flex: 1; min-width: 0;
  `,
  body: css`
    padding: 8px 10px;
    overflow-x: auto;
    max-width: 100%;
    box-sizing: border-box;
  `,
  sectionLabel: css`
    font-size: 11px; font-weight: 700; color: ${t.textTertiary};
    text-transform: uppercase; letter-spacing: 0.04em;
    margin-bottom: 4px;
  `,
  inputSection: css`
    padding-bottom: 4px;
  `,
  resultSection: css`
    padding-top: 8px;
    border-top: 1px solid ${t.inkLighter};
    margin-top: 8px;
  `,
  pendingPlaceholder: css`
    font-size: 12px; color: ${t.textMuted}; font-style: italic;
  `,
  emptyPlaceholder: css`
    font-size: 12px; color: ${t.textMuted};
  `,
  showMoreBtn: css`
    margin-top: 6px;
    background: transparent; border: none; cursor: pointer;
    font-size: 12px; color: ${t.textTertiary};
    padding: 2px 4px;
    &:hover { color: ${t.ink}; }
  `
}))

export interface ToolCallBlockProps {
  toolName: string
  toolId: string
  input?: Record<string, unknown>
  result?: unknown
  status: 'pending' | 'success' | 'error'
}

const INITIAL_RESULT_LIMIT = 1000
const SHOW_MORE_INCREMENT = 2000

function resultToString(result: unknown): string {
  if (result === null || result === undefined) return ''
  if (typeof result === 'string') return result
  try {
    return JSON.stringify(result, null, 2)
  } catch {
    return typeof result === 'number' || typeof result === 'boolean' || typeof result === 'bigint'
      ? String(result)
      : '[unserializable]'
  }
}

export default function ToolCallBlock({
  toolName,
  toolId: _toolId,
  input,
  result,
  status
}: ToolCallBlockProps) {
  const { styles } = useStyles()

  const summary = getToolSummary(toolName, input)
  const inputMd = useMemo(
    () => buildToolInputMarkdown(toolName, input),
    [toolName, input]
  )
  const resultStr = useMemo(() => resultToString(result), [result])

  const [open, setOpen] = useState(false)
  const [limit, setLimit] = useState<number>(INITIAL_RESULT_LIMIT)

  const escaped = useMemo(() => escapeCodeFences(resultStr), [resultStr])
  const lang = useMemo(() => detectResultLang(escaped), [escaped])
  const isTruncated = escaped.length > limit
  const display = isTruncated ? escaped.slice(0, limit) + '\n...(truncated)' : escaped
  const resultMd = `\`\`\`${lang}\n${display}\n\`\`\``

  const handleClick = () => {
    setOpen(o => !o)
  }

  return (
    <div className={`${styles.card} ${status === 'error' ? styles.cardError : ''}`}>
      <div
        className={`${styles.title} ${status === 'error' ? styles.titleError : ''}`}
        data-testid="tool-call-title"
        data-status={status}
        onClick={handleClick}
      >
        {open ? <CaretDownIcon size={10} /> : <CaretRightIcon size={10} />}
        {status === 'pending' && <SpinnerIcon size={12} color={t.textMuted} />}
        {status === 'success' && <CheckCircleIcon size={12} color={t.success} weight="fill" />}
        {status === 'error' && <XCircleIcon size={12} color={t.danger} weight="fill" />}
        <span className={styles.toolName}>{toolName}</span>
        {summary && <span className={styles.summary}>{summary}</span>}
      </div>
      {open && (
        <div className={styles.body}>
          {inputMd && (
            <div className={styles.inputSection}>
              <div className={styles.sectionLabel}>输入</div>
              <ChatMarkdown content={inputMd} />
            </div>
          )}
          <div className={styles.resultSection}>
            <div className={styles.sectionLabel}>Result</div>
            {status === 'pending' ? (
              <div className={styles.pendingPlaceholder}>等待结果…</div>
            ) : resultStr === '' ? (
              <div className={styles.emptyPlaceholder}>（无输出）</div>
            ) : (
              <>
                <ChatMarkdown content={resultMd} />
                {isTruncated && (
                  <button
                    type="button"
                    className={styles.showMoreBtn}
                    onClick={() => { setLimit(n => n + SHOW_MORE_INCREMENT); }}
                  >
                    Show More
                  </button>
                )}
              </>
            )}
          </div>
        </div>
      )}
    </div>
  )
}
