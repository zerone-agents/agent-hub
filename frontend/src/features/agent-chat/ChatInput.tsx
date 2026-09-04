// frontend/src/features/agent-chat/ChatInput.tsx
import { useState, useImperativeHandle, type ClipboardEvent, type DragEvent, type KeyboardEvent, type Ref } from 'react'
import { Input, Upload } from 'antd'
import { PaperPlaneRightIcon, PaperclipIcon, XIcon } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import { tokens as t } from '@/styles/tokens'
import PrimaryButton from '@/components/PrimaryButton'
import { formatBytes } from '@/utils/format'
import type { AttachmentItem } from './useAttachments'

const useStyles = createStyles(({ css }) => ({
  wrap: css`
    padding: 12px 16px;
    border-top: 1px solid color-mix(in srgb, var(--foreground) 6%, transparent);
    display: flex;
    flex-direction: column;
    gap: 8px;
    background: ${t.surface};
    transition: border-color 0.15s;
    &.dragover {
      border-top-color: ${t.ink};
      box-shadow: inset 0 2px 0 ${t.ink};
    }
  `,
  row: css`
    display: flex;
    gap: 8px;
    align-items: flex-end;
  `,
  textarea: css`
    flex: 1;
    resize: none;
  `,
  attachBtn: css`
    border: none;
    background: transparent;
    cursor: pointer;
    color: ${t.textSecondary};
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 32px;
    height: 32px;
    border-radius: 8px;
    flex-shrink: 0;
    align-self: flex-end;
    margin-bottom: 1px;
    &:hover { background: ${t.inkSubtle}; color: ${t.ink}; }
    &:disabled { opacity: 0.4; cursor: default; }
  `,
  tray: css`
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
  `,
  chip: css`
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 4px 8px;
    border: 1px solid ${t.inkLighter};
    border-radius: 8px;
    font-size: 12px;
    color: ${t.textSecondary};
    background: ${t.surface};
    max-width: 260px;
  `,
  thumb: css`
    width: 36px;
    height: 36px;
    object-fit: cover;
    border-radius: 6px;
  `,
  chipName: css`
    max-width: 140px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  `,
  chipRemove: css`
    border: none;
    background: transparent;
    cursor: pointer;
    display: inline-flex;
    padding: 2px;
    color: ${t.textMuted};
    &:hover { color: ${t.danger}; }
    &:disabled { opacity: 0.4; cursor: default; }
  `,
  attachError: css`
    font-size: 12px;
    color: ${t.danger};
  `,
}))

export interface ChatInputAttachments {
  enabled: boolean
  items: AttachmentItem[]
  uploading: boolean
  add: (files: File[]) => string | null
  remove: (id: string) => void
}

/** imperative 接口：文本写回/清空由页面控制（Task 11 fix round 1 + 终审 F2） */
export interface ChatInputHandle {
  /** attachment_missing 重试路径由页面写回文本 */
  restoreText: (text: string) => void
  /** SSE 建立后由页面调用清空文本（spec F2：fetch 200 前文本一律保留） */
  clearText: () => void
}

interface ChatInputProps {
  ref?: Ref<ChatInputHandle>
  disabled: boolean
  /** 返回值不再驱动文本清空（spec F2：清空责任在页面 onEstablished）；类型保留兼容既有调用方 */
  // eslint-disable-next-line @typescript-eslint/no-invalid-void-type -- spec contract: void keeps existing Promise<void> callers assignable; rule's allowInUnionAsReturn covers top-level union only, not nested Promise<void | boolean>
  onSend: (content: string) => void | Promise<void | boolean>
  attachments?: ChatInputAttachments
}

