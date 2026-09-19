import React, { useState } from 'react';
import { ChevronLeft, Share2, Sparkles, MessageSquarePlus, Check, ArrowUpRight } from 'lucide-react';
import { Agent } from '../types';

interface AgentDetailModalProps {
  agent: Agent | null;
  isOpen: boolean;
  onClose: () => void;
  onSummon: (agent: Agent, promptText?: string) => void;
}

export const AgentDetailModal: React.FC<AgentDetailModalProps> = ({
  agent,
  isOpen,
  onClose,
  onSummon,
}) => {
  const [copied, setCopied] = useState(false);

  if (!isOpen || !agent) return null;

  const handleShare = () => {
    setCopied(true);
    navigator.clipboard?.writeText(
      `【Zerone 专家推荐】${agent.name}（${agent.title}）- 专业智能协同顾问，已提供 ${agent.usageCount}`
    );
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div className="absolute inset-0 z-50 bg-white flex flex-col overflow-hidden animate-in slide-in-from-right duration-200">
      {/* Top Navigation Bar */}
      <div className="h-12 px-4 border-b border-gray-100 flex items-center justify-between shrink-0 bg-white/95 backdrop-blur-sm">
        <button
          onClick={onClose}
          className="w-8 h-8 -ml-1 rounded-full flex items-center justify-center text-gray-700 hover:bg-gray-100 cursor-pointer active:scale-95"
        >
          <ChevronLeft className="w-5 h-5" />
        </button>
        <span className="text-sm font-semibold text-gray-900">专家详情</span>
        <button
          onClick={handleShare}
          className="w-8 h-8 rounded-full flex items-center justify-center text-gray-600 hover:bg-gray-100 cursor-pointer"
          title="分享此专家"
        >
          {copied ? <Check className="w-4 h-4 text-emerald-600" /> : <Share2 className="w-4 h-4" />}
        </button>
      </div>

      {/* Scrollable Body */}
      <div className="flex-1 overflow-y-auto px-5 py-6 space-y-6">
        {/* Agent Header Profile */}
        <div className="flex flex-col items-center text-center">
          <div className="w-24 h-24 rounded-full bg-gradient-to-tr from-sky-100 via-indigo-50 to-amber-100 border-4 border-white shadow-lg flex items-center justify-center text-4xl mb-3.5 relative">
            <span>{agent.avatar}</span>
            <div className="absolute -bottom-1 bg-emerald-500 text-white text-[10px] font-bold px-2 py-0.5 rounded-full shadow-xs">
              官方认证
            </div>
          </div>
          <h2 className="text-lg font-bold text-gray-900 tracking-tight">{agent.name}</h2>
          <div className="text-xs text-gray-400 mt-1 font-medium">{agent.usageCount}</div>
        </div>

        {/* Section: 能力介绍 */}
        <div className="space-y-2">
          <div className="flex items-center gap-1.5 text-xs font-bold text-gray-800">
            <span className="text-amber-500">⚡</span>
            <span>能力介绍</span>
          </div>
          <div className="text-xs text-gray-600 leading-relaxed bg-gray-50/80 p-3.5 rounded-xl border border-gray-100">
            {agent.description}
          </div>
        </div>

        {/* Section: 擅长领域 */}
        <div className="space-y-2">
          <div className="flex items-center gap-1.5 text-xs font-bold text-gray-800">
            <span className="text-amber-500">⚡</span>
            <span>擅长领域</span>
          </div>
          <div className="flex flex-wrap gap-2">
            {agent.tags.map((tag) => (
              <span
                key={tag}
                className="px-3 py-1 bg-gray-100 hover:bg-gray-200 text-gray-700 text-xs font-medium rounded-lg transition-colors"
              >
                {tag}
              </span>
            ))}
          </div>
        </div>

        {/* Section: 试试这样问我 */}
        <div className="space-y-2.5 pb-6">
          <div className="flex items-center gap-1.5 text-xs font-bold text-gray-800">
            <span className="text-amber-500">💡</span>
            <span>试试这样问我</span>
          </div>
          <div className="space-y-2.5">
            {agent.suggestedQuestions.map((q, idx) => (
              <button
                key={idx}
                onClick={() => onSummon(agent, q)}
                className="w-full text-left p-3.5 bg-gray-50 hover:bg-emerald-50/50 hover:border-emerald-200 border border-gray-100 rounded-xl transition-all cursor-pointer group flex items-start justify-between gap-3 active:scale-[0.99]"
              >
                <span className="text-xs text-gray-700 group-hover:text-emerald-900 leading-relaxed">
                  “{q}”
                </span>
                <div className="shrink-0 w-6 h-6 rounded-full bg-white group-hover:bg-emerald-500 text-gray-400 group-hover:text-white flex items-center justify-center shadow-xs transition-colors mt-0.5">
                  <ArrowUpRight className="w-3.5 h-3.5" />
                </div>
              </button>
            ))}
          </div>
        </div>
      </div>

      {/* Fixed Bottom Action Bar (matching Screenshot 4) */}
      <div className="p-3 bg-white border-t border-gray-100 flex items-center gap-3 shrink-0 shadow-[0_-2px_10px_rgba(0,0,0,0.03)]">
        <button
          onClick={handleShare}
          className="flex-1 h-11 rounded-xl bg-gray-100 hover:bg-gray-200 text-gray-700 text-xs font-medium flex items-center justify-center gap-1.5 transition-colors cursor-pointer"
        >
          <Share2 className="w-4 h-4" />
          <span>{copied ? '已复制链接' : '分享'}</span>
        </button>
        <button
          onClick={() => onSummon(agent)}
          className="flex-2 h-11 rounded-xl bg-neutral-900 hover:bg-neutral-800 text-white text-xs font-semibold flex items-center justify-center gap-2 shadow-sm transition-all active:scale-[0.98] cursor-pointer"
        >
          <Sparkles className="w-4 h-4 text-emerald-400" />
          <span>召唤专家</span>
        </button>
      </div>
    </div>
  );
};
