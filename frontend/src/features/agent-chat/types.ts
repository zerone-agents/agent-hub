// SSE event payload types sent by the control-panel backend (proxied from runtime)

export interface SSEEvent {
  event: string
  data: string
}

export interface SystemInitData {
  type: 'system'
  subtype: 'init'
  session_id?: string
  [k: string]: unknown
}

export interface PartialTextData {
  type: 'partial_message'
  partial: { type: 'text'; text: string }
}

export interface PartialThinkingData {
  type: 'partial_message'
  partial: { type: 'thinking'; text: string }
}

export interface PartialToolUseData {
  type: 'partial_message'
  partial: {
    type: 'tool_use'
    id?: string
    tool_name?: string
    input?: Record<string, unknown>
  }
}

export interface AssistantData {
  type: 'assistant'
  message: {
    role: 'assistant'
    content: { type: string; [k: string]: unknown }[]
  }
}

export interface ToolResultData {
  type: 'tool_result'
  result: unknown
}

export interface ResultSuccessData {
  type: 'result'
  subtype: 'success'
  usage?: { input_tokens?: number; output_tokens?: number }
}

// Content part shape (same as MessageBubble in features/chat)
export interface ContentPart {
  type: string
  text?: string
  reasoning?: string
  duration?: number
  name?: string
  id?: string
  input?: Record<string, unknown>
  content?: unknown
  toolUseId?: string  // present on tool_result parts
  isError?: boolean   // present on tool_result parts
  [k: string]: unknown
}
