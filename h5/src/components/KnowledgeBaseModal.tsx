import React, { useState, useEffect } from 'react';
import { X, Database } from 'lucide-react';
import { KnowledgeFolder } from '../types';

interface KnowledgeBaseModalProps {
  isOpen: boolean;
  mode: 'create' | 'edit';
  folder: KnowledgeFolder | null;
  onClose: () => void;
  onSubmit: (data: {
    name: string;
    description: string;
    parseMethod: string;
    category: 'mine' | 'team';
  }) => void;
}

export const KnowledgeBaseModal: React.FC<KnowledgeBaseModalProps> = ({
  isOpen,
  mode,
  folder,
  onClose,
  onSubmit,
}) => {
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');

  useEffect(() => {
    if (folder && mode === 'edit') {
      setName(folder.name);
      setDescription(folder.description || '');
    } else {
      setName('');
      setDescription('');
    }
  }, [folder, mode, isOpen]);

  if (!isOpen) return null;

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim()) return;
    // 新建/编辑都只收集名称+描述，解析配置由 API 层按默认项固化
    onSubmit({
      name: name.trim(),
      description: description.trim(),
      parseMethod: 'naive',
      category: 'mine',
    });
    onClose();
  };

  return (
    <div className="absolute inset-0 z-50 bg-black/40 backdrop-blur-2xs flex items-center justify-center p-4 animate-in fade-in duration-150">
      <div
        className="w-full max-w-md bg-white rounded-2xl shadow-2xl overflow-hidden border border-gray-200 animate-in zoom-in-95 duration-200"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className="px-5 py-3.5 border-b border-gray-100 flex items-center justify-between">
          <div className="flex items-center gap-2">
            <div className="w-7 h-7 rounded-lg bg-emerald-50 text-emerald-600 flex items-center justify-center">
              <Database className="w-4 h-4" />
            </div>
            <h2 className="text-sm font-bold text-gray-900">
              {mode === 'create' ? '新建知识库' : '编辑知识库'}
            </h2>
          </div>
          <button
            onClick={onClose}
            className="w-7 h-7 rounded-full hover:bg-gray-100 flex items-center justify-center text-gray-400 hover:text-gray-700 cursor-pointer transition-colors"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        {/* Form */}
        <form onSubmit={handleSubmit} className="p-5 space-y-4 text-xs">
          {/* Name */}
          <div>
            <label className="block text-[11px] font-semibold text-gray-700 mb-1">
              知识库名称 <span className="text-red-500">*</span>
            </label>
            <input
              type="text"
              required
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="例如：产品手册库、内部规范"
              className="w-full px-3 py-2 bg-gray-50 border border-gray-200 rounded-xl text-xs text-gray-900 focus:outline-none focus:ring-1 focus:ring-emerald-500 focus:bg-white transition-all"
            />
          </div>

          {/* Description */}
          <div>
            <label className="block text-[11px] font-semibold text-gray-700 mb-1">
              描述说明
            </label>
            <textarea
              rows={2}
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="用于说明知识库定位与收录范围..."
              className="w-full px-3 py-2 bg-gray-50 border border-gray-200 rounded-xl text-xs text-gray-900 focus:outline-none focus:ring-1 focus:ring-emerald-500 focus:bg-white resize-none transition-all"
            />
          </div>

          {/* Footer Buttons */}
          <div className="pt-2 flex items-center justify-end gap-2">
            <button
              type="button"
              onClick={onClose}
              className="px-4 py-2 rounded-xl border border-gray-200 hover:bg-gray-50 text-xs font-semibold text-gray-600 cursor-pointer"
            >
              取消
            </button>
            <button
              type="submit"
              disabled={!name.trim()}
              className="px-5 py-2 rounded-xl bg-neutral-900 hover:bg-neutral-800 disabled:opacity-40 text-white font-semibold text-xs shadow-xs cursor-pointer transition-all active:scale-98"
            >
              {mode === 'create' ? '创建知识库' : '保存修改'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};
