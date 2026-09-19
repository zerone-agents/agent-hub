import { useState, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { CaretRightIcon, CaretDownIcon, CheckCircleIcon, XCircleIcon, SpinnerIcon } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import { tokens as tk } from '@/styles/tokens'
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
    background: ${tk.surface};
    overflow: hidden;
    max-width: 100%;
    box-sizing: border-box;
  `,
  cardError: css`
    border-color: rgba(220, 38, 38, 0.3);
  `,
  cardProcessing: css`border-color: color-mix(in srgb, var(--primary) 35%, transparent);`,
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
    font-family: ${tk.fontMono}; font-weight: 600; color: ${tk.text};
  `,
  summary: css`
    color: ${tk.textSecondary}; font-size: 12px;
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
    font-size: 11px; font-weight: 700; color: ${tk.textTertiary};
    text-transform: uppercase; letter-spacing: 0.04em;
    margin-bottom: 4px;
  `,
  inputSection: css`
    padding-bottom: 4px;
  `,
  resultSection: css`
    padding-top: 8px;
    border-top: 1px solid ${tk.inkLighter};
    margin-top: 8px;
  `,
  pendingPlaceholder: css`
    font-size: 12px; color: ${tk.textMuted}; font-style: italic;
  `,
  emptyPlaceholder: css`
    font-size: 12px; color: ${tk.textMuted};
  `,
  showMoreBtn: css`
    margin-top: 6px;
    background: transparent; border: none; cursor: pointer;
    font-size: 12px; color: ${tk.textTertiary};
    padding: 2px 4px;
    &:hover { color: ${tk.ink}; }
  `
}))

export interface ToolCallBlockProps {
  toolName: string
  toolId: string
  input?: Record<string, unknown>
  result?: unknown
  status: 'pending' | 'processing' | 'success' | 'error'
}

type ToolBusinessStatus = ToolCallBlockProps['status']

function parseResult(result: unknown): unknown {
  if (typeof result !== 'string') return result
  const value = result.trim()
  if (!value.startsWith('{') && !value.startsWith('[')) return result
  try { return JSON.parse(value) } catch { return result }
}

/** Transport success is not business success. MCP results often carry their
 * own state in a JSON envelope, so derive the visible state from that envelope. */
export function getToolBusinessStatus(transportStatus: ToolCallBlockProps['status'], result: unknown): ToolBusinessStatus {
  if (transportStatus === 'pending' || transportStatus === 'error') return transportStatus
  const parsed = parseResult(result)
  const objects: Record<string, unknown>[] = []
  const visit = (value: unknown, depth = 0) => {
    if (depth > 3 || !value || typeof value !== 'object') return
    if (Array.isArray(value)) { value.forEach((item) => { visit(item, depth + 1); }); return }
    const object = value as Record<string, unknown>
    objects.push(object)
    for (const key of ['result', 'data', 'structuredContent']) visit(object[key], depth + 1)
  }
  visit(parsed)
  const statusOf = (item: Record<string, unknown>) => typeof item.status === 'string' ? item.status.toLowerCase() : ''
  if (objects.some((item) => item.isError === true || ['error', 'failed', 'guarded', 'rejected', 'dead_letter'].includes(statusOf(item)))) return 'error'
  if (objects.some((item) => ['queued', 'running', 'pending', 'processing', 'retry'].includes(statusOf(item)))) return 'processing'
  const text = typeof parsed === 'string' ? parsed.toLowerCase() : ''
  if (/no enabled relation|没有已启用的关系|请求超时|route_not_found|action_not_allowed/.test(text)) return 'error'
  return transportStatus
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
  const { t } = useTranslation()
  const { styles } = useStyles()

  const summary = getToolSummary(toolName, input)
  const inputMd = useMemo(
    () => buildToolInputMarkdown(toolName, input),
    [toolName, input]
  )
  const resultStr = useMemo(() => resultToString(result), [result])
  const visibleStatus = useMemo(() => getToolBusinessStatus(status, result), [status, result])

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
    <div className={`${styles.card} ${visibleStatus === 'error' ? styles.cardError : ''} ${visibleStatus === 'processing' ? styles.cardProcessing : ''}`}>
      <div
        className={`${styles.title} ${visibleStatus === 'error' ? styles.titleError : ''}`}
        data-testid="tool-call-title"
        data-status={visibleStatus}
        onClick={handleClick}
      >
        {open ? <CaretDownIcon size={10} /> : <CaretRightIcon size={10} />}
        {(visibleStatus === 'pending' || visibleStatus === 'processing') && <SpinnerIcon size={12} color={visibleStatus === 'processing' ? 'var(--primary)' : tk.textMuted} />}
        {visibleStatus === 'success' && <CheckCircleIcon size={12} color={tk.success} weight="fill" />}
        {visibleStatus === 'error' && <XCircleIcon size={12} color={tk.danger} weight="fill" />}
        <span className={styles.toolName}>{toolName}</span>
        {summary && <span className={styles.summary}>{summary}</span>}
        {visibleStatus === 'processing' && <span className={styles.summary}>后台处理中，结果会在运行档案自动更新</span>}
      </div>
      {open && (
        <div className={styles.body}>
          {inputMd && (
            <div className={styles.inputSection}>
              <div className={styles.sectionLabel}>{t('chat.toolCall.input')}</div>
              <ChatMarkdown content={inputMd} />
            </div>
          )}
          <div className={styles.resultSection}>
            <div className={styles.sectionLabel}>Result</div>
            {visibleStatus === 'pending' ? (
              <div className={styles.pendingPlaceholder}>{t('chat.toolCall.pending')}</div>
            ) : resultStr === '' ? (
              <div className={styles.emptyPlaceholder}>{t('chat.toolCall.emptyOutput')}</div>
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
