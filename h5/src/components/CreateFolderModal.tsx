import React, { useState } from 'react';
import { X, FolderPlus, Check, Sparkles } from 'lucide-react';
import { KnowledgeFolder } from '../types';

interface CreateFolderModalProps {
  isOpen: boolean;
  onClose: () => void;
  onCreate: (folderData: Omit<KnowledgeFolder, 'id' | 'createdAt'>) => void;
  defaultCategory?: 'mine' | 'team';
}

const PRESET_ICONS = ['📁', '💊', '🦠', '📋', '🛡️', '🔬', '🧪', '📚', '🩺', '🔐'];
const PRESET_COLORS = [
  { id: 'emerald', bg: 'bg-emerald-500', ring: 'ring-emerald-400' },
  { id: 'blue', bg: 'bg-blue-500', ring: 'ring-blue-400' },
  { id: 'purple', bg: 'bg-purple-500', ring: 'ring-purple-400' },
  { id: 'amber', bg: 'bg-amber-500', ring: 'ring-amber-400' },
  { id: 'rose', bg: 'bg-rose-500', ring: 'ring-rose-400' },
  { id: 'teal', bg: 'bg-teal-500', ring: 'ring-teal-400' },
];

const SUGGESTED_NAMES = [
  '麻精药品与高危药品质控专库',
  '抗微生物药物临床应用(AMS)库',
  '处方前置审核与配伍禁忌库',
  '药品不良反应(ADR)监测上报库',
  '静脉用药调配(PIVAS)操作规范库',
  '国家集采中选药品使用监测库',
];

