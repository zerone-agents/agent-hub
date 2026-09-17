import React, { useState } from 'react';
import { ChevronDown, KeyRound, Loader2 } from 'lucide-react';
import { ZeroneLogo } from './ZeroneLogo';
import { AuthState, loginWithToken, storeAuth } from '../api/auth';

/**
 * 统一登录引导页 —— casdoor（线上）模式下，未登录用户在任何 Tab 看到的同一套页面。
 * 主按钮直跳 Casdoor SSO（不经过「我的」中转）；
 * 另保留折叠的「开发者 token 登录」入口（本地调试时 OAuth 回调回不到 localhost 用）。
 */
interface LoginGuideProps {
  /** 点 SSO 按钮：App 层 goLogin（跳 /auth/login → Casdoor 授权页） */
  onSsoLogin: () => void;
  /** 开发者 token 登录成功：App 层更新 authRole 等 */
  onAuthSuccess?: (auth: AuthState) => void;
}

export const LoginGuide: React.FC<LoginGuideProps> = ({ onSsoLogin, onAuthSuccess }) => {
  const [showDevLogin, setShowDevLogin] = useState(false);
  const [devToken, setDevToken] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleTokenLogin = async () => {
    if (!devToken.trim() || submitting) return;
    setSubmitting(true);
    setError(null);
    try {
      const state = await loginWithToken(devToken.trim());
      storeAuth(state);
      setDevToken('');
      onAuthSuccess?.(state);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'token 登录失败');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="flex-1 flex flex-col items-center px-6 pt-12 bg-[#F8F9FA] overflow-y-auto">
      <ZeroneLogo size={72} className="shadow-md" />
      <h2 className="text-lg font-black text-gray-900 tracking-tight mt-4">登录 Zerone</h2>
      <p className="text-[11px] text-gray-400 mt-1 text-center leading-relaxed">
        使用 Zerone 统一账号（SSO）登录
        <br />
        新用户由管理员在组织中开通
      </p>

      <button
        onClick={onSsoLogin}
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
              className="w-full py-2.5 rounded-xl bg-emerald-600 hover:bg-emerald-700 disabled:opacity-40 text-white text-xs font-semibold shadow-xs cursor-pointer transition-all active:scale-[0.99] flex items-center justify-center gap-1.5"
            >
              {submitting && <Loader2 className="w-3.5 h-3.5 animate-spin" />}
              {submitting ? '验证中…' : '使用 token 登录'}
            </button>
          </div>
        )}
      </div>
    </div>
  );
};
