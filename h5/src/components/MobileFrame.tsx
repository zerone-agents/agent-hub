import React, { useState } from 'react';
import { Smartphone, Maximize2, Minimize2 } from 'lucide-react';

interface MobileFrameProps {
  children: React.ReactNode;
}

export const MobileFrame: React.FC<MobileFrameProps> = ({ children }) => {
  const [isPhoneView, setIsPhoneView] = useState(true);

  return (
    <div className="h-screen w-full bg-[#F1F3F5] flex flex-col items-center justify-center sm:p-3 overflow-hidden">
      {/* Top desktop floating bar for switching views and demo banner */}
      <div className="hidden sm:flex items-center justify-between w-full max-w-[430px] mb-2 px-2 text-xs text-gray-500">
        <div className="flex items-center gap-1.5">
          <span className="w-2 h-2 rounded-full bg-emerald-500 animate-pulse" />
          <span className="font-medium text-gray-700">Zerone 智能工作空间</span>
        </div>
        <button
          onClick={() => setIsPhoneView(!isPhoneView)}
          className="flex items-center gap-1.5 px-2.5 py-1 rounded-lg bg-white hover:bg-gray-50 text-gray-700 transition-colors cursor-pointer border border-gray-200 shadow-2xs"
          title={isPhoneView ? '切换为宽屏视图' : '切换为手机外框'}
        >
          {isPhoneView ? (
            <>
              <Maximize2 className="w-3.5 h-3.5 text-gray-500" />
              <span>展开宽屏</span>
            </>
          ) : (
            <>
              <Smartphone className="w-3.5 h-3.5 text-gray-500" />
              <span>移动端视图</span>
            </>
          )}
        </button>
      </div>

      {/* Main Container - Clean modern mobile frame without thick black bezels or gaps */}
      <div
        className={`w-full h-full sm:h-[96vh] sm:max-h-[920px] transition-all duration-300 flex flex-col bg-white overflow-hidden relative shadow-xl ${
          isPhoneView
            ? 'max-w-[440px] sm:rounded-3xl border sm:border-gray-200/90'
            : 'max-w-3xl sm:rounded-2xl border sm:border-gray-200/90'
        }`}
      >
        {/* App Content（真实手机浏览器自带状态栏，不再渲染模拟状态栏） */}
        <div className="flex-1 flex flex-col min-h-0 overflow-hidden relative bg-[#F8F9FA]">
          {children}
        </div>

        {/* Mobile bottom swipe indicator bar */}
        <div className="w-full bg-white flex items-center justify-center pb-1 pt-0.5 select-none pointer-events-none">
          <div className="w-32 h-1 bg-neutral-300 rounded-full" />
        </div>
      </div>
    </div>
  );
};
