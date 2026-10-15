import { apiRequest } from "./base";

export interface ChatSession {
  id: string;
  user_id: string;
  title: string;
  agent_id: string;
  model?: string;
  status?: string;
  hidden?: boolean;
  user_name?: string;
  display_name?: string;
  created_at: string;
  updated_at: string;
}

export interface ChatMessage {
  id: string;
  session_id: string;
  role: string;
  content: string;
  created_at: string;
  hidden?: boolean;
  token_usage?: string;
  feedback?: string;
}

export interface ChatPage<T> {
  items: T[] | null;
  total: number;
  page: number;
  page_size: number;
  total_pages: number;
}

export interface ChatSessionQuery {
  page?: number;
  pageSize?: number;
  agentId?: string;
  user?: string;
  keyword?: string;
  messageContains?: string;
  from?: string;
  to?: string;
}

const BASE = "/api/v1/admin/chat";

// 只发送显式给出的参数：空串/undefined 一律不发，避免把「空关键词」误传到服务端，
// 也避免 CLI 与服务端各持一套默认值（分页默认口径在服务端）。
function sessionQuery(q: ChatSessionQuery): Record<string, string | number> {
  const out: Record<string, string | number> = {};
  if (q.page !== undefined) out["page"] = q.page;
  if (q.pageSize !== undefined) out["page_size"] = q.pageSize;
  if (q.agentId) out["agent_id"] = q.agentId;
  if (q.user) out["user"] = q.user;
  if (q.keyword) out["keyword"] = q.keyword;
  if (q.messageContains) out["message_contains"] = q.messageContains;
  if (q.from) out["from"] = q.from;
  if (q.to) out["to"] = q.to;
  return out;
}

export function listChatSessions(q: ChatSessionQuery = {}): Promise<ChatPage<ChatSession>> {
  return apiRequest<ChatPage<ChatSession>>(`${BASE}/sessions`, { query: sessionQuery(q) });
}

export function getChatSession(id: string): Promise<ChatSession> {
  return apiRequest<ChatSession>(`${BASE}/sessions/${encodeURIComponent(id)}`);
}

export function listChatMessages(
  id: string,
  page?: { page?: number; pageSize?: number }
): Promise<ChatPage<ChatMessage>> {
  const query: Record<string, string | number> = {};
  if (page?.page !== undefined) query["page"] = page.page;
  if (page?.pageSize !== undefined) query["page_size"] = page.pageSize;
  return apiRequest<ChatPage<ChatMessage>>(`${BASE}/sessions/${encodeURIComponent(id)}/messages`, { query });
}