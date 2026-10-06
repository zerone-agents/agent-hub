import React, { useState, useMemo } from "react";
import {
  Search,
  Plus,
  Edit2,
  Trash2,
  ArrowLeft,
  Upload,
  Folder,
  FolderPlus,
  FileText,
  FileSpreadsheet,
  Code,
  FileCode,
  ChevronRight,
} from "lucide-react";
import { KnowledgeDocument, KnowledgeFolder, DocumentType } from "../types";
import {
  KnowledgeBaseModal,
  type KnowledgeFolderFormInput,
} from "./KnowledgeBaseModal";
import type { MobileChunk } from "../api/knowledge";
import { KnowledgeRetrieval } from "./KnowledgeRetrieval";
import "./KnowledgeMobile.css";

interface KnowledgeBaseViewProps {
  loading?: boolean;
  error?: string;
  onRefresh: () => Promise<void>;
  documents: KnowledgeDocument[];
  folders: KnowledgeFolder[];
  /** 写权限（新建/上传/编辑/删除）：后端 /api/v1/admin/** 需 maintainer+，
   *  体验用户（guest）与 member 只读，不显示任何写操作入口 */
  canWrite?: boolean;
  onOpenUpload: (defaultFolderId?: string) => void;
  onOpenDocDetail: (doc: KnowledgeDocument, chunk?: MobileChunk) => void;
  onOpenDocEdit: (doc: KnowledgeDocument) => void;
  onDeleteDoc: (id: string) => Promise<void>;
  onCreateFolder: (
    folderData: KnowledgeFolderFormInput,
  ) => Promise<KnowledgeFolder>;
  onUpdateFolder: (
    folderId: string,
    data: {
      name: string;
      description: string;
      parseMethod: string;
      category: "mine" | "team";
    },
  ) => Promise<void>;
  onDeleteFolder: (folderId: string) => Promise<void>;
}

