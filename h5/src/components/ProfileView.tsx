import React, { useEffect, useState } from 'react';
import { LogOut, User, Lock, Check, Ticket, AtSign, Loader2, KeyRound, ChevronDown } from 'lucide-react';
import { ZeroneLogo } from './ZeroneLogo';
import {
  AuthState,
  AuthMode,
  AUTH_CHANGED_EVENT,
  fetchAuthMode,
  getStoredAuth,
  login,
  loginWithToken,
  logout,
  precheckInvite,
  register,
  storeAuth,
} from '../api/auth';

/**
 * 个人中心 —— 登录 / 注册 / 退出。
 * 认证模式由后端 /auth/mode 决定：
 * - builtin（本地/mock）：账号密码登录 + 邀请码注册（对齐 zerone 注册流）。
 * - casdoor（线上 console.zerone.life）：SSO OAuth 跳转登录（无密码/注册接口），
 *   回调经 /auth/login?redirect=/h5/ 落到 H5（部署在 console /static/h5/ 时生效），
 *   另提供「粘贴 token」开发者入口（本地调试用）。
 * 已登录：用户卡片（含角色标识） + 退出登录。
 * 登录/注册成功后通过 onAuthSuccess 通知外层（体验用户 guest 自动跳转聊天页）。
 */

/** OAuth 回调落地路径：H5 部署在 console.zerone.life/static/h5/ 时，callback 落 /static/h5/?token=... */
const OAUTH_REDIRECT_PATH = '/h5/';

const ROLE_LABELS: Record<string, string> = {
  guest: '体验用户',
  member: '成员',
  maintainer: '维护者',
  admin: '管理员',
};

interface ProfileViewProps {
  onAuthSuccess?: (auth: AuthState) => void;
  onLogout?: () => void;
}

