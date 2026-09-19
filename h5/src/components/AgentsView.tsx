import React, { useState, useMemo } from 'react';
import { Search, X, Bot } from 'lucide-react';
import { Agent } from '../types';

interface AgentsViewProps {
  agents: Agent[];
  /** 未登录（接口 401）时显示登录引导，不展示兜底假数据 */
  needLogin?: boolean;
  onGoLogin?: () => void;
  onSelectAgent: (agent: Agent) => void;
}

export const AgentsView: React.FC<AgentsViewProps> = ({ agents, needLogin = false, onGoLogin, onSelectAgent }) => {
  const [searchQuery, setSearchQuery] = useState('');
  const [selectedCategory, setSelectedCategory] = useState('全部');

  // 分类 chips 从接口数据的分组（agent-hub group）动态生成，不再写死
  const categories = useMemo(() => {
    const groups = Array.from(new Set(agents.map((a) => a.category).filter(Boolean)));
    return ['全部', ...groups];
  }, [agents]);

  const filteredAgents = useMemo(() => {
    return agents.filter((agent) => {
      const matchesSearch =
        searchQuery === '' ||
        agent.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
        agent.title.toLowerCase().includes(searchQuery.toLowerCase()) ||
        agent.description.toLowerCase().includes(searchQuery.toLowerCase()) ||
        agent.tags.some((t) => t.toLowerCase().includes(searchQuery.toLowerCase()));

      const matchesCategory =
        selectedCategory === '全部' || agent.category === selectedCategory;

      return matchesSearch && matchesCategory;
    });
  }, [agents, searchQuery, selectedCategory]);

  // 未登录：接口 401，显示登录引导（不展示兜底假数据）
  if (needLogin) {
    return (
      <div className="flex-1 flex flex-col h-full bg-[#F8F9FA] items-center justify-center px-8">
        <div className="w-16 h-16 rounded-2xl bg-neutral-900 flex items-center justify-center mb-4 shadow-md">
          <Bot size={30} className="text-emerald-400" />
        </div>
        <h2 className="text-base font-bold text-gray-900 mb-1.5">登录后查看可用 Agent</h2>
        <p className="text-xs text-gray-400 text-center leading-relaxed mb-6">
          这里的 Agent 列表来自线上平台，登录后按你的身份展示
        </p>
        <button
          onClick={onGoLogin}
          className="px-8 py-2.5 rounded-full bg-neutral-900 text-white text-sm font-semibold shadow-sm cursor-pointer transition-all active:scale-95"
        >
          使用 Zerone 账号登录
        </button>
      </div>
    );
  }

  return (
    <div className="flex-1 flex flex-col h-full bg-[#F8F9FA] overflow-hidden">
      {/* Header */}
      <div className="bg-white px-4 pt-3 pb-2 border-b border-gray-100 shrink-0">
        <h1 className="text-base font-bold text-gray-900 text-center mb-2.5">
          {searchQuery ? '搜索专家' : 'Zerone 专家'}
        </h1>

        {/* Search Bar matching screenshot */}
        <div className="relative flex items-center">
          <Search className="w-4 h-4 absolute left-3 text-gray-400" />
          <input
            type="text"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            placeholder={searchQuery ? '' : '搜索专家、能力、擅长领域...'}
            className="w-full pl-9 pr-8 py-2 bg-gray-100 hover:bg-gray-100/80 focus:bg-white rounded-full text-xs text-gray-800 placeholder-gray-400 focus:outline-none focus:ring-1 focus:ring-neutral-400 transition-all"
          />
          {searchQuery && (
            <button
              onClick={() => setSearchQuery('')}
              className="absolute right-3 w-4 h-4 rounded-full bg-gray-300 hover:bg-gray-400 flex items-center justify-center text-white cursor-pointer"
            >
              <X className="w-2.5 h-2.5" />
            </button>
          )}
        </div>
      </div>

      {/* Main Content Area */}
      <div className="flex-1 overflow-y-auto px-3.5 py-3 space-y-3">
        {/* Category Horizontal Filter Chips */}
        <div className="flex items-center gap-1.5 overflow-x-auto pb-1 -mx-3.5 px-3.5 no-scrollbar select-none">
          {categories.map((cat) => (
            <button
              key={cat}
              onClick={() => setSelectedCategory(cat)}
              className={`px-3 py-1 rounded-full text-xs font-medium whitespace-nowrap transition-all cursor-pointer ${
                selectedCategory === cat
                  ? 'bg-neutral-900 text-white shadow-xs'
                  : 'bg-white hover:bg-gray-100 text-gray-600 border border-gray-200/80'
              }`}
            >
              {cat}
            </button>
          ))}
        </div>

        {/* Agent Cards Grid matching screenshot 5 and 3 */}
        {filteredAgents.length === 0 ? (
          <div className="py-12 text-center text-gray-400 text-xs">
            {agents.length === 0 && !searchQuery
              ? '暂无可用 Agent，请联系管理员开通'
              : `未找到与 “${searchQuery}” 匹配的专家，请尝试切换搜索词`}
          </div>
        ) : (
          <div className="grid grid-cols-2 gap-2.5 pb-6">
            {filteredAgents.map((agent) => (
              <div
                key={agent.id}
                onClick={() => onSelectAgent(agent)}
                className="bg-white rounded-2xl p-3 border border-gray-100 shadow-xs hover:shadow-md hover:border-emerald-200 transition-all flex flex-col justify-between cursor-pointer active:scale-[0.98] group"
              >
                <div>
                  {/* Top Avatar & Name */}
                  <div className="flex items-start gap-2.5 mb-2">
                    <div className="w-10 h-10 rounded-full bg-gradient-to-tr from-emerald-50 to-teal-100 flex items-center justify-center text-xl shrink-0 border border-emerald-100 shadow-2xs">
                      {agent.avatar}
                    </div>
                    <div className="min-w-0 flex-1">
                      <h3 className="text-xs font-bold text-gray-900 line-clamp-1 leading-snug group-hover:text-emerald-700 transition-colors">
                        {agent.name}
                      </h3>
                      <p className="text-[10px] text-gray-400 line-clamp-1 mt-0.5">
                        {agent.title}
                      </p>
                    </div>
                  </div>

                  {/* Description snippet */}
                  <p className="text-[11px] text-gray-500 line-clamp-2 leading-relaxed mb-2.5">
                    {agent.description}
                  </p>
                </div>

                {/* Bottom Tag */}
                <div className="flex flex-wrap gap-1 pt-1 border-t border-gray-50">
                  {agent.tags.slice(0, 1).map((tag) => (
                    <span
                      key={tag}
                      className="px-2 py-0.5 bg-emerald-50 text-emerald-700 text-[10px] font-medium rounded-md"
                    >
                      {tag}
                    </span>
                  ))}
                  {agent.tags.length > 1 && (
                    <span className="px-1.5 py-0.5 bg-gray-50 text-gray-400 text-[10px] rounded-md">
                      +{agent.tags.length - 1}
                    </span>
                  )}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
};
