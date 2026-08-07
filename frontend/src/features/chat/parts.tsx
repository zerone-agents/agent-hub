import { useState } from 'react'
import { CaretRight, CaretDown } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import { tokens as t } from '@/styles/tokens'
import ChatMarkdown from './ChatMarkdown'
import ToolCallBlock from './ToolCallBlock'
import LegacyToolResult from './LegacyToolResult'

// 单段内容：text / reasoning / tool_use / tool_result / 默认 fallback
export interface ContentPart {
  type: string
  text?: string
  reasoning?: string
  duration?: number
  name?: string
  id?: string
  input?: unknown
  content?: unknown
  toolUseId?: string  // present on tool_result parts
  isError?: boolean   // present on tool_result parts
  [k: string]: unknown
}

const useStyles = createStyles(({ css }) => ({
  partWrap: css`
    & > * + * { margin-top: 8px; }
    max-width: 100%;
    overflow: hidden;
  `,
  reasoningWrap: css`
    background: rgba(245, 158, 11, 0.06);
    border-left: 2px solid rgba(245, 158, 11, 0.5);
    padding: 6px 10px; font-size: 12px; color: ${t.textSecondary};
    font-style: italic; white-space: pre-wrap; word-break: break-word;
    max-width: 100%;
    overflow-x: auto;
  `,
  errorWrap: css`
    background: rgba(220, 38, 38, 0.06);
    border-left: 2px solid ${t.danger};
    padding: 8px 10px; font-size: 12px; color: ${t.danger};
    white-space: pre-wrap; word-break: break-word;
  `,
  reasoningToggle: css`
    display: flex; align-items: center; gap: 4px; cursor: pointer;
    font-size: 11px; color: ${t.textTertiary}; user-select: none;
    &:hover { color: ${t.textSecondary}; }
  `,
  toolBody: css`
    padding: 8px 10px; font-family: ${t.fontMono}; font-size: 11px;
    white-space: pre-wrap; word-break: break-word;
    max-height: 400px; overflow-y: auto; color: ${t.text};
  `,
  pendingToolUse: css`
    display: inline-flex; align-items: center; gap: 6px;
    padding: 6px 10px;
    background: ${t.surface};
    border: 1px dashed ${t.inkLighter};
    border-radius: 6px;
    font-size: 12px; color: ${t.textSecondary};
    &::before {
      content: ''; width: 6px; height: 6px; border-radius: 50%;
      background: ${t.textMuted};
      animation: pendingPulse 1.2s infinite ease-in-out both;
    }
    @keyframes pendingPulse { 0%, 80%, 100% { opacity: 0.2; } 40% { opacity: 1; } }
  `,
}))

export function safeStringify(v: unknown): string {
  if (typeof v === 'string') return v
  try {
    return JSON.stringify(v, null, 2)
  } catch {
    return String(v)
  }
}

export function PartText({ text, enableStream }: { text: string; enableStream?: boolean }) {
  return <ChatMarkdown content={text} enableStream={enableStream} />
}

export function PartReasoning({
  reasoning = '',
  duration,
}: {
  reasoning?: string
  duration?: number
}) {
  const { styles } = useStyles()
  const [open, setOpen] = useState(false)
  const preview = reasoning.slice(0, 120)
  const truncated = reasoning.length > 120
  return (
    <div className={styles.reasoningWrap}>
      <div className={styles.reasoningToggle} onClick={() => { setOpen(!open); }}>
        {open ? <CaretDown size={10} /> : <CaretRight size={10} />}
        <span>思考过程{duration ? ` · ${duration}s` : ''}</span>
      </div>
      <div style={{ marginTop: 4 }}>
        {open ? (
          <ChatMarkdown content={reasoning} />
        ) : (
          truncated ? preview + '…' : preview
        )}
      </div>
    </div>
  )
}

export function PartError({ message }: { message?: string }) {
  const { styles } = useStyles()
  return <div className={styles.errorWrap}>{message ?? '发生错误'}</div>
}

