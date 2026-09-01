import { useEffect, useRef } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import PrimaryButton from '@/components/PrimaryButton'
import { createStyles } from 'antd-style'

const useStyles = createStyles(() => ({
  mask: {
    position: 'fixed',
    inset: 0,
    zIndex: 2000,
    background: 'rgba(0, 0, 0, 0.45)',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    padding: 24,
  },
  box: {
    background: '#fff',
    borderRadius: 8,
    padding: '20px 24px',
    maxWidth: 560,
    width: '100%',
    boxShadow: '0 6px 24px rgba(0, 0, 0, 0.2)',
  },
  hint: {
    marginBottom: 12,
    fontSize: 13,
    color: '#8c8c8c',
  },
  textarea: {
    width: '100%',
    minHeight: 96,
    resize: 'none',
    fontFamily: 'monospace',
    fontSize: 13,
    padding: 8,
    boxSizing: 'border-box' as const,
  },
  footer: {
    marginTop: 12,
    textAlign: 'right',
  },
}))

/**
 * 非安全上下文（纯 HTTP + IP）下现代浏览器无法可靠地将内容写入剪贴板：
 * - navigator.clipboard 不存在；
 * - document.execCommand('copy') 在 Chrome 中静默假成功（返回 true 但未写入，无法检出）；
 * - 剪贴板写入又无法回读验证。
 * 因此统一改为弹出手动复制框：只读 textarea 聚焦供用户自选，
 * 用户选中后按 ⌘C / Ctrl+C 复制、关闭。
 * 注意不能自动全选（也不能在 onFocus 里重选），否则会覆盖用户的手动选区。
 */
export function showManualCopy(text: string) {
  const wrap = document.createElement('div')
  document.body.appendChild(wrap)
  const root: Root = createRoot(wrap)
  root.render(
    <ManualCopyDialog
      text={text}
      onClose={() => {
        root.unmount()
        wrap.remove()
      }}
    />
  )
}

function ManualCopyDialog({ text, onClose }: { text: string; onClose: () => void }) {
  const { styles } = useStyles()
  const taRef = useRef<HTMLTextAreaElement>(null)

  useEffect(() => {
    taRef.current?.focus()
  }, [])

  return (
    <div className={styles.mask} role="dialog" aria-label="手动复制">
      <div className={styles.box}>
        <div className={styles.hint}>
          浏览器限制非 HTTPS 页面自动复制。请选中下方内容后按 ⌘C / Ctrl+C 复制，完成后关闭。
        </div>
        <textarea
          ref={taRef}
          className={styles.textarea}
          readOnly
          value={text}
        />
        <div className={styles.footer}>
          <PrimaryButton onClick={onClose}>
            关闭
          </PrimaryButton>
        </div>
      </div>
    </div>
  )
}