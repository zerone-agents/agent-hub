/**
 * chat.ts — 对客聊天 API 层（与 agent-hub 前端 /agents/chat 页面同一套接口）
 *
 * 接口（agent-hub 对客端点，guest 免 JWT；本地经 server.ts /api/v1 代理）：
 * - POST /api/v1/agents/:name/chat/sessions                 建会话
 * - GET  /api/v1/agents/:name/chat/sessions/:id/messages    历史消息
 * - POST /api/v1/agents/:name/chat/sessions/:id/messages    发消息（SSE 流式响应）
 *
 * SSE 协议（与 agent-hub useChatStream 一致）：
 *   system(init) → partial_message × N（增量）→ assistant（完整消息，替换增量）
 *   → result(success=统计 / subtype=error=失败) → done
 */

import { getAuthHeader } from './auth';

const BASE = '/api/v1/agents';

export interface ChatSession {
  id: string;
  title: string;
  agent_id: string;
  created_at?: string;
  updated_at?: string;
}

/** 历史消息（content 为 JSON 字符串：parts 数组 [{type:'text',text}...] 或纯文本） */
export interface HubChatMessage {
  id: string;
  session_id: string;
  role: string;
  content: string;
  created_at: string;
}

export interface StreamCallbacks {
  /** 流式增量（累计全文，不是单块） */
  onDelta: (fullText: string) => void;
  /** 流正常结束 */
  onDone: (fullText: string) => void;
  /** 失败（HTTP 非 200 / 网络错误 / result subtype=error） */
  onError: (message: string) => void;
}

/** 宽容解析 envelope：{code,data} / {success,data} / 裸数据都兼容 */
function unwrap<T>(body: unknown): T {
  const b = body as { data?: T };
  return (b?.data ?? body) as T;
}

async function parseError(resp: Response): Promise<string> {
  const text = await resp.text().catch(() => '');
  try {
    const body = JSON.parse(text) as { error?: string; message?: string };
    return body.error ?? body.message ?? `HTTP ${resp.status}`;
  } catch {
    return `HTTP ${resp.status}`;
  }
}

/** 创建会话（title 可选，后端一般自动取首条消息） */
export async function createChatSession(agentName: string, title?: string): Promise<ChatSession> {
  const res = await fetch(`${BASE}/${encodeURIComponent(agentName)}/chat/sessions`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', ...getAuthHeader() },
    body: JSON.stringify(title ? { title } : {}),
  });
  if (!res.ok) throw new Error(await parseError(res));
  return unwrap<ChatSession>(await res.json());
}

/** 拉历史消息（用于切换会话时恢复，当前 H5 默认新会话，备用） */
export async function listChatMessages(
  agentName: string,
  sessionId: string
): Promise<HubChatMessage[]> {
  const res = await fetch(
    `${BASE}/${encodeURIComponent(agentName)}/chat/sessions/${encodeURIComponent(sessionId)}/messages?page=1&page_size=50`,
    { headers: getAuthHeader() }
  );
  if (!res.ok) throw new Error(await parseError(res));
  const data = unwrap<{ items?: HubChatMessage[] }>(await res.json());
  return data.items ?? [];
}

/** 历史消息 content 字段可能是 JSON parts 字符串，统一抽纯文本 */
export function extractText(content: string): string {
  try {
    const parts = JSON.parse(content) as Array<{ type?: string; text?: string }>;
    if (Array.isArray(parts)) {
      return parts
        .filter((p) => p?.type === 'text' && p.text)
        .map((p) => p.text)
        .join('\n');
    }
  } catch {
    // 非 JSON，按纯文本返回
  }
  return content;
}

interface SSEPayload {
  partial?: { type?: string; text?: string };
  message?: { content?: Array<{ type?: string; text?: string }> };
  type?: string;
  subtype?: string;
  error_type?: string;
  errors?: unknown;
}

/**
 * 发消息并消费 SSE 流。
 * 文本累积规则与 agent-hub useChatStream 对齐：
 * partial_message 增量累加；assistant 事件为完整消息，替换本轮累积。
 */
export async function streamChatMessage(
  agentName: string,
  sessionId: string,
  content: string,
  callbacks: StreamCallbacks,
  signal?: AbortSignal
): Promise<void> {
  let resp: Response;
  try {
    resp = await fetch(
      `${BASE}/${encodeURIComponent(agentName)}/chat/sessions/${encodeURIComponent(sessionId)}/messages`,
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', ...getAuthHeader() },
        body: JSON.stringify({ content }),
        signal,
      }
    );
  } catch (err) {
    callbacks.onError(err instanceof Error ? err.message : '网络错误');
    return;
  }

  if (!resp.ok) {
    callbacks.onError(await parseError(resp));
    return;
  }
  if (!resp.body) {
    callbacks.onError('响应不支持流式读取');
    return;
  }

  const reader = resp.body.getReader();
  const decoder = new TextDecoder();
  let buf = '';
  let eventName = '';
  let acc = '';

  const finish = (fn: () => void) => {
    fn();
    void reader.cancel().catch(() => undefined);
  };

  try {
    // eslint-disable-next-line no-constant-condition
    while (true) {
      const { value, done } = await reader.read();
      if (done) break;
      buf += decoder.decode(value, { stream: true });

      const lines = buf.split('\n');
      buf = lines.pop() ?? '';

      for (const rawLine of lines) {
        const line = rawLine.replace(/\r$/, '');
        if (line === '') {
          eventName = '';
          continue;
        }
        if (line.startsWith('event:')) {
          eventName = line.slice(6).trim();
          continue;
        }
        if (!line.startsWith('data:')) continue;

        const payload = line.slice(5).trim();
        if (!payload || payload === '{}') continue;

        let data: SSEPayload;
        try {
          data = JSON.parse(payload) as SSEPayload;
        } catch {
          continue;
        }

        if (eventName === 'partial_message') {
          if (data.partial?.type === 'text' && data.partial.text) {
            acc += data.partial.text;
            callbacks.onDelta(acc);
          }
        } else if (eventName === 'assistant') {
          // 完整消息替换本轮增量
          const blocks = data.message?.content ?? [];
          const full = blocks
            .filter((b) => b?.type === 'text' && b.text)
            .map((b) => b.text)
            .join('');
          if (full) {
            acc = full;
            callbacks.onDelta(acc);
          }
        } else if (eventName === 'result') {
          if (data.type === 'result' && data.subtype === 'error') {
            const errs = Array.isArray(data.errors)
              ? (data.errors.filter((e): e is string => typeof e === 'string') as string[])
              : [];
            finish(() =>
              callbacks.onError(errs.join('\n') || data.error_type || 'Runtime 请求失败，请稍后重试')
            );
            return;
          }
        } else if (eventName === 'done') {
          finish(() => callbacks.onDone(acc));
          return;
        }
      }
    }
    // 流结束但没有 done 事件：按完成处理
    callbacks.onDone(acc);
  } catch (err) {
    if (err instanceof Error && err.name === 'AbortError') return; // 用户中断，静默
    callbacks.onError(err instanceof Error ? err.message : '流读取失败');
  }
}