type PairablePart =
  | { kind: 'plain'; part: ContentPart }
  | { kind: 'paired'; use: ContentPart; result: ContentPart }
  | { kind: 'orphan_use'; part: ContentPart }
  | { kind: 'pending_use'; part: ContentPart }

function pairParts(parts: ContentPart[]): PairablePart[] {
  const results = new Map<string, ContentPart>()
  for (const p of parts) {
    if (p.type === 'tool_result' && p.toolUseId) {
      results.set(p.toolUseId, p)
    }
  }

  // 记录带 id 的完整 tool_use 的 name，用于去重流式期间先推送的不完整 block
  const completeToolNames = new Set<string>()
  for (const p of parts) {
    if (p.type === 'tool_use' && p.id && p.name) {
      completeToolNames.add(p.name)
    }
  }

  const out: PairablePart[] = []
  for (const p of parts) {
    if (p.type === 'tool_use' && p.id) {
      const result = results.get(p.id)
      if (result) {
        out.push({ kind: 'paired', use: p, result })
      } else {
        out.push({ kind: 'orphan_use', part: p })
      }
    } else if (p.type === 'tool_use' && !p.id) {
      // SDK 流式期间先推送的不完整 tool_use block（只有 type/name，无 id/input）
      // 如果后续已有同 name 的完整版本，跳过；否则标记为 pending_use 显示占位符
      if (p.name && completeToolNames.has(p.name)) continue
      out.push({ kind: 'pending_use', part: p })
    } else if (p.type === 'tool_result' && p.toolUseId) {
      // already attached to its tool_use above; skip
      continue
    } else {
      out.push({ kind: 'plain', part: p })
    }
  }
  return out
}

export function ContentParts({ parts, enableStream }: { parts: ContentPart[]; enableStream?: boolean }) {
  const { styles } = useStyles()
  const paired = pairParts(parts)
  return (
    <div className={styles.partWrap}>
      {paired.map((p, i) => {
        if (p.kind === 'plain') {
          const part = p.part
          if (part.type === 'text' && part.text) {
            return <PartText key={i} text={part.text} enableStream={enableStream} />
          }
          if (part.type === 'reasoning') {
            return (
              <PartReasoning
                key={i}
                reasoning={part.reasoning}
                duration={part.duration}
              />
            )
          }
          if (part.type === 'error') {
            return <PartError key={i} message={part.message as string} />
          }
          if (part.type === 'tool_result') {
            // Legacy tool_result without toolUseId — render via fallback
            return <LegacyToolResult key={i} content={part.content} />
          }
          // Unknown part type — JSON dump
          return <pre key={i} className={styles.toolBody}>{safeStringify(part)}</pre>
        }
        if (p.kind === 'paired') {
          return (
            <ToolCallBlock
              key={i}
              toolName={p.use.name ?? 'tool'}
              toolId={p.use.id ?? ''}
              input={p.use.input as Record<string, unknown> | undefined}
              result={p.result.content}
              status={p.result.isError ? 'error' : 'success'}
            />
          )
        }
        if (p.kind === 'pending_use') {
          return (
            <div key={i} className={styles.pendingToolUse}>
              工具调用准备中{p.part.name ? `：${p.part.name}` : ''}…
            </div>
          )
        }
        // orphan_use
        return (
          <ToolCallBlock
            key={i}
            toolName={p.part.name ?? 'tool'}
            toolId={p.part.id ?? ''}
            input={p.part.input as Record<string, unknown> | undefined}
            status="pending"
          />
        )
      })}
    </div>
  )
}

// 解析 content：可能是 JSON 数组、JSON 对象、或纯文本
export function parseContent(content: string): ContentPart[] | null {
  if (!content) return null
  const trimmed = content.trim()
  if (!trimmed.startsWith('[') && !trimmed.startsWith('{')) return null
  try {
    const parsed: unknown = JSON.parse(trimmed)
    if (Array.isArray(parsed)) return parsed as ContentPart[]
    if (typeof parsed === 'object') return [parsed as ContentPart]
    return null
  } catch {
    return null
  }
}
