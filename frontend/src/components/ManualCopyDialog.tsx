import { useEffect, useRef, useState } from 'react'
import { Modal, Typography } from 'antd'
import PrimaryButton from '@/components/PrimaryButton'

/**
 * 非安全上下文（纯 HTTP + IP）下现代浏览器无法可靠地将内容写入剪贴板：
 * - navigator.clipboard 不存在；
 * - document.execCommand('copy') 在 Chrome 中静默假成功（返回 true 但未写入，无法检出）；
 * - 剪贴板写入又无法回读验证。
 * 因此统一改为弹出手动复制框：只读 textarea 供用户自由选区，选中后按 ⌘C / Ctrl+C 复制。
 *
 * 必须渲染在应用 React 树内（ConfigProvider 之下）：
 * - 独立 createRoot 会脱离 antd 主题上下文（主题色走 React context，不走 DOM 级联）；
 * - 独立浮层在 antd Modal 打开时会被 rc-dialog 的焦点守卫抢回焦点，textarea 选区无法保持。
 * 由 App 根部挂一次 <ManualCopyHost />，showManualCopy 经注册的处理器调起；
 * 用 antd Modal 实现——嵌套 Modal 的 z-index 与焦点管理由 antd 自动处理。
 */

let showHandler: ((text: string) => void) | null = null

export function showManualCopy(text: string) {
  if (showHandler) {
    showHandler(text)
  } else {
    console.warn('[ManualCopyDialog] host 未挂载，无法展示手动复制框')
  }
}

export function ManualCopyHost() {
  const [text, setText] = useState<string | null>(null)
  const taRef = useRef<HTMLTextAreaElement>(null)

  useEffect(() => {
    showHandler = setText
    return () => { showHandler = null }
  }, [])

  return (
    <Modal
      title="手动复制"
      open={text !== null}
      onCancel={() => { setText(null) }}
      footer={<PrimaryButton onClick={() => { setText(null) }}>关闭</PrimaryButton>}
      afterOpenChange={(open) => { if (open) taRef.current?.focus() }}
      destroyOnHidden
    >
      <Typography.Paragraph type="secondary" style={{ marginBottom: 12, fontSize: 13 }}>
        浏览器限制非 HTTPS 页面自动复制。请选中下方内容后按 ⌘C / Ctrl+C 复制，完成后关闭。
      </Typography.Paragraph>
      <textarea
        ref={taRef}
        readOnly
        value={text ?? ''}
        style={{
          width: '100%',
          minHeight: 96,
          resize: 'none',
          fontFamily: 'monospace',
          fontSize: 13,
          padding: 8,
          boxSizing: 'border-box',
        }}
      />
    </Modal>
  )
}