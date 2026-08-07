import { Markdown } from '@lobehub/ui'

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
