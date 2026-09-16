import React, { useState } from 'react';
import { X, FileText, Upload, Trash2, Edit3, Check, Eye, Folder } from 'lucide-react';
import { KnowledgeDocument, DocumentType, KnowledgeFolder } from '../types';

interface DocumentModalProps {
  mode: 'view' | 'edit' | 'upload';
  document?: KnowledgeDocument | null;
  folders?: KnowledgeFolder[];
  isOpen: boolean;
  onClose: () => void;
  onSave?: (doc: Partial<KnowledgeDocument>) => void;
  onDelete?: (id: string) => void;
}

export const DocumentModal: React.FC<DocumentModalProps> = ({
  mode,
  document,
  folders = [],
  isOpen,
  onClose,
  onSave,
  onDelete,
}) => {
  const [name, setName] = useState(document?.name || '');
  const [content, setContent] = useState(document?.content || '');
  const [category, setCategory] = useState<'mine' | 'team'>(
    document?.category === 'team' ? 'team' : 'mine'
  );
  const [folderId, setFolderId] = useState(document?.folderId || '');
  const [type, setType] = useState<DocumentType>(document?.type || 'docx');
  const [tagsInput, setTagsInput] = useState(document?.tags?.join(', ') || '');
  const [isConfirmDelete, setIsConfirmDelete] = useState(false);

  // Sync with document when opened
  React.useEffect(() => {
    if (document) {
      setName(document.name);
      setContent(document.content || '');
      setCategory(document.category === 'team' ? 'team' : 'mine');
      setFolderId(document.folderId || '');
      setType(document.type || 'docx');
      setTagsInput(document.tags?.join(', ') || '');
    } else if (mode === 'upload') {
      setName('');
      setContent('');
      setCategory('mine');
      setFolderId('');
      setType('docx');
      setTagsInput('');
    }
    setIsConfirmDelete(false);
  }, [document, mode, isOpen]);

  if (!isOpen) return null;

  const handleFileUpload = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (file) {
      setName(file.name);
      const ext = file.name.split('.').pop()?.toLowerCase();
      if (ext === 'xlsx') setType('xlsx');
      else if (ext === 'html') setType('html');
      else if (ext === 'pdf') setType('pdf');
      else if (ext === 'md') setType('md');
      else setType('docx');

      const reader = new FileReader();
      reader.onload = (event) => {
        const text = event.target?.result;
        if (typeof text === 'string') {
          setContent(text);
        } else {
          setContent(`已载入二进制或富文本格式文件：${file.name}（大小 ${(file.size / 1024).toFixed(1)} KB），已建立全文语义向量索引。`);
        }
      };
      // Try text read
      reader.readAsText(file);
    }
  };

  const handleSave = () => {
    if (!name.trim()) return;
    const tags = tagsInput
      .split(/[,， ]+/)
      .map((t) => t.trim())
      .filter(Boolean);

    const matchingFolder = folders.find((f) => f.id === folderId);

    onSave?.({
      ...(document || {}),
      name: name.trim(),
      content: content.trim(),
      category,
      folderId: folderId || undefined,
      folderName: matchingFolder ? matchingFolder.name : undefined,
      type,
      tags: tags.length > 0 ? tags : ['药剂科资料'],
      size: `${Math.max(12, Math.round(content.length / 30))} KB`,
      updatedAt: new Date().toISOString().replace('T', ' ').substring(0, 16),
      summary: content.slice(0, 120) + (content.length > 120 ? '...' : ''),
    });
    onClose();
  };

  const getFormatBadge = (t: DocumentType) => {
    switch (t) {
      case 'xlsx':
        return <span className="px-2 py-0.5 text-xs bg-emerald-100 text-emerald-700 rounded font-mono font-medium">.xlsx 电子表格</span>;
      case 'html':
        return <span className="px-2 py-0.5 text-xs bg-blue-100 text-blue-700 rounded font-mono font-medium">.html 网页方案</span>;
      case 'pdf':
        return <span className="px-2 py-0.5 text-xs bg-rose-100 text-rose-700 rounded font-mono font-medium">.pdf 报告文件</span>;
      case 'md':
        return <span className="px-2 py-0.5 text-xs bg-amber-100 text-amber-700 rounded font-mono font-medium">.md Markdown</span>;
      default:
        return <span className="px-2 py-0.5 text-xs bg-indigo-100 text-indigo-700 rounded font-mono font-medium">.docx 协作文档</span>;
    }
  };

  return (
    <div className="absolute inset-0 z-50 bg-black/50 backdrop-blur-xs flex items-end sm:items-center justify-center p-0 sm:p-4">
      <div
        className="w-full max-w-lg bg-white rounded-t-3xl sm:rounded-2xl max-h-[88%] flex flex-col overflow-hidden shadow-2xl animate-in fade-in slide-in-from-bottom-6 duration-200"
      >
        {/* Header */}
        <div className="px-5 py-3.5 border-b border-gray-100 flex items-center justify-between">
          <div className="flex items-center gap-2">
            <div className="w-8 h-8 rounded-lg bg-emerald-50 text-emerald-600 flex items-center justify-center">
              <FileText className="w-4 h-4" />
            </div>
            <div>
              <h3 className="text-sm font-semibold text-gray-900">
                {mode === 'view' && '资料详情与预览'}
                {mode === 'edit' && '编辑资料内容'}
                {mode === 'upload' && '上传/新建资料'}
              </h3>
              <p className="text-[11px] text-gray-400">Zerone 企业智能知识库</p>
            </div>
          </div>
          <button
            onClick={onClose}
            className="w-8 h-8 rounded-full hover:bg-gray-100 flex items-center justify-center text-gray-400 hover:text-gray-700 cursor-pointer"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        {/* Content Area */}
        <div className="flex-1 overflow-y-auto p-5 space-y-4 text-xs">
          {mode === 'upload' && (
            <div className="border-2 border-dashed border-emerald-200 rounded-xl p-4 text-center bg-emerald-50/40 hover:bg-emerald-50/70 transition-colors">
              <input
                type="file"
                id="file-upload-input"
                onChange={handleFileUpload}
                className="hidden"
                accept=".xlsx,.xls,.docx,.doc,.pdf,.html,.md,.txt"
              />
              <label
                htmlFor="file-upload-input"
                className="cursor-pointer flex flex-col items-center justify-center space-y-1.5"
              >
                <div className="w-10 h-10 rounded-full bg-emerald-500 text-white flex items-center justify-center shadow-md">
                  <Upload className="w-5 h-5" />
                </div>
                <div className="font-semibold text-emerald-800 text-xs">点击选择或拖拽本地文件</div>
                <div className="text-[10px] text-gray-500">
                  支持 Excel(.xlsx)、Word(.docx)、PDF、HTML、Markdown
                </div>
              </label>
            </div>
          )}

          {/* Form fields */}
          {mode === 'view' ? (
            <div className="space-y-3">
              <div>
                <div className="text-gray-400 text-[11px] mb-1">文档名称</div>
                <div className="text-sm font-semibold text-gray-900 break-all">{document?.name}</div>
              </div>

              <div className="flex flex-wrap gap-2 items-center">
                {document && getFormatBadge(document.type)}
                {document?.folderName && (
                  <span className="px-2 py-0.5 text-xs bg-emerald-50 text-emerald-800 border border-emerald-200 rounded font-medium flex items-center gap-1">
                    <Folder className="w-3 h-3 text-emerald-600" />
                    <span>{document.folderName}</span>
                  </span>
                )}
                <span className="px-2 py-0.5 text-xs bg-gray-100 text-gray-700 rounded">
                  {document?.category === 'team' ? '团队空间' : '我的资料'}
                </span>
                <span className="text-gray-400 text-[11px]">{document?.size} · 更新于 {document?.updatedAt}</span>
              </div>

              {document?.tags && document.tags.length > 0 && (
                <div>
                  <div className="text-gray-400 text-[11px] mb-1.5">标签分类</div>
                  <div className="flex flex-wrap gap-1.5">
                    {document.tags.map((tag) => (
                      <span key={tag} className="px-2 py-0.5 rounded-full bg-gray-100 text-gray-600 text-[10px]">
                        #{tag}
                      </span>
                    ))}
                  </div>
                </div>
              )}

              <div>
                <div className="text-gray-400 text-[11px] mb-1.5">内容详情预览</div>
                <div className="p-3 bg-gray-50 rounded-xl border border-gray-100 font-mono text-[11px] text-gray-700 max-h-56 overflow-y-auto whitespace-pre-wrap leading-relaxed">
                  {document?.content || '暂无详细文本内容'}
                </div>
              </div>
            </div>
          ) : (
            <div className="space-y-3">
              <div>
                <label className="block text-gray-600 font-medium mb-1">资料名称 *</label>
                <input
                  type="text"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="例如：安禾心屿_工商注册信息采集表.xlsx"
                  className="w-full px-3 py-2 border border-gray-200 rounded-lg focus:outline-none focus:ring-2 focus:ring-emerald-500/30 focus:border-emerald-500 text-xs"
                />
              </div>

              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="block text-gray-600 font-medium mb-1">存放空间</label>
                  <select
                    value={category}
                    onChange={(e) => setCategory(e.target.value as 'mine' | 'team')}
                    className="w-full px-3 py-2 border border-gray-200 rounded-lg focus:outline-none focus:ring-2 focus:ring-emerald-500/30 text-xs"
                  >
                    <option value="mine">我的资料（个人私有）</option>
                    <option value="team">团队空间（全员协同）</option>
                  </select>
                </div>
                <div>
                  <label className="block text-gray-600 font-medium mb-1">文件格式</label>
                  <select
                    value={type}
                    onChange={(e) => setType(e.target.value as DocumentType)}
                    className="w-full px-3 py-2 border border-gray-200 rounded-lg focus:outline-none focus:ring-2 focus:ring-emerald-500/30 text-xs"
                  >
                    <option value="docx">Word (.docx)</option>
                    <option value="xlsx">Excel (.xlsx)</option>
                    <option value="html">网页/方案 (.html)</option>
                    <option value="pdf">PDF 文档 (.pdf)</option>
                    <option value="md">Markdown (.md)</option>
                  </select>
                </div>
              </div>

              <div>
                <label className="block text-gray-600 font-medium mb-1">
                  归属知识库（文件夹）
                </label>
                <select
                  value={folderId}
                  onChange={(e) => setFolderId(e.target.value)}
                  className="w-full px-3 py-2 border border-gray-200 rounded-lg focus:outline-none focus:ring-2 focus:ring-emerald-500/30 text-xs text-gray-800 bg-white"
                >
                  <option value="">(未分类 / 根目录)</option>
                  {folders.map((f) => (
                    <option key={f.id} value={f.id}>
                      {f.icon || '📁'} {f.name} ({f.category === 'team' ? '团队空间' : '我的空间'})
                    </option>
                  ))}
                </select>
              </div>

              <div>
                <label className="block text-gray-600 font-medium mb-1">标签分类（逗号分隔）</label>
                <input
                  type="text"
                  value={tagsInput}
                  onChange={(e) => setTagsInput(e.target.value)}
                  placeholder="例如：工商, 注册, 备案"
                  className="w-full px-3 py-2 border border-gray-200 rounded-lg focus:outline-none focus:ring-2 focus:ring-emerald-500/30 text-xs"
                />
              </div>

              <div>
                <label className="block text-gray-600 font-medium mb-1">资料全文或说明内容</label>
                <textarea
                  rows={6}
                  value={content}
                  onChange={(e) => setContent(e.target.value)}
                  placeholder="在此输入或粘贴文档正文、数据或摘要，大模型将自动建立知识索引..."
                  className="w-full px-3 py-2 border border-gray-200 rounded-lg focus:outline-none focus:ring-2 focus:ring-emerald-500/30 text-xs leading-relaxed"
                />
              </div>
            </div>
          )}

          {/* Delete confirmation prompt */}
          {isConfirmDelete && (
            <div className="p-3 bg-red-50 border border-red-200 rounded-xl text-xs text-red-700 flex items-center justify-between animate-in fade-in">
              <span>确认彻底从资料库中删除此文件？</span>
              <div className="flex items-center gap-2">
                <button
                  onClick={() => setIsConfirmDelete(false)}
                  className="px-2.5 py-1 rounded bg-white text-gray-600 border border-gray-200 cursor-pointer"
                >
                  取消
                </button>
                <button
                  onClick={() => {
                    if (document?.id) onDelete?.(document.id);
                    onClose();
                  }}
                  className="px-2.5 py-1 rounded bg-red-600 text-white font-medium cursor-pointer"
                >
                  确定删除
                </button>
              </div>
            </div>
          )}
        </div>

        {/* Footer Actions */}
        <div className="px-5 py-3 border-t border-gray-100 bg-gray-50 flex items-center justify-between">
          {mode === 'view' ? (
            <>
              <div className="flex items-center gap-2" />
              <div className="flex items-center gap-2">
                <button
                  onClick={() => setIsConfirmDelete(true)}
                  className="flex items-center gap-1 px-2.5 py-1.5 rounded-lg text-red-600 hover:bg-red-50 cursor-pointer text-xs"
                >
                  <Trash2 className="w-3.5 h-3.5" />
                  <span>删除</span>
                </button>
                <button
                  onClick={() => {
                    onClose();
                    // trigger edit mode
                  }}
                  className="flex items-center gap-1 px-3 py-1.5 rounded-lg bg-neutral-900 text-white font-medium cursor-pointer text-xs"
                >
                  <Edit3 className="w-3.5 h-3.5" />
                  <span>修改</span>
                </button>
              </div>
            </>
          ) : (
            <div className="w-full flex items-center justify-end gap-2">
              <button
                onClick={onClose}
                className="px-4 py-1.5 rounded-lg border border-gray-200 text-gray-600 hover:bg-gray-100 cursor-pointer text-xs"
              >
                取消
              </button>
              <button
                onClick={handleSave}
                disabled={!name.trim()}
                className="flex items-center gap-1 px-4 py-1.5 rounded-lg bg-emerald-600 hover:bg-emerald-700 disabled:opacity-50 text-white font-medium cursor-pointer text-xs shadow-sm"
              >
                <Check className="w-3.5 h-3.5" />
                <span>{mode === 'upload' ? '确认上传入库' : '保存修改'}</span>
              </button>
            </div>
          )}
        </div>
      </div>
    </div>
  );
};
