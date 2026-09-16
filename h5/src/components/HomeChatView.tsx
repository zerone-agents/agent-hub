import React, { useState, useRef, useEffect } from 'react';
import {
  Menu,
  ChevronDown,
  X,
  RotateCw,
  Send,
  Sparkles,
  Paperclip,
  Check,
  Copy,
  FolderKanban,
  Clock,
  ExternalLink,
  Bot
} from 'lucide-react';
import { Agent, ChatMessage, ScenarioPrompt } from '../types';
import { ZeroneLogo } from './ZeroneLogo';

interface HomeChatViewProps {
  scenarioPrompts: ScenarioPrompt[];
  messages: ChatMessage[];
  activeAgent: Agent | null;
  allAgents: Agent[];
  /** 未登录（接口 401）时显示登录引导屏，不展示任何兜底假数据 */
  needLogin?: boolean;
  isLoading: boolean;
  onSendMessage: (text: string) => void;
  onSelectAgent: (agent: Agent) => void;
  onSaveMessageToKnowledge: (content: string, title?: string) => void;
  onNavigateToTab: (tab: 'chat' | 'agents' | 'knowledge' | 'profile') => void;
}

export const HomeChatView: React.FC<HomeChatViewProps> = ({
  scenarioPrompts,
  messages,
  activeAgent,
  allAgents,
  needLogin = false,
  isLoading,
  onSendMessage,
  onSelectAgent,
  onSaveMessageToKnowledge,
  onNavigateToTab,
}) => {
  const [inputText, setInputText] = useState('');
  const [scenarioOffset, setScenarioOffset] = useState(0);
  const [showAgentPicker, setShowAgentPicker] = useState(false);
  const [copiedMessageId, setCopiedMessageId] = useState<string | null>(null);
  const [showSideDrawer, setShowSideDrawer] = useState(false);

  const messagesEndRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  // Rotate through 4 scenarios at a time
  const visibleScenarios = scenarioPrompts.slice(scenarioOffset, scenarioOffset + 4);
  const handleShuffleScenarios = () => {
    setScenarioOffset((prev) => (prev + 4 >= scenarioPrompts.length ? 0 : prev + 4));
  };

  // 切换 Agent 后场景列表变化，翻页偏移归零防止越界
  useEffect(() => {
    setScenarioOffset(0);
  }, [scenarioPrompts]);

  useEffect(() => {
    if (messages.length > 0) {
      messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
    }
  }, [messages, isLoading]);

  const handleSend = () => {
    if (!inputText.trim() || isLoading) return;
    onSendMessage(inputText.trim());
    setInputText('');
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSend();
    }
  };

  const handleCopyMessage = (id: string, text: string) => {
    navigator.clipboard?.writeText(text);
    setCopiedMessageId(id);
    setTimeout(() => setCopiedMessageId(null), 2000);
  };

  const getExpertShortName = (agent: Agent | null) => {
    if (!agent) return 'Zerone 智能助手';
    if (agent.name.includes('·')) {
      const afterDot = agent.name.split('·')[1];
      return afterDot.split('(')[0].trim();
    }
    return agent.name.split('(')[0].trim();
  };

  // 未登录：接口 401，显示登录引导屏（不展示兜底假数据）
  if (needLogin) {
    return (
      <div className="flex-1 flex flex-col h-full bg-[#F8F9FA] items-center justify-center px-8">
        <div className="w-16 h-16 rounded-2xl bg-neutral-900 flex items-center justify-center mb-4 shadow-md">
          <Bot size={30} className="text-emerald-400" />
        </div>
        <h2 className="text-base font-bold text-gray-900 mb-1.5">登录后体验 Zerone 智能体</h2>
        <p className="text-xs text-gray-400 text-center leading-relaxed mb-6">
          登录或注册账号，即可与你可用的 Agent 开始对话
        </p>
        <button
          onClick={() => onNavigateToTab('profile')}
          className="px-8 py-2.5 rounded-full bg-neutral-900 text-white text-sm font-semibold shadow-sm cursor-pointer transition-all active:scale-95"
        >
          去登录 / 注册
        </button>
      </div>
    );
  }

  return (
    <div className="flex-1 flex flex-col h-full bg-[#F8F9FA] overflow-hidden relative">
      {/* Top Header: Displays current active expert in center with instant switcher dropdown */}
      <div className="h-12 bg-white px-4 flex items-center justify-between border-b border-gray-100 shrink-0 z-10">
        <button
          onClick={() => setShowSideDrawer(true)}
          className="w-8 h-8 -ml-1 rounded-full hover:bg-gray-100 flex items-center justify-center text-gray-700 cursor-pointer"
          title="打开侧边导航"
        >
          <Menu className="w-5 h-5 stroke-[2]" />
        </button>

        {/* Center: Current Expert Display & Switcher (User requirement: 显示当前专家，支持切换) */}
        <button
          onClick={() => setShowAgentPicker(true)}
          className="flex items-center gap-1.5 px-3 py-1.5 rounded-full bg-gray-100 hover:bg-emerald-50 hover:text-emerald-800 hover:border-emerald-200 border border-transparent text-gray-800 transition-all cursor-pointer shadow-2xs active:scale-95 group max-w-[210px]"
          title="点击切换当前对话专家"
        >
          <span className="text-sm shrink-0 leading-none">
            {activeAgent ? activeAgent.avatar : '🤖'}
          </span>
          <span className="text-xs font-bold text-gray-900 group-hover:text-emerald-800 truncate">
            {getExpertShortName(activeAgent)}
          </span>
          <ChevronDown className="w-3.5 h-3.5 text-gray-400 group-hover:text-emerald-600 shrink-0 transition-transform group-hover:translate-y-0.5" />
        </button>

        {/* Right side placeholder to keep center perfectly balanced */}
        <div className="w-8 h-8 flex items-center justify-center" />
      </div>

      {/* Main Chat / Homepage Area (Image 3 banner removed) */}
      <div className="flex-1 overflow-y-auto px-4 py-3 space-y-4">

        {/* If no conversation yet, show the iconic Welcome Mascot & Quick Scenario Cards */}
        {messages.length === 0 ? (
          <div className="flex flex-col items-center pt-2 pb-6 space-y-5">
            {/* Zerone 品牌区 */}
            <div className="flex flex-col items-center">
              <div className="relative group hover:scale-105 transition-transform">
                <ZeroneLogo size={96} radiusRatio={0.3} className="shadow-md" />
                <div className="absolute -bottom-2 left-1/2 -translate-x-1/2 bg-neutral-900 text-white rounded-md px-2 py-0.5 text-[10px] flex items-center gap-1 shadow-md">
                  <Sparkles className="w-2.5 h-2.5 text-emerald-400" />
                  <span>Zerone</span>
                </div>
              </div>
              <h2 className="text-xl font-black text-gray-900 tracking-tight mt-4">
                Zerone，我帮你
              </h2>
              <p className="text-xs text-gray-400 mt-0.5">
                你的智能体工作台
              </p>
            </div>

            {/* Scenario Prompt Cards —— 当前 Agent 在后台配置的场景（/api/v1/scenes），
                没配置就不显示，不用写死的假场景 */}
            {visibleScenarios.length > 0 && (
              <div className="w-full space-y-2 max-w-md">
                {visibleScenarios.map((sc) => (
                  <button
                    key={sc.id}
                    onClick={() => onSendMessage(sc.prompt)}
                    className="w-full text-left bg-white hover:bg-emerald-50/40 hover:border-emerald-300 border border-gray-100 rounded-2xl p-3.5 shadow-2xs flex items-center justify-between group transition-all duration-150 cursor-pointer active:scale-[0.99]"
                  >
                    <div className="flex items-center gap-3">
                      <span className="text-base">{sc.icon}</span>
                      <span className="text-xs font-semibold text-gray-800 group-hover:text-emerald-900 transition-colors">
                        {sc.title}
                      </span>
                    </div>
                    <span className="text-gray-300 group-hover:text-emerald-600 transition-colors text-xs font-mono">
                      →
                    </span>
                  </button>
                ))}

                {/* Shuffle button "换一换"：场景超过 4 个才有意义 */}
                {scenarioPrompts.length > 4 && (
                  <div className="flex justify-center pt-1">
                    <button
                      onClick={handleShuffleScenarios}
                      className="flex items-center gap-1.5 text-xs text-gray-500 hover:text-gray-800 px-3 py-1.5 rounded-full hover:bg-gray-100 transition-colors cursor-pointer"
                    >
                      <RotateCw className="w-3.5 h-3.5" />
                      <span>换一换</span>
                    </button>
                  </div>
                )}
              </div>
            )}
          </div>
        ) : (
          /* Active Chat Flow */
          <div className="space-y-4 pb-4">
            {messages.map((msg) => (
              <div
                key={msg.id}
                className={`flex gap-2.5 ${msg.role === 'user' ? 'justify-end' : 'justify-start'}`}
              >
                {msg.role !== 'user' && (
                  <div className="w-8 h-8 rounded-full bg-gradient-to-tr from-emerald-100 to-teal-100 flex items-center justify-center text-sm shadow-xs shrink-0 mt-0.5 border border-emerald-200">
                    {msg.agentAvatar || '🤖'}
                  </div>
                )}

                <div
                  className={`max-w-[85%] rounded-2xl px-4 py-3 text-xs leading-relaxed shadow-xs ${
                    msg.role === 'user'
                      ? 'bg-neutral-900 text-white rounded-tr-xs'
                      : 'bg-white text-gray-800 border border-gray-100 rounded-tl-xs'
                  }`}
                >
                  {msg.agentName && (
                    <div className="text-[10px] font-bold text-emerald-600 mb-1 flex items-center gap-1">
                      <Sparkles className="w-2.5 h-2.5" />
                      <span>{msg.agentName}</span>
                    </div>
                  )}

                  {/* Render content */}
                  <div className="whitespace-pre-wrap font-sans break-words space-y-1">
                    {msg.content}
                  </div>

                  {/* Assistant response toolbar */}
                  {msg.role !== 'user' && (
                    <div className="flex items-center justify-end gap-2 mt-2 pt-2 border-t border-gray-50 text-[10px] text-gray-400">
                      <button
                        onClick={() => handleCopyMessage(msg.id, msg.content)}
                        className="hover:text-gray-700 flex items-center gap-1 cursor-pointer"
                      >
                        {copiedMessageId === msg.id ? (
                          <>
                            <Check className="w-3 h-3 text-emerald-600" />
                            <span className="text-emerald-600">已复制</span>
                          </>
                        ) : (
                          <>
                            <Copy className="w-3 h-3" />
                            <span>复制</span>
                          </>
                        )}
                      </button>

                      <button
                        onClick={() => onSaveMessageToKnowledge(msg.content, `Zerone输出_${new Date().toLocaleTimeString('zh-CN')}`)}
                        className="hover:text-emerald-700 flex items-center gap-1 cursor-pointer text-emerald-600 font-medium"
                        title="将此回复转存为知识库资料"
                      >
                        <FolderKanban className="w-3 h-3" />
                        <span>存至资料库</span>
                      </button>
                    </div>
                  )}
                </div>
              </div>
            ))}

            {/* Loading indicator —— 流式内容开始输出后隐藏，避免与流式气泡并存 */}
            {isLoading && !messages.some((m) => m.isStreaming) && (
              <div className="flex gap-2.5 justify-start">
                <div className="w-8 h-8 rounded-full bg-emerald-100 flex items-center justify-center text-sm shrink-0 mt-0.5">
                  🤖
                </div>
                <div className="bg-white rounded-2xl px-4 py-3 text-xs border border-gray-100 shadow-xs flex items-center gap-1.5 text-gray-500">
                  <div className="w-1.5 h-1.5 bg-emerald-500 rounded-full animate-bounce" />
                  <div className="w-1.5 h-1.5 bg-emerald-500 rounded-full animate-bounce [animation-delay:0.2s]" />
                  <div className="w-1.5 h-1.5 bg-emerald-500 rounded-full animate-bounce [animation-delay:0.4s]" />
                  <span className="ml-1 text-[11px]">Zerone 正在协同思考中...</span>
                </div>
              </div>
            )}

            <div ref={messagesEndRef} />
          </div>
        )}
      </div>

      {/* Bottom Input Area matching Screenshot 1 */}
      <div className="bg-white border-t border-gray-100 p-2.5 shrink-0 z-20">
        <div className="flex items-center gap-2">
          {/* Center Input */}
          <div className="flex-1 flex items-center bg-gray-100 focus-within:bg-white focus-within:ring-2 focus-within:ring-neutral-900/10 rounded-full px-3.5 py-1.5 border border-transparent focus-within:border-gray-300 transition-all">
            <input
              ref={inputRef}
              type="text"
              value={inputText}
              onChange={(e) => setInputText(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder="发消息或输入业务需求..."
              disabled={isLoading}
              className="w-full bg-transparent text-xs text-gray-900 placeholder-gray-400 focus:outline-none"
            />
          </div>

          {/* Send button */}
          <button
            onClick={handleSend}
            disabled={!inputText.trim() || isLoading}
            className="w-9 h-9 rounded-full bg-neutral-900 hover:bg-neutral-800 disabled:opacity-30 disabled:hover:bg-neutral-900 text-white flex items-center justify-center cursor-pointer shadow-sm transition-all active:scale-95 shrink-0"
            title="发送"
          >
            <Send className="w-4 h-4" />
          </button>
        </div>
      </div>

      {/* Quick Agent Switcher Modal */}
      {showAgentPicker && (
        <div
          onClick={() => setShowAgentPicker(false)}
          className="absolute inset-0 z-40 bg-black/40 backdrop-blur-xs flex items-end animate-in fade-in duration-150"
        >
          <div
            onClick={(e) => e.stopPropagation()}
            className="w-full bg-white rounded-t-3xl max-h-[75%] flex flex-col overflow-hidden shadow-2xl p-4 animate-in slide-in-from-bottom duration-200"
          >
            <div className="flex items-center justify-between pb-3 border-b border-gray-100">
              <div>
                <h3 className="text-xs font-bold text-gray-900">切换智能体</h3>
                <p className="text-[10px] text-gray-400">选择一个智能体开始对话</p>
              </div>
              <button
                onClick={() => setShowAgentPicker(false)}
                className="w-7 h-7 rounded-full bg-gray-100 flex items-center justify-center text-gray-500 cursor-pointer"
              >
                <X className="w-3.5 h-3.5" />
              </button>
            </div>

            <div className="flex-1 overflow-y-auto divide-y divide-gray-50 py-2">
              {allAgents.map((agent) => {
                const isCurrent = activeAgent?.id === agent.id;
                return (
                  <div
                    key={agent.id}
                    onClick={() => {
                      onSelectAgent(agent);
                      setShowAgentPicker(false);
                    }}
                    className={`py-2.5 px-2 flex items-center justify-between hover:bg-gray-50 rounded-xl cursor-pointer ${
                      isCurrent ? 'bg-emerald-50/60' : ''
                    }`}
                  >
                    <div className="flex items-center gap-2.5 min-w-0">
                      <div className="w-8 h-8 rounded-full bg-gray-100 flex items-center justify-center text-base shrink-0">
                        {agent.avatar}
                      </div>
                      <div className="min-w-0">
                        <div className="text-xs font-semibold text-gray-900 truncate">{agent.name}</div>
                        <div className="text-[10px] text-gray-400 truncate">{agent.title}</div>
                      </div>
                    </div>
                    {isCurrent ? (
                      <span className="text-[10px] font-semibold text-emerald-700 bg-emerald-100 px-2 py-0.5 rounded-full">当前使用</span>
                    ) : (
                      <span className="text-[11px] font-medium text-emerald-600">切换 →</span>
                    )}
                  </div>
                );
              })}
            </div>
          </div>
        </div>
      )}

      {/* Side Menu Drawer (Zerone Workspace Directory) */}
      {showSideDrawer && (
        <div className="absolute inset-0 z-50 bg-black/40 backdrop-blur-xs flex">
          <div className="w-4/5 max-w-xs bg-white h-full shadow-2xl flex flex-col animate-in slide-in-from-left duration-200">
            <div className="p-4 border-b border-gray-100 flex items-center justify-between">
              <div className="flex items-center gap-2">
                <ZeroneLogo size={32} radiusRatio={0.25} />
                <div>
                  <div className="text-xs font-bold text-gray-900">Zerone 工作空间</div>
                  <div className="text-[10px] text-gray-400">Zerone Agent 协同空间</div>
                </div>
              </div>
              <button
                onClick={() => setShowSideDrawer(false)}
                className="w-7 h-7 rounded-full hover:bg-gray-100 flex items-center justify-center text-gray-400"
              >
                <X className="w-4 h-4" />
              </button>
            </div>

            <div className="flex-1 overflow-y-auto p-4 space-y-4 text-xs">
              <div>
                <div className="text-[10px] font-bold text-gray-400 uppercase tracking-wider mb-2">
                  快速导航
                </div>
                <div className="space-y-1">
                  <button
                    onClick={() => {
                      setShowSideDrawer(false);
                      onNavigateToTab('chat');
                    }}
                    className="w-full px-3 py-2 rounded-xl text-left bg-emerald-50 text-emerald-800 font-semibold flex items-center gap-2 cursor-pointer"
                  >
                    <span>💬</span>
                    <span>任务对话</span>
                  </button>
                  <button
                    onClick={() => {
                      setShowSideDrawer(false);
                      onNavigateToTab('agents');
                    }}
                    className="w-full px-3 py-2 rounded-xl text-left hover:bg-gray-50 text-gray-700 font-medium flex items-center gap-2 cursor-pointer"
                  >
                    <span>🤖</span>
                    <span>专家工坊</span>
                  </button>
                  <button
                    onClick={() => {
                      setShowSideDrawer(false);
                      onNavigateToTab('knowledge');
                    }}
                    className="w-full px-3 py-2 rounded-xl text-left hover:bg-gray-50 text-gray-700 font-medium flex items-center gap-2 cursor-pointer"
                  >
                    <span>📚</span>
                    <span>资料库</span>
                  </button>
                  <button
                    onClick={() => {
                      setShowSideDrawer(false);
                      onNavigateToTab('profile');
                    }}
                    className="w-full px-3 py-2 rounded-xl text-left hover:bg-gray-50 text-gray-700 font-medium flex items-center gap-2 cursor-pointer"
                  >
                    <span>👤</span>
                    <span>个人中心</span>
                  </button>
                </div>
              </div>

              <div>
                <div className="text-[10px] font-bold text-gray-400 uppercase tracking-wider mb-2">
                  快捷协同场景
                </div>
                <div className="space-y-1.5">
                  {scenarioPrompts.slice(0, 5).map((s) => (
                    <button
                      key={s.id}
                      onClick={() => {
                        setShowSideDrawer(false);
                        onSendMessage(s.prompt);
                      }}
                      className="w-full text-left p-2 rounded-lg bg-gray-50 hover:bg-gray-100 text-gray-700 text-[11px] truncate cursor-pointer"
                    >
                      {s.icon} {s.title}
                    </button>
                  ))}
                </div>
              </div>
            </div>

            <div className="p-3 border-t border-gray-100 bg-gray-50 text-[10px] text-gray-400 text-center">
              Zerone 协同移动端 · 演示版本 v2.6
            </div>
          </div>

          <div className="flex-1" onClick={() => setShowSideDrawer(false)} />
        </div>
      )}
    </div>
  );
};