export default function ChatInput({ ref, disabled, onSend, attachments }: ChatInputProps) {
  const { styles } = useStyles()
  const [value, setValue] = useState('')
  const [attachError, setAttachError] = useState<string | null>(null)
  const [dragOver, setDragOver] = useState(false)
  const [sending, setSending] = useState(false)
  useImperativeHandle(ref, () => ({
    restoreText: (text: string) => { setValue(text); },
    clearText: () => { setValue(''); },
  }), [])

  const hasAttachments = (attachments?.items.length ?? 0) > 0
  const inputDisabled = disabled || sending

  const submit = async () => {
    const trimmed = value.trim()
    if (inputDisabled || attachments?.uploading) return
    if (!trimmed && !hasAttachments) return
    setSending(true)
    try {
      // spec F2：发送失败（409/502/网络错误等）一律保留文本；清空责任在页面
      // onEstablished（SSE 建立后经 clearText），组件不再自行清空。
      await onSend(trimmed)
    } finally {
      setSending(false)
    }
  }

  const handleIncoming = (files: File[]) => {
    if (!attachments?.enabled || attachments.uploading || files.length === 0) return
    setAttachError(attachments.add(files))
  }

  const onKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      void submit()
    }
  }

  const onPaste = (e: ClipboardEvent<HTMLTextAreaElement>) => {
    const files = Array.from(e.clipboardData.files)
    if (files.length > 0) {
      e.preventDefault()
      handleIncoming(files)
    }
  }

  const onDrop = (e: DragEvent<HTMLDivElement>) => {
    e.preventDefault()
    setDragOver(false)
    handleIncoming(Array.from(e.dataTransfer.files))
  }

  return (
    <div
      className={dragOver ? `${styles.wrap} dragover` : styles.wrap}
      onDragOver={(e) => {
        e.preventDefault()
        setDragOver(true)
      }}
      onDragLeave={() => { setDragOver(false); }}
      onDrop={onDrop}
    >
      {attachments && attachments.items.length > 0 && (
        <div className={styles.tray}>
          {attachments.items.map((item) => (
            <span key={item.id} className={styles.chip} title={item.file.name}>
              {item.previewUrl ? (
                <img className={styles.thumb} src={item.previewUrl} alt={item.file.name} />
              ) : (
                <PaperclipIcon size={14} />
              )}
              <span className={styles.chipName}>{item.file.name}</span>
              <span>{formatBytes(item.file.size)}</span>
              {item.status === 'uploading' && <span>上传中…</span>}
              {item.status === 'uploaded' && <span style={{ color: t.ink }}>✓</span>}
              <button
                type="button"
                className={styles.chipRemove}
                onClick={() => { attachments.remove(item.id); }}
                disabled={attachments.uploading}
                aria-label={`移除 ${item.file.name}`}
              >
                <XIcon size={12} />
              </button>
            </span>
          ))}
        </div>
      )}
      {attachError && <div className={styles.attachError}>{attachError}</div>}
      <div className={styles.row}>
        {attachments?.enabled && (
          <Upload
            multiple
            showUploadList={false}
            beforeUpload={(file) => {
              handleIncoming([file])
              return Upload.LIST_IGNORE
            }}
          >
            <button
              type="button"
              className={styles.attachBtn}
              disabled={inputDisabled || attachments.uploading}
              aria-label="添加附件"
              title="添加附件（支持拖拽 / 粘贴）"
            >
              <PaperclipIcon size={16} />
            </button>
          </Upload>
        )}
        <Input.TextArea
          className={styles.textarea}
          placeholder="输入消息... (Enter 发送，Shift+Enter 换行)"
          autoSize={{ minRows: 1, maxRows: 6 }}
          value={value}
          onChange={(e) => { setValue(e.target.value); }}
          onKeyDown={onKeyDown}
          onPaste={onPaste}
          disabled={inputDisabled}
        />
        <PrimaryButton
          icon={<PaperPlaneRightIcon size={14} weight="bold" />}
          disabled={inputDisabled || (attachments?.uploading ?? false) || (!value.trim() && !hasAttachments)}
          onClick={() => { void submit(); }}
        >
          发送
        </PrimaryButton>
      </div>
    </div>
  )
}