export const CreateFolderModal: React.FC<CreateFolderModalProps> = ({
  isOpen,
  onClose,
  onCreate,
  defaultCategory = 'team',
}) => {
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [category, setCategory] = useState<'mine' | 'team'>(defaultCategory);
  const [icon, setIcon] = useState('📁');
  const [color, setColor] = useState('emerald');
  const [tagsInput, setTagsInput] = useState('');

  if (!isOpen) return null;

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim()) return;

    const tags = tagsInput
      .split(/[,， ]+/)
      .map((t) => t.trim())
      .filter(Boolean);

    onCreate({
      name: name.trim(),
      description: description.trim() || undefined,
      category,
      icon,
      color,
      tags: tags.length > 0 ? tags : ['药剂科知识库'],
    });

    // Reset and close
    setName('');
    setDescription('');
    setIcon('📁');
    setColor('emerald');
    setTagsInput('');
    onClose();
  };

  return (
    <div className="absolute inset-0 z-50 bg-black/50 backdrop-blur-xs flex items-end sm:items-center sm:justify-center">
      <div className="w-full bg-white rounded-t-3xl sm:rounded-3xl max-h-[85%] flex flex-col overflow-hidden shadow-2xl animate-in slide-in-from-bottom sm:zoom-in-95 duration-200">
        {/* Header */}
        <div className="p-4 border-b border-gray-100 flex items-center justify-between shrink-0">
          <div className="flex items-center gap-2">
            <div className="w-8 h-8 rounded-xl bg-emerald-100 text-emerald-700 flex items-center justify-center">
              <FolderPlus className="w-4 h-4" />
            </div>
            <div>
              <h2 className="text-sm font-bold text-gray-900">新建知识库（文件夹）</h2>
              <p className="text-[10px] text-gray-400">组织归集科室资料、规范与专科药学知识</p>
            </div>
          </div>
          <button
            onClick={onClose}
            className="w-7 h-7 rounded-full bg-gray-100 hover:bg-gray-200 flex items-center justify-center text-gray-400 hover:text-gray-700 cursor-pointer transition-colors"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        {/* Form Body */}
        <form onSubmit={handleSubmit} className="flex-1 overflow-y-auto p-4 space-y-4 text-xs">
          {/* Category / Scope */}
          <div>
            <label className="block text-[11px] font-semibold text-gray-700 mb-1.5">
              所属存储空间
            </label>
            <div className="grid grid-cols-2 gap-2">
              <button
                type="button"
                onClick={() => setCategory('team')}
                className={`py-2 px-3 rounded-xl border text-xs font-semibold cursor-pointer flex items-center justify-center gap-1.5 transition-all ${
                  category === 'team'
                    ? 'border-emerald-600 bg-emerald-50/80 text-emerald-800 shadow-2xs'
                    : 'border-gray-200 text-gray-600 hover:bg-gray-50'
                }`}
              >
                <span>👥</span>
                <span>药剂科团队空间 (全科共享)</span>
              </button>
              <button
                type="button"
                onClick={() => setCategory('mine')}
                className={`py-2 px-3 rounded-xl border text-xs font-semibold cursor-pointer flex items-center justify-center gap-1.5 transition-all ${
                  category === 'mine'
                    ? 'border-emerald-600 bg-emerald-50/80 text-emerald-800 shadow-2xs'
                    : 'border-gray-200 text-gray-600 hover:bg-gray-50'
                }`}
              >
                <span>👤</span>
                <span>我的知识库 (个人专属)</span>
              </button>
            </div>
          </div>

          {/* Name */}
          <div>
            <label className="block text-[11px] font-semibold text-gray-700 mb-1.5">
              知识库名称 <span className="text-red-500">*</span>
            </label>
            <input
              type="text"
              required
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="例如：抗微生物药物临床应用(AMS)库"
              className="w-full px-3 py-2 bg-gray-50 border border-gray-200 rounded-xl text-xs text-gray-900 focus:outline-none focus:ring-1 focus:ring-emerald-500 focus:bg-white transition-all"
            />

            {/* Suggestions Chips */}
            <div className="mt-2 flex flex-wrap gap-1">
              <span className="text-[10px] text-gray-400 flex items-center gap-0.5 mr-1 py-0.5">
                <Sparkles className="w-2.5 h-2.5 text-emerald-600" /> 推荐名称：
              </span>
              {SUGGESTED_NAMES.slice(0, 3).map((sug) => (
                <button
                  key={sug}
                  type="button"
                  onClick={() => setName(sug)}
                  className="px-2 py-0.5 rounded-full bg-gray-100 hover:bg-emerald-50 hover:text-emerald-700 text-[10px] text-gray-600 cursor-pointer transition-colors"
                >
                  {sug.slice(0, 10)}...
                </button>
              ))}
            </div>
          </div>

          {/* Icon & Theme Color */}
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-[11px] font-semibold text-gray-700 mb-1.5">
                知识库图标
              </label>
              <div className="flex flex-wrap gap-1 bg-gray-50 p-1.5 rounded-xl border border-gray-200">
                {PRESET_ICONS.map((ic) => (
                  <button
                    key={ic}
                    type="button"
                    onClick={() => setIcon(ic)}
                    className={`w-7 h-7 rounded-lg flex items-center justify-center text-sm cursor-pointer transition-transform ${
                      icon === ic
                        ? 'bg-white shadow-xs scale-110 ring-1 ring-emerald-500'
                        : 'hover:bg-gray-200/60'
                    }`}
                  >
                    {ic}
                  </button>
                ))}
              </div>
            </div>

            <div>
              <label className="block text-[11px] font-semibold text-gray-700 mb-1.5">
                标识主题色
              </label>
              <div className="flex items-center gap-2 bg-gray-50 p-2 rounded-xl border border-gray-200 h-[46px]">
                {PRESET_COLORS.map((c) => (
                  <button
                    key={c.id}
                    type="button"
                    onClick={() => setColor(c.id)}
                    className={`w-6 h-6 rounded-full ${c.bg} flex items-center justify-center text-white cursor-pointer transition-all ${
                      color === c.id ? 'ring-2 ring-offset-2 ring-emerald-500 scale-105' : 'hover:opacity-80'
                    }`}
                  >
                    {color === c.id && <Check className="w-3 h-3 stroke-[3]" />}
                  </button>
                ))}
              </div>
            </div>
          </div>

          {/* Description */}
          <div>
            <label className="block text-[11px] font-semibold text-gray-700 mb-1.5">
              知识库描述 / 用途说明
            </label>
            <textarea
              rows={2}
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="简要说明该知识库收集的文档类型、业务场景或临床指导方向..."
              className="w-full px-3 py-2 bg-gray-50 border border-gray-200 rounded-xl text-xs text-gray-900 focus:outline-none focus:ring-1 focus:ring-emerald-500 focus:bg-white resize-none transition-all"
            />
          </div>

          {/* Tags */}
          <div>
            <label className="block text-[11px] font-semibold text-gray-700 mb-1.5">
              业务标签 (逗号分隔)
            </label>
            <input
              type="text"
              value={tagsInput}
              onChange={(e) => setTagsInput(e.target.value)}
              placeholder="例如：合理用药, 处方前审, 抗感染"
              className="w-full px-3 py-2 bg-gray-50 border border-gray-200 rounded-xl text-xs text-gray-900 focus:outline-none focus:ring-1 focus:ring-emerald-500 focus:bg-white transition-all"
            />
          </div>

          {/* Footer Buttons */}
          <div className="pt-2 flex items-center gap-2">
            <button
              type="button"
              onClick={onClose}
              className="flex-1 py-2.5 rounded-xl border border-gray-200 hover:bg-gray-50 font-semibold text-gray-600 cursor-pointer text-center"
            >
              取消
            </button>
            <button
              type="submit"
              disabled={!name.trim()}
              className="flex-2 py-2.5 rounded-xl bg-emerald-600 hover:bg-emerald-700 disabled:opacity-40 text-white font-semibold shadow-xs cursor-pointer text-center transition-transform active:scale-98"
            >
              立即创建知识库
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};
