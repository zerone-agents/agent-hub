/**
 * auth.ts — 基础登录/退出（对接 agent-hub builtin 认证：POST /auth/login）
 * 本地经 server.ts /auth 代理到 mock；连真后端同样走 AGENT_HUB_URL。
 * 登录态持久化在 localStorage（zerone_auth），当前仅作为登录标记与用户名展示，
 * 后续接真后端时可把 access_token 带到 API 请求头。
 */

const STORAGE_KEY = 'zerone_auth';

/** 登录态变更事件：OAuth 回调/token 登录在 ProfileView 外完成时，广播通知其刷新 */
export const AUTH_CHANGED_EVENT = 'zerone-auth-changed';

export interface AuthState {
  token: string;
  username: string;
  /** 角色：guest=体验用户（登录后自动进聊天页）；admin/maintainer/member 为管理台角色 */
  role?: string;
  /** casdoor OAuth 签发的 refreshToken（ builtin 登录响应里的也会存，供后续续期） */
  refreshToken?: string;
}

export function getStoredAuth(): AuthState | null {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return null;
    const parsed = JSON.parse(raw) as AuthState;
    return parsed?.token ? parsed : null;
  } catch {
    return null;
  }
}

export function storeAuth(auth: AuthState): void {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(auth));
  } catch {
    // ignore
  }
}

export function clearAuth(): void {
  try {
    localStorage.removeItem(STORAGE_KEY);
  } catch {
    // ignore
  }
}

interface LoginResponse {
  access_token?: string;
  accessToken?: string;
  token?: string;
  user?: {
    username?: string;
    display_name?: string;
    name?: string;
    role?: string;
    roles?: string[];
  };
}

/** 从登录/注册响应里提取角色（单 role 或 roles 数组首项） */
function extractRole(data: LoginResponse): string | undefined {
  return data.user?.role ?? data.user?.roles?.[0];
}

/**
 * 真实 agent-hub 的 login/register 只回 TokenPair（accessToken/refreshToken/expiresIn），
 * 不含用户资料与角色 → 登录后必须再调 GET /auth/userinfo（带 Bearer，guest 白名单内）
 * 补全 display_name 与 roles。mock 不可达 userinfo 时回退到响应内字段。
 */
async function enrichWithUserInfo(auth: AuthState): Promise<AuthState> {
  try {
    const res = await fetch('/auth/userinfo', {
      headers: { Authorization: `Bearer ${auth.token}` },
    });
    if (!res.ok) return auth;
    const body = (await res.json()) as {
      data?: { display_name?: string; username?: string; name?: string; roles?: string[] };
    };
    const data = body?.data ??
      (body as { display_name?: string; username?: string; roles?: string[] });
    const roles = Array.isArray(data?.roles) ? data.roles : [];
    return {
      ...auth,
      username:
        data?.display_name ||
        (data as { username?: string })?.username ||
        auth.username,
      role: roles[0] ?? auth.role,
    };
  } catch {
    return auth;
  }
}

/** 统一鉴权头：所有 /api/v1/* 请求（agents/chat/knowledge）都需携带（真实后端整组 JWT 校验） */
export function getAuthHeader(): Record<string, string> {
  const auth = getStoredAuth();
  return auth?.token ? { Authorization: `Bearer ${auth.token}` } : {};
}

// ==================== Casdoor SSO 模式（线上 console.zerone.life 即此模式） ====================

export type AuthMode = 'builtin' | 'casdoor';

/**
 * 探测后端认证模式（GET /auth/mode，免鉴权）。
 * builtin = 账号密码 + 邀请码注册；casdoor = SSO OAuth，无密码登录/注册接口。
 */
export async function fetchAuthMode(): Promise<AuthMode> {
  try {
    const res = await fetch('/auth/mode');
    if (!res.ok) return 'builtin';
    const body = (await res.json()) as { data?: { mode?: string } };
    return body?.data?.mode === 'casdoor' ? 'casdoor' : 'builtin';
  } catch {
    return 'builtin';
  }
}

/**
 * 从 URL 落地参数接收 OAuth token。
 * casdoor 回调：/static{redirect}?token=xxx&refreshToken=yyy（redirect 由 /auth/login?redirect= 带入，
 * H5 部署在 console /static/h5/ 时用 /h5/）。读取后立即清掉 URL 上的 token 参数。
 */
