import { useState, useRef, useEffect } from 'react'
import { Input } from 'antd'
import type { InputRef } from 'antd'
import { XIcon, PlusIcon } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import { tokens as t } from '@/styles/tokens'

const useStyles = createStyles(({ css }) => ({
  wrap: css`
    position: relative;
    min-width: 0;
  `,
  idle: css`
    cursor: text;
    input { cursor: text; }
  `,
  idleMuted: css`
    color: var(--text-muted);
    input { color: var(--text-muted); }
  `,
  chip: css`
    display: inline-flex;
    align-items: center;
    gap: 2px;
    padding: 2px 4px 2px 6px;
    border-radius: 4px;
    background: var(--ink-subtle);
    font-size: 12px;
    color: var(--text-secondary);
    line-height: 20px;
  `,
  chipX: css`
    display: flex;
    align-items: center;
    border: none;
    background: transparent;
    padding: 0;
    margin-left: 2px;
    cursor: pointer;
    color: var(--text-muted);
    &:hover { color: ${t.danger}; }
  `,
  dropdown: css`
    position: absolute;
    top: 100%;
    left: 0;
    width: 100%;
    z-index: 10;
    margin-top: 4px;
    padding: 6px 8px;
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    border-radius: ${t.radiusSm}px;
    background: var(--surface);
    border: 1px solid color-mix(in srgb, var(--foreground) 10%, transparent);
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.08);
  `,
  addInline: css`
    border: none;
    background: transparent;
    color: var(--text-tertiary);
    cursor: pointer;
    padding: 0 4px;
    display: inline-flex;
    align-items: center;
    &:hover { color: ${t.ink}; }
  `,
}))

interface EffortCellProps {
  value?: string[]
  onChange: (efforts: string[]) => void
}

export default function EffortCell({ value, onChange }: EffortCellProps) {
  const { styles } = useStyles()
  const [draft, setDraft] = useState('')
  const [active, setActive] = useState(false)
  const efforts = value ?? []
  const inputRef = useRef<InputRef>(null)

  // Auto-focus input when activated from idle
  useEffect(() => {
    if (active) {
      inputRef.current?.focus()
    }
  }, [active])

  const add = () => {
    const v = draft.trim()
    if (v && !efforts.includes(v)) {
      onChange([...efforts, v])
      setDraft('')
    }
  }

  // Idle: readOnly Input 展示「已配置 N 档」/「不涉及」（空值时灰色弱化），
  // 用真实 antd Input 保证与同行其他输入框尺寸完全一致
  if (!active) {
    const empty = efforts.length === 0
    const label = empty ? '不涉及' : `已配置 ${efforts.length} 档`
    return (
      <div className={styles.wrap}>
        <Input
          size="small"
          readOnly
          value={label}
          className={empty ? `${styles.idle} ${styles.idleMuted}` : styles.idle}
          onClick={() => { setActive(true); }}
          aria-label={label}
        />
      </div>
    )
  }

  // Active: input + add button + chips dropdown
  return (
    <div className={styles.wrap}>
      <Input
        ref={inputRef}
        size="small"
        value={draft}
        onChange={(e) => { setDraft(e.target.value); }}
        onBlur={() => {
          // Defer so click on suffix/dropdown can register first
          setTimeout(() => { setActive(false); }, 120)
        }}
        suffix={
          <button
            type="button"
            className={styles.addInline}
            onClick={add}
            aria-label="添加 effort"
            style={{ visibility: draft.trim() ? 'visible' : 'hidden' }}
          >
            <PlusIcon size={12} weight="bold" />
          </button>
        }
        onPressEnter={(e) => {
          e.preventDefault()
          add()
        }}
      />
      {efforts.length > 0 && (
        <div className={styles.dropdown}>
          {efforts.map((effort) => (
            <span key={effort} className={styles.chip}>
              {effort}
              <button
                type="button"
                className={styles.chipX}
                aria-label={`删除 ${effort}`}
                onClick={() => { onChange(efforts.filter((e) => e !== effort)); }}
              >
                <XIcon size={10} />
              </button>
            </span>
          ))}
        </div>
      )}
    </div>
  )
}