export const KnowledgeBaseView: React.FC<KnowledgeBaseViewProps> = ({
  documents,
  loading,
  error,
  onRefresh,
  folders,
  canWrite = true,
  onOpenUpload,
  onOpenDocDetail,
  onCreateFolder,
  onUpdateFolder,
  onDeleteFolder,
}) => {
  const [actionError, setActionError] = useState("");
  const [busy, setBusy] = useState(false);
  const perform = async (action: () => Promise<void>) => {
    if (busy) return;
    setBusy(true);
    setActionError("");
    try {
      await action();
    } catch (err) {
      setActionError(err instanceof Error ? err.message : "操作失败，请重试");
    } finally {
      setBusy(false);
    }
  };
  const [searchQuery, setSearchQuery] = useState("");
  const [selectedFolderId, setSelectedFolderId] = useState<string | null>(null);
  const [fabOpen, setFabOpen] = useState(false);
  const [folderTab, setFolderTab] = useState<"documents" | "retrieval">(
    "documents",
  );

  // Folder create / edit modal
  const [folderModalState, setFolderModalState] = useState<{
    isOpen: boolean;
    mode: "create" | "edit";
    folder: KnowledgeFolder | null;
  }>({
    isOpen: false,
    mode: "create",
    folder: null,
  });

  const foldersWithCounts = folders;

  // Filtered folders by search（首页只展示知识库文件夹）
  const filteredFolders = useMemo(() => {
    return foldersWithCounts.filter((f) => {
      if (!searchQuery.trim()) return true;
      return f.name.toLowerCase().includes(searchQuery.toLowerCase());
    });
  }, [foldersWithCounts, searchQuery]);

  // Currently opened folder
  const currentFolder = useMemo(() => {
    if (!selectedFolderId) return null;
    return foldersWithCounts.find((f) => f.id === selectedFolderId) || null;
  }, [selectedFolderId, foldersWithCounts]);

  // Files in the currently opened folder
  const currentFolderDocs = useMemo(() => {
    if (!selectedFolderId) return [];
    return documents.filter((d) => d.folderId === selectedFolderId);
  }, [selectedFolderId, documents]);

  const renderFileIcon = (type: DocumentType) => {
    switch (type) {
      case "xlsx":
        return (
          <FileSpreadsheet className="w-5 h-5 text-emerald-600 shrink-0" />
        );
      case "html":
        return <Code className="w-5 h-5 text-blue-600 shrink-0" />;
      case "pdf":
        return <FileText className="w-5 h-5 text-rose-600 shrink-0" />;
      case "md":
        return <FileCode className="w-5 h-5 text-amber-600 shrink-0" />;
      default:
        return <FileText className="w-5 h-5 text-neutral-600 shrink-0" />;
    }
  };

  const openCreateFolder = () =>
    setFolderModalState({ isOpen: true, mode: "create", folder: null });

  return (
    <div className="knowledge-mobile flex-1 flex flex-col h-full knowledge-background overflow-hidden relative text-gray-900">
      <div className="shrink-0 px-4 py-2 text-xs bg-white border-b border-gray-100">
        <button
          disabled={busy || loading}
          onClick={() => void perform(onRefresh)}
          className="text-emerald-700 disabled:opacity-40"
        >
          {loading ? "加载中…" : "刷新知识库"}
        </button>
        {(error || actionError) && (
          <p role="alert" className="mt-2 text-red-700">
            {actionError || error}
          </p>
        )}
      </div>
      {/* CASE 1: INSIDE A FOLDER (文件夹内详情视图) */}
      {currentFolder ? (
        <div className="flex-1 flex flex-col h-full overflow-hidden bg-white">
          {/* Header: back + folder name + edit */}
          <div className="px-4 py-3 border-b border-gray-100 flex items-center justify-between bg-white shrink-0 shadow-2xs">
            <div className="flex items-center gap-2 min-w-0 flex-1 pr-2">
              <button
                onClick={() => setSelectedFolderId(null)}
                className="p-1.5 -ml-1 rounded-xl hover:bg-gray-100 text-gray-700 cursor-pointer transition-colors active:scale-95"
                title="返回知识库列表"
              >
                <ArrowLeft className="w-5 h-5" />
              </button>
              <h1 className="text-sm font-bold text-gray-900 truncate flex items-center gap-1.5">
                <Folder className="w-4 h-4 text-emerald-600 shrink-0" />
                <span className="truncate">{currentFolder.name}</span>
                <span className="text-[11px] text-gray-400 font-medium shrink-0">
                  ({currentFolderDocs.length})
                </span>
              </h1>
            </div>

            {canWrite && (
              <button
                onClick={() =>
                  setFolderModalState({
                    isOpen: true,
                    mode: "edit",
                    folder: currentFolder,
                  })
                }
                className="p-2 rounded-xl text-gray-400 hover:text-gray-700 hover:bg-gray-100 cursor-pointer transition-colors shrink-0"
                title="编辑知识库"
              >
                <Edit2 className="w-4 h-4" />
              </button>
            )}
          </div>

          <div className="px-4 pt-3 flex gap-2 text-sm">
            <button
              onClick={() => setFolderTab("documents")}
              aria-pressed={folderTab === "documents"}
              className={`flex-1 rounded-xl border border-gray-200 ${folderTab === "documents" ? "bg-neutral-900 text-white" : ""}`}
            >
              资料
            </button>
            {canWrite && (
              <button
                onClick={() => setFolderTab("retrieval")}
                aria-pressed={folderTab === "retrieval"}
                className={`flex-1 rounded-xl border border-gray-200 ${folderTab === "retrieval" ? "bg-neutral-900 text-white" : ""}`}
              >
                检索验证
              </button>
            )}
          </div>
          {!canWrite && (
            <p className="px-4 pt-2 text-xs text-gray-500">
              当前角色可核对原文与切片；检索验证需维护者权限。
            </p>
          )}
          {folderTab === "retrieval" && canWrite ? (
            <div className="flex-1 overflow-y-auto p-4 pb-24">
              <KnowledgeRetrieval
                key={currentFolder.id}
                datasetId={currentFolder.id}
                documents={currentFolderDocs}
                onOpenDocument={onOpenDocDetail}
              />
            </div>
          ) : (
            <>
              {/* Files inside this folder */}
              <div className="flex-1 overflow-y-auto p-4 space-y-2.5 pb-24">
                {loading && !currentFolderDocs.length ? (
                  <p role="status">正在加载资料…</p>
                ) : error && !currentFolderDocs.length ? (
                  <p>资料加载失败，请点击上方刷新重试。</p>
                ) : currentFolderDocs.length === 0 ? (
                  <div className="h-64 flex flex-col items-center justify-center text-center p-6 border border-dashed border-gray-200 rounded-2xl bg-gray-50/70 mt-2">
                    <div className="w-12 h-12 rounded-2xl bg-emerald-50 text-emerald-600 flex items-center justify-center mb-3">
                      <FileText className="w-6 h-6" />
                    </div>
                    <div className="text-xs font-bold text-gray-800">
                      暂无文件
                    </div>
                    {canWrite && (
                      <div className="text-[11px] text-gray-400 mt-1">
                        点右下角 + 上传文件
                      </div>
                    )}
                  </div>
                ) : (
                  currentFolderDocs.map((doc) => (
                    <div
                      key={doc.id}
                      className="p-3 bg-white rounded-2xl border border-gray-200/90 hover:border-gray-300 shadow-2xs transition-all flex items-center justify-between gap-3 group"
                    >
                      <div
                        role="button"
                        tabIndex={0}
                        aria-label={`查看文档 ${doc.name}`}
                        onKeyDown={(event) => {
                          if (event.key === "Enter" || event.key === " ") {
                            event.preventDefault();
                            onOpenDocDetail(doc);
                          }
                        }}
                        onClick={() => onOpenDocDetail(doc)}
                        className="flex items-center gap-3 min-w-0 min-h-11 flex-1 cursor-pointer"
                      >
                        <div className="w-10 h-10 rounded-xl bg-gray-50 border border-gray-100 flex items-center justify-center shrink-0">
                          {renderFileIcon(doc.type)}
                        </div>
                        <div className="min-w-0 flex-1">
                          <div className="text-xs font-bold text-gray-900 truncate group-hover:text-emerald-700 transition-colors">
                            {doc.name}
                          </div>
                          <div className="text-[10px] text-gray-400 flex items-center gap-2 mt-0.5">
                            <span className="font-medium text-gray-500">
                              {doc.size}
                            </span>
                            <span>•</span>
                            <span>{doc.updatedAt}</span>
                          </div>
                          <div className="text-[10px] text-gray-500 mt-1">
                            {doc.summary}
                          </div>
                        </div>
                      </div>
                    </div>
                  ))
                )}
              </div>
            </>
          )}
        </div>
      ) : (
        /* CASE 2: ROOT KNOWLEDGE BASE VIEW (知识库首页 - 只列文件夹) */
        <div className="flex-1 flex flex-col h-full overflow-hidden knowledge-background">
          {/* Top Bar: title only */}
          <div className="px-4 py-3 bg-white border-b border-gray-100 shrink-0 shadow-2xs">
            <h1 className="text-base font-bold text-gray-900 tracking-tight">
              知识库
            </h1>
          </div>

          {/* Search Bar */}
          <div className="p-4 pb-3 shrink-0 bg-white border-b border-gray-100">
            <div className="relative">
              <Search className="w-4 h-4 text-gray-400 absolute left-3 top-1/2 -translate-y-1/2" />
              <input
                type="text"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                placeholder="搜索知识库..."
                className="w-full pl-9 pr-3 py-2 bg-gray-100/80 hover:bg-gray-100 focus:bg-white border border-transparent focus:border-gray-200 rounded-xl text-xs text-gray-900 focus:outline-none transition-all placeholder:text-gray-400"
              />
            </div>
          </div>

          {/* Folder List */}
          <div className="flex-1 overflow-y-auto p-4 pb-24">
            <div className="flex items-center gap-1.5 mb-2.5 px-1">
              <span className="text-sm">🗂️</span>
              <h2 className="text-xs font-bold text-gray-900">知识库</h2>
              <span className="text-[10px] text-gray-400 font-medium">
                ({filteredFolders.length})
              </span>
            </div>

            {loading && !folders.length ? (
              <p role="status">正在加载知识库…</p>
            ) : error && !folders.length ? (
              <p>知识库加载失败，请点击上方刷新重试。</p>
            ) : filteredFolders.length === 0 ? (
              <div className="p-6 text-center bg-white rounded-2xl border border-gray-200/80 text-gray-400 text-xs">
                {searchQuery.trim()
                  ? "没有匹配的知识库，请调整搜索词"
                  : canWrite
                    ? "暂无知识库，点右下角 + 新建"
                    : "暂无知识库"}
              </div>
            ) : (
              <div className="space-y-2">
                {filteredFolders.map((folder) => (
                  <div
                    key={folder.id}
                    role="button"
                    tabIndex={0}
                    aria-label={`打开知识库 ${folder.name}`}
                    onKeyDown={(event) => {
                      if (
                        event.target === event.currentTarget &&
                        (event.key === "Enter" || event.key === " ")
                      ) {
                        event.preventDefault();
                        setFolderTab("documents");
                        setSelectedFolderId(folder.id);
                      }
                    }}
                    onClick={() => {
                      setFolderTab("documents");
                      setSelectedFolderId(folder.id);
                    }}
                    className="p-3 bg-white hover:bg-emerald-50/30 rounded-2xl border border-gray-200/90 hover:border-emerald-200 shadow-2xs transition-all flex items-center justify-between gap-3 cursor-pointer group active:scale-[0.99]"
                  >
                    {/* Left Folder Icon */}
                    <div className="w-10 h-10 rounded-xl bg-emerald-50 border border-emerald-100 text-emerald-600 flex items-center justify-center shrink-0 group-hover:scale-105 transition-transform">
                      <Folder className="w-5 h-5 fill-emerald-50" />
                    </div>

                    {/* Name + count */}
                    <div className="min-w-0 flex-1 flex items-center gap-1.5">
                      <h3 className="text-xs font-bold text-gray-900 group-hover:text-emerald-700 transition-colors truncate">
                        {folder.name}
                      </h3>
                      <span className="text-[10px] text-gray-400 font-medium shrink-0">
                        ({folder.docCount ?? 0})
                      </span>
                    </div>

                    {/* Right Actions（编辑/删除按写权限显示，只读角色只剩进入箭头） */}
                    <div className="flex items-center gap-1 shrink-0">
                      {canWrite && (
                        <>
                          <button
                            onClick={(e) => {
                              e.stopPropagation();
                              setFolderModalState({
                                isOpen: true,
                                mode: "edit",
                                folder,
                              });
                            }}
                            className="p-1.5 text-gray-400 hover:text-gray-700 hover:bg-gray-100 rounded-lg cursor-pointer transition-colors"
                            title="编辑知识库"
                          >
                            <Edit2 className="w-3.5 h-3.5" />
                          </button>
                          <button
                            disabled={busy}
                            onClick={(e) => {
                              e.stopPropagation();
                              if (
                                confirm(
                                  `确定要删除知识库「${folder.name}」吗？`,
                                )
                              ) {
                                void perform(() => onDeleteFolder(folder.id));
                              }
                            }}
                            className="p-1.5 text-gray-400 hover:text-red-600 hover:bg-red-50 rounded-lg cursor-pointer transition-colors"
                            title="删除知识库"
                          >
                            <Trash2 className="w-3.5 h-3.5" />
                          </button>
                        </>
                      )}
                      <ChevronRight className="w-4 h-4 text-gray-300 group-hover:text-emerald-600 group-hover:translate-x-0.5 transition-all ml-0.5" />
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>
      )}

      {/* FAB Action Sheet (点击 + 弹出)——仅写权限角色可打开 */}
      {canWrite && fabOpen && (
        <div
          className="absolute inset-0 z-40 bg-black/30 backdrop-blur-2xs animate-in fade-in duration-150"
          onClick={() => setFabOpen(false)}
        >
          <div
            className="absolute bottom-24 left-4 right-4 bg-white rounded-2xl shadow-2xl border border-gray-100 overflow-hidden animate-in slide-in-from-bottom-4 duration-200"
            onClick={(e) => e.stopPropagation()}
          >
            {/* 首页：可新建知识库 + 上传文件（默认根目录，即不预选好知识库，上传时再选）；
                文件夹内：只允许上传文件到当前文件夹，不再支持新建文件夹 */}
            {!currentFolder && (
              <button
                onClick={() => {
                  setFabOpen(false);
                  openCreateFolder();
                }}
                className="w-full px-4 py-3.5 flex items-center gap-3 hover:bg-gray-50 active:bg-gray-100 cursor-pointer transition-colors border-b border-gray-100"
              >
                <div className="w-9 h-9 rounded-xl bg-emerald-50 text-emerald-600 flex items-center justify-center shrink-0">
                  <FolderPlus className="w-4.5 h-4.5" />
                </div>
                <div className="text-left">
                  <div className="text-xs font-bold text-gray-900">
                    新建知识库
                  </div>
                  <div className="text-[10px] text-gray-400 mt-0.5">
                    创建一个新的知识库文件夹
                  </div>
                </div>
              </button>
            )}
            <button
              onClick={() => {
                setFabOpen(false);
                onOpenUpload(currentFolder ? currentFolder.id : undefined);
              }}
              className="w-full px-4 py-3.5 flex items-center gap-3 hover:bg-gray-50 active:bg-gray-100 cursor-pointer transition-colors"
            >
              <div className="w-9 h-9 rounded-xl bg-neutral-900 text-white flex items-center justify-center shrink-0">
                <Upload className="w-4 h-4" />
              </div>
              <div className="text-left">
                <div className="text-xs font-bold text-gray-900">上传文件</div>
                <div className="text-[10px] text-gray-400 mt-0.5">
                  {currentFolder
                    ? `上传到「${currentFolder.name}」`
                    : "默认根目录，上传时可改选知识库"}
                </div>
              </div>
            </button>
          </div>
        </div>
      )}

      {/* Floating + Button（写权限角色才显示；体验用户/成员只读） */}
      {canWrite && (
        <button
          onClick={() => setFabOpen((v) => !v)}
          className={`absolute bottom-6 right-4 z-50 w-13 h-13 rounded-full shadow-lg flex items-center justify-center cursor-pointer transition-all active:scale-90 ${
            fabOpen
              ? "bg-white text-gray-700 border border-gray-200 rotate-45"
              : "bg-neutral-900 hover:bg-neutral-800 text-white"
          }`}
          style={{ width: 52, height: 52 }}
          title={fabOpen ? "收起" : "新建 / 上传"}
        >
          <Plus className="w-6 h-6 stroke-[2.2]" />
        </button>
      )}

      {/* Modal for Creating / Editing Folder */}
      <KnowledgeBaseModal
        isOpen={folderModalState.isOpen}
        mode={folderModalState.mode}
        folder={folderModalState.folder}
        onClose={() =>
          setFolderModalState({ isOpen: false, mode: "create", folder: null })
        }
        onSubmit={async (data) => {
          if (folderModalState.mode === "create") {
            const folder = await onCreateFolder(data);
            setSelectedFolderId(folder.id);
            setFolderTab("documents");
            onOpenUpload(folder.id);
          } else if (folderModalState.folder) {
            await onUpdateFolder(folderModalState.folder.id, data);
          } else throw new Error("请选择知识库");
        }}
      />
    </div>
  );
};
