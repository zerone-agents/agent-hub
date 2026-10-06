import React, { useEffect, useState } from "react";
import { Trash2, Edit3, Check } from "lucide-react";
import { KnowledgeDocument, KnowledgeFolder } from "../types";
import type { MobileChunk } from "../api/knowledge";
import { KnowledgeDialog } from "./KnowledgeDialog";
import { desktopDocumentUrl } from "./knowledgeMobileState";
import { KnowledgeDocumentContent } from "./KnowledgeDocumentContent";

interface DocumentModalProps {
  mode: "view" | "edit" | "upload";
  document?: KnowledgeDocument | null;
  initialChunk?: MobileChunk;
  folders?: KnowledgeFolder[];
  isOpen: boolean;
  canWrite?: boolean;
  onClose: () => void;
  onRefresh?: () => Promise<void>;
  onSave?: (doc: Partial<KnowledgeDocument>) => Promise<void>;
  onDelete?: (id: string) => Promise<void>;
  onParse?: (
    id: string,
    options: { delete: boolean; apply_kb: boolean },
  ) => Promise<void>;
  onStop?: (id: string) => Promise<void>;
}

export const DocumentModal: React.FC<DocumentModalProps> = ({
  mode,
  document,
  initialChunk,
  isOpen,
  canWrite = false,
  onClose,
  onRefresh,
  onSave,
  onDelete,
  onParse,
  onStop,
}) => {
  const [name, setName] = useState("");
  const [editing, setEditing] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [confirmParse, setConfirmParse] = useState(false);
  const [deleteChunks, setDeleteChunks] = useState(false);
  const [applyKB, setApplyKB] = useState(false);
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => {
    setName(document?.name || "");
    setEditing(mode === "edit");
    setConfirmDelete(false);
    setConfirmParse(false);
    setDeleteChunks(false);
    setApplyKB(false);
    setNotice("");
    setError("");
  }, [document?.id, mode, isOpen]);
  const desktopUrl = document?.folderId
    ? desktopDocumentUrl(
        (
          import.meta as ImportMeta & {
            env?: { VITE_HUB_DESKTOP_URL?: string };
          }
        ).env?.VITE_HUB_DESKTOP_URL,
        document.folderId,
        document.id,
      )
    : undefined;
  if (!isOpen) return null;
  const perform = async (
    action: () => Promise<void>,
    close = true,
    write = true,
  ) => {
    if (busy || (write && !canWrite)) return;
    setBusy(true);
    setError("");
    try {
      await action();
      if (close) onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : "操作失败，请重试");
    } finally {
      setBusy(false);
    }
  };
  return (
    <KnowledgeDialog
      title={editing ? "重命名文档" : "文档核对"}
      busy={busy}
      onClose={onClose}
    >
      <div className="p-4 space-y-3 text-xs">
        {error && (
          <p role="alert" className="text-red-700">
            {error}
          </p>
        )}
        {notice && (
          <p role="status" className="text-emerald-700">
            {notice}
          </p>
        )}
        {document ? (
          <>
            {editing && canWrite ? (
              <label className="block">
                文档名称
                <input
                  aria-label="文档名称"
                  value={name}
                  disabled={busy}
                  onChange={(e) => setName(e.target.value)}
                  className="mt-2 w-full border border-gray-200 rounded-lg p-2"
                />
              </label>
            ) : (
              <p className="font-semibold break-all">{document.name}</p>
            )}
            <p className="text-gray-500">
              {document.folderName} · {document.size} · {document.updatedAt}
            </p>
            <p>{document.summary}</p>
            {desktopUrl ? (
              <a
                href={desktopUrl}
                className="inline-flex min-h-11 items-center text-emerald-700 underline"
                target="_blank"
                rel="noopener noreferrer"
              >
                在桌面管理台核对此文档
              </a>
            ) : (
              document.folderId && (
                <details className="text-gray-500">
                  <summary className="min-h-11 flex items-center cursor-pointer">
                    桌面排查说明
                  </summary>
                  <p>
                    此 H5 未配置桌面管理台地址。请在 Agent Hub
                    桌面打开以下知识库及文档；本页仍可核对原文和切片。
                  </p>
                  <p className="mt-2 break-all">
                    知识库：{document.folderName} · {document.folderId}
                    <br />
                    文档：{document.name} · {document.id}
                  </p>
                </details>
              )
            )}
            {onRefresh && (
              <button
                disabled={busy}
                onClick={() => void perform(onRefresh, false, false)}
                className="text-emerald-700"
              >
                {busy ? "刷新中…" : "刷新解析状态"}
              </button>
            )}
            {document.folderId && !editing && (
              <KnowledgeDocumentContent
                key={document.id}
                datasetId={document.folderId}
                documentId={document.id}
                name={document.name}
                initialChunk={initialChunk}
              />
            )}
            {confirmParse && canWrite && (
              <div className="border rounded-lg p-3 space-y-3">
                <p>
                  提交仅代表受理，请继续查看解析状态和分块数，完成且有切片后返回本库的检索验证核对内容。
                </p>
                <label className="flex items-center gap-2">
                  <input
                    type="checkbox"
                    checked={deleteChunks}
                    disabled={busy}
                    onChange={(e) => setDeleteChunks(e.target.checked)}
                  />
                  清除旧切片后重新解析
                </label>
                {deleteChunks && (
                  <p className="text-amber-700">
                    旧切片会立即不可用，新解析完成前检索结果可能不完整。
                  </p>
                )}
                <label className="flex items-center gap-2">
                  <input
                    type="checkbox"
                    checked={applyKB}
                    disabled={busy}
                    onChange={(e) => setApplyKB(e.target.checked)}
                  />
                  应用知识库元数据模板
                </label>
              </div>
            )}
            {confirmDelete && (
              <div className="p-3 bg-red-50 rounded-lg text-red-700 space-y-2">
                <p>确定删除此文档及其切片？</p>
                <button
                  disabled={busy}
                  onClick={() =>
                    void perform(async () => {
                      if (onDelete) await onDelete(document.id);
                      else throw new Error("删除入口不可用");
                    })
                  }
                  className="px-3 py-1 rounded bg-red-600 text-white"
                >
                  确定删除
                </button>
                <button
                  disabled={busy}
                  onClick={() => setConfirmDelete(false)}
                  className="ml-2 px-3 py-1"
                >
                  取消
                </button>
              </div>
            )}
          </>
        ) : (
          <p>请使用知识库内的上传入口添加文档。</p>
        )}
      </div>
      <div className="p-4 border-t border-gray-100 flex flex-wrap items-center justify-end gap-2 text-xs">
        {canWrite && document && (
          <>
            <button
              disabled={busy}
              onClick={() => {
                if (document.run !== "1" && !confirmParse) {
                  setConfirmParse(true);
                  return;
                }
                void perform(async () => {
                  if (document.run === "1" && onStop) await onStop(document.id);
                  else if (onParse) {
                    await onParse(document.id, {
                      delete: deleteChunks,
                      apply_kb: applyKB,
                    });
                    setConfirmParse(false);
                    setNotice("解析请求已受理，请查看解析状态和分块数。");
                  } else throw new Error("解析入口不可用");
                }, document.run === "1");
              }}
              className="text-emerald-700 disabled:opacity-40"
            >
              {document.run === "1"
                ? "请求停止解析"
                : confirmParse
                  ? "提交解析请求"
                  : "解析选项"}
            </button>
            <button
              disabled={busy}
              onClick={() => setConfirmDelete(true)}
              className="flex gap-1 items-center text-red-600"
            >
              <Trash2 className="w-4 h-4" />
              删除
            </button>
            {editing ? (
              <button
                disabled={busy || !name.trim()}
                onClick={() =>
                  void perform(async () => {
                    if (onSave)
                      await onSave({ id: document.id, name: name.trim() });
                    else throw new Error("保存入口不可用");
                  })
                }
                className="flex gap-1 items-center px-3 py-2 rounded-lg bg-neutral-900 text-white disabled:opacity-40"
              >
                <Check className="w-4 h-4" />
                {busy ? "提交中…" : "保存名称"}
              </button>
            ) : (
              <button
                disabled={busy}
                onClick={() => setEditing(true)}
                className="flex gap-1 items-center px-3 py-2 rounded-lg bg-neutral-900 text-white"
              >
                <Edit3 className="w-4 h-4" />
                重命名
              </button>
            )}
          </>
        )}
        <button disabled={busy} onClick={onClose}>
          关闭
        </button>
      </div>
    </KnowledgeDialog>
  );
};
