/**
 * agents.ts — Agent 列表 API 层
 *
 * 数据源：agent-hub 对客公开端点 GET /api/v1/agents?view=chat
 * （guest 白名单端点，无需管理后台 JWT；本地经 server.ts /api/v1 代理到 mock 8081，
 *  连真后端设 AGENT_HUB_URL 环境变量即可）。
 *
 * 字段映射说明（agent-hub Agent → H5 UI Agent）：
 * - agent-hub 侧有：name / config.title{zh,en} / config.description{zh,en} / config.icon / group
 * - H5 卡片还需要「能力介绍、擅长领域、试试这样问我」，后端暂无对应字段 → 统一给默认值，
 *   后续后端补字段后在 mapHubAgent 里优先取真实值即可。
 */

import type { Agent } from '../types';
import { getAuthHeader, getStoredAuth } from './auth';

/** agent-hub 公开列表返回的 Agent 结构（字段全可选，防御性处理） */
interface HubAgentConfig {
  title?: Record<string, string>;
  description?: Record<string, string>;
  icon?: string;
  group?: string;
}

interface HubAgent {
  id?: number;
  name: string;
  config?: HubAgentConfig;
  group?: string;
}

interface ApiEnvelope<T> {
  code: number;
  message?: string;
  data: T;
}

/** 擅长领域默认标签（后端无字段时的兜底） */
const DEFAULT_TAGS = ['智能问答', '多轮对话'];
/** 能力介绍默认文案 */
const DEFAULT_CAPABILITIES = '支持多轮对话、知识检索与任务执行，可在对话中随时调用。';
/** 头像兜底池（按 name 散列取一个，保证同一 Agent 稳定不变） */
const AVATAR_POOL = ['✨', '🤖', '🧠', '💡', '🔍', '📊', '🛠️', '🎯'];

function hashPick(name: string, pool: string[]): string {
  let h = 0;
  for (let i = 0; i < name.length; i++) h = (h * 31 + name.charCodeAt(i)) >>> 0;
  return pool[h % pool.length];
}

/** 试试这样问我 — 默认引导问题（按显示名拼一条，保证个体感） */
function defaultSuggestedQuestions(displayName: string): string[] {
  return [
    `你能帮我做什么？`,
    `介绍一下${displayName}的能力`,
    `给我一个使用示例`,
  ];
}

export function mapHubAgent(a: HubAgent): Agent {
  const displayName = a.config?.title?.zh || a.config?.title?.en || a.name;
  const category = a.group || a.config?.group || '通用';
  return {
    // id 用 hub 的 name（唯一英文标识）：聊天/会话接口的路径参数都走它
    id: a.name,
    name: displayName,
    // 副标题：后端暂无「职称/定位」字段，先用分组名兜底
    title: category,
    avatar: a.config?.icon || hashPick(a.name, AVATAR_POOL),
    category,
    usageCount: '',
    description:
      a.config?.description?.zh ||
      a.config?.description?.en ||
      `由 Zerone 驱动的智能助手，为你提供「${category}」方向的专业支持。`,
    capabilities: DEFAULT_CAPABILITIES,
    tags: a.group ? [a.group, ...DEFAULT_TAGS.slice(1)] : DEFAULT_TAGS,
    suggestedQuestions: defaultSuggestedQuestions(displayName),
    isTeam: true,
  };
}

/** 带 HTTP 状态码的 API 错误（调用方据此区分 401 未登录与网络/服务异常） */
export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

/** 拉取 Agent 列表；失败时抛错由调用方决定兜底。
 *  可见规则（真实后端）：
 *  - 管理角色（admin/maintainer/member）：走 /api/v1/admin/agents 全量列表，
 *    不按部署状态过滤——未部署的 Agent 也能在专家页看到；
 *  - guest / 未登录：走对客视图 /api/v1/agents?view=chat（仅 running + guest_enabled）。
 *  未登录返回 401（ApiError），调用方应显示登录引导而不是兜底假数据。 */
export async function fetchPublicAgents(): Promise<Agent[]> {
  const role = getStoredAuth()?.role;
  if (role && role !== 'guest') {
    try {
      const res = await fetch('/api/v1/admin/agents', { headers: getAuthHeader() });
      // member 等角色可能无 admin 读权限 → 403 时回落对客视图
      if (res.ok) {
        const json = (await res.json()) as ApiEnvelope<{ agents?: HubAgent[] }>;
        return (json?.data?.agents ?? []).map(mapHubAgent);
      }
      if (res.status === 401) throw new ApiError(401, '登录态失效');
    } catch (err) {
      if (err instanceof ApiError) throw err;
      // 网络异常等 → 回落对客视图再试
    }
  }
  const res = await fetch('/api/v1/agents?view=chat', { headers: getAuthHeader() });
  if (!res.ok) throw new ApiError(res.status, `agents 接口返回 ${res.status}`);
  const json = (await res.json()) as ApiEnvelope<{ agents?: HubAgent[] }>;
  const list = json?.data?.agents ?? [];
  return list.map(mapHubAgent);
}
