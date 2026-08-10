import { lazy, Suspense } from 'react'
import { Spin } from 'antd'

// Lazy-load @lobehub/ui Markdown — pulls in 3.6 MB of dependencies
// (shiki, mermaid, cytoscape). Only needed on the chat page.
const Markdown = lazy(() => import('@lobehub/ui').then((m) => ({ default: m.Markdown })))

interface ChatMarkdownProps {
  content: string
  enableStream?: boolean
}

/**
 * Thin wrapper around lobe-ui Markdown component.
 * The .msg-text class from chat-markdown.css styles the output.
 */
export default function ChatMarkdown({ content, enableStream }: ChatMarkdownProps) {
  return (
    <div className="msg-text">
      <Suspense fallback={<Spin size="small" />}>
        <Markdown
          enableStream={enableStream}
          animated={enableStream}
          streamSmoothingPreset="silky"
          variant="chat"
        >
          {content}
        </Markdown>
      </Suspense>
    </div>
  )
}