export const ProfileView: React.FC<ProfileViewProps> = ({ onAuthSuccess, onLogout }) => {
  const [auth, setAuth] = useState<AuthState | null>(() => getStoredAuth());
  const [mode, setMode] = useState<'login' | 'register'>('login');
  /** 后端认证模式（null=探测中） */
  const [authMode, setAuthMode] = useState<AuthMode | null>(null);
  // casdoor 模式：开发者 token 登录
  const [showDevLogin, setShowDevLogin] = useState(false);
  const [devToken, setDevToken] = useState('');

  // Login fields
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');

  // Register fields
  const [inviteToken, setInviteToken] = useState('');
  const [regUsername, setRegUsername] = useState('');
  const [displayName, setDisplayName] = useState('');
  const [regPassword, setRegPassword] = useState('');
  const [inviteNote, setInviteNote] = useState<string | null>(null);
  const [inviteChecking, setInviteChecking] = useState(false);

  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [toast, setToast] = useState<string | null>(null);

  const showToast = (msg: string) => {
    setToast(msg);
    setTimeout(() => setToast(null), 2000);
  };

  // 邀请码预检（非空才查；无效就地报错，与 zerone 注册页一致）
  const checkInvite = (token: string) => {
    const t = token.trim();
    if (!t) {
      setInviteNote(null);
      return;
    }
    setInviteChecking(true);
    precheckInvite(t)
      .then((res) => {
        setInviteNote(res.note || '邀请码有效');
        setError(null);
      })
      .catch((err) => {
        setInviteNote(null);
        setError(err instanceof Error ? err.message : '邀请链接无效或已失效');
      })
      .finally(() => setInviteChecking(false));
  };

  // 探测认证模式；订阅外部登录态变更（OAuth 回调落地在 App 层完成）
  useEffect(() => {
    fetchAuthMode().then(setAuthMode);
    const sync = () => setAuth(getStoredAuth());
    window.addEventListener(AUTH_CHANGED_EVENT, sync);
    return () => window.removeEventListener(AUTH_CHANGED_EVENT, sync);
  }, []);

  // 支持邀请链接直达：?token=xxx 自动进注册模式并预检（仅 builtin 模式；
  // casdoor 模式下 URL 的 token 参数是 OAuth access_token，由 App 层消费，不是邀请码）
  useEffect(() => {
    if (authMode !== 'builtin') return;
    const token = new URLSearchParams(window.location.search).get('token');
    if (token) {
      setMode('register');
      setInviteToken(token);
      checkInvite(token);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [authMode]);

  const handleLogin = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!username.trim() || !password || submitting) return;
    setSubmitting(true);
    setError(null);
    try {
      const state = await login(username.trim(), password);
      storeAuth(state);
      setAuth(state);
      setPassword('');
      onAuthSuccess?.(state);
    } catch (err) {
      setError(err instanceof Error ? err.message : '登录失败，请重试');
    } finally {
      setSubmitting(false);
    }
  };

  const handleRegister = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!inviteToken.trim() || !regUsername.trim() || !regPassword || submitting) return;
    setSubmitting(true);
    setError(null);
    try {
      const state = await register(
        inviteToken.trim(),
        regUsername.trim(),
        regPassword,
        displayName.trim() || undefined
      );
      storeAuth(state);
      setAuth(state);
      showToast('注册成功，已自动登录');
      onAuthSuccess?.(state);
    } catch (err) {
      setError(err instanceof Error ? err.message : '注册失败，请重试');
    } finally {
      setSubmitting(false);
    }
  };

  // casdoor 模式：粘贴 access_token 开发者登录（本地调试；线上请用 SSO 按钮）
  const handleTokenLogin = async () => {
    if (!devToken.trim() || submitting) return;
    setSubmitting(true);
    setError(null);
    try {
      const state = await loginWithToken(devToken.trim());
      storeAuth(state);
      setAuth(state);
      setDevToken('');
      onAuthSuccess?.(state);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'token 登录失败');
    } finally {
      setSubmitting(false);
    }
  };

  const handleLogout = async () => {
    await logout();
    setAuth(null);
    setUsername('');
    // 会话 id 缓存是按账号的（后端 GetSessionForUser 校验 user_id），
    // 退出必须清掉，否则换账号登录后沿用旧会话会 404。
    try {
      localStorage.removeItem('workbuddy_chat_sessions');
    } catch {
      // ignore
    }
    onLogout?.();
    showToast('已退出登录');
  };

  const inputCls =
    'w-full pl-10 pr-3 py-2.5 bg-white border border-gray-200 rounded-xl text-xs text-gray-900 placeholder:text-gray-400 focus:outline-none focus:ring-1 focus:ring-emerald-500 focus:border-emerald-500 transition-all';

  return (
    <div className="flex-1 flex flex-col h-full bg-[#F8F9FA] overflow-y-auto">
      {/* Header（与其他页面一致的居中标题白栏） */}
      <div className="bg-white px-4 pt-3 pb-3 border-b border-gray-100 shrink-0">
        <h1 className="text-base font-bold text-gray-900 text-center">我的</h1>
      </div>

      {auth ? (
        /* ── 已登录 ─────────────────────────────── */
        <div className="p-4 space-y-3">
          <div className="flex items-center gap-3.5 bg-white p-4 rounded-2xl border border-gray-100 shadow-xs">
            <div className="w-14 h-14 rounded-full bg-gradient-to-tr from-emerald-400 to-teal-600 text-white flex items-center justify-center font-bold text-xl shadow-sm shrink-0">
              {auth.username.slice(0, 1).toUpperCase()}
            </div>
            <div className="flex-1 min-w-0">
              <h2 className="text-sm font-bold text-gray-900 truncate">{auth.username}</h2>
              <p className="text-[11px] text-gray-400 mt-0.5 flex items-center gap-1.5">
                <span>Zerone 用户</span>
                {auth.role && (
                  <span
                    className={`px-1.5 py-px rounded-full text-[9px] font-semibold border ${
                      auth.role === 'guest'
                        ? 'bg-amber-50 text-amber-600 border-amber-200'
                        : 'bg-emerald-50 text-emerald-600 border-emerald-200'
                    }`}
                  >
                    {ROLE_LABELS[auth.role] ?? auth.role}
                  </span>
                )}
              </p>
            </div>
          </div>

          <button
            onClick={() => { void handleLogout(); }}
            className="w-full bg-white rounded-2xl border border-gray-100 shadow-xs px-4 py-3.5 flex items-center justify-center gap-2 text-xs font-semibold text-red-500 hover:bg-red-50/60 active:scale-[0.99] transition-all cursor-pointer"
          >
            <LogOut className="w-4 h-4" />
            退出登录
          </button>
        </div>
      ) : authMode === null ? (
        /* ── 认证模式探测中 ───────────────────────── */
        <div className="flex-1 flex items-center justify-center">
          <Loader2 className="w-6 h-6 text-gray-300 animate-spin" />
        </div>
      ) : authMode === 'casdoor' ? (
        /* ── 未登录 · casdoor SSO（线上模式：无密码/注册接口） ── */
        <div className="flex-1 flex flex-col items-center px-6 pt-12">
          <ZeroneLogo size={72} className="shadow-md" />
          <h2 className="text-lg font-black text-gray-900 tracking-tight mt-4">登录 Zerone</h2>
          <p className="text-[11px] text-gray-400 mt-1 text-center leading-relaxed">
            使用 Zerone 统一账号（SSO）登录
            <br />
            新用户由管理员在组织中开通
          </p>

          <button
            onClick={() => {
              window.location.href = `/auth/login?redirect=${encodeURIComponent(OAUTH_REDIRECT_PATH)}`;
            }}
            className="w-full mt-6 py-2.5 rounded-xl bg-neutral-900 hover:bg-neutral-800 text-white text-xs font-semibold shadow-xs cursor-pointer transition-all active:scale-[0.99] flex items-center justify-center gap-2"
          >
            <KeyRound className="w-4 h-4" />
            使用 Zerone 账号登录
          </button>

          {error && (
            <div className="w-full mt-3 text-[11px] text-red-500 bg-red-50 border border-red-100 rounded-xl px-3 py-2">
              {error}
            </div>
          )}

          {/* 开发者入口：粘贴 token（本地调试时 OAuth 回调无法落到 localhost） */}
          <div className="w-full mt-6">
            <button
              onClick={() => setShowDevLogin((v) => !v)}
              className="w-full flex items-center justify-center gap-1 text-[11px] text-gray-400 hover:text-gray-600 cursor-pointer transition-colors"
            >
              <ChevronDown
                className={`w-3.5 h-3.5 transition-transform ${showDevLogin ? 'rotate-180' : ''}`}
              />
              开发者选项：粘贴 token 登录
            </button>
            {showDevLogin && (
              <div className="mt-3 space-y-2.5">
                <textarea
                  value={devToken}
                  onChange={(e) => setDevToken(e.target.value)}
                  placeholder="从 console.zerone.life 登录后，复制浏览器 localStorage 里的 access_token 粘贴到这里"
                  rows={3}
                  className="w-full px-3 py-2.5 bg-white border border-gray-200 rounded-xl text-[11px] text-gray-900 placeholder:text-gray-300 focus:outline-none focus:ring-1 focus:ring-emerald-500 focus:border-emerald-500 transition-all resize-none font-mono"
                />
                <button
                  onClick={() => { void handleTokenLogin(); }}
                  disabled={!devToken.trim() || submitting}
                  className="w-full py-2.5 rounded-xl bg-emerald-600 hover:bg-emerald-700 disabled:opacity-40 text-white text-xs font-semibold shadow-xs cursor-pointer transition-all active:scale-[0.99]"
                >
                  {submitting ? '验证中…' : '使用 token 登录'}
                </button>
              </div>
            )}
          </div>
        </div>
      ) : (
        /* ── 未登录 · builtin：登录 / 注册 ─────────────────────── */
        <div className="flex-1 flex flex-col items-center px-6 pt-12">
          <ZeroneLogo size={72} className="shadow-md" />
          <h2 className="text-lg font-black text-gray-900 tracking-tight mt-4">
            {mode === 'login' ? '登录 Zerone' : '加入 Zerone'}
          </h2>
          <p className="text-[11px] text-gray-400 mt-1">
            {mode === 'login' ? '使用账号密码登录，同步你的会话与知识库' : '填写信息完成注册'}
          </p>

          {/* 登录 / 注册 切换 */}
          <div className="flex mt-5 p-1 bg-gray-100 rounded-xl w-full">
            {(['login', 'register'] as const).map((m) => (
              <button
                key={m}
                onClick={() => {
                  setMode(m);
                  setError(null);
                }}
                className={`flex-1 py-1.5 rounded-lg text-xs font-semibold transition-all cursor-pointer ${
                  mode === m
                    ? 'bg-white text-gray-900 shadow-2xs'
                    : 'text-gray-400 hover:text-gray-600'
                }`}
              >
                {m === 'login' ? '登 录' : '注 册'}
              </button>
            ))}
          </div>

          {mode === 'login' ? (
            <form onSubmit={(e) => { void handleLogin(e); }} className="w-full mt-5 space-y-3">
              <div className="relative">
                <User className="w-4 h-4 text-gray-400 absolute left-3.5 top-1/2 -translate-y-1/2" />
                <input
                  type="text"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  placeholder="用户名"
                  autoComplete="username"
                  className={inputCls}
                />
              </div>
              <div className="relative">
                <Lock className="w-4 h-4 text-gray-400 absolute left-3.5 top-1/2 -translate-y-1/2" />
                <input
                  type="password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  placeholder="密码"
                  autoComplete="current-password"
                  className={inputCls}
                />
              </div>

              {error && (
                <div className="text-[11px] text-red-500 bg-red-50 border border-red-100 rounded-xl px-3 py-2">
                  {error}
                </div>
              )}

              <button
                type="submit"
                disabled={!username.trim() || !password || submitting}
                className="w-full py-2.5 rounded-xl bg-neutral-900 hover:bg-neutral-800 disabled:opacity-40 text-white text-xs font-semibold shadow-xs cursor-pointer transition-all active:scale-[0.99]"
              >
                {submitting ? '登录中…' : '登 录'}
              </button>
            </form>
          ) : (
            <form
              noValidate
              autoComplete="off"
              onSubmit={(e) => { void handleRegister(e); }}
              className="w-full mt-5 space-y-3"
            >
              <div className="relative">
                <Ticket className="w-4 h-4 text-gray-400 absolute left-3.5 top-1/2 -translate-y-1/2" />
                <input
                  type="text"
                  value={inviteToken}
                  onChange={(e) => {
                    setInviteToken(e.target.value);
                    setInviteNote(null);
                  }}
                  onBlur={() => checkInvite(inviteToken)}
                  placeholder="邀请码"
                  autoComplete="off"
                  className={inputCls}
                />
                {inviteChecking && (
                  <Loader2 className="w-3.5 h-3.5 text-gray-400 animate-spin absolute right-3.5 top-1/2 -translate-y-1/2" />
                )}
              </div>

              {inviteNote && (
                <div className="text-[11px] text-emerald-700 bg-emerald-50 border border-emerald-100 rounded-xl px-3 py-2 flex items-center gap-1.5">
                  <Check className="w-3.5 h-3.5 shrink-0" />
                  <span>邀请备注:{inviteNote}</span>
                </div>
              )}

              <div className="relative">
                <User className="w-4 h-4 text-gray-400 absolute left-3.5 top-1/2 -translate-y-1/2" />
                <input
                  type="text"
                  value={regUsername}
                  onChange={(e) => setRegUsername(e.target.value)}
                  placeholder="用户名（3-32 位字母数字下划线连字符）"
                  autoComplete="off"
                  className={inputCls}
                />
              </div>
              <div className="relative">
                <AtSign className="w-4 h-4 text-gray-400 absolute left-3.5 top-1/2 -translate-y-1/2" />
                <input
                  type="text"
                  value={displayName}
                  onChange={(e) => setDisplayName(e.target.value)}
                  placeholder="昵称（可选）"
                  autoComplete="off"
                  className={inputCls}
                />
              </div>
              <div className="relative">
                <Lock className="w-4 h-4 text-gray-400 absolute left-3.5 top-1/2 -translate-y-1/2" />
                <input
                  type="password"
                  value={regPassword}
                  onChange={(e) => setRegPassword(e.target.value)}
                  placeholder="密码（至少 8 位，含字母和数字）"
                  autoComplete="new-password"
                  className={inputCls}
                />
              </div>

              {error && (
                <div className="text-[11px] text-red-500 bg-red-50 border border-red-100 rounded-xl px-3 py-2">
                  {error}
                </div>
              )}

              <button
                type="submit"
                disabled={
                  !inviteToken.trim() || !regUsername.trim() || !regPassword || submitting
                }
                className="w-full py-2.5 rounded-xl bg-neutral-900 hover:bg-neutral-800 disabled:opacity-40 text-white text-xs font-semibold shadow-xs cursor-pointer transition-all active:scale-[0.99]"
              >
                {submitting ? '注册中…' : '注册并登录'}
              </button>
            </form>
          )}
        </div>
      )}

      {/* Toast */}
      {toast && (
        <div className="fixed bottom-16 left-1/2 -translate-x-1/2 z-50 bg-neutral-900/90 text-white text-xs px-4 py-2 rounded-full shadow-lg flex items-center gap-1.5 animate-in fade-in zoom-in-95">
          <Check className="w-3.5 h-3.5 text-emerald-400" />
          <span>{toast}</span>
        </div>
      )}
    </div>
  );
};
