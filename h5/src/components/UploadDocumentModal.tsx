import React, { useState, useRef } from 'react';
import { X, Upload, FileText, Check, Trash2, Loader2, Folder } from 'lucide-react';
import { KnowledgeFolder, DocumentType } from '../types';

interface UploadDocumentModalProps {
  isOpen: boolean;
  onClose: () => void;
  folders: KnowledgeFolder[];
  defaultFolderId?: string | null;
  /** 确认上传：把原始 File 队列交给外层，走 agent-hub multipart 接口（字段名 files） */
  onUploadFiles: (files: File[], folderId: string, autoParse: boolean) => void | Promise<void>;
}

interface QueuedFile {
  id: string;
  file: File;
  name: string;
  sizeStr: string;
  type: DocumentType;
  content: string;
}

export const UploadDocumentModal: React.FC<UploadDocumentModalProps> = ({
  isOpen,
  onClose,
  folders,
  defaultFolderId,
  onUploadFiles,
}) => {
  const [isDragOver, setIsDragOver] = useState(false);
  const [autoParse, setAutoParse] = useState(true);
  const [queue, setQueue] = useState<QueuedFile[]>([]);
  const [targetFolderId, setTargetFolderId] = useState<string>(defaultFolderId || '');
  const [isSubmitting, setIsSubmitting] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

  React.useEffect(() => {
    if (defaultFolderId) {
      setTargetFolderId(defaultFolderId);
    } else if (folders.length > 0 && !targetFolderId) {
      setTargetFolderId(folders[0].id);
    }
  }, [defaultFolderId, folders]);

  if (!isOpen) return null;

  const processFiles = (files: FileList | File[]) => {
    const newItems: QueuedFile[] = [];

    Array.from(files).forEach((file) => {
      const ext = file.name.split('.').pop()?.toLowerCase();
      let docType: DocumentType = 'docx';
      if (ext === 'xlsx' || ext === 'xls') docType = 'xlsx';
      else if (ext === 'html' || ext === 'htm') docType = 'html';
      else if (ext === 'pdf') docType = 'pdf';
      else if (ext === 'md') docType = 'md';

      const queuedItem: QueuedFile = {
        id: 'queue-' + Date.now() + '-' + Math.random().toString(36).substr(2, 5),
        file,
        name: file.name,
        sizeStr: `${Math.max(1, Math.round(file.size / 1024))} KB`,
        type: docType,
        content: '',
      };

      // Read content
      const reader = new FileReader();
      reader.onload = (e) => {
        const result = e.target?.result;
        if (typeof result === 'string') {
          queuedItem.content = result;
        } else {
          queuedItem.content = `【已成功解析二进制文档 ${file.name}】大小：${(file.size / 1024).toFixed(1)} KB。已切块并录入语义索引向量库。`;
        }
      };
      reader.readAsText(file);

      newItems.push(queuedItem);
    });

    setQueue((prev) => [...prev, ...newItems]);
  };

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    if (e.target.files && e.target.files.length > 0) {
      processFiles(e.target.files);
    }
  };

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    setIsDragOver(false);
    if (e.dataTransfer.files && e.dataTransfer.files.length > 0) {
      processFiles(e.dataTransfer.files);
    }
  };

  const handleRemoveFromQueue = (id: string) => {
    setQueue((prev) => prev.filter((item) => item.id !== id));
  };

  const handleStartUpload = async () => {
    if (queue.length === 0 || !targetFolderId) return;
    setIsSubmitting(true);
    try {
      await onUploadFiles(queue.map((item) => item.file), targetFolderId, autoParse);
      setQueue([]);
      onClose();
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <div className="absolute inset-0 z-50 bg-black/40 backdrop-blur-2xs flex items-center justify-center p-3 animate-in fade-in duration-150">
      {/* Modal Container matching Image 4 */}
      <div
        className="w-full max-w-lg bg-white rounded-2xl shadow-2xl overflow-hidden flex flex-col border border-gray-200 animate-in zoom-in-95 duration-200"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header matching Image 4 */}
        <div className="px-5 py-3.5 flex items-center justify-between border-b border-gray-100">
          <h2 className="text-sm font-bold text-gray-900">上传文档</h2>
          <button
            onClick={onClose}
            className="w-7 h-7 rounded-full hover:bg-gray-100 flex items-center justify-center text-gray-400 hover:text-gray-700 cursor-pointer transition-colors"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        {/* Modal Body */}
        <div className="p-5 space-y-3.5">
          {/* Dashed Drag & Drop Box matching Image 4 */}
          <div
            onDragOver={(e) => {
              e.preventDefault();
              setIsDragOver(true);
            }}
            onDragLeave={() => setIsDragOver(false)}
            onDrop={handleDrop}
            onClick={() => fileInputRef.current?.click()}
            className={`border border-dashed rounded-2xl p-7 flex flex-col items-center justify-center text-center cursor-pointer transition-all ${
              isDragOver
                ? 'border-[#C85A32] bg-[#FDF6F2]'
                : 'border-gray-300 hover:border-gray-400 bg-gray-50/40 hover:bg-gray-50'
            }`}
          >
            <input
              ref={fileInputRef}
              type="file"
              multiple
              onChange={handleFileChange}
              className="hidden"
              accept=".docx,.xlsx,.xls,.pdf,.html,.md,.txt,.csv"
            />
            <div className="w-10 h-10 rounded-full flex items-center justify-center text-gray-700 mb-2">
              <Upload className="w-6 h-6 stroke-[1.8]" />
            </div>
            <div className="text-sm font-semibold text-gray-900 tracking-tight">
              拖入文件， 或点击选择
            </div>
            <div className="text-[11px] text-gray-400 mt-1">
              队列会在确认后统一上传，避免误触即提交。
            </div>
          </div>

          {/* Options Row: Toggle "上传后解析" matching Image 4 + Target Folder */}
          <div className="flex items-center justify-between gap-3 pt-0.5">
            {/* Pill Toggle matching Image 4 (Warm terracotta background with pill text and toggle button) */}
            <button
              type="button"
              onClick={() => setAutoParse(!autoParse)}
              className={`inline-flex items-center gap-2 px-3 py-1.5 rounded-full text-xs font-medium cursor-pointer transition-all ${
                autoParse
                  ? 'bg-emerald-600 text-white shadow-2xs'
                  : 'bg-gray-200 text-gray-600'
              }`}
            >
              <span>上传后解析</span>
              <div
                className={`w-4 h-4 rounded-full bg-white transition-transform ${
                  autoParse ? 'translate-x-0' : '-translate-x-1 opacity-70'
                }`}
              />
            </button>

            {/* Target Knowledge Base selector */}
            <div className="flex items-center gap-1.5 text-xs text-gray-600 min-w-0">
              <Folder className="w-3.5 h-3.5 text-gray-400 shrink-0" />
              <span className="text-gray-400 text-[11px] shrink-0">存入知识库:</span>
              <select
                value={targetFolderId}
                onChange={(e) => setTargetFolderId(e.target.value)}
                className="text-xs font-medium text-gray-800 bg-gray-100 hover:bg-gray-200/70 py-1 px-2 rounded-lg border-0 focus:ring-1 focus:ring-[#C85A32] truncate max-w-[150px] cursor-pointer"
              >
                {folders.map((f) => (
                  <option key={f.id} value={f.id}>
                    {f.name}
                  </option>
                ))}
              </select>
            </div>
          </div>

          {/* Queue Container Box matching Image 4 */}
          <div className="border border-gray-200 rounded-xl bg-gray-50/50 p-2.5 min-h-[64px] max-h-[140px] overflow-y-auto">
            {queue.length === 0 ? (
              <div className="h-10 flex items-center justify-center text-xs text-gray-400">
                队列为空
              </div>
            ) : (
              <div className="space-y-1.5">
                {queue.map((item) => (
                  <div
                    key={item.id}
                    className="flex items-center justify-between gap-2 p-1.5 px-2.5 bg-white rounded-lg border border-gray-200/80 shadow-2xs text-xs"
                  >
                    <div className="flex items-center gap-2 min-w-0 flex-1">
                      <FileText className="w-3.5 h-3.5 text-gray-500 shrink-0" />
                      <span className="font-medium text-gray-800 truncate">{item.name}</span>
                      <span className="text-[10px] text-gray-400 shrink-0">{item.sizeStr}</span>
                    </div>
                    <button
                      onClick={() => handleRemoveFromQueue(item.id)}
                      className="w-5 h-5 rounded-md hover:bg-gray-100 flex items-center justify-center text-gray-400 hover:text-red-500 cursor-pointer"
                    >
                      <X className="w-3 h-3" />
                    </button>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>

        {/* Footer Actions matching Image 4: 取消 & 开始上传 */}
        <div className="px-5 py-3.5 bg-white border-t border-gray-100 flex items-center justify-end gap-2.5">
          <button
            type="button"
            onClick={onClose}
            className="px-4 py-2 rounded-xl border border-gray-200 bg-white hover:bg-gray-50 text-xs font-semibold text-gray-700 cursor-pointer transition-colors"
          >
            取消
          </button>
          <button
            type="button"
            disabled={queue.length === 0 || isSubmitting}
            onClick={handleStartUpload}
            className="px-5 py-2 rounded-xl bg-neutral-900 hover:bg-neutral-800 disabled:opacity-40 text-xs font-semibold text-white shadow-xs cursor-pointer flex items-center gap-1.5 transition-all active:scale-98"
          >
            {isSubmitting ? (
              <>
                <Loader2 className="w-3.5 h-3.5 animate-spin" />
                <span>正在上传...</span>
              </>
            ) : (
              <span>开始上传</span>
            )}
          </button>
        </div>
      </div>
    </div>
  );
};
