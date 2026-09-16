/**
 * scenes.ts — 场景（预设问题）API 层
 *
 * 数据源：agent-hub GET /api/v1/scenes（guest 白名单端点，需 JWT）。
 * 后台场景按 agent 名分组，H5 首页的预设卡片 = 当前 Agent 的启用场景：
 * 卡片标题 ← scene.title，点击发送 ← scene.prompt。
 */

import type { ScenarioPrompt } from '../types';
import { getAuthHeader } from './auth';

interface HubScene {
  id?: number;
  name?: string;
  /** 所属 Agent 的 hub name（与 Agent.id 一致） */
  agent?: string;
  title?: string;
  titleEn?: string;
  prompt?: string;
  promptEn?: string;
  enabled?: boolean;
}

/** 拉取全部场景并按 agent 名转成 ScenarioPrompt（category = agent hub name）。
 *  失败/未登录时返回空数组——没有场景就不显示卡片，不用假数据充数。 */
export async function fetchScenes(): Promise<ScenarioPrompt[]> {
  const res = await fetch('/api/v1/scenes', { headers: getAuthHeader() });
  if (!res.ok) return [];
  const json = (await res.json()) as { data?: HubScene[] };
  const list = json?.data ?? [];
  return list
    .filter((s) => s.enabled !== false && s.agent && (s.title || s.prompt))
    .map((s, i) => ({
      id: s.name || `scene-${s.id ?? i}`,
      icon: '💬',
      title: s.title || s.titleEn || s.name || '',
      prompt: s.prompt || s.promptEn || s.title || '',
      category: s.agent as string,
    }));
}
