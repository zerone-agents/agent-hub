import { useState, type KeyboardEvent } from 'react'
import { Input } from 'antd'
import { PaperPlaneRightIcon } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import { tokens as t } from '@/styles/tokens'
import PrimaryButton from '@/components/PrimaryButton'

const useStyles = createStyles(({ css }) => ({
  wrap: css`
    padding: 12px 16px;
    border-top: 1px solid color-mix(in srgb, var(--foreground) 6%, transparent);
    display: flex;
    gap: 8px;
    align-items: flex-end;
    background: ${t.surface};
  `,
  textarea: css`
    flex: 1;
    resize: none;
  `
}))

interface ChatInputProps {
  disabled: boolean
  onSend: (content: string) => void
}

export default function ChatInput({ disabled, onSend }: ChatInputProps) {
  const { styles } = useStyles()
  const [value, setValue] = useState('')

  const submit = () => {
    const trimmed = value.trim()
    if (!trimmed || disabled) return
    onSend(trimmed)
    setValue('')
  }

  const onKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      submit()
    }
  }

  return (
    <div className={styles.wrap}>
      <Input.TextArea
        className={styles.textarea}
        placeholder="输入消息... (Enter 发送，Shift+Enter 换行)"
        autoSize={{ minRows: 1, maxRows: 6 }}
        value={value}
        onChange={(e) => { setValue(e.target.value); }}
        onKeyDown={onKeyDown}
        disabled={disabled}
      />
      <PrimaryButton
        icon={<PaperPlaneRightIcon size={14} weight="bold" />}
        disabled={disabled || !value.trim()}
        onClick={submit}
      >
        发送
      </PrimaryButton>
    </div>
  )
}
