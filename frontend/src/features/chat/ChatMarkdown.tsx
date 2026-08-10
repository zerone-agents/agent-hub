import { Markdown } from '@lobehub/ui'

interface ChatMarkdownProps {
  content: string
  enableStream?: boolean
}

/**
 * Thin wrapper around lobe-ui Markdown component.
 * The .msg-text class from chat-markdown.css styles the output.
 *
 * Note: this file statically imports @lobehub/ui (3.6 MB chunk), but the
 * chunk is only loaded when the chat route is activated — the route
 * definition in routes/index.tsx uses React Router's `lazy` property to
 * code-split each page. No component-level React.lazy is needed here.
 */
export default function ChatMarkdown({ content, enableStream }: ChatMarkdownProps) {
  return (
    <div className="msg-text">
      <Markdown
        enableStream={enableStream}
        animated={enableStream}
        streamSmoothingPreset="silky"
        variant="chat"
      >
        {content}
      </Markdown>
    </div>
  )
}
