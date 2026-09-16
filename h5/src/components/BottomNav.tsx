import React from 'react';
import { MessageSquare, Users, FolderKanban, User } from 'lucide-react';
import { ActiveTab } from '../types';

interface BottomNavProps {
  activeTab: ActiveTab;
  onChangeTab: (tab: ActiveTab) => void;
  knowledgeCount?: number;
  /** guest（体验用户）为 false：知识库走 /api/v1/admin/**，guest 恒 403，不展示入口 */
  showKnowledge?: boolean;
}

export const BottomNav: React.FC<BottomNavProps> = ({
  activeTab,
  onChangeTab,
  knowledgeCount = 0,
  showKnowledge = true,
}) => {
  const navItems = [
    {
      id: 'chat' as ActiveTab,
      label: '任务',
      icon: MessageSquare,
      showBubble: activeTab === 'chat',
    },
    {
      id: 'agents' as ActiveTab,
      label: '专家',
      icon: Users,
      showBubble: activeTab === 'agents',
    },
    {
      id: 'knowledge' as ActiveTab,
      label: '知识库',
      icon: FolderKanban,
      showBubble: activeTab === 'knowledge',
      badge: knowledgeCount > 0 ? knowledgeCount : undefined,
    },
    {
      id: 'profile' as ActiveTab,
      label: '我的',
      icon: User,
      showBubble: activeTab === 'profile',
    },
  ];

  return (
    <nav
      id="workbuddy-bottom-nav"
      className="bg-white/95 backdrop-blur-md border-t border-gray-100 px-4 py-1.5 flex items-center justify-around z-30 select-none shadow-[0_-4px_12px_rgba(0,0,0,0.03)]"
    >
      {navItems
        .filter((item) => item.id !== 'knowledge' || showKnowledge)
        .map((item) => {
        const Icon = item.icon;
        const isActive = activeTab === item.id;

        return (
          <button
            key={item.id}
            id={`nav-item-${item.id}`}
            onClick={() => onChangeTab(item.id)}
            className="flex flex-col items-center justify-center min-w-[56px] py-1 relative group cursor-pointer transition-all active:scale-95"
          >
            {/* Pill or icon styling */}
            <div
              className={`flex items-center justify-center transition-all duration-200 ${
                isActive
                  ? 'bg-neutral-900 text-white w-10 h-7 rounded-full shadow-sm'
                  : 'text-gray-400 hover:text-gray-700 w-10 h-7'
              }`}
            >
              <Icon className={`w-4 h-4 ${isActive ? 'stroke-[2.5]' : 'stroke-2'}`} />
              {item.badge && !isActive && (
                <span className="absolute top-0 right-2 w-2 h-2 rounded-full bg-emerald-500 ring-2 ring-white" />
              )}
            </div>
            <span
              className={`text-[11px] mt-0.5 tracking-tight font-medium transition-colors ${
                isActive ? 'text-neutral-900 font-semibold' : 'text-gray-500'
              }`}
            >
              {item.label}
            </span>
          </button>
        );
      })}
    </nav>
  );
};