export function extractOAuthTokensFromUrl(): { token: string; refreshToken?: string } | null {
  if (typeof window === 'undefined') return null;
  const params = new URLSearchParams(window.location.search);
  const token = params.get('token');
  if (!token) return null;
  const refreshToken = params.get('refreshToken') ?? undefined;
  params.delete('token');
  params.delete('refreshToken');
  const qs = params.toString();
  window.history.replaceState(
    null,
    '',
    window.location.pathname + (qs ? `?${qs}` : '') + window.location.hash
  );
  return { token, refreshToken };
}

/** 用现成 access_token 登录（OAuth 回调落地 / 开发者粘贴 token）：调 /auth/userinfo 验证并补全资料 */
export async function loginWithToken(token: string, refreshToken?: string): Promise<AuthState> {
  const auth = await enrichWithUserInfo({ token, refreshToken, username: '' });
  if (!auth.username && !auth.role) {
    throw new Error('token 无效或已过期，请重新获取');
  }
  return { ...auth, username: auth.username || 'Zerone 用户' };
}

/** 登录：成功返回 AuthState，失败抛错（错误信息来自后端） */
export async function login(username: string, password: string): Promise<AuthState> {
  const res = await fetch('/auth/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password }),
  });

  const text = await res.text().catch(() => '');
  let body: { data?: LoginResponse; error?: string; message?: string } & LoginResponse = {};
  try {
    body = JSON.parse(text);
  } catch {
    // ignore
  }

  if (!res.ok) {
    throw new Error(body.error ?? body.message ?? `登录失败（HTTP ${res.status}）`);
  }

  const data = body.data ?? body;
  const token = data.access_token ?? data.accessToken ?? data.token ?? '';
  if (!token) throw new Error('登录响应缺少 token');

  return enrichWithUserInfo({
    token,
    username: data.user?.display_name ?? data.user?.name ?? data.user?.username ?? username,
    role: extractRole(data),
  });
}

/** 邀请码预检：返回邀请备注，无效抛错（对齐 agent-hub GET /auth/invite/{token}） */
export async function precheckInvite(token: string): Promise<{ valid: boolean; note: string }> {
  const res = await fetch(`/auth/invite/${encodeURIComponent(token)}`);
  const text = await res.text().catch(() => '');
  let body: { data?: { valid: boolean; note: string }; error?: string; message?: string } & {
    valid?: boolean;
    note?: string;
  } = {};
  try {
    body = JSON.parse(text);
  } catch {
    // ignore
  }
  if (!res.ok) {
    throw new Error(body.error ?? body.message ?? '邀请链接无效或已失效');
  }
  const data = body.data ?? (body as { valid?: boolean; note?: string });
  return { valid: data.valid ?? true, note: data.note ?? '' };
}

/** 注册：邀请码 + 用户名 + 密码（+可选昵称），成功自动登录（对齐 agent-hub POST /auth/register） */
export async function register(
  inviteToken: string,
  username: string,
  password: string,
  displayName?: string
): Promise<AuthState> {
  const res = await fetch('/auth/register', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ inviteToken, username, password, displayName: displayName || undefined }),
  });

  const text = await res.text().catch(() => '');
  let body: { data?: LoginResponse; error?: string; message?: string } & LoginResponse = {};
  try {
    body = JSON.parse(text);
  } catch {
    // ignore
  }

  if (!res.ok) {
    throw new Error(body.error ?? body.message ?? `注册失败（HTTP ${res.status}）`);
  }

  const data = body.data ?? body;
  const token = data.access_token ?? data.accessToken ?? data.token ?? '';
  if (!token) throw new Error('注册响应缺少 token');

  return enrichWithUserInfo({
    token,
    username: data.user?.display_name ?? data.user?.name ?? data.user?.username ?? username,
    role: extractRole(data),
  });
}

/** 退出：通知后端（需带 Bearer，失败也继续清本地态） */
export async function logout(): Promise<void> {
  try {
    await fetch('/auth/logout', { method: 'POST', headers: getAuthHeader() });
  } catch {
    // 后端不可达也允许本地退出
  }
  clearAuth();
}
