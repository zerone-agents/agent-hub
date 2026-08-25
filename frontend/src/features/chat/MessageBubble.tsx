import { useState, memo } from 'react'
import { UserIcon, RobotIcon } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import type { ChatMessage } from '@/api/chat'
import { formatTime } from '@/utils/time'
import { tokens as t } from '@/styles/tokens'
import ChatMarkdown from './ChatMarkdown'
import { ContentParts, parseContent } from './parts'

const useStyles = createStyles(({ css }) => ({
  row: css`
    display: flex; gap: 10px; margin-bottom: 20px;
    animation: msgIn 0.25s ease;
    @keyframes msgIn { from { opacity: 0; transform: translateY(4px); } to { opacity: 1; transform: translateY(0); } }
  `,
  rowUser: css`
    flex-direction: row-reverse;
  `,
  avatar: css`
    width: 28px; height: 28px; border-radius: 50%; flex-shrink: 0;
    display: flex; align-items: center; justify-content: center;
    color: var(--primary-foreground);
  `,
  avatarUser: css`background: ${t.ink};`,
  avatarAssistant: css`background: #059669;`,
  avatarSystem: css`background: #6B7280;`,
  avatarTool: css`background: #D97706;`,
  content: css`flex: 1; min-width: 0;`,
  contentUser: css`
    display: flex; flex-direction: column; align-items: flex-end;
  `,
  role: css`font-size: 11px; font-weight: 600; color: ${t.textTertiary}; margin-bottom: 3px; text-transform: capitalize;`,
  bubble: css`
    background: ${t.surface}; border-radius: 0 8px 8px 8px;
    padding: 12px 14px; box-shadow: ${t.elevation1};
    display: block; width: 85%; max-width: 85%;
    box-sizing: border-box;
  `,
  bubbleUser: css`
    background: ${t.softAccent}; border-radius: 8px 0 8px 8px;
    display: inline-block; width: auto; max-width: 85%;
  `,
  footer: css`
    display: flex; align-items: center; gap: 8px; margin-top: 5px;
    font-size: 11px; color: ${t.textMuted};
  `,
  footerUser: css`
    flex-direction: row-reverse;
  `,
  rawToggle: css`
    border: none; background: transparent; cursor: pointer;
    font-size: 11px; color: ${t.textTertiary};
    &:hover { color: ${t.ink}; }
  `,
  hiddenTag: css`
    padding: 1px 5px; border-radius: 2px; font-size: 10px;
    background: rgba(220, 38, 38, 0.08); color: ${t.danger};
  `,
  raw: css`
    margin: 0; font-family: ${t.fontMono}; font-size: 12px;
    white-space: pre-wrap; word-break: break-word; color: ${t.text};
  `
}))

const ROLE_LABELS: Record<string, string> = {
  user: '用户', assistant: '助手', system: '系统', tool: '工具'
}

/**
 * token_usage 是自由文本展示字段，但 agent-runtime 的回传推送发的是
 * {"total_input","total_output","total_tokens"} JSON 字符串——原样渲染
 * 生硬。识别该形状时格式化为人类可读，其余（如 "50 tokens"）原样显示。
 */
function formatTokenUsage(raw: string): string {
  try {
    // JSON.parse("null") 运行时返回 null，类型须显式可空
    const v = JSON.parse(raw) as Record<string, unknown> | null
    if (v && typeof v === 'object' && typeof v.total_tokens === 'number') {
      const parts: string[] = []
      if (typeof v.total_input === 'number') parts.push(`↑${v.total_input}`)
      if (typeof v.total_output === 'number') parts.push(`↓${v.total_output}`)
      if (parts.length > 0) return `${parts.join(' ')} · ${v.total_tokens} tokens`
    }
  } catch {
    // 非 JSON（调用方自带的展示字符串），原样返回
  }
  return raw
}

const AVATAR_STYLES: Record<string, string> = {
  user: 'avatarUser',
  assistant: 'avatarAssistant',
  system: 'avatarSystem',
  tool: 'avatarTool'
}

function formatAigc(aigc: string | undefined): string | null {
  if (!aigc) return null
  try {
    return JSON.stringify(JSON.parse(aigc), null, 2)
  } catch {
    return aigc
  }
}

interface MessageBubbleProps {
  message: ChatMessage
  enableStream?: boolean
}

function MessageBubbleInner({ message: msg, enableStream }: MessageBubbleProps) {
  const { styles } = useStyles()
  const [raw, setRaw] = useState(false)

  const isUser = msg.role === 'user'
  const avatarStyleKey = AVATAR_STYLES[msg.role] || 'avatarSystem'
  const avatarStyle = styles[avatarStyleKey as keyof typeof styles] || styles.avatarSystem

  return (
    <div className={`${styles.row} ${isUser ? styles.rowUser : ''}`}>
      <div className={`${styles.avatar} ${avatarStyle}`}>
        {isUser ? <UserIcon size={14} weight="bold" /> : <RobotIcon size={14} weight="bold" />}
      </div>
      <div className={`${styles.content} ${isUser ? styles.contentUser : ''}`}>
        <div className={styles.role}>{ROLE_LABELS[msg.role] || msg.role}</div>
        <div className={`${styles.bubble} ${isUser ? styles.bubbleUser : ''}`}>
          {raw ? (
            <pre className={styles.raw}>
              {msg.content}
              {(() => {
                const aigc = formatAigc(msg.aigc)
                if (!aigc) return null
                return `\n\n—— AIGC Label ——\n${aigc}`
              })()}
            </pre>
          ) : (
            (() => {
              const parts = parseContent(msg.content)
              if (!parts) return <ChatMarkdown content={msg.content} enableStream={enableStream} />
              return <ContentParts parts={parts} enableStream={enableStream} />
            })()
          )}
        </div>
        <div className={`${styles.footer} ${isUser ? styles.footerUser : ''}`}>
          <span>{formatTime(msg.created_at)}</span>
          {msg.hidden && <span className={styles.hiddenTag}>已隐藏</span>}
          {msg.token_usage && <span>{formatTokenUsage(msg.token_usage)}</span>}
          <button type="button" className={styles.rawToggle} onClick={() => { setRaw(!raw); }}>
            {raw ? 'Markdown' : 'Raw'}
          </button>
        </div>
      </div>
    </div>
  )
}

const MessageBubble = memo(MessageBubbleInner)
export default MessageBubble
